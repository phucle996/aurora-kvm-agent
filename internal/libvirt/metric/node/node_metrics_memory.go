package node

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	golibvirt "github.com/digitalocean/go-libvirt"
)

func (r *NodeMetricsReader) computeMemoryRates(cur vmStatSnapshot) memoryRateMetrics {
	out := memoryRateMetrics{
		OOMKillCount: cur.OOMKillCount,
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.hasPrevVMStat {
		r.prevVMStatSnapshot = cur
		r.hasPrevVMStat = true
		return out
	}

	prev := r.prevVMStatSnapshot
	r.prevVMStatSnapshot = cur
	r.hasPrevVMStat = true

	seconds := cur.CollectedAt.Sub(prev.CollectedAt).Seconds()
	if seconds <= 0 {
		return out
	}

	pageSize := float64(os.Getpagesize())
	out.SwapInBytesPerSec = float64(deltaCounter(cur.SwapInPages, prev.SwapInPages)) * pageSize / seconds
	out.SwapOutBytesPerSec = float64(deltaCounter(cur.SwapOutPages, prev.SwapOutPages)) * pageSize / seconds
	out.PageFaultsPerSec = float64(deltaCounter(cur.PageFaults, prev.PageFaults)) / seconds
	out.MajorPageFaultsPerSec = float64(deltaCounter(cur.MajorPageFaults, prev.MajorPageFaults)) / seconds
	return out
}

func (r *NodeMetricsReader) memoryUsageFromLibvirt(client *golibvirt.Libvirt) (usedBytes, totalBytes uint64, err error) {
	stats, _, err := client.NodeGetMemoryStats(0, -1, 0)
	if err != nil {
		return 0, 0, err
	}
	if len(stats) == 0 {
		return 0, 0, fmt.Errorf("empty node memory stats")
	}
	vals := map[string]uint64{}
	for _, st := range stats {
		vals[strings.ToLower(st.Field)] = st.Value
	}
	total := vals["total"] * 1024
	free := vals["free"] * 1024
	buffers := vals["buffers"] * 1024
	cached := vals["cached"] * 1024
	if total == 0 {
		return 0, 0, fmt.Errorf("total memory is zero")
	}
	used := total
	if free+buffers+cached <= total {
		used = total - free - buffers - cached
	}
	return used, total, nil
}

func readMemoryInfoFromProc() (memoryInfoProc, error) {
	raw, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return memoryInfoProc{}, fmt.Errorf("read /proc/meminfo: %w", err)
	}

	values := make(map[string]uint64)
	lines := strings.Split(string(raw), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimSuffix(parts[0], ":")
		values[key] = parseMeminfoValue(parts[1:])
	}

	out := memoryInfoProc{
		TotalBytes:       values["MemTotal"],
		FreeBytes:        values["MemFree"],
		AvailableBytes:   values["MemAvailable"],
		CachedBytes:      values["Cached"],
		BufferBytes:      values["Buffers"],
		SharedBytes:      values["Shmem"],
		SlabBytes:        values["Slab"],
		ActiveBytes:      values["Active"],
		InactiveBytes:    values["Inactive"],
		DirtyBytes:       values["Dirty"],
		WritebackBytes:   values["Writeback"],
		SwapTotalBytes:   values["SwapTotal"],
		SwapFreeBytes:    values["SwapFree"],
		HugePagesTotal:   values["HugePages_Total"],
		HugePagesFree:    values["HugePages_Free"],
		HugePageSizeByte: values["Hugepagesize"],
	}

	if out.TotalBytes > 0 {
		if out.AvailableBytes > 0 && out.AvailableBytes <= out.TotalBytes {
			out.UsedBytes = out.TotalBytes - out.AvailableBytes
		} else if out.FreeBytes+out.BufferBytes+out.CachedBytes <= out.TotalBytes {
			out.UsedBytes = out.TotalBytes - out.FreeBytes - out.BufferBytes - out.CachedBytes
		}
	}
	if out.SwapTotalBytes >= out.SwapFreeBytes {
		out.SwapUsedBytes = out.SwapTotalBytes - out.SwapFreeBytes
	}
	if out.HugePagesTotal >= out.HugePagesFree {
		out.HugePagesUsed = out.HugePagesTotal - out.HugePagesFree
	}
	return out, nil
}

func parseMeminfoValue(tokens []string) uint64 {
	if len(tokens) == 0 {
		return 0
	}
	numberText := strings.TrimSpace(tokens[0])
	number, err := strconv.ParseFloat(numberText, 64)
	if err != nil || number < 0 {
		return 0
	}

	multiplier := float64(1)
	if len(tokens) >= 2 {
		unit := strings.ToLower(strings.TrimSpace(tokens[1]))
		switch unit {
		case "kb", "kib":
			multiplier = 1024
		case "mb", "mib":
			multiplier = 1024 * 1024
		case "gb", "gib":
			multiplier = 1024 * 1024 * 1024
		}
	}
	return uint64(number * multiplier)
}

func readVMStatSnapshot(at time.Time) vmStatSnapshot {
	out := vmStatSnapshot{CollectedAt: at}
	raw, err := os.ReadFile("/proc/vmstat")
	if err != nil {
		return out
	}
	lines := strings.Split(string(raw), "\n")
	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		key := fields[0]
		value := parseUintFlexible(fields[1])
		switch key {
		case "pswpin":
			out.SwapInPages = value
		case "pswpout":
			out.SwapOutPages = value
		case "pgfault":
			out.PageFaults = value
		case "pgmajfault":
			out.MajorPageFaults = value
		case "oom_kill":
			out.OOMKillCount = value
		}
	}
	return out
}

