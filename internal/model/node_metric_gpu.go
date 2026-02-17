package model

type GpuVendor int32

const (
	GpuVendorUnknown GpuVendor = 0
	GpuVendorNvidia  GpuVendor = 1
	GpuVendorAmd     GpuVendor = 2
	GpuVendorIntel   GpuVendor = 3
)

type SubscribeGpuRequest struct {
	NodeID   string `json:"node_id"`
	GPUIndex uint64 `json:"gpu_index"`
	GPUUUID  string `json:"gpu_uuid"`
}

type NodeGpuMetricLite struct {
	NodeID                   string       `json:"node_id"`
	GPUIndex                 uint64       `json:"gpu_index"`
	GPUUUID                  string       `json:"gpu_uuid"`
	Vendor                   GpuVendor    `json:"vendor"`
	TimestampUnix            int64        `json:"timestamp_unix"`
	GPUUtilPercent           float64      `json:"gpu_util_percent"`
	MemoryUtilPercent        float64      `json:"memory_util_percent"`
	MemoryTotalBytes         uint64       `json:"memory_total_bytes"`
	MemoryUsedBytes          uint64       `json:"memory_used_bytes"`
	MemoryFreeBytes          uint64       `json:"memory_free_bytes"`
	PowerUsageWatts          float64      `json:"power_usage_watts"`
	PowerLimitWatts          float64      `json:"power_limit_watts"`
	TemperatureCelsius       float64      `json:"temperature_celsius"`
	FanSpeedPercent          float64      `json:"fan_speed_percent"`
	ClockCoreMhz             uint64       `json:"clock_core_mhz"`
	ClockMemMhz              uint64       `json:"clock_mem_mhz"`
	PCIeRxMBS                float64      `json:"pcie_rx_mb_s"`
	PCIeTxMBS                float64      `json:"pcie_tx_mb_s"`
	PCIeBandwidthUtilPercent float64      `json:"pcie_bandwidth_util_percent"`
	EncoderUtilPercent       float64      `json:"encoder_util_percent"`
	DecoderUtilPercent       float64      `json:"decoder_util_percent"`
	ComputeUtilPercent       float64      `json:"compute_util_percent"`
	Nvidia                   *NvidiaExtra `json:"nvidia,omitempty"`
	Amd                      *AmdExtra    `json:"amd,omitempty"`
}

type NvidiaExtra struct {
	MIGEnabled               bool    `json:"mig_enabled"`
	MIGInstances             uint64  `json:"mig_instances"`
	TensorUtilPercent        float64 `json:"tensor_util_percent"`
	SMUtilPercent            float64 `json:"sm_util_percent"`
	NVLinkRxMBS              float64 `json:"nvlink_rx_mb_s"`
	NVLinkTxMBS              float64 `json:"nvlink_tx_mb_s"`
	TemperatureMemoryCelsius float64 `json:"temperature_memory_celsius"`
	PState                   uint64  `json:"pstate"`
	ComputeMode              string  `json:"compute_mode"`
}

type AmdExtra struct {
	GFXUtilPercent              float64 `json:"gfx_util_percent"`
	MemoryControllerUtilPercent float64 `json:"memory_controller_util_percent"`
	TemperatureEdgeCelsius      float64 `json:"temperature_edge_celsius"`
	TemperatureHotspotCelsius   float64 `json:"temperature_hotspot_celsius"`
	PowerAverageWatts           float64 `json:"power_average_watts"`
	ClockSocMhz                 uint64  `json:"clock_soc_mhz"`
	ClockMemoryMhz              uint64  `json:"clock_memory_mhz"`
}

type NodeGpuMetricFrame struct {
	Metrics NodeGpuMetricLite `json:"metrics"`
}
