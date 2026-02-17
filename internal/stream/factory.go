package stream

import (
	"crypto/tls"
	"log/slog"

	"aurora-kvm-agent/internal/config"
)

const (
	defaultNodeStreamMethod      = "/aurora.metrics.v1.MetricsService/StreamNodeMetrics"
	defaultVMInfoSyncMethod      = "/aurora.metrics.v1.MetricsService/SyncVMInfo"
	defaultVMStreamMethod        = "/aurora.metrics.v1.MetricsService/StreamVMMetrics"
	defaultVMRuntimeStreamMethod = "/aurora.metrics.v1.MetricsService/StreamVMRuntimeMetrics"
)

func NewSinkFromConfig(cfg config.Config, tlsCfg *tls.Config, logger *slog.Logger) (Sink, error) {
	return NewGRPCClient(
		cfg.BackendGRPCAddr,
		tlsCfg,
		cfg.BackendToken,
		defaultNodeStreamMethod,
		defaultVMInfoSyncMethod,
		defaultVMStreamMethod,
		defaultVMRuntimeStreamMethod,
		logger,
	), nil
}
