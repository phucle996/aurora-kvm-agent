package libvirt

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"time"

	golibvirt "github.com/digitalocean/go-libvirt"

	"aurora-kvm-agent/internal/model"
)

type vmSample struct {
	cpuNs uint64
	at    time.Time
}

type VMMetricsReader struct {
	conn   *ConnManager
	logger *slog.Logger

	mu    sync.Mutex
	prev  map[string]vmSample
	cores float64
}

func NewVMMetricsReader(conn *ConnManager, logger *slog.Logger) *VMMetricsReader {
	return &VMMetricsReader{
		conn:   conn,
		logger: logger,
		prev:   map[string]vmSample{},
		cores:  float64(runtime.NumCPU()),
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

		diskRead, diskWrite := sumBySuffix(fields, golibvirt.DomainStatsBlockSuffixRdBytes, golibvirt.DomainStatsBlockSuffixWrBytes)
		netRx, netTx := sumBySuffix(fields, golibvirt.DomainStatsNetSuffixRxBytes, golibvirt.DomainStatsNetSuffixTxBytes)

		out = append(out, model.VMMetrics{
			NodeID:         nodeID,
			VMID:           vmID,
			VMName:         rec.Dom.Name,
			State:          state,
			Timestamp:      now,
			CPUUsagePct:    cpuPct,
			VCPUCount:      fields[golibvirt.DomainStatsVCPUCurrent],
			RAMUsedBytes:   ramUsed,
			RAMTotalBytes:  ramTotal,
			DiskReadBytes:  diskRead,
			DiskWriteBytes: diskWrite,
			NetRxBytes:     netRx,
			NetTxBytes:     netTx,
		})
	}
	return out, nil
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
