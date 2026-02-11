package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

func (a *Agent) runProbeListener(ctx context.Context) error {
	addr := strings.TrimSpace(a.cfg.ProbeListenAddr)
	if addr == "" {
		return fmt.Errorf("empty probe listen address")
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen probe endpoint %s: %w", addr, err)
	}
	defer func() { _ = ln.Close() }()

	a.logger.Info("probe endpoint listening", "addr", addr)

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			if ctx.Err() != nil {
				return nil
			}
			if ne, ok := acceptErr.(net.Error); ok && ne.Temporary() {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			if errors.Is(acceptErr, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("accept probe endpoint %s: %w", addr, acceptErr)
		}

		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		_, _ = conn.Write([]byte("aurora-kvm-agent:ok\n"))
		_ = conn.Close()
	}
}
