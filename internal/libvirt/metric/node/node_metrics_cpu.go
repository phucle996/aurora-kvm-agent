package node

import (
	"aurora-kvm-agent/internal/system"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (r *NodeMetricsReader) computeCPURealtime(cur procCPUSnapshot, snapshotReady bool) cpuRealtimeMetrics {
	out := cpuRealtimeMetrics{PerCoreUsagePct: []float64{}}
	if !snapshotReady {
		return out
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.hasPrevSnapshot {
		r.prevSnapshot = cur
		r.hasPrevSnapshot = true
		out.PerCoreUsagePct = make([]float64, len(cur.PerCore))
		return out
	}

	prev := r.prevSnapshot
	r.prevSnapshot = cur
	r.hasPrevSnapshot = true

	totalDelta := deltaCounter(cur.Aggregate.Total, prev.Aggregate.Total)
	if totalDelta == 0 {
		out.PerCoreUsagePct = make([]float64, len(cur.PerCore))
		return out
	}

	out.UserPercent = percentDelta(cur.Aggregate.User, prev.Aggregate.User, totalDelta)
	out.SystemPercent = percentDelta(cur.Aggregate.System, prev.Aggregate.System, totalDelta)
	out.IdlePercent = percentDelta(cur.Aggregate.Idle, prev.Aggregate.Idle, totalDelta)
	out.IOWaitPercent = percentDelta(cur.Aggregate.IOWait, prev.Aggregate.IOWait, totalDelta)
	out.StealPercent = percentDelta(cur.Aggregate.Steal, prev.Aggregate.Steal, totalDelta)
	out.IRQPercent = percentDelta(cur.Aggregate.IRQ, prev.Aggregate.IRQ, totalDelta)
	out.SoftIRQPercent = percentDelta(cur.Aggregate.SoftIRQ, prev.Aggregate.SoftIRQ, totalDelta)
	out.NicePercent = percentDelta(cur.Aggregate.Nice, prev.Aggregate.Nice, totalDelta)

	out.TotalPercent = clampPercent(
		out.UserPercent +
			out.SystemPercent +
			out.IOWaitPercent +
			out.StealPercent +
			out.IRQPercent +
			out.SoftIRQPercent +
			out.NicePercent,
	)

	coreCount := len(cur.PerCore)
	if len(prev.PerCore) < coreCount {
		coreCount = len(prev.PerCore)
	}
	perCore := make([]float64, 0, coreCount)
	for i := 0; i < coreCount; i++ {
		curCore := cur.PerCore[i]
		prevCore := prev.PerCore[i]
		coreTotalDelta := deltaCounter(curCore.Total, prevCore.Total)
		if coreTotalDelta == 0 {
			perCore = append(perCore, 0)
			continue
		}
		coreUsage := percentDelta(curCore.User, prevCore.User, coreTotalDelta) +
			percentDelta(curCore.System, prevCore.System, coreTotalDelta) +
			percentDelta(curCore.IOWait, prevCore.IOWait, coreTotalDelta) +
			percentDelta(curCore.Steal, prevCore.Steal, coreTotalDelta) +
			percentDelta(curCore.IRQ, prevCore.IRQ, coreTotalDelta) +
			percentDelta(curCore.SoftIRQ, prevCore.SoftIRQ, coreTotalDelta) +
			percentDelta(curCore.Nice, prevCore.Nice, coreTotalDelta)
		perCore = append(perCore, clampPercent(coreUsage))
	}
	out.PerCoreUsagePct = perCore

	seconds := cur.CapturedAt.Sub(prev.CapturedAt).Seconds()
	if seconds > 0 {
		out.ContextSwitchesSec = float64(deltaCounter(cur.ContextSwitches, prev.ContextSwitches)) / seconds
		out.InterruptsSec = float64(deltaCounter(cur.Interrupts, prev.Interrupts)) / seconds
	}

	return out
}

func (r *NodeMetricsReader) loadCPUStaticInfo(defaultMHz uint64, defaultLogical uint64) cpuStaticInfo {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cpuInfoLoaded {
		return r.cpuStatic
	}

	info := cpuStaticInfo{}
	lscpu := readLscpuInfo()
	modelFromProc, vendorFromProc := readCPUInfoFromProc()

	info.ModelName = firstNonEmpty(lscpu["Model name"], modelFromProc)
	info.Vendor = firstNonEmpty(lscpu["Vendor ID"], vendorFromProc)
	info.Architecture = lscpu["Architecture"]
	info.VirtualizationSupport = normalizeVirtualizationSupport(
		firstNonEmpty(readVirtualizationSupportFromProc(), lscpu["Virtualization"]),
	)
	info.CacheL1 = joinCacheL1(lscpu)
	info.CacheL2 = firstNonEmpty(lscpu["L2 cache"], lscpu["L2 Cache"])
	info.CacheL3 = firstNonEmpty(lscpu["L3 cache"], lscpu["L3 Cache"])

	info.ThreadCountLogical = parseUintFlexible(lscpu["CPU(s)"])
	if info.ThreadCountLogical == 0 {
		info.ThreadCountLogical = defaultLogical
	}
	info.SocketCount = parseUintFlexible(lscpu["Socket(s)"])
	coresPerSocket := parseUintFlexible(lscpu["Core(s) per socket"])
	if info.SocketCount > 0 && coresPerSocket > 0 {
		info.CoreCountPhysical = info.SocketCount * coresPerSocket
	}
	if info.CoreCountPhysical == 0 {
		info.CoreCountPhysical = info.ThreadCountLogical
	}

	info.FreqMinMhz = parseMHzFlexible(firstNonEmpty(lscpu["CPU min MHz"], lscpu["CPU Min MHz"]))
	info.FreqMaxMhz = parseMHzFlexible(firstNonEmpty(lscpu["CPU max MHz"], lscpu["CPU Max MHz"]))
	if info.FreqMinMhz == 0 {
		info.FreqMinMhz = defaultMHz
	}
	if info.FreqMaxMhz == 0 {
		info.FreqMaxMhz = defaultMHz
	}
	if info.FreqMaxMhz < info.FreqMinMhz {
		info.FreqMaxMhz = info.FreqMinMhz
	}

	r.cpuStatic = info
	r.cpuInfoLoaded = true
	return r.cpuStatic
}

func readProcCPUSnapshot(at time.Time) (procCPUSnapshot, error) {
	raw, err := os.ReadFile("/proc/stat")
	if err != nil {
		return procCPUSnapshot{}, fmt.Errorf("read /proc/stat: %w", err)
	}

	type indexedCore struct {
		index int
		data  system.CPUCounters
	}
	cores := make([]indexedCore, 0, 16)

	var snapshot procCPUSnapshot
	snapshot.CapturedAt = at

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
		key := parts[0]
		switch {
		case key == "cpu":
			counters, parseErr := parseCPUCounters(parts[1:])
			if parseErr != nil {
				return procCPUSnapshot{}, parseErr
			}
			snapshot.Aggregate = counters
		case strings.HasPrefix(key, "cpu") && len(key) > 3:
			coreIndex, parseErr := strconv.Atoi(key[3:])
			if parseErr != nil {
				continue
			}
			counters, parseErr := parseCPUCounters(parts[1:])
			if parseErr != nil {
				continue
			}
			cores = append(cores, indexedCore{index: coreIndex, data: counters})
		case key == "ctxt":
			snapshot.ContextSwitches = parseUintFlexible(parts[1])
		case key == "intr":
			snapshot.Interrupts = parseUintFlexible(parts[1])
		case key == "procs_running":
			snapshot.ProcsRunning = parseUintFlexible(parts[1])
		}
	}

	sort.Slice(cores, func(i, j int) bool {
		return cores[i].index < cores[j].index
	})
	snapshot.PerCore = make([]system.CPUCounters, 0, len(cores))
	for _, core := range cores {
		snapshot.PerCore = append(snapshot.PerCore, core.data)
	}
	return snapshot, nil
}

func parseCPUCounters(fields []string) (system.CPUCounters, error) {
	if len(fields) < 8 {
		return system.CPUCounters{}, fmt.Errorf("unexpected cpu fields: %v", fields)
	}
	vals := make([]uint64, 0, len(fields))
	for _, field := range fields {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return system.CPUCounters{}, fmt.Errorf("parse cpu stat %q: %w", field, err)
		}
		vals = append(vals, value)
	}
	counters := system.CPUCounters{}
	if len(vals) > 0 {
		counters.User = vals[0]
	}
	if len(vals) > 1 {
		counters.Nice = vals[1]
	}
	if len(vals) > 2 {
		counters.System = vals[2]
	}
	if len(vals) > 3 {
		counters.Idle = vals[3]
	}
	if len(vals) > 4 {
		counters.IOWait = vals[4]
	}
	if len(vals) > 5 {
		counters.IRQ = vals[5]
	}
	if len(vals) > 6 {
		counters.SoftIRQ = vals[6]
	}
	if len(vals) > 7 {
		counters.Steal = vals[7]
	}
	for _, value := range vals {
		counters.Total += value
	}
	return counters, nil
}

