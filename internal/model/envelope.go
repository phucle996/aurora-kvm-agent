package model

type MetricType string

const (
	MetricTypeNodeLite   MetricType = "node_metrics_lite"
	MetricTypeVMLite     MetricType = "vm_lite_metrics"
	MetricTypeVMRuntime  MetricType = "vm_runtime_metrics"
	MetricTypeNodeInfo   MetricType = "node_info"
	MetricTypeVMInfoSync MetricType = "vm_info_sync"
)

// Envelope is transport-agnostic framing for stream payloads.
type Envelope struct {
	Type          MetricType `json:"type"`
	NodeID        string     `json:"node_id"`
	TimestampUnix int64      `json:"timestamp_unix"`
	Payload       any        `json:"payload"`
}
