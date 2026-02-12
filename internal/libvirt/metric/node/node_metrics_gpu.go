package node

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"aurora-kvm-agent/internal/model"
)

func (r *NodeMetricsReader) collectGPUMetrics(ctx context.Context) model.NodeGPUMetrics {
	if devices := collectNvidiaGPUMetrics(ctx); len(devices) > 0 {
		return summarizeGPUDevices(devices)
	}
	if devices := collectAmdGPUMetrics(ctx); len(devices) > 0 {
		return summarizeGPUDevices(devices)
	}
	return model.NodeGPUMetrics{
		GPUs: []model.NodeGPUDeviceMetrics{},
	}
}

func summarizeGPUDevices(devices []model.NodeGPUDeviceMetrics) model.NodeGPUMetrics {
	if len(devices) == 0 {
		return model.NodeGPUMetrics{GPUs: []model.NodeGPUDeviceMetrics{}}
	}
	sort.Slice(devices, func(i, j int) bool {
		return devices[i].Index < devices[j].Index
	})

	var totalMem uint64
	var totalMemUsed uint64
	var totalUtil float64
	var totalPower float64
	modelName := ""
	for _, gpu := range devices {
		totalMem += gpu.MemoryTotalBytes
		totalMemUsed += gpu.MemoryUsedBytes
		totalUtil += gpu.GPUUtilPercent
		totalPower += gpu.PowerUsageWatts
		if modelName == "" {
			modelName = gpu.Model
		} else if modelName != gpu.Model && gpu.Model != "" {
			modelName = "mixed"
		}
	}
	count := uint64(len(devices))
	avgUtil := 0.0
	if count > 0 {
		avgUtil = totalUtil / float64(count)
	}

	return model.NodeGPUMetrics{
		// legacy
		GPUCount:            count,
		GPUModel:            modelName,
		GPUUsagePct:         avgUtil,
		GPUMemoryTotalBytes: totalMem,
		GPUMemoryUsedBytes:  totalMemUsed,

		// new
		GPUs:                    devices,
		GPUTotalCount:           count,
		GPUTotalMemoryBytes:     totalMem,
		GPUTotalMemoryUsedBytes: totalMemUsed,
		GPUTotalUtilPercent:     avgUtil,
		GPUTotalPowerWatts:      totalPower,
	}
}

