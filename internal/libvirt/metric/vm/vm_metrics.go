package vm

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	golibvirt "github.com/digitalocean/go-libvirt"

	"aurora-kvm-agent/internal/libvirt/metric"
	"aurora-kvm-agent/internal/model"
)

type vmSample struct {
	cpuNs uint64
	at    time.Time
}

type vmLiteSample struct {
	netRXBytes uint64
	netTXBytes uint64
	at         time.Time
}

type vmRuntimeSample struct {
	diskReadBytes  uint64
	diskWriteBytes uint64
	diskReadReqs   uint64
	diskWriteReqs  uint64
	netRXBytes     uint64
	netTXBytes     uint64
	netRXPkts      uint64
	netTXPkts      uint64
	at             time.Time
}

type VMMetricsReader struct {
	conn   *metric.ConnManager
	logger *slog.Logger

	mu          sync.Mutex
	prev        map[string]vmSample
	prevLite    map[string]vmLiteSample
	prevRuntime map[string]vmRuntimeSample
	cores       float64
}

// Compile-time assertion for collectors that require runtime VM metrics.
var _ interface {
	CollectRuntime(context.Context, string) ([]model.VMRuntimeMetrics, error)
} = (*VMMetricsReader)(nil)

func NewVMMetricsReader(conn *metric.ConnManager, logger *slog.Logger) *VMMetricsReader {
	return &VMMetricsReader{
		conn:        conn,
		logger:      logger,
		prev:        map[string]vmSample{},
		prevLite:    map[string]vmLiteSample{},
		prevRuntime: map[string]vmRuntimeSample{},
		cores:       float64(runtime.NumCPU()),
	}
}

func (r *VMMetricsReader) Collect(ctx context.Context, nodeID string) ([]model.VMMetrics, error) {
	client, err := r.conn.Client(ctx)
	if err != nil {
		return nil, err
	}

	doms, _, err := client.ConnectListAllDomains(0, 0)
	if err != nil {
		return nil, fmt.Errorf("ConnectListAllDomains: %w", err)
	}
	if len(doms) == 0 {
		return []model.VMMetrics{}, nil
	}

	statsMask := uint32(golibvirt.DomainStatsCPUTotal | golibvirt.DomainStatsBalloon | golibvirt.DomainStatsInterface | golibvirt.DomainStatsBlock | golibvirt.DomainStatsState | golibvirt.DomainStatsVCPU)
	records, err := client.ConnectGetAllDomainStats(doms, statsMask, 0)
	if err != nil {
		return nil, fmt.Errorf("ConnectGetAllDomainStats: %w", err)
	}

	now := time.Now().UTC()
	out := make([]model.VMMetrics, 0, len(records))
	for _, rec := range records {
		vmID := uuidToString(rec.Dom.UUID)
		fields := map[string]uint64{}
		state := "unknown"
		for _, p := range rec.Params {
			if strings.EqualFold(p.Field, golibvirt.DomainStatsStateState) {
				state = domainStateString(asUint64(p.Value.I))
				continue
			}
			fields[p.Field] = asUint64(p.Value.I)
		}

		cpuNs := fields[golibvirt.DomainStatsCPUTime]
		cpuPct := r.computeCPU(vmID, cpuNs, now)
		ramUsed := fields[golibvirt.DomainStatsBalloonCurrent] * 1024
		ramTotal := fields[golibvirt.DomainStatsBalloonMaximum] * 1024
		if ramTotal == 0 {
			ramTotal = ramUsed
		}

		netRx, netTx := sumBySuffix(fields, golibvirt.DomainStatsNetSuffixRxBytes, golibvirt.DomainStatsNetSuffixTxBytes)
		netRxPS, netTxPS := r.computeLiteNetRates(vmID, netRx, netTx, now)

		out = append(out, model.VMMetrics{
			NodeID:           nodeID,
			VMID:             vmID,
			VMName:           rec.Dom.Name,
			State:            state,
			TimestampUnix:    now.Unix(),
			CPUUsagePct:      cpuPct,
			VCPUCount:        fields[golibvirt.DomainStatsVCPUCurrent],
			RAMUsedBytes:     ramUsed,
			RAMTotalBytes:    ramTotal,
			NetRxBytesPerSec: netRxPS,
			NetTxBytesPerSec: netTxPS,
		})
	}
	return out, nil
}

