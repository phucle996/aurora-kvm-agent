package model

type GetNodeHardwareRequest struct {
	NodeID string `json:"node_id"`
}

type NodeCpuInfo struct {
	Model                 string `json:"model"`
	Vendor                string `json:"vendor"`
	Cores                 uint64 `json:"cores"`
	Threads               uint64 `json:"threads"`
	Sockets               uint64 `json:"sockets"`
	BaseMhz               uint64 `json:"base_mhz"`
	MaxMhz                uint64 `json:"max_mhz"`
	Architecture          string `json:"architecture"`
	VirtualizationSupport bool   `json:"virtualization_support"`
}

type NodeRamStick struct {
	Slot         string `json:"slot"`
	SizeBytes    uint64 `json:"size_bytes"`
	SpeedMhz     uint64 `json:"speed_mhz"`
	Type         string `json:"type"`
	Model        string `json:"model"`
	Manufacturer string `json:"manufacturer"`
	Serial       string `json:"serial"`
}

type NodeDiskInfo struct {
	Name       string `json:"name"`
	Model      string `json:"model"`
	Serial     string `json:"serial"`
	Type       string `json:"type"`
	SizeBytes  uint64 `json:"size_bytes"`
	Mountpoint string `json:"mountpoint"`
	Filesystem string `json:"filesystem"`
}

type NodeNetIfInfo struct {
	Name       string `json:"name"`
	MacAddress string `json:"mac_address"`
	MTU        uint64 `json:"mtu"`
	SpeedMbps  uint64 `json:"speed_mbps"`
	Driver     string `json:"driver"`
	PCIAddress string `json:"pci_address"`
	Type       string `json:"type"`
}

type NodeGpuInfo struct {
	Index            uint64 `json:"index"`
	UUID             string `json:"uuid"`
	Vendor           string `json:"vendor"`
	Model            string `json:"model"`
	MemoryTotalBytes uint64 `json:"memory_total_bytes"`
	PCIAddress       string `json:"pci_address"`
	DriverVersion    string `json:"driver_version"`
}

type NodeHardwareInfo struct {
	NodeID          string          `json:"node_id"`
	Hostname        string          `json:"hostname"`
	CPU             NodeCpuInfo     `json:"cpu"`
	RAMSticks       []NodeRamStick  `json:"ram_sticks"`
	Disks           []NodeDiskInfo  `json:"disks"`
	NetIfs          []NodeNetIfInfo `json:"netifs"`
	GPUs            []NodeGpuInfo   `json:"gpus"`
	CollectedAtUnix int64           `json:"collected_at_unix"`
}
