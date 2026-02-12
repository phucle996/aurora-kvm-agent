package metric

import (
	"context"
	"log/slog"
	"time"
)

type DomainEvent struct {
	Type      string    `json:"type"`
	DomainID  string    `json:"domain_id"`
	Domain    string    `json:"domain"`
	Timestamp time.Time `json:"timestamp"`
}

// EventMonitor is intentionally lightweight: it can be replaced by full libvirt event callbacks later.
// For now we expose a heartbeat signal showing the event loop is alive.
type EventMonitor struct {
	logger *slog.Logger
}

func NewEventMonitor(logger *slog.Logger) *EventMonitor {
	return &EventMonitor{logger: logger}
}

func (m *EventMonitor) Run(ctx context.Context, out chan<- DomainEvent) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			select {
			case out <- DomainEvent{Type: "heartbeat", Timestamp: t.UTC()}:
			default:
				m.logger.Debug("dropping event heartbeat because channel is full")
			}
		}
	}
}
