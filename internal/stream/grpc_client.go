package stream

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/metadata"

	"aurora-kvm-agent/internal/model"
)

type jsonCodec struct{}

func (jsonCodec) Name() string {
	return "json"
}

func (jsonCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (jsonCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

type GRPCClient struct {
	mu sync.Mutex

	logger       *slog.Logger
	addr         string
	tlsConfig    *tls.Config
	token        string
	nodeMethod   string
	vmMethod     string
	conn         *grpc.ClientConn
	nodeStream   grpc.ClientStream
	vmStream     grpc.ClientStream
	dialTimeout  time.Duration
	maxRetries   int
	retryBackoff time.Duration
}

func NewGRPCClient(addr string, tlsCfg *tls.Config, token, nodeMethod, vmMethod string, logger *slog.Logger) *GRPCClient {
	encoding.RegisterCodec(jsonCodec{})
	return &GRPCClient{
		logger:       logger,
		addr:         addr,
		tlsConfig:    tlsCfg,
		token:        token,
		nodeMethod:   nodeMethod,
		vmMethod:     vmMethod,
		dialTimeout:  8 * time.Second,
		maxRetries:   5,
		retryBackoff: time.Second,
	}
}

func (c *GRPCClient) SendNodeMetrics(ctx Context, m model.NodeMetrics) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	frame := NewNodeFrame(m)
	return c.sendNodeFrameWithRetryLocked(ctx, frame)
}

func (c *GRPCClient) SendVMMetrics(ctx Context, metrics []model.VMMetrics) error {
	if len(metrics) == 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	frame := NewVMFrame(metrics)
	return c.sendVMFrameWithRetryLocked(ctx, frame)
}

func (c *GRPCClient) Close(ctx Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.nodeStream != nil {
		_ = c.nodeStream.CloseSend()
		c.nodeStream = nil
	}
	if c.vmStream != nil {
		_ = c.vmStream.CloseSend()
		c.vmStream = nil
	}
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	_ = ctx
	return nil
}

func (c *GRPCClient) ensureConnLocked(ctx Context) error {
	if c.conn != nil {
		state := c.conn.GetState()
		if state != connectivity.Shutdown {
			return nil
		}
		c.logger.Warn("grpc conn is shutdown, resetting before redial", "addr", c.addr)
		c.resetConnLocked()
	}

	baseCtx := context.Background()
	if typedCtx, ok := ctx.(context.Context); ok {
		baseCtx = typedCtx
	}
	dialCtx, cancel := context.WithTimeout(baseCtx, c.dialTimeout)
	defer cancel()

	var creds credentials.TransportCredentials
	if c.tlsConfig != nil {
		creds = credentials.NewTLS(c.tlsConfig)
	} else {
		creds = insecure.NewCredentials()
	}

	conn, err := grpc.DialContext(
		dialCtx,
		c.addr,
		grpc.WithTransportCredentials(creds),
		grpc.WithBlock(),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(jsonCodec{}), grpc.CallContentSubtype("json")),
	)
	if err != nil {
		return fmt.Errorf("grpc dial %s: %w", c.addr, err)
	}
	c.conn = conn
	c.logger.Info("grpc stream connected", "addr", c.addr)
	return nil
}

func (c *GRPCClient) sendNodeFrameWithRetryLocked(ctx Context, frame NodeFrame) error {
	var lastErr error
	maxRetries := c.maxRetries
	if maxRetries <= 0 {
		maxRetries = 1
	}
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := c.ensureConnLocked(ctx); err != nil {
			lastErr = err
			c.resetConnLocked()
			if !c.waitRetryLocked(ctx, attempt, maxRetries) {
				break
			}
			continue
		}
		if c.nodeStream == nil {
			if err := c.openNodeStreamLocked(ctx); err != nil {
				lastErr = err
				c.resetConnLocked()
				if !c.waitRetryLocked(ctx, attempt, maxRetries) {
					break
				}
				continue
			}
		}
		if err := c.nodeStream.SendMsg(frame); err == nil {
			return nil
		} else {
			lastErr = fmt.Errorf("send node frame: %w", err)
			c.logger.Warn(
				"grpc node send failed, resetting conn",
				"addr",
				c.addr,
				"attempt",
				attempt,
				"max_attempts",
				maxRetries,
				"error",
				err,
			)
			c.resetConnLocked()
			if !c.waitRetryLocked(ctx, attempt, maxRetries) {
				break
			}
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("unknown stream error")
	}
	return fmt.Errorf("send node frame failed after %d attempts: %w", maxRetries, lastErr)
}