func readLoadAvg() loadAverage {
	raw, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return loadAverage{}
	}
	fields := strings.Fields(strings.TrimSpace(string(raw)))
	if len(fields) < 4 {
		return loadAverage{}
	}
	load1, _ := strconv.ParseFloat(fields[0], 64)
	load5, _ := strconv.ParseFloat(fields[1], 64)
	load15, _ := strconv.ParseFloat(fields[2], 64)

	runQueue := uint64(0)
	queueParts := strings.SplitN(fields[3], "/", 2)
	if len(queueParts) == 2 {
		runQueue = parseUintFlexible(queueParts[0])
	}
	return loadAverage{
		Load1:         load1,
		Load5:         load5,
		Load15:        load15,
		RunQueueDepth: runQueue,
	}
}

func readCPUCurrentMHz(defaultMHz uint64) uint64 {
	readFirst := func(globPattern string) uint64 {
		paths, err := filepath.Glob(globPattern)
		if err != nil || len(paths) == 0 {
			return 0
		}
		var total uint64
		var count uint64
		for _, path := range paths {
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				continue
			}
			value := strings.TrimSpace(string(raw))
			if value == "" {
				continue
			}
			khz, parseErr := strconv.ParseUint(value, 10, 64)
			if parseErr != nil || khz == 0 {
				continue
			}
			total += khz / 1000
			count++
		}
		if count == 0 {
			return 0
		}
		return total / count
	}

	current := readFirst("/sys/devices/system/cpu/cpu[0-9]*/cpufreq/scaling_cur_freq")
	if current == 0 {
		current = readFirst("/sys/devices/system/cpu/cpu[0-9]*/cpufreq/cpuinfo_cur_freq")
	}
	if current != 0 {
		return current
	}

	raw, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return defaultMHz
	}
	lines := strings.Split(string(raw), "\n")
	var total float64
	var count float64
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		if key != "cpu mhz" {
			continue
		}
		freq, parseErr := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if parseErr != nil || freq <= 0 {
			continue
		}
		total += freq
		count++
	}
	if count > 0 {
		return uint64(total / count)
	}
	return defaultMHz
}