func (r *VMMetricsReader) CollectRuntime(ctx context.Context, nodeID string) ([]model.VMRuntimeMetrics, error) {
	client, err := r.conn.Client(ctx)
	if err != nil {
		return nil, err
	}

	doms, _, err := client.ConnectListAllDomains(0, 0)
	if err != nil {
		return nil, fmt.Errorf("ConnectListAllDomains: %w", err)
	}
	if len(doms) == 0 {
		return []model.VMRuntimeMetrics{}, nil
	}

	statsMask := uint32(golibvirt.DomainStatsCPUTotal | golibvirt.DomainStatsBalloon | golibvirt.DomainStatsInterface | golibvirt.DomainStatsBlock | golibvirt.DomainStatsState | golibvirt.DomainStatsVCPU)
	records, err := client.ConnectGetAllDomainStats(doms, statsMask, 0)
	if err != nil {
		return nil, fmt.Errorf("ConnectGetAllDomainStats: %w", err)
	}

	now := time.Now().UTC()
	load1, load5, load15 := readLoadAvg()
	out := make([]model.VMRuntimeMetrics, 0, len(records))
	for _, rec := range records {
		vmID := uuidToString(rec.Dom.UUID)
		uintFields := map[string]uint64{}
		strFields := map[string]string{}
		state := "unknown"
		for _, p := range rec.Params {
			if strings.EqualFold(p.Field, golibvirt.DomainStatsStateState) {
				state = domainStateString(asUint64(p.Value.I))
			}
			if v, ok := p.Value.I.(string); ok {
				strFields[p.Field] = v
				continue
			}
			uintFields[p.Field] = asUint64(p.Value.I)
		}

		cpuNs := uintFields[golibvirt.DomainStatsCPUTime]
		cpuPct := r.computeCPU(vmID, cpuNs, now)
		ramUsed := uintFields[golibvirt.DomainStatsBalloonCurrent] * 1024
		ramTotal := uintFields[golibvirt.DomainStatsBalloonMaximum] * 1024
		if ramTotal == 0 {
			ramTotal = ramUsed
		}
		ramUsagePct := percentOf(ramUsed, ramTotal)

		diskReadTotal, diskWriteTotal := sumBySuffix(uintFields, golibvirt.DomainStatsBlockSuffixRdBytes, golibvirt.DomainStatsBlockSuffixWrBytes)
		diskReadReqsTotal, diskWriteReqsTotal := sumBySuffix(uintFields, golibvirt.DomainStatsBlockSuffixRdReqs, golibvirt.DomainStatsBlockSuffixWrReqs)
		netRXTotal, netTXTotal := sumBySuffix(uintFields, golibvirt.DomainStatsNetSuffixRxBytes, golibvirt.DomainStatsNetSuffixTxBytes)
		netRXPktsTotal, netTXPktsTotal := sumBySuffix(uintFields, golibvirt.DomainStatsNetSuffixRxPkts, golibvirt.DomainStatsNetSuffixTxPkts)
		netRXErrTotal, netTXErrTotal := sumBySuffix(uintFields, golibvirt.DomainStatsNetSuffixRxErrs, golibvirt.DomainStatsNetSuffixTxErrs)

		dReadPS, dWritePS, dReadIOPS, dWriteIOPS, nRXPS, nTXPS, nRXPktPS, nTXPktPS := r.computeRuntimeRates(vmID, vmRuntimeSample{
			diskReadBytes:  diskReadTotal,
			diskWriteBytes: diskWriteTotal,
			diskReadReqs:   diskReadReqsTotal,
			diskWriteReqs:  diskWriteReqsTotal,
			netRXBytes:     netRXTotal,
			netTXBytes:     netTXTotal,
			netRXPkts:      netRXPktsTotal,
			netTXPkts:      netTXPktsTotal,
			at:             now,
		})

		perDisk := buildPerDiskMetrics(uintFields, strFields)
		out = append(out, model.VMRuntimeMetrics{
			NodeID:        nodeID,
			VMID:          vmID,
			VMName:        rec.Dom.Name,
			TimestampUnix: now.Unix(),

			CPUUsagePercent: cpuPct,
			CPUStealPercent: 0,
			VCPUCount:       uintFields[golibvirt.DomainStatsVCPUCurrent],
			Load1:           load1,
			Load5:           load5,
			Load15:          load15,

			RAMTotalBytes:    ramTotal,
			RAMUsedBytes:     ramUsed,
			RAMUsagePercent:  ramUsagePct,
			SwapUsedBytes:    0,
			SwapUsagePercent: 0,

			DiskReadBytesPerSec:  dReadPS,
			DiskWriteBytesPerSec: dWritePS,
			DiskReadIOPS:         dReadIOPS,
			DiskWriteIOPS:        dWriteIOPS,
			DiskUtilPercent:      0,
			DiskReadBytesTotal:   diskReadTotal,
			DiskWriteBytesTotal:  diskWriteTotal,
			Disks:                perDisk,

			NetRxBytesPerSec:   nRXPS,
			NetTxBytesPerSec:   nTXPS,
			NetRxPacketsPerSec: nRXPktPS,
			NetTxPacketsPerSec: nTXPktPS,
			NetRxErrors:        netRXErrTotal,
			NetTxErrors:        netTXErrTotal,
			NetRxBytesTotal:    netRXTotal,
			NetTxBytesTotal:    netTXTotal,

			GPUCount:             0,
			GPUUtilPercent:       0,
			GPUMemoryUsedBytes:   0,
			GPUMemoryTotalBytes:  0,
			GPUMemoryUtilPercent: 0,
			GPUProcessCount:      0,
			GPUs:                 []model.VMRuntimeGPUMetric{},

			VMState:       state,
			UptimeSeconds: uintFields["state.state_time"],
		})
	}

	return out, nil
}