func readMemoryPressurePercent() float64 {
	raw, err := os.ReadFile("/proc/pressure/memory")
	if err != nil {
		return 0
	}
	lines := strings.Split(string(raw), "\n")
	maxValue := 0.0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		for _, field := range parts[1:] {
			if !strings.HasPrefix(field, "avg10=") {
				continue
			}
			valueText := strings.TrimPrefix(field, "avg10=")
			value, parseErr := strconv.ParseFloat(valueText, 64)
			if parseErr != nil || value < 0 {
				continue
			}
			if value > maxValue {
				maxValue = value
			}
		}
	}
	return maxValue
}

func (r *NodeMetricsReader) loadMemoryHardwareInfo(defaultTotalBytes uint64) memoryHardwareInfo {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.memInfoLoaded {
		return r.memHardware
	}

	info := memoryHardwareInfo{}
	raw, err := exec.Command("dmidecode", "-t", "memory").Output()
	if err == nil {
		info = parseDmidecodeMemoryInfo(string(raw))
	}

	if info.TotalPhysicalBytes == 0 {
		info.TotalPhysicalBytes = defaultTotalBytes
	}

	r.memHardware = info
	r.memInfoLoaded = true
	return r.memHardware
}

func parseDmidecodeMemoryInfo(raw string) memoryHardwareInfo {
	info := memoryHardwareInfo{}
	lines := strings.Split(raw, "\n")

	var inMemoryDevice bool
	current := memoryModuleInfo{}
	commit := func() {
		if !inMemoryDevice {
			return
		}
		if current.SizeBytes == 0 {
			current = memoryModuleInfo{}
			return
		}
		info.StickSlot = append(info.StickSlot, firstNonEmpty(current.Slot, fmt.Sprintf("slot-%d", len(info.StickSlot))))
		info.StickSizeBytes = append(info.StickSizeBytes, current.SizeBytes)
		info.StickSpeedMhz = append(info.StickSpeedMhz, current.SpeedMhz)
		info.StickType = append(info.StickType, firstNonEmpty(current.Type, "unknown"))
		info.StickModel = append(info.StickModel, firstNonEmpty(current.Model, current.PartNumber, "unknown"))
		info.StickManufacturer = append(info.StickManufacturer, firstNonEmpty(current.Manufacturer, "unknown"))
		info.StickPartNumber = append(info.StickPartNumber, firstNonEmpty(current.PartNumber, "unknown"))
		info.StickSerial = append(info.StickSerial, firstNonEmpty(current.Serial, "unknown"))
		info.TotalPhysicalBytes += current.SizeBytes
		current = memoryModuleInfo{}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "Physical Memory Array") {
			inMemoryDevice = false
			continue
		}
		if strings.HasPrefix(trimmed, "Memory Device") {
			commit()
			inMemoryDevice = true
			current = memoryModuleInfo{}
			continue
		}
		if strings.HasPrefix(trimmed, "Error Correction Type:") {
			value := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(trimmed, "Error Correction Type:")))
			if value != "" && value != "none" && value != "unknown" {
				info.ECCEnabled = true
			}
			continue
		}
		if !inMemoryDevice {
			continue
		}

		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		switch key {
		case "Size":
			current.SizeBytes = parseMemorySizeToBytes(value)
		case "Locator":
			current.Slot = value
		case "Bank Locator":
			if current.Slot == "" {
				current.Slot = value
			}
		case "Speed":
			current.SpeedMhz = parseSpeedToMHz(value)
		case "Configured Memory Speed":
			if current.SpeedMhz == 0 {
				current.SpeedMhz = parseSpeedToMHz(value)
			}
		case "Type":
			current.Type = value
		case "Part Number":
			current.PartNumber = value
			if current.Model == "" {
				current.Model = value
			}
		case "Manufacturer":
			current.Manufacturer = value
		case "Serial Number":
			current.Serial = value
		}
	}
	commit()

	info.StickCount = uint64(len(info.StickSizeBytes))
	return info
}

func parseMemorySizeToBytes(raw string) uint64 {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" || strings.Contains(value, "no module installed") || strings.Contains(value, "not installed") || strings.Contains(value, "unknown") {
		return 0
	}
	parts := strings.Fields(value)
	if len(parts) < 2 {
		return 0
	}
	number, err := strconv.ParseFloat(parts[0], 64)
	if err != nil || number <= 0 {
		return 0
	}
	switch parts[1] {
	case "kb", "kib":
		return uint64(number * 1024)
	case "mb", "mib":
		return uint64(number * 1024 * 1024)
	case "gb", "gib":
		return uint64(number * 1024 * 1024 * 1024)
	case "tb", "tib":
		return uint64(number * 1024 * 1024 * 1024 * 1024)
	default:
		return 0
	}
}

func parseSpeedToMHz(raw string) uint64 {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" || strings.Contains(value, "unknown") {
		return 0
	}
	parts := strings.Fields(value)
	if len(parts) == 0 {
		return 0
	}
	number, err := strconv.ParseFloat(parts[0], 64)
	if err != nil || number <= 0 {
		return 0
	}
	return uint64(number)
}
