package model

import "time"

type NodeCPUStaticInfo struct {
	CPUModel                  string `json:"cpu_model"`
	CPUVendor                 string `json:"cpu_vendor"`
	CPUCacheL1                string `json:"cpu_cache_l1"`
	CPUCacheL2                string `json:"cpu_cache_l2"`
	CPUCacheL3                string `json:"cpu_cache_l3"`
	CPUArchitecture           string `json:"cpu_architecture"`
	CPUVirtualizationSupport  string `json:"cpu_virtualization_support"`
	CPUBaseMhz                uint64 `json:"cpu_base_mhz"`
	CPUBurstMhz               uint64 `json:"cpu_burst_mhz"`
}

type NodeRAMStickStaticInfo struct {
	Slot         string `json:"slot"`
	Model        string `json:"model"`
	Serial       string `json:"serial"`
	Manufacturer string `json:"manufacturer"`
}

type NodeDiskStaticInfo struct {
	Name       string `json:"name"`
	Model      string `json:"model"`
	Serial     string `json:"serial"`
	Filesystem string `json:"filesystem"`
	Mountpoint string `json:"mountpoint"`
}

type NodeNetIfStaticInfo struct {
	Name       string `json:"name"`
	MacAddress string `json:"mac_address"`
	Driver     string `json:"driver"`
	PCIAddress string `json:"pci_address"`
}

type NodeGPUStaticInfo struct {
	Index  uint64 `json:"index"`
	Vendor string `json:"vendor"`
	Model  string `json:"model"`
	UUID   string `json:"uuid"`
}

type NodeStaticMetrics struct {
	NodeID    string    `json:"node_id"`
	Hostname  string    `json:"hostname"`
	Timestamp time.Time `json:"timestamp"`

	CPU       NodeCPUStaticInfo        `json:"cpu"`
	RAMSticks []NodeRAMStickStaticInfo `json:"ram_sticks"`
	Disks     []NodeDiskStaticInfo     `json:"disks"`
	NetIfs    []NodeNetIfStaticInfo    `json:"netifs"`
	GPUs      []NodeGPUStaticInfo      `json:"gpus"`
}

func BuildNodeStaticMetrics(m NodeMetrics) NodeStaticMetrics {
	static := NodeStaticMetrics{
		NodeID:    m.NodeID,
		Hostname:  m.Hostname,
		Timestamp: m.Timestamp,
		CPU: NodeCPUStaticInfo{
			CPUModel:                 m.CPUModel,
			CPUVendor:                m.CPUVendor,
			CPUCacheL1:               m.CPUCacheL1,
			CPUCacheL2:               m.CPUCacheL2,
			CPUCacheL3:               m.CPUCacheL3,
			CPUArchitecture:          m.CPUArchitecture,
			CPUVirtualizationSupport: m.CPUVirtualizationSupport,
			CPUBaseMhz:               m.CPUBaseMhz,
			CPUBurstMhz:              m.CPUBurstMhz,
		},
		RAMSticks: make([]NodeRAMStickStaticInfo, 0, len(m.RAMStickSlot)),
		Disks:     make([]NodeDiskStaticInfo, 0, len(m.Disks)),
		NetIfs:    make([]NodeNetIfStaticInfo, 0, len(m.Netifs)),
		GPUs:      make([]NodeGPUStaticInfo, 0, len(m.GPUs)),
	}

	for idx, slot := range m.RAMStickSlot {
		stick := NodeRAMStickStaticInfo{Slot: slot}
		if idx < len(m.RAMStickModel) {
			stick.Model = m.RAMStickModel[idx]
		}
		if idx < len(m.RAMStickSerial) {
			stick.Serial = m.RAMStickSerial[idx]
		}
		if idx < len(m.RAMStickManufacturer) {
			stick.Manufacturer = m.RAMStickManufacturer[idx]
		}
		static.RAMSticks = append(static.RAMSticks, stick)
	}

	for _, disk := range m.Disks {
		static.Disks = append(static.Disks, NodeDiskStaticInfo{
			Name:       disk.Name,
			Model:      disk.Model,
			Serial:     disk.Serial,
			Filesystem: disk.Filesystem,
			Mountpoint: disk.Mountpoint,
		})
	}

	for _, netIf := range m.Netifs {
		static.NetIfs = append(static.NetIfs, NodeNetIfStaticInfo{
			Name:       netIf.Name,
			MacAddress: netIf.MacAddress,
			Driver:     netIf.Driver,
			PCIAddress: netIf.PCIAddress,
		})
	}

	for _, gpu := range m.GPUs {
		static.GPUs = append(static.GPUs, NodeGPUStaticInfo{
			Index:  gpu.Index,
			Vendor: gpu.Vendor,
			Model:  gpu.Model,
			UUID:   gpu.UUID,
		})
	}

	return static
}
