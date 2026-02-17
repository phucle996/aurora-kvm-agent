package node

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"aurora-kvm-agent/internal/model"
)

func (r *NodeMetricsReader) collectNodeGPUMetrics(ctx context.Context, now time.Time, nodeID string) ([]model.NodeGpuMetricLite, gpuAggregateLite) {
	if gpus := r.collectNvidiaGpuLite(ctx, now, nodeID); len(gpus) > 0 {
		return summarizeGpuLite(gpus)
	}
	if gpus := collectAmdGpuLite(ctx, now, nodeID); len(gpus) > 0 {
		return summarizeGpuLite(gpus)
	}
	return []model.NodeGpuMetricLite{}, gpuAggregateLite{}
}

func summarizeGpuLite(gpus []model.NodeGpuMetricLite) ([]model.NodeGpuMetricLite, gpuAggregateLite) {
	sort.Slice(gpus, func(i, j int) bool { return gpus[i].GPUIndex < gpus[j].GPUIndex })
	agg := gpuAggregateLite{Count: uint64(len(gpus))}
	for _, g := range gpus {
		agg.TotalUtil += g.GPUUtilPercent
		agg.MemoryUsedBytes += g.MemoryUsedBytes
		agg.MemoryTotalByte += g.MemoryTotalBytes
	}
	if agg.Count > 0 {
		agg.TotalUtil = agg.TotalUtil / float64(agg.Count)
	}
	return gpus, agg
}

func (r *NodeMetricsReader) collectNvidiaGpuLite(ctx context.Context, now time.Time, nodeID string) []model.NodeGpuMetricLite {
	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		return nil
	}

	fields := []string{
		"index",
		"uuid",
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
		"pcie.rx_util",
		"pcie.tx_util",
	}
	rows, err := runNvidiaQueryCSV(ctx, fields)
	if err != nil || len(rows) == 0 {
		return nil
	}

	energyByUUID := r.queryNvidiaEnergyCounter(ctx, now)

	out := make([]model.NodeGpuMetricLite, 0, len(rows))
	for _, row := range rows {
		if len(row) < len(fields) {
			continue
		}
		uuid := normalizeField(row[1])
		m := model.NodeGpuMetricLite{
			NodeID:                   nodeID,
			GPUIndex:                 parseUintFlexible(row[0]),
			GPUUUID:                  uuid,
			Vendor:                   model.GpuVendorNvidia,
			TimestampUnix:            now.Unix(),
			GPUUtilPercent:           parseFloatFlexible(row[2]),
			MemoryUtilPercent:        parseFloatFlexible(row[3]),
			MemoryTotalBytes:         mibToBytes(parseUintFlexible(row[4])),
			MemoryUsedBytes:          mibToBytes(parseUintFlexible(row[5])),
			MemoryFreeBytes:          mibToBytes(parseUintFlexible(row[6])),
			PowerUsageWatts:          parseFloatFlexible(row[7]),
			PowerLimitWatts:          parseFloatFlexible(row[8]),
			TemperatureCelsius:       parseFloatFlexible(row[9]),
			FanSpeedPercent:          parseFloatFlexible(row[10]),
			ClockCoreMhz:             parseUintFlexible(row[11]),
			ClockMemMhz:              parseUintFlexible(row[12]),
			PCIeRxMBS:                kibToMib(parseFloatFlexible(row[13])),
			PCIeTxMBS:                kibToMib(parseFloatFlexible(row[14])),
			ComputeUtilPercent:       parseFloatFlexible(row[2]),
			EncoderUtilPercent:       0,
			DecoderUtilPercent:       0,
			PCIeBandwidthUtilPercent: 0,
		}
		if m.PowerUsageWatts == 0 {
			if watts, ok := energyByUUID[uuid]; ok {
				m.PowerUsageWatts = watts
			}
		}
		out = append(out, m)
	}

	applyNvidiaCodecUtil(ctx, out)
	return out
}

// Some GPU drivers expose cumulative energy counter; delta engine converts it to watt.
func (r *NodeMetricsReader) queryNvidiaEnergyCounter(ctx context.Context, now time.Time) map[string]float64 {
	rows, err := runNvidiaQueryCSV(ctx, []string{"uuid", "total_energy_consumption"})
	if err != nil {
		return map[string]float64{}
	}
	out := make(map[string]float64, len(rows))
	for _, row := range rows {
		if len(row) < 2 {
			continue
		}
		uuid := normalizeField(row[0])
		if uuid == "" {
			continue
		}
		energyMilliJoule := parseUintFlexible(row[1])
		if delta, sec, ok := r.delta.ObserveCounter("gpu:"+uuid+":energy_mj", now, energyMilliJoule); ok && sec > 0 {
			out[uuid] = (float64(delta) / 1000) / sec
		}
	}
	return out
}

