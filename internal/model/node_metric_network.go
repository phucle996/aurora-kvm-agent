package model

type NodeNetworkInterfaceMetrics struct {
	Name       string `json:"name"`
	MacAddress string `json:"mac_address"`
	MTU        uint64 `json:"mtu"`
	SpeedMbps  uint64 `json:"speed_mbps"`
	Duplex     string `json:"duplex"`
	Operstate  string `json:"operstate"`
	Type       string `json:"type"`
	Driver     string `json:"driver"`
	PCIAddress string `json:"pci_address"`
	LinkUp     bool   `json:"link_up"`

	RxBytesPerSec   float64 `json:"rx_bytes_per_sec"`
	TxBytesPerSec   float64 `json:"tx_bytes_per_sec"`
	RxPacketsPerSec float64 `json:"rx_packets_per_sec"`
	TxPacketsPerSec float64 `json:"tx_packets_per_sec"`

	RxBytesTotal   uint64 `json:"rx_bytes_total"`
	TxBytesTotal   uint64 `json:"tx_bytes_total"`
	RxPacketsTotal uint64 `json:"rx_packets_total"`
	TxPacketsTotal uint64 `json:"tx_packets_total"`

	RxErrors      uint64 `json:"rx_errors"`
	TxErrors      uint64 `json:"tx_errors"`
	RxDropped     uint64 `json:"rx_dropped"`
	TxDropped     uint64 `json:"tx_dropped"`
	RxOverruns    uint64 `json:"rx_overruns"`
	TxOverruns    uint64 `json:"tx_overruns"`
	RxFIFOErrors  uint64 `json:"rx_fifo_errors"`
	TxFIFOErrors  uint64 `json:"tx_fifo_errors"`
	Collisions    uint64 `json:"collisions"`
	CarrierErrors uint64 `json:"carrier_errors"`

	RxQueueLength uint64 `json:"rx_queue_length"`
	TxQueueLength uint64 `json:"tx_queue_length"`

	RxCompressed uint64 `json:"rx_compressed"`
	TxCompressed uint64 `json:"tx_compressed"`
	Multicast    uint64 `json:"multicast"`
	Broadcast    uint64 `json:"broadcast"`

	BandwidthUtilPercent float64 `json:"bandwidth_util_percent"`
	RxLatencyMs          float64 `json:"rx_latency_ms"`
	TxLatencyMs          float64 `json:"tx_latency_ms"`

	Bridge     string   `json:"bridge"`
	VLANID     uint64   `json:"vlan_id"`
	BondMaster string   `json:"bond_master"`
	BondMode   string   `json:"bond_mode"`
	BondSlaves []string `json:"bond_slaves"`

	TCPConnectionsTotal uint64 `json:"tcp_connections_total"`
	TCPEstablished      uint64 `json:"tcp_established"`
	TCPSynSent          uint64 `json:"tcp_syn_sent"`
	TCPSynRecv          uint64 `json:"tcp_syn_recv"`
	TCPFinWait1         uint64 `json:"tcp_fin_wait1"`
	TCPFinWait2         uint64 `json:"tcp_fin_wait2"`
	TCPTimeWait         uint64 `json:"tcp_time_wait"`
	TCPClose            uint64 `json:"tcp_close"`
	TCPCloseWait        uint64 `json:"tcp_close_wait"`
	TCPLastAck          uint64 `json:"tcp_last_ack"`
	TCPListen           uint64 `json:"tcp_listen"`
	TCPClosing          uint64 `json:"tcp_closing"`

	UDPInDatagrams  uint64 `json:"udp_in_datagrams"`
	UDPOutDatagrams uint64 `json:"udp_out_datagrams"`
	UDPInErrors     uint64 `json:"udp_in_errors"`
	UDPNoPorts      uint64 `json:"udp_no_ports"`

	IPInPackets  uint64 `json:"ip_in_packets"`
	IPOutPackets uint64 `json:"ip_out_packets"`
	IPInErrors   uint64 `json:"ip_in_errors"`
	IPOutErrors  uint64 `json:"ip_out_errors"`
	IPForwarded  uint64 `json:"ip_forwarded"`

	DropsTotal  uint64 `json:"drops_total"`
	ErrorsTotal uint64 `json:"errors_total"`
}
