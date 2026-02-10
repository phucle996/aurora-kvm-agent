package model

import "time"

// NodeMetrics represents host-level measurements sampled from libvirt + kernel counters.
type NodeMetrics struct {
	NodeID         string    `json:"node_id"`
	Hostname       string    `json:"hostname"`
	Timestamp      time.Time `json:"timestamp"`
	CPUUsagePct    float64   `json:"cpu_usage_pct"`
	CPUCores       uint64    `json:"cpu_cores"`
	CPUMhz         uint64    `json:"cpu_mhz"`
	Load1          float64   `json:"load_1"`
	Load5          float64   `json:"load_5"`
	Load15         float64   `json:"load_15"`
	RAMUsedBytes   uint64    `json:"ram_used_bytes"`
	RAMTotalBytes  uint64    `json:"ram_total_bytes"`
	DiskReadBytes  uint64    `json:"disk_read_bytes"`
	DiskWriteBytes uint64    `json:"disk_write_bytes"`
	NetRxBytes     uint64    `json:"net_rx_bytes"`
	NetTxBytes     uint64    `json:"net_tx_bytes"`
}
