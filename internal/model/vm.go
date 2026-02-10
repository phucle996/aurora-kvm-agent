package model

import "time"

// VMMetrics represents per-domain runtime measurements from libvirt domain stats.
type VMMetrics struct {
	NodeID         string    `json:"node_id"`
	VMID           string    `json:"vm_id"`
	VMName         string    `json:"vm_name"`
	State          string    `json:"state"`
	Timestamp      time.Time `json:"timestamp"`
	CPUUsagePct    float64   `json:"cpu_usage_pct"`
	VCPUCount      uint64    `json:"vcpu_count"`
	RAMUsedBytes   uint64    `json:"ram_used_bytes"`
	RAMTotalBytes  uint64    `json:"ram_total_bytes"`
	DiskReadBytes  uint64    `json:"disk_read_bytes"`
	DiskWriteBytes uint64    `json:"disk_write_bytes"`
	NetRxBytes     uint64    `json:"net_rx_bytes"`
	NetTxBytes     uint64    `json:"net_tx_bytes"`
}
