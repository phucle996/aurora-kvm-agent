package model

type NodeLoadMetrics struct {
	Load1          float64 `json:"load_1"`
	Load5          float64 `json:"load_5"`
	Load15         float64 `json:"load_15"`
	RunQueueLength uint64  `json:"run_queue_length"`
}

type NodeIOMetrics struct {
	DiskReadBytes  uint64                        `json:"disk_read_bytes"`
	DiskWriteBytes uint64                        `json:"disk_write_bytes"`
	NetRxBytes     uint64                        `json:"net_rx_bytes"`
	NetTxBytes     uint64                        `json:"net_tx_bytes"`
	Disks          []NodeDiskMetrics             `json:"disks"`
	Netifs         []NodeNetworkInterfaceMetrics `json:"netifs"`

	NetTotalRxBytesPerSec   float64 `json:"net_total_rx_bytes_per_sec"`
	NetTotalTxBytesPerSec   float64 `json:"net_total_tx_bytes_per_sec"`
	NetTotalRxPacketsPerSec float64 `json:"net_total_rx_packets_per_sec"`
	NetTotalTxPacketsPerSec float64 `json:"net_total_tx_packets_per_sec"`
	NetTotalConnections     uint64  `json:"net_total_connections"`
	NetConntrackCount       uint64  `json:"net_conntrack_count"`
	NetConntrackMax         uint64  `json:"net_conntrack_max"`
}

type NodeProcessMetrics struct {
	ProcessTotal uint64 `json:"process_total"`
	ThreadTotal  uint64 `json:"thread_total"`
	ProcessCount uint64 `json:"process_count"`
	ThreadCount  uint64 `json:"thread_count"`
}
