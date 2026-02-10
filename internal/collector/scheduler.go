package collector

import (
	"context"
	"log/slog"
	"time"

	"golang.org/x/sync/errgroup"

	"aurora-kvm-agent/internal/model"
	"aurora-kvm-agent/internal/stream"
)

type Scheduler struct {
	logger       *slog.Logger
	node         *NodeCollector
	vm           *VMCollector
	sink         stream.Sink
	nodeInterval time.Duration
	vmInterval   time.Duration
	errorBackoff time.Duration
}

func NewScheduler(
	logger *slog.Logger,
	node *NodeCollector,
	vm *VMCollector,
	sink stream.Sink,
	nodeInterval, vmInterval, errorBackoff time.Duration,
) *Scheduler {
	if errorBackoff <= 0 {
		errorBackoff = time.Second
	}
	return &Scheduler{
		logger:       logger,
		node:         node,
		vm:           vm,
		sink:         sink,
		nodeInterval: nodeInterval,
		vmInterval:   vmInterval,
		errorBackoff: errorBackoff,
	}
}

func (s *Scheduler) Run(ctx context.Context) error {
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return s.runNodeLoop(gctx)
	})
	g.Go(func() error {
		return s.runVMLoop(gctx)
	})
	return g.Wait()
}

func (s *Scheduler) runNodeLoop(ctx context.Context) error {
	ticker := time.NewTicker(s.nodeInterval)
	defer ticker.Stop()

	if err := s.collectAndSendNode(ctx); err != nil {
		s.logger.Warn("initial node collect failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.collectAndSendNode(ctx); err != nil {
				s.logger.Error("node collect/send failed", "error", err)
				s.sleepWithContext(ctx, s.errorBackoff)
			}
		}
	}
}

func (s *Scheduler) runVMLoop(ctx context.Context) error {
	ticker := time.NewTicker(s.vmInterval)
	defer ticker.Stop()

	if err := s.collectAndSendVM(ctx); err != nil {
		s.logger.Warn("initial vm collect failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.collectAndSendVM(ctx); err != nil {
				s.logger.Error("vm collect/send failed", "error", err)
				s.sleepWithContext(ctx, s.errorBackoff)
			}
		}
	}
}

func (s *Scheduler) collectAndSendNode(ctx context.Context) error {
	m, err := s.node.Collect(ctx)
	if err != nil {
		return err
	}
	return s.sink.SendNodeMetrics(ctx, m)
}

func (s *Scheduler) collectAndSendVM(ctx context.Context) error {
	metrics, err := s.vm.Collect(ctx)
	if err != nil {
		return err
	}
	if len(metrics) == 0 {
		return nil
	}
	return s.sink.SendVMMetrics(ctx, metrics)
}

func (s *Scheduler) sleepWithContext(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func AsNodeEnvelope(m model.NodeMetrics) model.Envelope {
	return model.Envelope{Type: model.MetricTypeNode, NodeID: m.NodeID, Timestamp: m.Timestamp, Payload: m}
}

func AsVMEnvelope(nodeID string, payload []model.VMMetrics, at time.Time) model.Envelope {
	return model.Envelope{Type: model.MetricTypeVM, NodeID: nodeID, Timestamp: at, Payload: payload}
}