func (r *VMMetricsReader) computeLiteNetRates(vmID string, netRXBytes, netTXBytes uint64, at time.Time) (float64, float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	prev, ok := r.prevLite[vmID]
	r.prevLite[vmID] = vmLiteSample{
		netRXBytes: netRXBytes,
		netTXBytes: netTXBytes,
		at:         at,
	}
	if !ok {
		return 0, 0
	}

	dt := at.Sub(prev.at).Seconds()
	if dt <= 0 {
		return 0, 0
	}
	rxPerSec := deltaPerSec(netRXBytes, prev.netRXBytes, dt)
	txPerSec := deltaPerSec(netTXBytes, prev.netTXBytes, dt)
	return rxPerSec, txPerSec
}

func (r *VMMetricsReader) computeCPU(vmID string, cpuNs uint64, at time.Time) float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	prev, ok := r.prev[vmID]
	r.prev[vmID] = vmSample{cpuNs: cpuNs, at: at}
	if !ok || cpuNs <= prev.cpuNs {
		return 0
	}
	dt := at.Sub(prev.at).Seconds()
	if dt <= 0 {
		return 0
	}
	cpuDeltaSeconds := float64(cpuNs-prev.cpuNs) / float64(time.Second)
	usage := (cpuDeltaSeconds / dt) * (100.0 / r.cores)
	if usage < 0 {
		return 0
	}
	if usage > 100 {
		return 100
	}
	return usage
}

func sumBySuffix(fields map[string]uint64, readSuffix, writeSuffix string) (uint64, uint64) {
	var read, write uint64
	for k, v := range fields {
		switch {
		case strings.HasSuffix(k, readSuffix):
			read += v
		case strings.HasSuffix(k, writeSuffix):
			write += v
		}
	}
	return read, write
}

func buildPerDiskMetrics(uintFields map[string]uint64, strFields map[string]string) []model.VMRuntimeDiskMetric {
	const (
		prefixA = "block."
		prefixB = "block.count."
	)
	nameByIndex := make(map[string]string)
	for key, value := range strFields {
		if strings.HasPrefix(key, prefixA) && strings.HasSuffix(key, golibvirt.DomainStatsBlockSuffixName) {
			idx := strings.TrimSuffix(strings.TrimPrefix(key, prefixA), golibvirt.DomainStatsBlockSuffixName)
			if idx != "" {
				nameByIndex[idx] = strings.TrimSpace(value)
			}
		}
		if strings.HasPrefix(key, prefixB) && strings.HasSuffix(key, golibvirt.DomainStatsBlockSuffixName) {
			idx := strings.TrimSuffix(strings.TrimPrefix(key, prefixB), golibvirt.DomainStatsBlockSuffixName)
			if idx != "" {
				nameByIndex[idx] = strings.TrimSpace(value)
			}
		}
	}
	if len(nameByIndex) == 0 {
		return []model.VMRuntimeDiskMetric{}
	}
	out := make([]model.VMRuntimeDiskMetric, 0, len(nameByIndex))
	for idx, name := range nameByIndex {
		if name == "" {
			name = "disk" + idx
		}
		readTotal := uintFields["block."+idx+golibvirt.DomainStatsBlockSuffixRdBytes] + uintFields["block.count."+idx+golibvirt.DomainStatsBlockSuffixRdBytes]
		writeTotal := uintFields["block."+idx+golibvirt.DomainStatsBlockSuffixWrBytes] + uintFields["block.count."+idx+golibvirt.DomainStatsBlockSuffixWrBytes]
		capacity := uintFields["block."+idx+golibvirt.DomainStatsBlockSuffixCapacity] + uintFields["block.count."+idx+golibvirt.DomainStatsBlockSuffixCapacity]
		allocation := uintFields["block."+idx+golibvirt.DomainStatsBlockSuffixAllocation] + uintFields["block.count."+idx+golibvirt.DomainStatsBlockSuffixAllocation]
		out = append(out, model.VMRuntimeDiskMetric{
			Name:             name,
			ReadBytesPerSec:  0,
			WriteBytesPerSec: 0,
			UtilPercent:      0,
			FSUsagePercent:   percentOf(allocation, capacity),
		})
		_ = readTotal
		_ = writeTotal
	}
	return out
}

