package libvirt

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	golibvirt "github.com/digitalocean/go-libvirt"

	"aurora-kvm-agent/internal/model"
	"aurora-kvm-agent/internal/system"
)

type NodeMetricsReader struct {
	conn   *ConnManager
	logger *slog.Logger

	mu           sync.Mutex
	prevCPUUsed  uint64
	prevCPUTotal uint64
	prevAt       time.Time
}

func NewNodeMetricsReader(conn *ConnManager, logger *slog.Logger) *NodeMetricsReader {
	return &NodeMetricsReader{conn: conn, logger: logger}
}

func (r *NodeMetricsReader) Collect(ctx context.Context, nodeID, hostname string) (model.NodeMetrics, error) {
	client, err := r.conn.Client(ctx)
	if err != nil {
		return model.NodeMetrics{}, err
	}

	_, memoryKiB, cpus, mhz, _, _, _, _, err := client.NodeGetInfo()
	if err != nil {
		return model.NodeMetrics{}, fmt.Errorf("NodeGetInfo: %w", err)
	}

	cpuUsage, err := r.cpuUsageFromLibvirt(client)
	if err != nil {
		r.logger.Warn("libvirt cpu usage fallback to /proc", "error", err)
		cpuUsage = r.cpuUsageFromProc()
	}

	ramTotalBytes := memoryKiB * 1024
	ramUsedBytes := uint64(0)
	if used, total, memErr := r.memoryUsageFromLibvirt(client); memErr == nil {
		ramUsedBytes = used
		ramTotalBytes = total
	} else {
		r.logger.Warn("libvirt memory stats fallback to /proc", "error", memErr)
		if mem, mErr := system.ReadMemoryInfo(); mErr == nil {
			ramUsedBytes = mem.UsedBytes
			ramTotalBytes = mem.TotalBytes
		}
	}

	diskCounters, _ := system.ReadDiskCounters()
	netCounters, _ := system.ReadNetCounters()
	load1, load5, load15 := readLoadAvg()

	if hostname == "" {
		hostname = os.Getenv("HOSTNAME")
	}

	return model.NodeMetrics{
		NodeID:         nodeID,
		Hostname:       hostname,
		Timestamp:      time.Now().UTC(),
		CPUUsagePct:    cpuUsage,
		CPUCores:       uint64(cpus),
		CPUMhz:         uint64(mhz),
		Load1:          load1,
		Load5:          load5,
		Load15:         load15,
		RAMUsedBytes:   ramUsedBytes,
		RAMTotalBytes:  ramTotalBytes,
		DiskReadBytes:  diskCounters.ReadBytes,
		DiskWriteBytes: diskCounters.WriteBytes,
		NetRxBytes:     netCounters.RxBytes,
		NetTxBytes:     netCounters.TxBytes,
	}, nil
}

func (r *NodeMetricsReader) cpuUsageFromLibvirt(client *golibvirt.Libvirt) (float64, error) {
	stats, _, err := client.NodeGetCPUStats(-1, 0, 0)
	if err != nil {
		return 0, err
	}
	if len(stats) == 0 {
		return 0, fmt.Errorf("empty node cpu stats")
	}

	var total uint64
	var idle uint64
	var iowait uint64
	for _, st := range stats {
		total += st.Value
		switch strings.ToLower(st.Field) {
		case "idle":
			idle = st.Value
		case "iowait":
			iowait = st.Value
		}
	}
	used := total - idle - iowait

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.prevAt.IsZero() {
		r.prevCPUUsed = used
		r.prevCPUTotal = total
		r.prevAt = time.Now()
		return 0, nil
	}

	usedDelta := used - r.prevCPUUsed
	totalDelta := total - r.prevCPUTotal
	r.prevCPUUsed = used
	r.prevCPUTotal = total
	r.prevAt = time.Now()
	if totalDelta == 0 {
		return 0, nil
	}
	usage := (float64(usedDelta) / float64(totalDelta)) * 100
	if usage < 0 {
		return 0, nil
	}
	if usage > 100 {
		return 100, nil
	}
	return usage, nil
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

func (r *NodeMetricsReader) cpuUsageFromProc() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, err := system.ReadCPUCounters()
	if err != nil {
		return 0
	}
	if r.prevAt.IsZero() {
		r.prevCPUTotal = cur.Total
		r.prevCPUUsed = cur.Total - cur.Idle - cur.IOWait
		r.prevAt = time.Now()
		return 0
	}
	prev := system.CPUCounters{Total: r.prevCPUTotal, Idle: 0, IOWait: 0}
	prev.Idle = cur.Total - r.prevCPUUsed
	usage := system.CPUUsage(prev, cur)
	r.prevCPUTotal = cur.Total
	r.prevCPUUsed = cur.Total - cur.Idle - cur.IOWait
	r.prevAt = time.Now()
	return usage
}

func readLoadAvg() (float64, float64, float64) {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0
	}
	parts := strings.Fields(strings.TrimSpace(string(b)))
	if len(parts) < 3 {
		return 0, 0, 0
	}
	load1, _ := strconv.ParseFloat(parts[0], 64)
	load5, _ := strconv.ParseFloat(parts[1], 64)
	load15, _ := strconv.ParseFloat(parts[2], 64)
	return load1, load5, load15
}