func collectNvidiaGPUMetrics(ctx context.Context) []model.NodeGPUDeviceMetrics {
	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		return nil
	}

	baseFields := []string{
		"index",
		"uuid",
		"name",
		"driver_version",
		"vbios_version",
		"pci.bus_id",
		"serial",
		"utilization.gpu",
		"utilization.memory",
		"memory.total",
		"memory.used",
		"memory.free",
		"power.draw",
		"power.limit",
		"temperature.gpu",
		"fan.speed",
		"clocks.current.graphics",
		"clocks.current.memory",
		"pcie.link.gen.current",
		"pcie.link.width.current",
		"pcie.tx_util",
		"pcie.rx_util",
		"persistence_mode",
		"compute_mode",
	}
	rows, err := runNvidiaQueryCSV(ctx, baseFields)
	if err != nil || len(rows) == 0 {
		return nil
	}

	byUUID := make(map[string]*model.NodeGPUDeviceMetrics, len(rows))
	for _, row := range rows {
		if len(row) < len(baseFields) {
			continue
		}
		g := &model.NodeGPUDeviceMetrics{
			Index:              parseUintFlexible(row[0]),
			UUID:               normalizeField(row[1]),
			Name:               normalizeField(row[2]),
			Vendor:             "nvidia",
			Model:              normalizeField(row[2]),
			DriverVersion:      normalizeField(row[3]),
			VBIOSVersion:       normalizeField(row[4]),
			PCIAddress:         normalizeField(row[5]),
			BusID:              normalizeField(row[5]),
			Serial:             normalizeField(row[6]),
			Architecture:       "",
			ComputeCapability:  "",
			GPUUtilPercent:     parseFloatFlexible(row[7]),
			MemoryUtilPercent:  parseFloatFlexible(row[8]),
			MemoryTotalBytes:   mibToBytes(parseUintFlexible(row[9])),
			MemoryUsedBytes:    mibToBytes(parseUintFlexible(row[10])),
			MemoryFreeBytes:    mibToBytes(parseUintFlexible(row[11])),
			PowerUsageWatts:    parseFloatFlexible(row[12]),
			PowerLimitWatts:    parseFloatFlexible(row[13]),
			TemperatureCelsius: parseFloatFlexible(row[14]),
			FanSpeedPercent:    parseFloatFlexible(row[15]),
			ClockCoreMhz:       parseUintFlexible(row[16]),
			ClockMemMhz:        parseUintFlexible(row[17]),
			PCIELinkGen:        parseUintFlexible(row[18]),
			PCIELinkWidth:      parseUintFlexible(row[19]),
			PCIETxBytesPerSec:  kibToBytesFloat(parseFloatFlexible(row[20])),
			PCIERxBytesPerSec:  kibToBytesFloat(parseFloatFlexible(row[21])),
			PersistenceMode:    normalizeField(row[22]),
			ComputeMode:        normalizeField(row[23]),
			Processes:          []model.NodeGPUProcessMetrics{},
			MIGProfiles:        []string{},
		}
		if g.MemoryTotalBytes > 0 {
			g.MemoryReservedBytes = g.MemoryTotalBytes - minUint64(g.MemoryTotalBytes, g.MemoryUsedBytes+g.MemoryFreeBytes)
		}
		g.PCIEBandwidthUtilPercent = computePCIeBandwidthUtil(g.PCIELinkGen, g.PCIELinkWidth, g.PCIERxBytesPerSec, g.PCIETxBytesPerSec)
		g.UtilizationGPUAvg1m = g.GPUUtilPercent
		g.UtilizationGPUAvg5m = g.GPUUtilPercent
		g.UtilizationGPUAvg15m = g.GPUUtilPercent
		g.SMUtilPercent = g.GPUUtilPercent
		byUUID[g.UUID] = g
	}

	optionalFieldMap := map[string]func(*model.NodeGPUDeviceMetrics, string){
		"temperature.memory": func(g *model.NodeGPUDeviceMetrics, raw string) {
			g.TemperatureMemoryCelsius = parseFloatFlexible(raw)
		},
		"clocks.current.sm": func(g *model.NodeGPUDeviceMetrics, raw string) {
			g.ClockSMMhz = parseUintFlexible(raw)
		},
		"clocks.current.video": func(g *model.NodeGPUDeviceMetrics, raw string) {
			g.ClockVideoMhz = parseUintFlexible(raw)
		},
		"bar1.memory.used": func(g *model.NodeGPUDeviceMetrics, raw string) {
			g.BAR1MemoryBytes = mibToBytes(parseUintFlexible(raw))
		},
		"ecc.mode.current": func(g *model.NodeGPUDeviceMetrics, raw string) {
			v := strings.ToLower(normalizeField(raw))
			g.ECCEnabled = v == "enabled" || v == "on" || v == "true"
		},
		"mig.mode.current": func(g *model.NodeGPUDeviceMetrics, raw string) {
			v := strings.ToLower(normalizeField(raw))
			g.MIGEnabled = v == "enabled" || v == "on" || v == "true"
		},
		"power.max_limit": func(g *model.NodeGPUDeviceMetrics, raw string) {
			g.PowerDrawMaxWatts = parseFloatFlexible(raw)
		},
		"power.min_limit": func(g *model.NodeGPUDeviceMetrics, raw string) {
			g.PowerDrawMinWatts = parseFloatFlexible(raw)
		},
		"utilization.encoder": func(g *model.NodeGPUDeviceMetrics, raw string) {
			g.EncoderUtilPercent = parseFloatFlexible(raw)
		},
		"utilization.decoder": func(g *model.NodeGPUDeviceMetrics, raw string) {
			g.DecoderUtilPercent = parseFloatFlexible(raw)
		},
	}
	optionalFields := make([]string, 0, len(optionalFieldMap))
	for f := range optionalFieldMap {
		optionalFields = append(optionalFields, f)
	}
	sort.Strings(optionalFields)
	for _, field := range optionalFields {
		rowsOpt, err := runNvidiaQueryCSV(ctx, []string{"uuid", field})
		if err != nil {
			continue
		}
		for _, row := range rowsOpt {
			if len(row) < 2 {
				continue
			}
			uuid := normalizeField(row[0])
			g := byUUID[uuid]
			if g == nil {
				continue
			}
			optionalFieldMap[field](g, row[1])
		}
	}

	processRows, err := runNvidiaComputeProcessQuery(ctx)
	if err == nil {
		for _, p := range processRows {
			g := byUUID[p.GPUUUID]
			if g == nil {
				continue
			}
			g.Processes = append(g.Processes, p.Process)
			g.ProcessCount = uint64(len(g.Processes))
		}
	}

	out := make([]model.NodeGPUDeviceMetrics, 0, len(byUUID))
	for _, g := range byUUID {
		sort.Slice(g.Processes, func(i, j int) bool { return g.Processes[i].PID < g.Processes[j].PID })
		out = append(out, *g)
	}
	return out
}

