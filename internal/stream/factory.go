package stream

import (
	"crypto/tls"
	"fmt"
	"log/slog"

	"aurora-kvm-agent/internal/config"
)

func NewSinkFromConfig(cfg config.Config, tlsCfg *tls.Config, logger *slog.Logger) (Sink, error) {
	switch cfg.StreamMode {
	case config.StreamModeGRPC:
		return NewGRPCClient(
			cfg.BackendGRPCAddr,
			tlsCfg,
			cfg.BackendToken,
			cfg.GRPCNodeStreamMethod,
			cfg.GRPCNodeStaticMethod,
			cfg.GRPCNodeInfoSyncMethod,
			cfg.GRPCVMInfoSyncMethod,
			cfg.GRPCVMStreamMethod,
			cfg.GRPCVMRuntimeStreamMethod,
			logger,
		), nil
	case config.StreamModeWebSocket:
		return NewWebSocketClient(cfg.BackendWSURL, cfg.BackendToken, tlsCfg, cfg.WebSocketWriteTimeout, cfg.WebSocketPingInterval, logger), nil
	default:
		return nil, fmt.Errorf("unsupported stream mode %q", cfg.StreamMode)
	}
}
