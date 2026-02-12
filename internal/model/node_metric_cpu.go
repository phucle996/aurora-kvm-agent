package model

type NodeCPUMetrics struct {
	CPUUsagePct              float64   `json:"cpu_usage_pct"`
	CPUUsageTotalPercent     float64   `json:"cpu_usage_total_percent"`
	CPUUserPercent           float64   `json:"cpu_user_percent"`
	CPUSystemPercent         float64   `json:"cpu_system_percent"`
	CPUIdlePercent           float64   `json:"cpu_idle_percent"`
	CPUIOWaitPercent         float64   `json:"cpu_iowait_percent"`
	CPUStealPercent          float64   `json:"cpu_steal_percent"`
	CPUIRQPercent            float64   `json:"cpu_irq_percent"`
	CPUSoftIRQPercent        float64   `json:"cpu_softirq_percent"`
	CPUNicePercent           float64   `json:"cpu_nice_percent"`
	CPUCores                 uint64    `json:"cpu_cores"`
	CPUMhz                   uint64    `json:"cpu_mhz"`
	CPUFreqCurrentMhz        uint64    `json:"cpu_freq_current_mhz"`
	CPUFreqMinMhz            uint64    `json:"cpu_freq_min_mhz"`
	CPUFreqMaxMhz            uint64    `json:"cpu_freq_max_mhz"`
	CPUCoreCountPhysical     uint64    `json:"cpu_core_count_physical"`
	CPUThreadCountLogical    uint64    `json:"cpu_thread_count_logical"`
	CPUSocketCount           uint64    `json:"cpu_socket_count"`
	CPUCoreUsagePercent      []float64 `json:"cpu_core_usage_percent"`
	ContextSwitchesPerSec    float64   `json:"context_switches_per_sec"`
	InterruptsPerSec         float64   `json:"interrupts_per_sec"`
	CPUTemperatureCelsius    float64   `json:"cpu_temperature_celsius"`
	CPUModel                 string    `json:"cpu_model"`
	CPUVendor                string    `json:"cpu_vendor"`
	CPUModelName             string    `json:"cpu_model_name"`
	CPUArchitecture          string    `json:"cpu_architecture"`
	CPUVirtualizationSupport string    `json:"cpu_virtualization_support"`
	CPUCacheL1               string    `json:"cpu_cache_l1"`
	CPUCacheL2               string    `json:"cpu_cache_l2"`
	CPUCacheL3               string    `json:"cpu_cache_l3"`
	CPUBaseMhz               uint64    `json:"cpu_base_mhz"`
	CPUBurstMhz              uint64    `json:"cpu_burst_mhz"`
}
