package libvirt

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net/url"
	"sync"
	"time"

	golibvirt "github.com/digitalocean/go-libvirt"
)

// ConnManager owns a single libvirt RPC connection and reconnect flow.
type ConnManager struct {
	mu        sync.RWMutex
	client    *golibvirt.Libvirt
	uri       string
	logger    *slog.Logger
	retryWait time.Duration
	maxJitter time.Duration
	randSrc   *rand.Rand
}

func NewConnManager(uri string, retryWait, maxJitter time.Duration, logger *slog.Logger) *ConnManager {
	if retryWait <= 0 {
		retryWait = 3 * time.Second
	}
	if maxJitter < 0 {
		maxJitter = 0
	}
	return &ConnManager{
		uri:       uri,
		logger:    logger,
		retryWait: retryWait,
		maxJitter: maxJitter,
		randSrc:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (m *ConnManager) Connect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.connectLocked(ctx)
}

func (m *ConnManager) Client(ctx context.Context) (*golibvirt.Libvirt, error) {
	m.mu.RLock()
	c := m.client
	m.mu.RUnlock()
	if c != nil {
		return c, nil
	}
	if err := m.Connect(ctx); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return nil, fmt.Errorf("libvirt client is nil after connect")
	}
	return m.client, nil
}

func (m *ConnManager) Reconnect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.client != nil {
		if err := m.client.Disconnect(); err != nil {
			m.logger.Warn("libvirt disconnect failed", "error", err)
		}
		m.client = nil
	}
	return m.connectLocked(ctx)
}

func (m *ConnManager) Healthy(ctx context.Context) error {
	c, err := m.Client(ctx)
	if err != nil {
		return err
	}
	_, err = c.Version()
	if err != nil {
		return fmt.Errorf("libvirt version check failed: %w", err)
	}
	return nil
}

func (m *ConnManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.client == nil {
		return nil
	}
	err := m.client.Disconnect()
	m.client = nil
	return err
}

func (m *ConnManager) connectLocked(ctx context.Context) error {
	if m.client != nil {
		if _, err := m.client.Version(); err == nil {
			return nil
		}
		_ = m.client.Disconnect()
		m.client = nil
	}

	uri, err := m.parseURI()
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		c, dialErr := golibvirt.ConnectToURI(uri)
		if dialErr == nil {
			m.client = c
			m.logger.Info("libvirt connected", "uri", uri.Redacted())
			return nil
		}

		wait := m.retryWait + m.jitter()
		m.logger.Error("libvirt connect failed", "uri", uri.Redacted(), "error", dialErr, "retry_in", wait)

		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}

func (m *ConnManager) parseURI() (*url.URL, error) {
	raw := m.uri
	if raw == "" {
		raw = string(golibvirt.QEMUSystem)
	}
	uri, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse libvirt uri %q: %w", raw, err)
	}
	if uri.Scheme == "" {
		uri, err = url.Parse(string(golibvirt.QEMUSystem))
		if err != nil {
			return nil, fmt.Errorf("parse fallback uri: %w", err)
		}
	}
	return uri, nil
}

func (m *ConnManager) jitter() time.Duration {
	if m.maxJitter == 0 {
		return 0
	}
	return time.Duration(m.randSrc.Int63n(int64(m.maxJitter)))
}
