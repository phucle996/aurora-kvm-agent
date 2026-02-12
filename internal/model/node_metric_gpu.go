package model

type NodeGPUProcessMetrics struct {
	PID             uint64 `json:"pid"`
	Name            string `json:"name"`
	MemoryUsedBytes uint64 `json:"memory_used_bytes"`
	Username        string `json:"username"`
	ContainerID     string `json:"container_id"`
}

type NodeGPUDeviceMetrics struct {
	Index             uint64 `json:"index"`
	UUID              string `json:"uuid"`
	Name              string `json:"name"`
	Vendor            string `json:"vendor"`
	Model             string `json:"model"`
	DriverVersion     string `json:"driver_version"`
	VBIOSVersion      string `json:"vbios_version"`
	PCIAddress        string `json:"pci_address"`
	BusID             string `json:"bus_id"`
	Serial            string `json:"serial"`
	Architecture      string `json:"architecture"`
	ComputeCapability string `json:"compute_capability"`

	GPUUtilPercent    float64 `json:"gpu_util_percent"`
	MemoryUtilPercent float64 `json:"memory_util_percent"`

	MemoryTotalBytes    uint64 `json:"memory_total_bytes"`
	MemoryUsedBytes     uint64 `json:"memory_used_bytes"`
	MemoryFreeBytes     uint64 `json:"memory_free_bytes"`
	MemoryReservedBytes uint64 `json:"memory_reserved_bytes"`

	PowerUsageWatts         float64 `json:"power_usage_watts"`
	PowerLimitWatts         float64 `json:"power_limit_watts"`
	EnergyConsumptionJoules float64 `json:"energy_consumption_joules"`

	TemperatureCelsius       float64 `json:"temperature_celsius"`
	TemperatureMemoryCelsius float64 `json:"temperature_memory_celsius"`
	FanSpeedPercent          float64 `json:"fan_speed_percent"`

	ClockCoreMhz uint64 `json:"clock_core_mhz"`
	ClockMemMhz  uint64 `json:"clock_mem_mhz"`

	PCIERxBytesPerSec        float64 `json:"pcie_rx_bytes_per_sec"`
	PCIETxBytesPerSec        float64 `json:"pcie_tx_bytes_per_sec"`
	PCIELinkGen              uint64  `json:"pcie_link_gen"`
	PCIELinkWidth            uint64  `json:"pcie_link_width"`
	PCIEBandwidthUtilPercent float64 `json:"pcie_bandwidth_util_percent"`

	ProcessCount uint64                  `json:"process_count"`
	Processes    []NodeGPUProcessMetrics `json:"processes"`

	VirtualizationMode string `json:"virtualization_mode"`
	ComputeMode        string `json:"compute_mode"`
	PersistenceMode    string `json:"persistence_mode"`

	ThrottleReasons string `json:"throttle_reasons"`
	ThermalThrottle bool   `json:"thermal_throttle"`
	PowerThrottle   bool   `json:"power_throttle"`
	ClockThrottle   bool   `json:"clock_throttle"`

	// NVIDIA-only
	SMUtilPercent      float64 `json:"sm_util_percent"`
	TensorUtilPercent  float64 `json:"tensor_util_percent"`
	EncoderUtilPercent float64 `json:"encoder_util_percent"`
	DecoderUtilPercent float64 `json:"decoder_util_percent"`

	ClockSMMhz    uint64 `json:"clock_sm_mhz"`
	ClockVideoMhz uint64 `json:"clock_video_mhz"`

	BAR1MemoryBytes      uint64 `json:"bar1_memory_bytes"`
	ECCEnabled           bool   `json:"ecc_enabled"`
	ECCErrorsCorrected   uint64 `json:"ecc_errors_corrected"`
	ECCErrorsUncorrected uint64 `json:"ecc_errors_uncorrected"`

	MIGEnabled          bool     `json:"mig_enabled"`
	MIGInstances        uint64   `json:"mig_instances"`
	MIGMemoryBytes      uint64   `json:"mig_memory_bytes"`
	MIGComputeInstances uint64   `json:"mig_compute_instances"`
	MIGProfiles         []string `json:"mig_profiles"`

	NVLinkRxBytesPerSec float64 `json:"nvlink_rx_bytes_per_sec"`
	NVLinkTxBytesPerSec float64 `json:"nvlink_tx_bytes_per_sec"`
	NVLinkLinkCount     uint64  `json:"nvlink_link_count"`

	UtilizationGPUAvg1m  float64 `json:"utilization_gpu_avg_1m"`
	UtilizationGPUAvg5m  float64 `json:"utilization_gpu_avg_5m"`
	UtilizationGPUAvg15m float64 `json:"utilization_gpu_avg_15m"`

	PowerDrawMaxWatts float64 `json:"power_draw_max_watts"`
	PowerDrawMinWatts float64 `json:"power_draw_min_watts"`

	ClocksThrottleReasons string `json:"clocks_throttle_reasons"`

	// AMD-only
	GPUBusyPercent       float64 `json:"gpu_busy_percent"`
	MemoryBusyPercent    float64 `json:"memory_busy_percent"`
	TemperatureEdgeC     float64 `json:"temperature_edge_celsius"`
	TemperatureHotspotC  float64 `json:"temperature_hotspot_celsius"`
	PowerCapWatts        float64 `json:"power_cap_watts"`
	PowerAverageWatts    float64 `json:"power_average_watts"`
	ClockSocMhz          uint64  `json:"clock_soc_mhz"`
	ClockMemoryMhz       uint64  `json:"clock_memory_mhz"`
	PCIeReplayCount      uint64  `json:"pcie_replay_count"`
	PCIeBandwidthMbps    float64 `json:"pcie_bandwidth_mbps"`
	RASErrorsCorrected   uint64  `json:"ras_errors_corrected"`
	RASErrorsUncorrected uint64  `json:"ras_errors_uncorrected"`
	ComputePartition     string  `json:"compute_partition"`
	MemoryPartition      string  `json:"memory_partition"`
	GFXVersion           string  `json:"gfx_version"`
	ASICName             string  `json:"asic_name"`
}

type NodeGPUMetrics struct {
	// Legacy fields (keep compatibility)
	GPUCount            uint64  `json:"gpu_count"`
	GPUModel            string  `json:"gpu_model"`
	GPUUsagePct         float64 `json:"gpu_usage_pct"`
	GPUMemoryUsedBytes  uint64  `json:"gpu_memory_used_bytes"`
	GPUMemoryTotalBytes uint64  `json:"gpu_memory_total_bytes"`

	// New fields
	GPUs                    []NodeGPUDeviceMetrics `json:"gpus"`
	GPUTotalCount           uint64                 `json:"gpu_total_count"`
	GPUTotalMemoryBytes     uint64                 `json:"gpu_total_memory_bytes"`
	GPUTotalMemoryUsedBytes uint64                 `json:"gpu_total_memory_used_bytes"`
	GPUTotalUtilPercent     float64                `json:"gpu_total_util_percent"`
	GPUTotalPowerWatts      float64                `json:"gpu_total_power_watts"`
}
