package model

// VMLiteMetrics mirrors proto/vm/vm_metric.proto:VMLiteMetrics.
type VMLiteMetrics struct {
	NodeID           string  `json:"node_id"`
	VMID             string  `json:"vm_id"`
	VMName           string  `json:"vm_name"`
	State            string  `json:"state"`
	TimestampUnix    int64   `json:"timestamp_unix"`
	CPUUsagePct      float64 `json:"cpu_usage_pct"`
	VCPUCount        uint64  `json:"vcpu_count"`
	RAMUsedBytes     uint64  `json:"ram_used_bytes"`
	RAMTotalBytes    uint64  `json:"ram_total_bytes"`
	NetRxBytesPerSec float64 `json:"net_rx_bytes_per_sec"`
	NetTxBytesPerSec float64 `json:"net_tx_bytes_per_sec"`
}

type VMLiteMetricsStreamFrame struct {
	NodeID        string          `json:"node_id"`
	TimestampUnix int64           `json:"timestamp_unix"`
	Metrics       []VMLiteMetrics `json:"metrics"`
}

type VMLiteSubscribeRequest struct {
	NodeID          string `json:"node_id"`
	IntervalSeconds uint32 `json:"interval_seconds"`
}
