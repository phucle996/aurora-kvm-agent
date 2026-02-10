package config

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type StreamMode string

const (
	StreamModeGRPC      StreamMode = "grpc"
	StreamModeWebSocket StreamMode = "websocket"
)

type Config struct {
	NodeID                string
	Hostname              string
	LibvirtURI            string
	NodePollInterval      time.Duration
	VMPollInterval        time.Duration
	HealthInterval        time.Duration
	ReconnectInterval     time.Duration
	StreamMode            StreamMode
	BackendGRPCAddr       string
	BackendWSURL          string
	BackendToken          string
	TLSEnabled            bool
	TLSSkipVerify         bool
	TLSCAPath             string
	TLSCertPath           string
	TLSKeyPath            string
	LogJSON               bool
	LogLevel              string
	GRPCNodeStreamMethod  string
	GRPCVMStreamMethod    string
	WebSocketWriteTimeout time.Duration
	WebSocketReadTimeout  time.Duration
	WebSocketPingInterval time.Duration
	StreamBufferSize      int
	CollectorErrorBackoff time.Duration
	MaxReconnectJitter    time.Duration
}

func Load() (Config, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown-host"
	}

	cfg := Config{
		NodeID:                env("AURORA_NODE_ID", hostname),
		Hostname:              hostname,
		LibvirtURI:            env("AURORA_LIBVIRT_URI", "qemu+unix:///system"),
		NodePollInterval:      envDuration("AURORA_NODE_POLL_INTERVAL", 3*time.Second),
		VMPollInterval:        envDuration("AURORA_VM_POLL_INTERVAL", 1*time.Second),
		HealthInterval:        envDuration("AURORA_HEALTH_INTERVAL", 10*time.Second),
		ReconnectInterval:     envDuration("AURORA_RECONNECT_INTERVAL", 4*time.Second),
		StreamMode:            StreamMode(strings.ToLower(env("AURORA_STREAM_MODE", string(StreamModeGRPC)))),
		BackendGRPCAddr:       env("AURORA_BACKEND_GRPC_ADDR", "127.0.0.1:8443"),
		BackendWSURL:          env("AURORA_BACKEND_WS_URL", "ws://127.0.0.1:8080/ws/metrics"),
		BackendToken:          env("AURORA_BACKEND_TOKEN", ""),
		TLSEnabled:            envBool("AURORA_TLS_ENABLED", false),
		TLSSkipVerify:         envBool("AURORA_TLS_SKIP_VERIFY", false),
		TLSCAPath:             env("AURORA_TLS_CA_PATH", ""),
		TLSCertPath:           env("AURORA_TLS_CERT_PATH", ""),
		TLSKeyPath:            env("AURORA_TLS_KEY_PATH", ""),
		LogJSON:               envBool("AURORA_LOG_JSON", true),
		LogLevel:              strings.ToLower(env("AURORA_LOG_LEVEL", "info")),
		GRPCNodeStreamMethod:  env("AURORA_GRPC_NODE_STREAM_METHOD", "/aurora.metrics.v1.MetricsService/StreamNodeMetrics"),
		GRPCVMStreamMethod:    env("AURORA_GRPC_VM_STREAM_METHOD", "/aurora.metrics.v1.MetricsService/StreamVMMetrics"),
		WebSocketWriteTimeout: envDuration("AURORA_WS_WRITE_TIMEOUT", 5*time.Second),
		WebSocketReadTimeout:  envDuration("AURORA_WS_READ_TIMEOUT", 15*time.Second),
		WebSocketPingInterval: envDuration("AURORA_WS_PING_INTERVAL", 10*time.Second),
		StreamBufferSize:      envInt("AURORA_STREAM_BUFFER_SIZE", 1024),
		CollectorErrorBackoff: envDuration("AURORA_COLLECTOR_ERROR_BACKOFF", 1500*time.Millisecond),
		MaxReconnectJitter:    envDuration("AURORA_RECONNECT_MAX_JITTER", 900*time.Millisecond),
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.NodeID == "" {
		return errors.New("AURORA_NODE_ID is required")
	}
	if c.LibvirtURI == "" {
		return errors.New("AURORA_LIBVIRT_URI is required")
	}
	if c.NodePollInterval <= 0 || c.VMPollInterval <= 0 {
		return errors.New("poll intervals must be > 0")
	}
	switch c.StreamMode {
	case StreamModeGRPC, StreamModeWebSocket:
	default:
		return fmt.Errorf("unsupported stream mode %q", c.StreamMode)
	}
	if c.StreamMode == StreamModeGRPC && c.BackendGRPCAddr == "" {
		return errors.New("AURORA_BACKEND_GRPC_ADDR is required for grpc mode")
	}
	if c.StreamMode == StreamModeWebSocket && c.BackendWSURL == "" {
		return errors.New("AURORA_BACKEND_WS_URL is required for websocket mode")
	}
	return nil
}

func (c Config) TLSConfig() (*tls.Config, error) {
	if !c.TLSEnabled {
		return nil, nil
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: c.TLSSkipVerify}
	if c.TLSCAPath != "" {
		caBytes, err := os.ReadFile(c.TLSCAPath)
		if err != nil {
			return nil, fmt.Errorf("read CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caBytes) {
			return nil, errors.New("append CA cert failed")
		}
		tlsCfg.RootCAs = pool
	}
	if c.TLSCertPath != "" || c.TLSKeyPath != "" {
		if c.TLSCertPath == "" || c.TLSKeyPath == "" {
			return nil, errors.New("both TLS cert and key are required")
		}
		crt, err := tls.LoadX509KeyPair(c.TLSCertPath, c.TLSKeyPath)
		if err != nil {
			return nil, fmt.Errorf("load mTLS cert/key: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{crt}
	}
	return tlsCfg, nil
}

func env(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return i
}

func envBool(key string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
