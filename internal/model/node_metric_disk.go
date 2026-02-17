package model

type SubscribeNodeDiskRequest struct {
	NodeID   string `json:"node_id"`
	DiskName string `json:"disk_name"`
}

type NodeDiskMetricLite struct {
	NodeID         string  `json:"node_id"`
	DiskName       string  `json:"disk_name"`
	TimestampUnix  int64   `json:"timestamp_unix"`
	ReadMBS        float64 `json:"read_mb_s"`
	WriteMBS       float64 `json:"write_mb_s"`
	TotalMBS       float64 `json:"total_mb_s"`
	ReadIOPS       float64 `json:"read_iops"`
	WriteIOPS      float64 `json:"write_iops"`
	TotalIOPS      float64 `json:"total_iops"`
	UtilPercent    float64 `json:"util_percent"`
	ReadLatencyMs  float64 `json:"read_latency_ms"`
	WriteLatencyMs float64 `json:"write_latency_ms"`
	FSUsagePercent float64 `json:"fs_usage_percent"`
	SizeBytes      uint64  `json:"size_bytes"`
	Mountpoint     string  `json:"mountpoint"`
}

type NodeDiskMetricFrame struct {
	Metrics NodeDiskMetricLite `json:"metrics"`
}
