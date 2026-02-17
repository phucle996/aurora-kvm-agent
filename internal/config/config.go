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

const (
	HardcodedVersion string = "V0.2"
)

type Config struct {
	NodeID            string
	Hostname          string
	LibvirtURI        string
	ProbeListenAddr   string
	HealthInterval    time.Duration
	ReconnectInterval time.Duration
	ShutdownTimeout   time.Duration
	BackendGRPCAddr   string
	BackendToken      string
	AgentVersion      string
	TLSEnabled        bool
	TLSSkipVerify     bool
	TLSCAPath         string
	TLSCertPath       string
	TLSKeyPath        string
	LogLevel          string

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
		NodeID:            env("AURORA_NODE_ID", hostname),
		Hostname:          hostname,
		LibvirtURI:        env("AURORA_LIBVIRT_URI", "qemu+unix:///system"),
		ProbeListenAddr:   env("AURORA_AGENT_PROBE_ADDR", "0.0.0.0:7443"),
		HealthInterval:    envDuration("AURORA_HEALTH_INTERVAL", 10*time.Second),
		ReconnectInterval: envDuration("AURORA_RECONNECT_INTERVAL", 4*time.Second),
		ShutdownTimeout:   envDuration("AURORA_SHUTDOWN_TIMEOUT", 20*time.Second),
		BackendGRPCAddr:   env("AURORA_BACKEND_GRPC_ADDR", "127.0.0.1:3001"),
		BackendToken:      env("AURORA_BACKEND_TOKEN", ""),
		AgentVersion:      HardcodedVersion,
		TLSEnabled:        envBool("AURORA_TLS_ENABLED", false),
		TLSSkipVerify:     envBool("AURORA_TLS_SKIP_VERIFY", false),
		TLSCAPath:         env("AURORA_TLS_CA_PATH", ""),
		TLSCertPath:       env("AURORA_TLS_CERT_PATH", ""),
		TLSKeyPath:        env("AURORA_TLS_KEY_PATH", ""),
		LogLevel:          strings.ToLower(env("AURORA_LOG_LEVEL", "info")),

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
	if strings.TrimSpace(c.AgentVersion) == "" {
		return errors.New("agent version must not be empty")
	}
	if c.LibvirtURI == "" {
		return errors.New("AURORA_LIBVIRT_URI is required")
	}
	if strings.TrimSpace(c.ProbeListenAddr) == "" {
		return errors.New("AURORA_AGENT_PROBE_ADDR is required")
	}

	if c.ShutdownTimeout <= 0 {
		return errors.New("AURORA_SHUTDOWN_TIMEOUT must be > 0")
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