func collectAmdGPUMetrics(ctx context.Context) []model.NodeGPUDeviceMetrics {
	if _, err := exec.LookPath("rocm-smi"); err != nil {
		return nil
	}
	cmd := exec.CommandContext(ctx, "rocm-smi", "--json")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var raw map[string]any
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil
	}

	devices := make([]model.NodeGPUDeviceMetrics, 0, 4)
	for key, value := range raw {
		lk := strings.ToLower(strings.TrimSpace(key))
		if !strings.Contains(lk, "card") && !strings.Contains(lk, "gpu") {
			continue
		}
		obj, ok := value.(map[string]any)
		if !ok {
			continue
		}
		g := model.NodeGPUDeviceMetrics{
			Index:                    parseUintFlexible(extractDigits(lk)),
			UUID:                     findMapStringByContains(obj, "unique id", "uuid"),
			Name:                     findMapStringByContains(obj, "card series", "name", "product name"),
			Vendor:                   "amd",
			Model:                    findMapStringByContains(obj, "card model", "product name", "name"),
			DriverVersion:            findMapStringByContains(obj, "driver"),
			VBIOSVersion:             findMapStringByContains(obj, "vbios"),
			PCIAddress:               findMapStringByContains(obj, "pci bus", "bus"),
			BusID:                    findMapStringByContains(obj, "pci bus", "bus"),
			Serial:                   findMapStringByContains(obj, "serial"),
			Architecture:             findMapStringByContains(obj, "gfx", "architecture"),
			GPUUtilPercent:           findMapFloatByContains(obj, "gpu use", "gpu busy"),
			GPUBusyPercent:           findMapFloatByContains(obj, "gpu busy"),
			MemoryUtilPercent:        findMapFloatByContains(obj, "memory use", "memory busy"),
			MemoryBusyPercent:        findMapFloatByContains(obj, "memory busy"),
			MemoryTotalBytes:         mibToBytes(uint64(findMapFloatByContains(obj, "vram total", "memory total"))),
			MemoryUsedBytes:          mibToBytes(uint64(findMapFloatByContains(obj, "vram used", "memory used"))),
			PowerUsageWatts:          findMapFloatByContains(obj, "power", "average power"),
			PowerAverageWatts:        findMapFloatByContains(obj, "average power"),
			PowerCapWatts:            findMapFloatByContains(obj, "power cap", "max power"),
			TemperatureCelsius:       findMapFloatByContains(obj, "temperature"),
			TemperatureEdgeC:         findMapFloatByContains(obj, "edge temperature"),
			TemperatureHotspotC:      findMapFloatByContains(obj, "hotspot"),
			TemperatureMemoryCelsius: findMapFloatByContains(obj, "memory temperature"),
			ClockCoreMhz:             uint64(findMapFloatByContains(obj, "sclk", "gpu clock")),
			ClockMemMhz:              uint64(findMapFloatByContains(obj, "mclk", "memory clock")),
			ClockSocMhz:              uint64(findMapFloatByContains(obj, "socclk")),
			ClockMemoryMhz:           uint64(findMapFloatByContains(obj, "mclk", "memory clock")),
			PCIeBandwidthMbps:        findMapFloatByContains(obj, "pcie bandwidth"),
			PCIeReplayCount:          uint64(findMapFloatByContains(obj, "pcie replay")),
			ECCEnabled:               strings.Contains(strings.ToLower(findMapStringByContains(obj, "ecc")), "enabled"),
			ECCErrorsCorrected:       uint64(findMapFloatByContains(obj, "ecc corrected")),
			ECCErrorsUncorrected:     uint64(findMapFloatByContains(obj, "ecc uncorrected")),
			RASErrorsCorrected:       uint64(findMapFloatByContains(obj, "ras corrected")),
			RASErrorsUncorrected:     uint64(findMapFloatByContains(obj, "ras uncorrected")),
			ComputePartition:         findMapStringByContains(obj, "compute partition"),
			MemoryPartition:          findMapStringByContains(obj, "memory partition"),
			GFXVersion:               findMapStringByContains(obj, "gfx version"),
			ASICName:                 findMapStringByContains(obj, "asic"),
			Processes:                []model.NodeGPUProcessMetrics{},
			MIGProfiles:              []string{},
		}
		if g.MemoryTotalBytes > g.MemoryUsedBytes {
			g.MemoryFreeBytes = g.MemoryTotalBytes - g.MemoryUsedBytes
		}
		g.UtilizationGPUAvg1m = g.GPUUtilPercent
		g.UtilizationGPUAvg5m = g.GPUUtilPercent
		g.UtilizationGPUAvg15m = g.GPUUtilPercent
		devices = append(devices, g)
	}
	return devices
}