func (r *VMMetricsReader) computeRuntimeRates(vmID string, current vmRuntimeSample) (float64, float64, float64, float64, float64, float64, float64, float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	prev, ok := r.prevRuntime[vmID]
	r.prevRuntime[vmID] = current
	if !ok {
		return 0, 0, 0, 0, 0, 0, 0, 0
	}
	dt := current.at.Sub(prev.at).Seconds()
	if dt <= 0 {
		return 0, 0, 0, 0, 0, 0, 0, 0
	}
	dReadPS := deltaPerSec(current.diskReadBytes, prev.diskReadBytes, dt)
	dWritePS := deltaPerSec(current.diskWriteBytes, prev.diskWriteBytes, dt)
	dReadIOPS := deltaPerSec(current.diskReadReqs, prev.diskReadReqs, dt)
	dWriteIOPS := deltaPerSec(current.diskWriteReqs, prev.diskWriteReqs, dt)
	nRXPS := deltaPerSec(current.netRXBytes, prev.netRXBytes, dt)
	nTXPS := deltaPerSec(current.netTXBytes, prev.netTXBytes, dt)
	nRXPktPS := deltaPerSec(current.netRXPkts, prev.netRXPkts, dt)
	nTXPktPS := deltaPerSec(current.netTXPkts, prev.netTXPkts, dt)
	return dReadPS, dWritePS, dReadIOPS, dWriteIOPS, nRXPS, nTXPS, nRXPktPS, nTXPktPS
}

func deltaPerSec(current, previous uint64, dt float64) float64 {
	if current <= previous || dt <= 0 {
		return 0
	}
	return float64(current-previous) / dt
}

func percentOf(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	p := (float64(used) / float64(total)) * 100
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

func readLoadAvg() (float64, float64, float64) {
	raw, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0
	}
	parts := strings.Fields(string(raw))
	if len(parts) < 3 {
		return 0, 0, 0
	}
	l1, _ := strconv.ParseFloat(parts[0], 64)
	l5, _ := strconv.ParseFloat(parts[1], 64)
	l15, _ := strconv.ParseFloat(parts[2], 64)
	return l1, l5, l15
}

func asUint64(v any) uint64 {
	switch t := v.(type) {
	case uint64:
		return t
	case uint32:
		return uint64(t)
	case uint16:
		return uint64(t)
	case uint8:
		return uint64(t)
	case int64:
		if t < 0 {
			return 0
		}
		return uint64(t)
	case int32:
		if t < 0 {
			return 0
		}
		return uint64(t)
	case int:
		if t < 0 {
			return 0
		}
		return uint64(t)
	case float64:
		if t < 0 {
			return 0
		}
		return uint64(t)
	case float32:
		if t < 0 {
			return 0
		}
		return uint64(t)
	default:
		return 0
	}
}

func uuidToString(u golibvirt.UUID) string {
	if len(u) != 16 {
		return ""
	}
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uint32(u[0])<<24|uint32(u[1])<<16|uint32(u[2])<<8|uint32(u[3]),
		uint16(u[4])<<8|uint16(u[5]),
		uint16(u[6])<<8|uint16(u[7]),
		uint16(u[8])<<8|uint16(u[9]),
		uint64(u[10])<<40|uint64(u[11])<<32|uint64(u[12])<<24|uint64(u[13])<<16|uint64(u[14])<<8|uint64(u[15]),
	)
}

func domainStateString(v uint64) string {
	switch v {
	case 0:
		return "nostate"
	case 1:
		return "running"
	case 2:
		return "blocked"
	case 3:
		return "paused"
	case 4:
		return "shutdown"
	case 5:
		return "shutoff"
	case 6:
		return "crashed"
	case 7:
		return "pmsuspended"
	default:
		return "unknown"
	}
}
