package model

type NodeMemoryMetrics struct {
	RAMTotalBytes         uint64   `json:"ram_total_bytes"`
	RAMUsedBytes          uint64   `json:"ram_used_bytes"`
	RAMFreeBytes          uint64   `json:"ram_free_bytes"`
	RAMAvailableBytes     uint64   `json:"ram_available_bytes"`
	RAMCachedBytes        uint64   `json:"ram_cached_bytes"`
	RAMBufferBytes        uint64   `json:"ram_buffer_bytes"`
	RAMSharedBytes        uint64   `json:"ram_shared_bytes"`
	RAMSlabBytes          uint64   `json:"ram_slab_bytes"`
	RAMActiveBytes        uint64   `json:"ram_active_bytes"`
	RAMInactiveBytes      uint64   `json:"ram_inactive_bytes"`
	RAMDirtyBytes         uint64   `json:"ram_dirty_bytes"`
	RAMWritebackBytes     uint64   `json:"ram_writeback_bytes"`
	RAMUsagePercent       float64  `json:"ram_usage_percent"`
	RAMAvailablePercent   float64  `json:"ram_available_percent"`
	SwapTotalBytes        uint64   `json:"swap_total_bytes"`
	SwapUsedBytes         uint64   `json:"swap_used_bytes"`
	SwapFreeBytes         uint64   `json:"swap_free_bytes"`
	SwapUsagePercent      float64  `json:"swap_usage_percent"`
	SwapInBytesPerSec     float64  `json:"swap_in_bytes_per_sec"`
	SwapOutBytesPerSec    float64  `json:"swap_out_bytes_per_sec"`
	PageFaultsPerSec      float64  `json:"page_faults_per_sec"`
	MajorPageFaultsPerSec float64  `json:"major_page_faults_per_sec"`
	OOMKillCount          uint64   `json:"oom_kill_count"`
	RAMStickCount         uint64   `json:"ram_stick_count"`
	RAMStickSlot          []string `json:"ram_stick_slot"`
	RAMStickSizeBytes     []uint64 `json:"ram_stick_size_bytes"`
	RAMStickSpeedMhz      []uint64 `json:"ram_stick_speed_mhz"`
	RAMStickType          []string `json:"ram_stick_type"`
	RAMStickModel         []string `json:"ram_stick_model"`
	RAMStickManufacturer  []string `json:"ram_stick_manufacturer"`
	RAMStickPartNumber    []string `json:"ram_stick_part_number"`
	RAMStickSerial        []string `json:"ram_stick_serial"`
	RAMTotalPhysicalBytes uint64   `json:"ram_total_physical_bytes"`
	RAMECCEnabled         bool     `json:"ram_ecc_enabled"`
	HugePagesTotal        uint64   `json:"hugepages_total"`
	HugePagesUsed         uint64   `json:"hugepages_used"`
	HugePagesFree         uint64   `json:"hugepages_free"`
	MemoryPressurePercent float64  `json:"memory_pressure_percent"`
}
