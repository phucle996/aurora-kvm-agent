package model

type VMRuntimeDiskMetric struct {
	Name             string  `json:"name"`
	ReadBytesPerSec  float64 `json:"read_bytes_per_sec"`
	WriteBytesPerSec float64 `json:"write_bytes_per_sec"`
	UtilPercent      float64 `json:"util_percent"`
	FSUsagePercent   float64 `json:"fs_usage_percent"`
}

type VMRuntimeGPUMetric struct {
	GPUIndex         uint32  `json:"gpu_index"`
	GPUUUID          string  `json:"gpu_uuid"`
	GPUModel         string  `json:"gpu_model"`
	GPUUtilPercent   float64 `json:"gpu_util_percent"`
	MemoryUsedBytes  uint64  `json:"memory_used_bytes"`
	MemoryTotalBytes uint64  `json:"memory_total_bytes"`
}

type VMRuntimeMetrics struct {
	NodeID               string                `json:"node_id"`
	VMID                 string                `json:"vm_id"`
	VMName               string                `json:"vm_name"`
	TimestampUnix        int64                 `json:"timestamp_unix"`
	CPUUsagePercent      float64               `json:"cpu_usage_percent"`
	CPUStealPercent      float64               `json:"cpu_steal_percent"`
	VCPUCount            uint64                `json:"vcpu_count"`
	Load1                float64               `json:"load_1"`
	Load5                float64               `json:"load_5"`
	Load15               float64               `json:"load_15"`
	RAMTotalBytes        uint64                `json:"ram_total_bytes"`
	RAMUsedBytes         uint64                `json:"ram_used_bytes"`
	RAMUsagePercent      float64               `json:"ram_usage_percent"`
	SwapUsedBytes        uint64                `json:"swap_used_bytes"`
	SwapUsagePercent     float64               `json:"swap_usage_percent"`
	DiskReadBytesPerSec  float64               `json:"disk_read_bytes_per_sec"`
	DiskWriteBytesPerSec float64               `json:"disk_write_bytes_per_sec"`
	DiskReadIOPS         float64               `json:"disk_read_iops"`
	DiskWriteIOPS        float64               `json:"disk_write_iops"`
	DiskUtilPercent      float64               `json:"disk_util_percent"`
	DiskReadBytesTotal   uint64                `json:"disk_read_bytes_total"`
	DiskWriteBytesTotal  uint64                `json:"disk_write_bytes_total"`
	Disks                []VMRuntimeDiskMetric `json:"disks"`
	NetRxBytesPerSec     float64               `json:"net_rx_bytes_per_sec"`
	NetTxBytesPerSec     float64               `json:"net_tx_bytes_per_sec"`
	NetRxPacketsPerSec   float64               `json:"net_rx_packets_per_sec"`
	NetTxPacketsPerSec   float64               `json:"net_tx_packets_per_sec"`
	NetRxErrors          uint64                `json:"net_rx_errors"`
	NetTxErrors          uint64                `json:"net_tx_errors"`
	NetRxBytesTotal      uint64                `json:"net_rx_bytes_total"`
	NetTxBytesTotal      uint64                `json:"net_tx_bytes_total"`
	GPUCount             uint64                `json:"gpu_count"`
	GPUUtilPercent       float64               `json:"gpu_util_percent"`
	GPUMemoryUsedBytes   uint64                `json:"gpu_memory_used_bytes"`
	GPUMemoryTotalBytes  uint64                `json:"gpu_memory_total_bytes"`
	GPUMemoryUtilPercent float64               `json:"gpu_memory_util_percent"`
	GPUProcessCount      uint64                `json:"gpu_process_count"`
	GPUs                 []VMRuntimeGPUMetric  `json:"gpus"`
	VMState              string                `json:"vm_state"`
	UptimeSeconds        uint64                `json:"uptime_seconds"`
}

type VMRuntimeMetricsStreamFrame struct {
	NodeID        string           `json:"node_id"`
	TimestampUnix int64            `json:"timestamp_unix"`
	Metrics       VMRuntimeMetrics `json:"metrics"`
}

type VMRuntimeSubscribeRequest struct {
	NodeID          string `json:"node_id"`
	VMID            string `json:"vm_id"`
	IntervalSeconds uint32 `json:"interval_seconds"`
}
