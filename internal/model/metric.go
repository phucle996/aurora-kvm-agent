package model

import "time"

type MetricType string

const (
	MetricTypeNode MetricType = "node_metrics"
	MetricTypeVM   MetricType = "vm_metrics"
)

// Envelope is transport-agnostic framing for stream payloads.
type Envelope struct {
	Type      MetricType `json:"type"`
	NodeID    string     `json:"node_id"`
	Timestamp time.Time  `json:"timestamp"`
	Payload   any        `json:"payload"`
}
