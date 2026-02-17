package node

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"aurora-kvm-agent/internal/model"
)

// CollectHardware builds static node hardware information for periodic sync.
func (r *NodeMetricsReader) CollectHardware(ctx context.Context, nodeID, hostname string) (model.NodeHardwareInfo, error) {
	client, err := r.conn.Client(ctx)
	if err != nil {
		return model.NodeHardwareInfo{}, err
	}

	_, memoryKiB, cpus, mhz, _, _, _, _, err := client.NodeGetInfo()
	if err != nil {
		return model.NodeHardwareInfo{}, err
	}

	now := time.Now().UTC()
	cpuStatic := r.loadCPUStaticInfo(uint64(mhz), uint64(cpus))
	memStatic := r.loadMemoryHardwareInfo(memoryKiB * 1024)

	if strings.TrimSpace(hostname) == "" {
		hostname = strings.TrimSpace(os.Getenv("HOSTNAME"))
	}

	return model.NodeHardwareInfo{
		NodeID:   nodeID,
		Hostname: hostname,
		CPU: model.NodeCpuInfo{
			Model:                 strings.TrimSpace(cpuStatic.ModelName),
			Vendor:                strings.TrimSpace(cpuStatic.Vendor),
			Cores:                 maxU64(cpuStatic.CoreCountPhysical, uint64(cpus)),
			Threads:               maxU64(cpuStatic.ThreadCountLogical, uint64(cpus)),
			Sockets:               maxU64(cpuStatic.SocketCount, 1),
			BaseMhz:               maxU64(cpuStatic.FreqMinMhz, uint64(mhz)),
			MaxMhz:                maxU64(cpuStatic.FreqMaxMhz, uint64(mhz)),
			Architecture:          firstNonEmpty(strings.TrimSpace(cpuStatic.Architecture), runtime.GOARCH),
			VirtualizationSupport: virtualizationSupported(cpuStatic.VirtualizationSupport),
		},
		RAMSticks:       buildNodeRAMSticks(memStatic),
		Disks:           collectNodeDiskStaticInfos(),
		NetIfs:          collectNodeNetIfStaticInfos(),
		GPUs:            r.collectNodeGPUStaticInfos(ctx, now, nodeID),
		CollectedAtUnix: now.Unix(),
	}, nil
}

func buildNodeRAMSticks(mem memoryHardwareInfo) []model.NodeRamStick {
	if len(mem.StickSlot) == 0 {
		return []model.NodeRamStick{}
	}

	out := make([]model.NodeRamStick, 0, len(mem.StickSlot))
	for i, slot := range mem.StickSlot {
		out = append(out, model.NodeRamStick{
			Slot:         strings.TrimSpace(slot),
			SizeBytes:    valueAtU64(mem.StickSizeBytes, i),
			SpeedMhz:     valueAtU64(mem.StickSpeedMhz, i),
			Type:         strings.TrimSpace(valueAtString(mem.StickType, i)),
			Model:        strings.TrimSpace(valueAtString(mem.StickModel, i)),
			Manufacturer: strings.TrimSpace(valueAtString(mem.StickManufacturer, i)),
			Serial:       strings.TrimSpace(valueAtString(mem.StickSerial, i)),
		})
	}
	return out
}

func collectNodeDiskStaticInfos() []model.NodeDiskInfo {
	nameSet := map[string]struct{}{}

	if snapshots, err := readDiskStatSnapshots(); err == nil {
		for name := range snapshots {
			nameSet[name] = struct{}{}
		}
	}

	for name := range readDiskMountUsages() {
		nameSet[name] = struct{}{}
	}

	if entries, err := os.ReadDir("/sys/class/block"); err == nil {
		for _, entry := range entries {
			name := strings.TrimSpace(entry.Name())
			if isWholeBlockDevice(name) {
				nameSet[name] = struct{}{}
			}
		}
	}

	if len(nameSet) == 0 {
		return []model.NodeDiskInfo{}
	}

	mountUsages := readDiskMountUsages()
	names := make([]string, 0, len(nameSet))
	for name := range nameSet {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]model.NodeDiskInfo, 0, len(names))
	for _, name := range names {
		sysInfo := readDiskSysInfo(name)
		usage := mountUsages[name]

		mountpoint := ""
		filesystem := ""
		if usage != nil {
			if len(usage.Mountpoints) > 0 {
				mountpoint = strings.Join(usage.Mountpoints, ",")
			}
			if len(usage.Filesystems) > 0 {
				filesystem = strings.Join(usage.Filesystems, ",")
			}
		}

		out = append(out, model.NodeDiskInfo{
			Name:       name,
			Model:      strings.TrimSpace(sysInfo.Model),
			Serial:     strings.TrimSpace(sysInfo.Serial),
			Type:       strings.TrimSpace(sysInfo.Type),
			SizeBytes:  sysInfo.SizeBytes,
			Mountpoint: mountpoint,
			Filesystem: filesystem,
		})
	}

	return out
}

