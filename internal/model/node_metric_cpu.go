package model

type SubscribeNodeCpuRequest struct {
	NodeID string `json:"node_id"`
}

type NodeCpuMetrics struct {
	NodeID                string    `json:"node_id"`
	TimestampUnix         int64     `json:"timestamp_unix"`
	CPUUsagePercent       float64   `json:"cpu_usage_percent"`
	CPUUserPercent        float64   `json:"cpu_user_percent"`
	CPUSystemPercent      float64   `json:"cpu_system_percent"`
	CPUIdlePercent        float64   `json:"cpu_idle_percent"`
	CPUIOWaitPercent      float64   `json:"cpu_iowait_percent"`
	CPUStealPercent       float64   `json:"cpu_steal_percent"`
	Load1                 float64   `json:"load_1"`
	Load5                 float64   `json:"load_5"`
	Load15                float64   `json:"load_15"`
	CPUCoreUsagePercent   []float64 `json:"cpu_core_usage_percent"`
	CPUFreqCurrentMhz     uint64    `json:"cpu_freq_current_mhz"`
	CPUFreqMinMhz         uint64    `json:"cpu_freq_min_mhz"`
	CPUFreqMaxMhz         uint64    `json:"cpu_freq_max_mhz"`
	RunQueueLength        uint64    `json:"run_queue_length"`
	ContextSwitchesPerSec float64   `json:"context_switches_per_sec"`
	InterruptsPerSec      float64   `json:"interrupts_per_sec"`
	CPUTemperatureCelsius float64   `json:"cpu_temperature_celsius"`
}

type NodeCpuMetricsFrame struct {
	Metrics NodeCpuMetrics `json:"metrics"`
}