func (c *GRPCClient) sendVMFrameWithRetryLocked(ctx Context, frame VMFrame) error {
	var lastErr error
	maxRetries := c.maxRetries
	if maxRetries <= 0 {
		maxRetries = 1
	}
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := c.ensureConnLocked(ctx); err != nil {
			lastErr = err
			c.resetConnLocked()
			if !c.waitRetryLocked(ctx, attempt, maxRetries) {
				break
			}
			continue
		}
		if c.vmStream == nil {
			if err := c.openVMStreamLocked(ctx); err != nil {
				lastErr = err
				c.resetConnLocked()
				if !c.waitRetryLocked(ctx, attempt, maxRetries) {
					break
				}
				continue
			}
		}
		if err := c.vmStream.SendMsg(frame); err == nil {
			return nil
		} else {
			lastErr = fmt.Errorf("send vm frame: %w", err)
			c.logger.Warn(
				"grpc vm send failed, resetting conn",
				"addr",
				c.addr,
				"attempt",
				attempt,
				"max_attempts",
				maxRetries,
				"error",
				err,
			)
			c.resetConnLocked()
			if !c.waitRetryLocked(ctx, attempt, maxRetries) {
				break
			}
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("unknown stream error")
	}
	return fmt.Errorf("send vm frame failed after %d attempts: %w", maxRetries, lastErr)
}

func (c *GRPCClient) resetConnLocked() {
	if c.nodeStream != nil {
		_ = c.nodeStream.CloseSend()
		c.nodeStream = nil
	}
	if c.vmStream != nil {
		_ = c.vmStream.CloseSend()
		c.vmStream = nil
	}
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}

func (c *GRPCClient) waitRetryLocked(ctx Context, attempt int, maxRetries int) bool {
	if attempt >= maxRetries {
		return false
	}
	backoff := c.retryBackoff * time.Duration(attempt)
	if backoff <= 0 {
		backoff = time.Second
	}
	timer := time.NewTimer(backoff)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (c *GRPCClient) openNodeStreamLocked(ctx Context) error {
	if c.conn == nil {
		return fmt.Errorf("grpc conn is nil")
	}
	streamCtx := c.decorateContext(ctx)
	s, err := c.conn.NewStream(streamCtx, &grpc.StreamDesc{ClientStreams: true}, c.nodeMethod)
	if err != nil {
		return fmt.Errorf("open node stream: %w", err)
	}
	c.nodeStream = s
	return nil
}

func (c *GRPCClient) openVMStreamLocked(ctx Context) error {
	if c.conn == nil {
		return fmt.Errorf("grpc conn is nil")
	}
	streamCtx := c.decorateContext(ctx)
	s, err := c.conn.NewStream(streamCtx, &grpc.StreamDesc{ClientStreams: true}, c.vmMethod)
	if err != nil {
		return fmt.Errorf("open vm stream: %w", err)
	}
	c.vmStream = s
	return nil
}

func (c *GRPCClient) decorateContext(ctx Context) context.Context {
	out := context.Background()
	if typedCtx, ok := ctx.(context.Context); ok {
		out = typedCtx
	}
	if c.token != "" {
		out = metadata.AppendToOutgoingContext(out, "authorization", "Bearer "+c.token)
	}
	return out
}
