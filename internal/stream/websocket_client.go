package stream

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"

	"aurora-kvm-agent/internal/model"
)

type WebSocketClient struct {
	mu sync.Mutex

	logger       *slog.Logger
	url          string
	token        string
	tlsConfig    *tls.Config
	writeTimeout time.Duration
	pingInterval time.Duration
	conn         *websocket.Conn
	pingCancel   context.CancelFunc
}

func NewWebSocketClient(url, token string, tlsCfg *tls.Config, writeTimeout, pingInterval time.Duration, logger *slog.Logger) *WebSocketClient {
	if writeTimeout <= 0 {
		writeTimeout = 5 * time.Second
	}
	if pingInterval <= 0 {
		pingInterval = 10 * time.Second
	}
	return &WebSocketClient{
		logger:       logger,
		url:          url,
		token:        token,
		tlsConfig:    tlsCfg,
		writeTimeout: writeTimeout,
		pingInterval: pingInterval,
	}
}

func (c *WebSocketClient) SendNodeMetrics(ctx Context, m model.NodeMetrics) error {
	return c.sendEnvelope(ctx, model.Envelope{Type: model.MetricTypeNode, NodeID: m.NodeID, Timestamp: m.Timestamp, Payload: NewNodeFrame(m)})
}

func (c *WebSocketClient) SendVMMetrics(ctx Context, metrics []model.VMMetrics) error {
	if len(metrics) == 0 {
		return nil
	}
	frame := NewVMFrame(metrics)
	return c.sendEnvelope(ctx, model.Envelope{Type: model.MetricTypeVM, NodeID: frame.NodeID, Timestamp: frame.Timestamp, Payload: frame})
}

func (c *WebSocketClient) Close(ctx Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pingCancel != nil {
		c.pingCancel()
		c.pingCancel = nil
	}
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close(websocket.StatusNormalClosure, "shutdown")
	c.conn = nil
	_ = ctx
	return err
}

func (c *WebSocketClient) sendEnvelope(ctx Context, envelope model.Envelope) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnLocked(ctx); err != nil {
		return err
	}
	payload, err := EncodeEnvelope(envelope)
	if err != nil {
		return fmt.Errorf("encode envelope: %w", err)
	}
	wctx, cancel := context.WithTimeout(context.Background(), c.writeTimeout)
	defer cancel()
	if err := c.conn.Write(wctx, websocket.MessageText, payload); err != nil {
		c.logger.Warn("websocket write failed, reconnecting", "error", err)
		_ = c.conn.Close(websocket.StatusInternalError, "reconnect")
		c.conn = nil
		if err2 := c.ensureConnLocked(ctx); err2 != nil {
			return err2
		}
		if err2 := c.conn.Write(wctx, websocket.MessageText, payload); err2 != nil {
			return fmt.Errorf("write envelope retry: %w", err2)
		}
	}
	return nil
}

func (c *WebSocketClient) ensureConnLocked(ctx Context) error {
	if c.conn != nil {
		return nil
	}
	h := http.Header{}
	if c.token != "" {
		h.Set("Authorization", "Bearer "+c.token)
	}
	opt := &websocket.DialOptions{HTTPHeader: h}
	if c.tlsConfig != nil {
		opt.HTTPClient = &http.Client{Transport: &http.Transport{TLSClientConfig: c.tlsConfig}}
	}
	dialCtx := context.Background()
	if dl, ok := ctx.Deadline(); ok {
		var cancel context.CancelFunc
		dialCtx, cancel = context.WithDeadline(dialCtx, dl)
		defer cancel()
	}
	conn, _, err := websocket.Dial(dialCtx, c.url, opt)
	if err != nil {
		return fmt.Errorf("websocket dial %s: %w", c.url, err)
	}
	conn.SetReadLimit(10 << 20)
	c.conn = conn
	c.startPingLoopLocked()
	c.logger.Info("websocket stream connected", "url", c.url)
	return nil
}

func (c *WebSocketClient) startPingLoopLocked() {
	if c.pingCancel != nil {
		c.pingCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.pingCancel = cancel
	go func(conn *websocket.Conn, interval time.Duration) {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pingCtx, pingCancel := context.WithTimeout(context.Background(), 3*time.Second)
				_ = conn.Ping(pingCtx)
				pingCancel()
			}
		}
	}(c.conn, c.pingInterval)
}
