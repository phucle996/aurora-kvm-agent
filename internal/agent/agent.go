package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"aurora-kvm-agent/internal/collector"
	"aurora-kvm-agent/internal/config"
	"aurora-kvm-agent/internal/libvirt"
	"aurora-kvm-agent/internal/model"
	"aurora-kvm-agent/internal/stream"
)

type Agent struct {
	cfg       config.Config
	logger    *slog.Logger
	conn      *libvirt.ConnManager
	scheduler *collector.Scheduler
	sink      stream.Sink
	events    *libvirt.EventMonitor
	health    *HealthStatus
}

func New(cfg config.Config, logger *slog.Logger) (*Agent, error) {
	tlsCfg, err := cfg.TLSConfig()
	if err != nil {
		return nil, fmt.Errorf("tls config: %w", err)
	}

	sink, err := stream.NewSinkFromConfig(cfg, tlsCfg, logger)
	if err != nil {
		return nil, fmt.Errorf("stream sink: %w", err)
	}

	conn := libvirt.NewConnManager(cfg.LibvirtURI, cfg.ReconnectInterval, cfg.MaxReconnectJitter, logger)
	nodeReader := libvirt.NewNodeMetricsReader(conn, logger)
	vmReader := libvirt.NewVMMetricsReader(conn, logger)
	nodeCollector := collector.NewNodeCollector(nodeReader, cfg.NodeID, cfg.Hostname)
	vmCollector := collector.NewVMCollector(vmReader, cfg.NodeID)

	health := NewHealthStatus()
	wrappedSink := &healthSink{sink: sink, health: health}
	scheduler := collector.NewScheduler(logger, nodeCollector, vmCollector, wrappedSink, cfg.NodePollInterval, cfg.VMPollInterval, cfg.CollectorErrorBackoff)

	return &Agent{
		cfg:       cfg,
		logger:    logger,
		conn:      conn,
		scheduler: scheduler,
		sink:      wrappedSink,
		events:    libvirt.NewEventMonitor(logger),
		health:    health,
	}, nil
}

func (a *Agent) Run(ctx context.Context) error {
	a.logger.Info("starting aurora-kvm-agent", "node_id", a.cfg.NodeID, "libvirt_uri", a.cfg.LibvirtURI, "stream_mode", a.cfg.StreamMode)
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	err := a.run(ctx)
	a.shutdown(context.Background())
	if err != nil {
		return err
	}
	a.logger.Info("aurora-kvm-agent stopped")
	return nil
}

func (a *Agent) runEventLoop(ctx context.Context) error {
	events := make(chan libvirt.DomainEvent, 32)
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
	if cfg.LogJSON {
		return slog.New(slog.NewJSONHandler(os.Stdout, hOpts))
	}
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
	s.health.MarkNodeSample(m.Timestamp)
	return nil
}

func (s *healthSink) SendVMMetrics(ctx stream.Context, metrics []model.VMMetrics) error {
	err := s.sink.SendVMMetrics(ctx, metrics)
	if err != nil {
		s.health.SetStreamConnected(false)
		return err
	}
	s.health.SetStreamConnected(true)
	if len(metrics) > 0 {
		s.health.MarkVMSample(metrics[0].Timestamp)
	}
	return nil
}

func (s *healthSink) Close(ctx stream.Context) error {
	return s.sink.Close(ctx)
}
