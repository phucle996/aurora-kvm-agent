package model

type NodeDiskMetrics struct {
	Name             string  `json:"name"`
	Model            string  `json:"model"`
	Serial           string  `json:"serial"`
	Type             string  `json:"type"`
	SizeBytes        uint64  `json:"size_bytes"`
	Mountpoint       string  `json:"mountpoint"`
	Filesystem       string  `json:"filesystem"`
	ReadBytesPerSec  float64 `json:"read_bytes_per_sec"`
	WriteBytesPerSec float64 `json:"write_bytes_per_sec"`
	ReadIOPS         float64 `json:"read_iops"`
	WriteIOPS        float64 `json:"write_iops"`
	ReadLatencyMs    float64 `json:"read_latency_ms"`
	WriteLatencyMs   float64 `json:"write_latency_ms"`
	QueueLength      uint64  `json:"queue_length"`
	UtilPercent      float64 `json:"util_percent"`
	ReadBytesTotal   uint64  `json:"read_bytes_total"`
	WriteBytesTotal  uint64  `json:"write_bytes_total"`
	ReadOpsTotal     uint64  `json:"read_ops_total"`
	WriteOpsTotal    uint64  `json:"write_ops_total"`
	FSTotalBytes     uint64  `json:"fs_total_bytes"`
	FSUsedBytes      uint64  `json:"fs_used_bytes"`
	FSFreeBytes      uint64  `json:"fs_free_bytes"`
	FSUsagePercent   float64 `json:"fs_usage_percent"`
	InodeTotal       uint64  `json:"inode_total"`
	InodeUsed        uint64  `json:"inode_used"`
	InodeFree        uint64  `json:"inode_free"`
}

