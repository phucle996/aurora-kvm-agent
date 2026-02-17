package model

// NodeMetricsLite mirrors proto/metrics/node_metric_lite.proto:NodeMetricsLite.
type NodeMetricsLite struct {
	NodeID               string  `json:"node_id"`
	Hostname             string  `json:"hostname"`
	TimestampUnix        int64   `json:"timestamp_unix"`
	CPUUsagePercent      float64 `json:"cpu_usage_percent"`
	CPULoad1             float64 `json:"cpu_load_1"`
	CPULoad5             float64 `json:"cpu_load_5"`
	CPULoad15            float64 `json:"cpu_load_15"`
	CPUCores             uint64  `json:"cpu_cores"`
	CPURunQueue          float64 `json:"cpu_run_queue"`
	RAMUsagePercent      float64 `json:"ram_usage_percent"`
	RAMUsedBytes         uint64  `json:"ram_used_bytes"`
	RAMTotalBytes        uint64  `json:"ram_total_bytes"`
	DiskReadMBS          float64 `json:"disk_read_mb_s"`
	DiskWriteMBS         float64 `json:"disk_write_mb_s"`
	DiskTotalMBS         float64 `json:"disk_total_mb_s"`
	DiskUtilPercent      float64 `json:"disk_util_percent"`
	DiskIOPS             float64 `json:"disk_iops"`
	NetRxMBS             float64 `json:"net_rx_mb_s"`
	NetTxMBS             float64 `json:"net_tx_mb_s"`
	NetTotalMBS          float64 `json:"net_total_mb_s"`
	NetPacketRate        float64 `json:"net_packet_rate"`
	GPUCount             uint64  `json:"gpu_count"`
	GPUTotalUtilPercent  float64 `json:"gpu_total_util_percent"`
	GPUMemoryUtilPercent float64 `json:"gpu_memory_util_percent"`
	GPUMemoryUsedBytes   uint64  `json:"gpu_memory_used_bytes"`
	GPUMemoryTotalBytes  uint64  `json:"gpu_memory_total_bytes"`
	ProcessCount         uint64  `json:"process_count"`
	ThreadCount          uint64  `json:"thread_count"`
	SystemLoadPercent    float64 `json:"system_load_percent"`
}

type NodeMetricsLiteFrame struct {
	NodeID        string          `json:"node_id"`
	TimestampUnix int64           `json:"timestamp_unix"`
	Metrics       NodeMetricsLite `json:"metrics"`
}

type NodeLiteSubscribeRequest struct {
	NodeID          string `json:"node_id"`
	IntervalSeconds uint32 `json:"interval_seconds"`
}
