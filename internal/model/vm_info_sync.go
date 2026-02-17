package model

import "sort"

const (
	SyncModeFull  = "full"
	SyncModeDelta = "delta"
)

type VMInfo struct {
	NodeID        string `json:"node_id"`
	VMID          string `json:"vm_id"`
	VMName        string `json:"vm_name"`
	State         string `json:"state"`
	VCPUCount     uint64 `json:"vcpu_count"`
	RAMTotalBytes uint64 `json:"ram_total_bytes"`
	DiskCount     uint64 `json:"disk_count"`
	GPUCount      uint64 `json:"gpu_count"`
}

type VMInfoSync struct {
	NodeID        string   `json:"node_id"`
	TimestampUnix int64    `json:"timestamp_unix"`
	SyncMode      string   `json:"sync_mode"`
	Reason        string   `json:"reason"`
	Upserts       []VMInfo `json:"upserts"`
	RemovedVMIDs  []string `json:"removed_vm_ids"`
}

func BuildVMInfoFromLiteMetric(m VMLiteMetrics) VMInfo {
	return VMInfo{
		NodeID:        m.NodeID,
		VMID:          m.VMID,
		VMName:        m.VMName,
		State:         m.State,
		VCPUCount:     m.VCPUCount,
		RAMTotalBytes: m.RAMTotalBytes,
		DiskCount:     0,
		GPUCount:      0,
	}
}

func BuildVMInfoMapFromLite(metrics []VMLiteMetrics) map[string]VMInfo {
	out := make(map[string]VMInfo, len(metrics))
	for _, metric := range metrics {
		if metric.VMID == "" {
			continue
		}
		out[metric.VMID] = BuildVMInfoFromLiteMetric(metric)
	}
	return out
}

func BuildVMInfoFromRuntimeMetric(m VMRuntimeMetrics) VMInfo {
	return VMInfo{
		NodeID:        m.NodeID,
		VMID:          m.VMID,
		VMName:        m.VMName,
		State:         m.VMState,
		VCPUCount:     m.VCPUCount,
		RAMTotalBytes: m.RAMTotalBytes,
		DiskCount:     uint64(len(m.Disks)),
		GPUCount:      m.GPUCount,
	}
}

func BuildVMInfoMapFromRuntime(metrics []VMRuntimeMetrics) map[string]VMInfo {
	out := make(map[string]VMInfo, len(metrics))
	for _, metric := range metrics {
		if metric.VMID == "" {
			continue
		}
		out[metric.VMID] = BuildVMInfoFromRuntimeMetric(metric)
	}
	return out
}

func SortedVMInfosByID(m map[string]VMInfo) []VMInfo {
	if len(m) == 0 {
		return []VMInfo{}
	}
	ids := make([]string, 0, len(m))
	for vmID := range m {
		ids = append(ids, vmID)
	}
	sort.Strings(ids)
	out := make([]VMInfo, 0, len(ids))
	for _, vmID := range ids {
		out = append(out, m[vmID])
	}
	return out
}
