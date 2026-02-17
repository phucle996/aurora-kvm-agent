package model

type SubscribeNodeNetIfRequest struct {
	NodeID        string `json:"node_id"`
	InterfaceName string `json:"interface_name"`
}

type NodeNetIfMetricLite struct {
	NodeID               string  `json:"node_id"`
	InterfaceName        string  `json:"interface_name"`
	TimestampUnix        int64   `json:"timestamp_unix"`
	RxMBS                float64 `json:"rx_mb_s"`
	TxMBS                float64 `json:"tx_mb_s"`
	TotalMBS             float64 `json:"total_mb_s"`
	RxPacketsPerSec      float64 `json:"rx_packets_per_sec"`
	TxPacketsPerSec      float64 `json:"tx_packets_per_sec"`
	RxErrors             uint64  `json:"rx_errors"`
	TxErrors             uint64  `json:"tx_errors"`
	RxDropped            uint64  `json:"rx_dropped"`
	TxDropped            uint64  `json:"tx_dropped"`
	BandwidthUtilPercent float64 `json:"bandwidth_util_percent"`
	SpeedMbps            uint64  `json:"speed_mbps"`
	LinkUp               bool    `json:"link_up"`
}

type NodeNetIfMetricFrame struct {
	Metrics NodeNetIfMetricLite `json:"metrics"`
}