func readLscpuInfo() map[string]string {
	out, err := exec.Command("lscpu").Output()
	if err != nil {
		return map[string]string{}
	}
	result := make(map[string]string)
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" || value == "" {
			continue
		}
		result[key] = value
	}
	return result
}

func readCPUInfoFromProc() (string, string) {
	raw, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return "", ""
	}
	var model string
	var vendor string
	lines := strings.Split(string(raw), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])
		if value == "" {
			continue
		}
		switch key {
		case "model name", "hardware":
			if model == "" {
				model = value
			}
		case "vendor_id", "vendor":
			if vendor == "" {
				vendor = value
			}
		}
		if model != "" && vendor != "" {
			break
		}
	}
	return model, vendor
}

func readVirtualizationSupportFromProc() string {
	raw, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	lines := strings.Split(string(raw), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		if key != "flags" && key != "features" {
			continue
		}
		flags := strings.Fields(strings.ToLower(parts[1]))
		for _, flag := range flags {
			if flag == "vmx" || flag == "svm" {
				return flag
			}
		}
	}
	return ""
}

func joinCacheL1(info map[string]string) string {
	l1d := firstNonEmpty(info["L1d cache"], info["L1d Cache"])
	l1i := firstNonEmpty(info["L1i cache"], info["L1i Cache"])
	if l1d != "" && l1i != "" {
		if l1d == l1i {
			return l1d
		}
		return l1d + " / " + l1i
	}
	if l1d != "" {
		return l1d
	}
	if l1i != "" {
		return l1i
	}
	return firstNonEmpty(info["L1 cache"], info["L1 Cache"])
}

func normalizeVirtualizationSupport(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return ""
	}
	switch {
	case strings.Contains(v, "vmx"), strings.Contains(v, "vt-x"), strings.Contains(v, "intel"):
		return "vmx"
	case strings.Contains(v, "svm"), strings.Contains(v, "amd"), strings.Contains(v, "amd-v"):
		return "svm"
	default:
		return v
	}
}

func readCPUTemperatureCelsius() float64 {
	paths, err := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
	if err != nil || len(paths) == 0 {
		return 0
	}

	readTemp := func(path string) float64 {
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return 0
		}
		value := strings.TrimSpace(string(raw))
		if value == "" {
			return 0
		}
		temp, parseErr := strconv.ParseFloat(value, 64)
		if parseErr != nil || temp <= 0 {
			return 0
		}
		if temp > 1000 {
			temp = temp / 1000
		}
		if temp > 200 {
			return 0
		}
		return temp
	}

	var cpuTemps []float64
	var fallbackTemps []float64
	for _, tempPath := range paths {
		temp := readTemp(tempPath)
		if temp == 0 {
			continue
		}
		fallbackTemps = append(fallbackTemps, temp)
		typePath := strings.TrimSuffix(tempPath, "/temp") + "/type"
		typeRaw, readErr := os.ReadFile(typePath)
		if readErr != nil {
			continue
		}
		typeValue := strings.ToLower(strings.TrimSpace(string(typeRaw)))
		if strings.Contains(typeValue, "cpu") ||
			strings.Contains(typeValue, "x86_pkg_temp") ||
			strings.Contains(typeValue, "package") {
			cpuTemps = append(cpuTemps, temp)
		}
	}

	candidates := cpuTemps
	if len(candidates) == 0 {
		candidates = fallbackTemps
	}
	if len(candidates) == 0 {
		return 0
	}
	var sum float64
	for _, value := range candidates {
		sum += value
	}
	return sum / float64(len(candidates))
}