func runNvidiaQueryCSV(ctx context.Context, fields []string) ([][]string, error) {
	args := []string{
		"--query-gpu=" + strings.Join(fields, ","),
		"--format=csv,noheader,nounits",
	}
	cmd := exec.CommandContext(ctx, "nvidia-smi", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	reader := csv.NewReader(strings.NewReader(string(out)))
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	return rows, nil
}

type nvidiaComputeProcRow struct {
	GPUUUID string
	Process model.NodeGPUProcessMetrics
}

func runNvidiaComputeProcessQuery(ctx context.Context) ([]nvidiaComputeProcRow, error) {
	cmd := exec.CommandContext(
		ctx,
		"nvidia-smi",
		"--query-compute-apps=gpu_uuid,pid,process_name,used_gpu_memory",
		"--format=csv,noheader,nounits",
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	reader := csv.NewReader(strings.NewReader(string(out)))
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	result := make([]nvidiaComputeProcRow, 0, len(rows))
	for _, row := range rows {
		if len(row) < 4 {
			continue
		}
		pid := parseUintFlexible(row[1])
		proc := model.NodeGPUProcessMetrics{
			PID:             pid,
			Name:            normalizeField(row[2]),
			MemoryUsedBytes: mibToBytes(parseUintFlexible(row[3])),
			Username:        lookupUsernameByPID(pid),
			ContainerID:     lookupContainerIDByPID(pid),
		}
		result = append(result, nvidiaComputeProcRow{
			GPUUUID: normalizeField(row[0]),
			Process: proc,
		})
	}
	return result, nil
}

func lookupUsernameByPID(pid uint64) string {
	if pid == 0 {
		return ""
	}
	info, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	if err != nil {
		return ""
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	u, err := user.LookupId(strconv.FormatUint(uint64(stat.Uid), 10))
	if err != nil {
		return ""
	}
	return u.Username
}

var containerIDRegex = regexp.MustCompile(`[a-f0-9]{64}|[a-f0-9]{32}`)

func lookupContainerIDByPID(pid uint64) string {
	if pid == 0 {
		return ""
	}
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", pid))
	if err != nil {
		return ""
	}
	lines := strings.Split(string(raw), "\n")
	for _, line := range lines {
		match := containerIDRegex.FindString(strings.ToLower(line))
		if match != "" {
			return match
		}
	}
	return ""
}

func normalizeField(raw string) string {
	v := strings.TrimSpace(raw)
	switch strings.ToLower(v) {
	case "", "n/a", "[not supported]", "not supported", "unknown", "-", "none":
		return ""
	default:
		return v
	}
}

func parseFloatFlexible(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	raw = strings.TrimSuffix(raw, "%")
	raw = strings.TrimSuffix(raw, "C")
	raw = strings.TrimSuffix(raw, "W")
	raw = strings.Fields(raw)[0]
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return v
}

func mibToBytes(v uint64) uint64 {
	return v * 1024 * 1024
}

func kibToBytesFloat(v float64) float64 {
	return v * 1024
}

func minUint64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

func computePCIeBandwidthUtil(gen, width uint64, rxBps, txBps float64) float64 {
	if gen == 0 || width == 0 {
		return 0
	}
	// rough payload throughput per lane
	gtps := map[uint64]float64{
		1: 2.0,
		2: 4.0,
		3: 7.877,
		4: 15.754,
		5: 31.508,
		6: 63.015,
	}
	laneGBits, ok := gtps[gen]
	if !ok {
		return 0
	}
	capacityBytesPerSec := (laneGBits * float64(width) * 1_000_000_000) / 8
	if capacityBytesPerSec <= 0 {
		return 0
	}
	return clampPercent(((rxBps + txBps) / capacityBytesPerSec) * 100)
}

func findMapStringByContains(m map[string]any, contains ...string) string {
	for k, v := range m {
		lk := strings.ToLower(k)
		for _, needle := range contains {
			if strings.Contains(lk, strings.ToLower(needle)) {
				switch typed := v.(type) {
				case string:
					return normalizeField(typed)
				default:
					return normalizeField(fmt.Sprintf("%v", typed))
				}
			}
		}
	}
	return ""
}

func findMapFloatByContains(m map[string]any, contains ...string) float64 {
	for k, v := range m {
		lk := strings.ToLower(k)
		for _, needle := range contains {
			if strings.Contains(lk, strings.ToLower(needle)) {
				switch typed := v.(type) {
				case float64:
					return typed
				case string:
					return parseFloatFlexible(typed)
				default:
					return parseFloatFlexible(fmt.Sprintf("%v", typed))
				}
			}
		}
	}
	return 0
}

func extractDigits(raw string) string {
	var out strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			out.WriteRune(r)
		}
	}
	return out.String()
}