func applyNvidiaCodecUtil(ctx context.Context, out []model.NodeGpuMetricLite) {
	if len(out) == 0 {
		return
	}
	byUUID := make(map[string]*model.NodeGpuMetricLite, len(out))
	for i := range out {
		byUUID[out[i].GPUUUID] = &out[i]
	}

	encRows, err := runNvidiaQueryCSV(ctx, []string{"uuid", "utilization.encoder"})
	if err == nil {
		for _, row := range encRows {
			if len(row) < 2 {
				continue
			}
			uuid := normalizeField(row[0])
			if g := byUUID[uuid]; g != nil {
				g.EncoderUtilPercent = parseFloatFlexible(row[1])
			}
		}
	}
	decRows, err := runNvidiaQueryCSV(ctx, []string{"uuid", "utilization.decoder"})
	if err == nil {
		for _, row := range decRows {
			if len(row) < 2 {
				continue
			}
			uuid := normalizeField(row[0])
			if g := byUUID[uuid]; g != nil {
				g.DecoderUtilPercent = parseFloatFlexible(row[1])
			}
		}
	}
}

func collectAmdGpuLite(ctx context.Context, now time.Time, nodeID string) []model.NodeGpuMetricLite {
	if _, err := exec.LookPath("rocm-smi"); err != nil {
		return nil
	}
	cmd := exec.CommandContext(ctx, "rocm-smi", "--json")
	raw, err := cmd.Output()
	if err != nil {
		return nil
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}

	out := make([]model.NodeGpuMetricLite, 0, 4)
	for key, value := range decoded {
		lk := strings.ToLower(strings.TrimSpace(key))
		if !strings.Contains(lk, "card") && !strings.Contains(lk, "gpu") {
			continue
		}
		obj, ok := value.(map[string]any)
		if !ok {
			continue
		}
		totalBytes := mibToBytes(uint64(findMapFloatByContains(obj, "vram total", "memory total")))
		usedBytes := mibToBytes(uint64(findMapFloatByContains(obj, "vram used", "memory used")))
		freeBytes := uint64(0)
		if totalBytes > usedBytes {
			freeBytes = totalBytes - usedBytes
		}

		m := model.NodeGpuMetricLite{
			NodeID:                   nodeID,
			GPUIndex:                 parseUintFlexible(extractDigits(lk)),
			GPUUUID:                  findMapStringByContains(obj, "unique id", "uuid"),
			Vendor:                   model.GpuVendorAmd,
			TimestampUnix:            now.Unix(),
			GPUUtilPercent:           findMapFloatByContains(obj, "gpu use", "gpu busy"),
			MemoryUtilPercent:        findMapFloatByContains(obj, "memory use", "memory busy"),
			MemoryTotalBytes:         totalBytes,
			MemoryUsedBytes:          usedBytes,
			MemoryFreeBytes:          freeBytes,
			PowerUsageWatts:          findMapFloatByContains(obj, "power", "average power"),
			PowerLimitWatts:          findMapFloatByContains(obj, "power cap", "max power"),
			TemperatureCelsius:       findMapFloatByContains(obj, "temperature"),
			ClockCoreMhz:             uint64(findMapFloatByContains(obj, "sclk", "gpu clock")),
			ClockMemMhz:              uint64(findMapFloatByContains(obj, "mclk", "memory clock")),
			ComputeUtilPercent:       findMapFloatByContains(obj, "gpu use", "gpu busy"),
			EncoderUtilPercent:       0,
			DecoderUtilPercent:       0,
			PCIeBandwidthUtilPercent: 0,
			Amd: &model.AmdExtra{
				GFXUtilPercent:              findMapFloatByContains(obj, "gpu busy"),
				MemoryControllerUtilPercent: findMapFloatByContains(obj, "memory busy"),
				TemperatureEdgeCelsius:      findMapFloatByContains(obj, "edge temperature"),
				TemperatureHotspotCelsius:   findMapFloatByContains(obj, "hotspot"),
				PowerAverageWatts:           findMapFloatByContains(obj, "average power"),
				ClockSocMhz:                 uint64(findMapFloatByContains(obj, "socclk")),
				ClockMemoryMhz:              uint64(findMapFloatByContains(obj, "mclk", "memory clock")),
			},
		}
		out = append(out, m)
	}
	return out
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

func kibToMib(v float64) float64 {
	return v / 1024
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
