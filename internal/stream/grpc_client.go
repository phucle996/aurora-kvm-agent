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

	logger      *slog.Logger
	addr        string
	tlsConfig   *tls.Config
	token       string
	nodeMethod  string
	vmMethod    string
	conn        *grpc.ClientConn
	nodeStream  grpc.ClientStream
	vmStream    grpc.ClientStream
	dialTimeout time.Duration
}

func NewGRPCClient(addr string, tlsCfg *tls.Config, token, nodeMethod, vmMethod string, logger *slog.Logger) *GRPCClient {
	encoding.RegisterCodec(jsonCodec{})
	return &GRPCClient{
		logger:      logger,
		addr:        addr,
		tlsConfig:   tlsCfg,
		token:       token,
		nodeMethod:  nodeMethod,
		vmMethod:    vmMethod,
		dialTimeout: 8 * time.Second,
	}
}

func (c *GRPCClient) SendNodeMetrics(ctx Context, m model.NodeMetrics) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnLocked(ctx); err != nil {
		return err
	}
	if c.nodeStream == nil {
		if err := c.openNodeStreamLocked(ctx); err != nil {
			return err
		}
	}
	frame := NewNodeFrame(m)
	if err := c.nodeStream.SendMsg(frame); err != nil {
		c.logger.Warn("grpc node send failed, reopening stream", "error", err)
		c.nodeStream = nil
		if err2 := c.openNodeStreamLocked(ctx); err2 != nil {
			return fmt.Errorf("reopen node stream: %w", err2)
		}
		if err2 := c.nodeStream.SendMsg(frame); err2 != nil {
			return fmt.Errorf("send node frame: %w", err2)
		}
	}
	return nil
}

func (c *GRPCClient) SendVMMetrics(ctx Context, metrics []model.VMMetrics) error {
	if len(metrics) == 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnLocked(ctx); err != nil {
		return err
	}
	if c.vmStream == nil {
		if err := c.openVMStreamLocked(ctx); err != nil {
			return err
		}
	}
	frame := NewVMFrame(metrics)
	if err := c.vmStream.SendMsg(frame); err != nil {
		c.logger.Warn("grpc vm send failed, reopening stream", "error", err)
		c.vmStream = nil
		if err2 := c.openVMStreamLocked(ctx); err2 != nil {
			return fmt.Errorf("reopen vm stream: %w", err2)
		}
		if err2 := c.vmStream.SendMsg(frame); err2 != nil {
			return fmt.Errorf("send vm frame: %w", err2)
		}
	}
	return nil
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
		return nil
	}
	dialCtx, cancel := context.WithTimeout(context.Background(), c.dialTimeout)
	defer cancel()
	if dl, ok := ctx.Deadline(); ok {
		dialCtx, cancel = context.WithDeadline(context.Background(), dl)
		defer cancel()
	}

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
	if dl, ok := ctx.Deadline(); ok {
		var cancel context.CancelFunc
		out, cancel = context.WithDeadline(out, dl)
		_ = cancel
	}
	if c.token != "" {
		out = metadata.AppendToOutgoingContext(out, "authorization", "Bearer "+c.token)
	}
	return out
}