func collectNodeNetIfStaticInfos() []model.NodeNetIfInfo {
	samples := readNetworkInterfaceSamples()
	if len(samples) == 0 {
		return []model.NodeNetIfInfo{}
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i].Name < samples[j].Name })

	out := make([]model.NodeNetIfInfo, 0, len(samples))
	for _, sample := range samples {
		basePath := filepath.Join("/sys/class/net", sample.Name)
		out = append(out, model.NodeNetIfInfo{
			Name:       sample.Name,
			MacAddress: strings.TrimSpace(readTextFile(filepath.Join(basePath, "address"))),
			MTU:        readUintFile(filepath.Join(basePath, "mtu")),
			SpeedMbps:  sample.SpeedMbps,
			Driver:     readNetIfDriver(basePath),
			PCIAddress: readNetIfPCIAddress(basePath),
			Type:       inferNetIfType(basePath),
		})
	}
	return out
}

func (r *NodeMetricsReader) collectNodeGPUStaticInfos(ctx context.Context, now time.Time, nodeID string) []model.NodeGpuInfo {
	if nvidia := collectNvidiaGPUStaticInfos(ctx); len(nvidia) > 0 {
		return nvidia
	}

	if amd := collectAmdGPUStaticInfos(ctx, now, nodeID); len(amd) > 0 {
		return amd
	}

	if runtimeGPUs, _ := r.collectNodeGPUMetrics(ctx, now, nodeID); len(runtimeGPUs) > 0 {
		out := make([]model.NodeGpuInfo, 0, len(runtimeGPUs))
		for _, gpu := range runtimeGPUs {
			out = append(out, model.NodeGpuInfo{
				Index:            gpu.GPUIndex,
				UUID:             strings.TrimSpace(gpu.GPUUUID),
				Vendor:           gpuVendorName(gpu.Vendor),
				Model:            "",
				MemoryTotalBytes: gpu.MemoryTotalBytes,
			})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
		return out
	}

	return collectGPUStaticInfosFromLspci(ctx)
}

func collectNvidiaGPUStaticInfos(ctx context.Context) []model.NodeGpuInfo {
	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		return []model.NodeGpuInfo{}
	}

	rows, err := runNvidiaQueryCSV(ctx, []string{"index", "uuid", "name", "memory.total", "pci.bus_id", "driver_version"})
	if err != nil || len(rows) == 0 {
		return []model.NodeGpuInfo{}
	}

	out := make([]model.NodeGpuInfo, 0, len(rows))
	for _, row := range rows {
		if len(row) < 6 {
			continue
		}
		out = append(out, model.NodeGpuInfo{
			Index:            parseUintFlexible(row[0]),
			UUID:             normalizeField(row[1]),
			Vendor:           "nvidia",
			Model:            normalizeField(row[2]),
			MemoryTotalBytes: mibToBytes(parseUintFlexible(row[3])),
			PCIAddress:       normalizeField(row[4]),
			DriverVersion:    normalizeField(row[5]),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out
}

func collectAmdGPUStaticInfos(ctx context.Context, now time.Time, nodeID string) []model.NodeGpuInfo {
	gpus := collectAmdGpuLite(ctx, now, nodeID)
	if len(gpus) == 0 {
		return []model.NodeGpuInfo{}
	}
	out := make([]model.NodeGpuInfo, 0, len(gpus))
	for _, gpu := range gpus {
		out = append(out, model.NodeGpuInfo{
			Index:            gpu.GPUIndex,
			UUID:             strings.TrimSpace(gpu.GPUUUID),
			Vendor:           "amd",
			Model:            "",
			MemoryTotalBytes: gpu.MemoryTotalBytes,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out
}

func collectGPUStaticInfosFromLspci(ctx context.Context) []model.NodeGpuInfo {
	if _, err := exec.LookPath("lspci"); err != nil {
		return []model.NodeGpuInfo{}
	}

	cmd := exec.CommandContext(ctx, "lspci", "-Dnn")
	raw, err := cmd.Output()
	if err != nil {
		return []model.NodeGpuInfo{}
	}

	lines := strings.Split(string(raw), "\n")
	out := make([]model.NodeGpuInfo, 0, 2)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if !strings.Contains(lower, "vga compatible controller") &&
			!strings.Contains(lower, "3d controller") &&
			!strings.Contains(lower, "display controller") {
			continue
		}

		parts := strings.SplitN(line, " ", 2)
		pciAddress := ""
		desc := line
		if len(parts) == 2 {
			pciAddress = strings.TrimSpace(parts[0])
			desc = strings.TrimSpace(parts[1])
		}
		if idx := strings.Index(desc, ": "); idx >= 0 {
			desc = strings.TrimSpace(desc[idx+2:])
		}

		vendor := "unknown"
		descLower := strings.ToLower(desc)
		switch {
		case strings.Contains(descLower, "nvidia"):
			vendor = "nvidia"
		case strings.Contains(descLower, "amd"), strings.Contains(descLower, "advanced micro devices"), strings.Contains(descLower, "ati"):
			vendor = "amd"
		case strings.Contains(descLower, "intel"):
			vendor = "intel"
		}

		out = append(out, model.NodeGpuInfo{
			Index:      uint64(len(out)),
			Vendor:     vendor,
			Model:      desc,
			PCIAddress: pciAddress,
		})
	}

	return out
}

func readNetIfDriver(basePath string) string {
	driverLink := filepath.Join(basePath, "device", "driver")
	target, err := os.Readlink(driverLink)
	if err != nil {
		return ""
	}
	if !filepath.IsAbs(target) {
		target = filepath.Clean(filepath.Join(filepath.Dir(driverLink), target))
	}
	return strings.TrimSpace(filepath.Base(target))
}

func readNetIfPCIAddress(basePath string) string {
	deviceLink := filepath.Join(basePath, "device")
	target, err := os.Readlink(deviceLink)
	if err != nil {
		return ""
	}
	if !filepath.IsAbs(target) {
		target = filepath.Clean(filepath.Join(filepath.Dir(deviceLink), target))
	}
	candidate := strings.TrimSpace(filepath.Base(target))
	if strings.Count(candidate, ":") >= 2 {
		return candidate
	}
	return ""
}

func inferNetIfType(basePath string) string {
	if _, err := os.Stat(filepath.Join(basePath, "wireless")); err == nil {
		return "wireless"
	}
	switch readTextFile(filepath.Join(basePath, "type")) {
	case "1":
		return "ethernet"
	case "772":
		return "loopback"
	default:
		return "unknown"
	}
}

func virtualizationSupported(raw string) bool {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	return normalized == "supported" || normalized == "yes" || normalized == "true" || normalized == "on"
}

func maxU64(a, b uint64) uint64 {
	if a >= b {
		return a
	}
	return b
}

func valueAtString(values []string, idx int) string {
	if idx < 0 || idx >= len(values) {
		return ""
	}
	return values[idx]
}

func valueAtU64(values []uint64, idx int) uint64 {
	if idx < 0 || idx >= len(values) {
		return 0
	}
	return values[idx]
}

func gpuVendorName(v model.GpuVendor) string {
	switch v {
	case model.GpuVendorNvidia:
		return "nvidia"
	case model.GpuVendorAmd:
		return "amd"
	case model.GpuVendorIntel:
		return "intel"
	default:
		return "unknown"
	}
}
