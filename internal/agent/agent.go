package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aurora-kvm-agent/internal/collector"
	"aurora-kvm-agent/internal/config"
	libvirtmetric "aurora-kvm-agent/internal/libvirt/metric"
	libvirtnode "aurora-kvm-agent/internal/libvirt/metric/node"
	libvirtvm "aurora-kvm-agent/internal/libvirt/metric/vm"
	"aurora-kvm-agent/internal/model"
	"aurora-kvm-agent/internal/stream"
)

type Agent struct {
	cfg       config.Config
	logger    *slog.Logger
	conn      *libvirtmetric.ConnManager
	scheduler *collector.Scheduler
	sink      stream.Sink
	events    *libvirtmetric.EventMonitor
	health    *HealthStatus
}

const (
	nodeLiteStreamInterval = 2 * time.Second
	vmLiteStreamInterval   = 2 * time.Second
	vmDetailStreamInterval = 5 * time.Second
	vmInfoSyncInterval     = 10 * time.Minute
)

func New(cfg config.Config, logger *slog.Logger) (*Agent, error) {
	tlsCfg, err := cfg.TLSConfig()
	if err != nil {
		return nil, fmt.Errorf("tls config: %w", err)
	}

	sink, err := stream.NewSinkFromConfig(cfg, tlsCfg, logger)
	if err != nil {
		return nil, fmt.Errorf("stream sink: %w", err)
	}

	conn := libvirtmetric.NewConnManager(cfg.LibvirtURI, cfg.ReconnectInterval, cfg.MaxReconnectJitter, logger)
	nodeReader := libvirtnode.NewNodeMetricsReader(conn, logger)
	vmReader := libvirtvm.NewVMMetricsReader(conn, logger)
	nodeCollector := collector.NewNodeCollector(nodeReader, cfg.NodeID, cfg.Hostname)
	nodeHardwareCollector := collector.NewNodeHardwareCollector(nodeReader, cfg.NodeID, cfg.Hostname)
	vmCollector := collector.NewVMCollector(vmReader, cfg.NodeID)
	vmRuntimeCollector := collector.NewVMRuntimeCollector(vmReader, cfg.NodeID)

	health := NewHealthStatus()
	wrappedSink := &healthSink{sink: sink, health: health}
	scheduler := collector.NewScheduler(
		logger,
		nodeCollector,
		nodeHardwareCollector,
		vmCollector,
		vmRuntimeCollector,
		wrappedSink,
		nodeLiteStreamInterval,
		vmInfoSyncInterval,
		vmLiteStreamInterval,
		vmDetailStreamInterval,
		cfg.CollectorErrorBackoff,
		cfg.AgentVersion,
	)

	return &Agent{
		cfg:       cfg,
		logger:    logger,
		conn:      conn,
		scheduler: scheduler,
		sink:      wrappedSink,
		events:    libvirtmetric.NewEventMonitor(logger),
		health:    health,
	}, nil
}

func (a *Agent) Run(ctx context.Context) error {
	a.logger.Info("starting aurora-kvm-agent", "node_id", a.cfg.NodeID, "libvirt_uri", a.cfg.LibvirtURI)
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- a.run(runCtx)
	}()

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	var runErr error
	select {
	case runErr = <-runErrCh:
		// Agent terminated by itself (startup error/runtime error/parent ctx canceled).
	case sig := <-sigCh:
		a.logger.Info("shutdown signal received, starting graceful shutdown", "signal", sig.String(), "timeout", a.cfg.ShutdownTimeout)
		cancelRun()

		graceTimer := time.NewTimer(a.cfg.ShutdownTimeout)
		defer graceTimer.Stop()

		select {
		case runErr = <-runErrCh:
			// graceful stop completed in time
		case sig2 := <-sigCh:
			a.logger.Warn("second signal received, forcing immediate shutdown", "signal", sig2.String())
			runErr = context.Canceled
		case <-graceTimer.C:
			a.logger.Warn("graceful shutdown timeout reached, forcing shutdown", "timeout", a.cfg.ShutdownTimeout)
			runErr = context.DeadlineExceeded
		}
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
	defer cancelShutdown()
	a.shutdown(shutdownCtx)

	if runErr != nil && !errors.Is(runErr, context.Canceled) && !errors.Is(runErr, context.DeadlineExceeded) {
		return runErr
	}
	a.logger.Info("aurora-kvm-agent stopped")
	return nil
}

func (a *Agent) runEventLoop(ctx context.Context) error {
	events := make(chan libvirtmetric.DomainEvent, 32)
	go a.events.Run(ctx, events)

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-events:
			a.logger.Debug("libvirt event", "type", ev.Type, "domain", ev.Domain, "domain_id", ev.DomainID, "ts", ev.Timestamp)
		}
	}
}

func BuildLogger(cfg config.Config) *slog.Logger {
	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	hOpts := &slog.HandlerOptions{Level: level}
	return slog.New(slog.NewTextHandler(os.Stdout, hOpts))
}

type healthSink struct {
	sink   stream.Sink
	health *HealthStatus
}

func (s *healthSink) SendNodeMetrics(ctx stream.Context, m model.NodeMetrics) error {
	err := s.sink.SendNodeMetrics(ctx, m)
	if err != nil {
		s.health.SetStreamConnected(false)
		return err
	}
	s.health.SetStreamConnected(true)
	if m.TimestampUnix > 0 {
		s.health.MarkNodeSample(time.Unix(m.TimestampUnix, 0).UTC())
	}
	return nil
}

func (s *healthSink) SendNodeHardwareInfo(ctx stream.Context, info model.NodeHardwareInfo) error {
	err := s.sink.SendNodeHardwareInfo(ctx, info)
	if err != nil {
		s.health.SetStreamConnected(false)
		return err
	}
	s.health.SetStreamConnected(true)
	return nil
}

func (s *healthSink) SendVMMetrics(ctx stream.Context, metrics []model.VMMetrics) error {
	err := s.sink.SendVMMetrics(ctx, metrics)
	if err != nil {
		s.health.SetStreamConnected(false)
		return err
	}
	s.health.SetStreamConnected(true)
	if len(metrics) > 0 && metrics[0].TimestampUnix > 0 {
		s.health.MarkVMSample(time.Unix(metrics[0].TimestampUnix, 0).UTC())
	}
	return nil
}

func (s *healthSink) SendVMRuntimeMetrics(ctx stream.Context, metrics []model.VMRuntimeMetrics) error {
	err := s.sink.SendVMRuntimeMetrics(ctx, metrics)
	if err != nil {
		s.health.SetStreamConnected(false)
		return err
	}
	s.health.SetStreamConnected(true)
	if len(metrics) > 0 && metrics[0].TimestampUnix > 0 {
		s.health.MarkVMSample(time.Unix(metrics[0].TimestampUnix, 0).UTC())
	}
	return nil
}

func (s *healthSink) SendVMInfoSync(ctx stream.Context, info model.VMInfoSync) error {
	err := s.sink.SendVMInfoSync(ctx, info)
	if err != nil {
		s.health.SetStreamConnected(false)
		return err
	}
	s.health.SetStreamConnected(true)
	return nil
}

func (s *healthSink) Close(ctx stream.Context) error {
	return s.sink.Close(ctx)
}
