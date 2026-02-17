package model

type SubscribeNodeRamRequest struct {
	NodeID string `json:"node_id"`
}

type NodeRamMetrics struct {
	NodeID                string  `json:"node_id"`
	TimestampUnix         int64   `json:"timestamp_unix"`
	RAMTotalBytes         uint64  `json:"ram_total_bytes"`
	RAMUsedBytes          uint64  `json:"ram_used_bytes"`
	RAMFreeBytes          uint64  `json:"ram_free_bytes"`
	RAMAvailableBytes     uint64  `json:"ram_available_bytes"`
	RAMUsagePercent       float64 `json:"ram_usage_percent"`
	RAMAvailablePercent   float64 `json:"ram_available_percent"`
	RAMCachedBytes        uint64  `json:"ram_cached_bytes"`
	RAMBufferBytes        uint64  `json:"ram_buffer_bytes"`
	RAMSharedBytes        uint64  `json:"ram_shared_bytes"`
	RAMSlabBytes          uint64  `json:"ram_slab_bytes"`
	RAMActiveBytes        uint64  `json:"ram_active_bytes"`
	RAMInactiveBytes      uint64  `json:"ram_inactive_bytes"`
	SwapTotalBytes        uint64  `json:"swap_total_bytes"`
	SwapUsedBytes         uint64  `json:"swap_used_bytes"`
	SwapFreeBytes         uint64  `json:"swap_free_bytes"`
	SwapUsagePercent      float64 `json:"swap_usage_percent"`
	SwapInBytesPerSec     float64 `json:"swap_in_bytes_per_sec"`
	SwapOutBytesPerSec    float64 `json:"swap_out_bytes_per_sec"`
	MemoryPressurePercent float64 `json:"memory_pressure_percent"`
	PageFaultsPerSec      float64 `json:"page_faults_per_sec"`
	MajorPageFaultsPerSec float64 `json:"major_page_faults_per_sec"`
	OOMKillCount          uint64  `json:"oom_kill_count"`
	HugepagesTotal        uint64  `json:"hugepages_total"`
	HugepagesUsed         uint64  `json:"hugepages_used"`
	HugepagesFree         uint64  `json:"hugepages_free"`
}

type NodeRamMetricsFrame struct {
	Metrics NodeRamMetrics `json:"metrics"`
}
