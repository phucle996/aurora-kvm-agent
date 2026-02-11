package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/sync/errgroup"
)

func (a *Agent) run(ctx context.Context) error {
	if err := a.conn.Connect(ctx); err != nil {
		return fmt.Errorf("initial libvirt connect: %w", err)
	}
	a.health.SetLibvirtConnected(true)
	a.health.SetStreamConnected(true)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return a.scheduler.Run(gctx)
	})
	g.Go(func() error {
		return a.runHealthLoop(gctx)
	})
	g.Go(func() error {
		return a.runEventLoop(gctx)
	})
	g.Go(func() error {
		return a.runProbeListener(gctx)
	})

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func (a *Agent) runHealthLoop(ctx context.Context) error {
	t := time.NewTicker(a.cfg.HealthInterval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			if err := a.conn.Healthy(ctx); err != nil {
				a.logger.Warn("libvirt health check failed, reconnecting", "error", err)
				a.health.SetLibvirtConnected(false)
				if recErr := a.conn.Reconnect(ctx); recErr != nil {
					a.logger.Error("libvirt reconnect failed", "error", recErr)
					continue
				}
				if err := a.logHealth("recovered"); err != nil {
					a.logger.Debug("health snapshot log failed", "error", err)
				}
				a.health.SetLibvirtConnected(true)
			} else {
				a.health.SetLibvirtConnected(true)
				_ = a.logHealth("ok")
			}
		}
	}
}

func (a *Agent) logHealth(status string) error {
	a.logger.Log(context.Background(), slog.LevelDebug, "agent health", "status", status, "snapshot", a.health.Snapshot())
	return nil
}

func (a *Agent) shutdown(ctx context.Context) {
	if err := a.sink.Close(ctx); err != nil {
		a.logger.Warn("stream sink close failed", "error", err)
	}
	a.health.SetStreamConnected(false)
	if err := a.conn.Close(); err != nil {
		a.logger.Warn("libvirt close failed", "error", err)
	}
	a.health.SetLibvirtConnected(false)
}
