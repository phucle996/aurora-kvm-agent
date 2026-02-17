package node

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"aurora-kvm-agent/internal/libvirt/metric"
	"aurora-kvm-agent/internal/model"
	"aurora-kvm-agent/internal/system"
)

func NewNodeMetricsReader(conn *metric.ConnManager, logger *slog.Logger) *NodeMetricsReader {
	return &NodeMetricsReader{
		conn:   conn,
		logger: logger,
		delta:  newDeltaEngine(),
	}
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

	now := time.Now().UTC()

	snapshot, snapshotErr := readProcCPUSnapshot(now)
	if snapshotErr != nil {
		r.logger.Warn("read /proc cpu snapshot failed", "error", snapshotErr)
	}
	cpuRealtime := r.computeCPURealtime(snapshot, snapshotErr == nil)

	memInfo, memInfoErr := readMemoryInfoFromProc()
	if memInfoErr != nil {
		r.logger.Warn("read /proc/meminfo failed", "error", memInfoErr)
	}
	vmStat := readVMStatSnapshot(now)
	_ = r.computeMemoryRates(vmStat)

	diskMetrics, diskAgg := r.collectNodeDiskMetrics(now, nodeID)
	_ = diskMetrics
	netMetrics, netAgg := r.collectNodeNetworkMetrics(now, nodeID)
	_ = netMetrics
	gpuMetrics, gpuAgg := r.collectNodeGPUMetrics(ctx, now, nodeID)
	_ = gpuMetrics

	ramTotalBytes := memInfo.TotalBytes
	ramUsedBytes := memInfo.UsedBytes
	if ramTotalBytes == 0 || ramUsedBytes == 0 {
		if used, total, memErr := r.memoryUsageFromLibvirt(client); memErr == nil {
			ramUsedBytes = used
			if ramTotalBytes == 0 {
				ramTotalBytes = total
			}
		} else {
			r.logger.Warn("libvirt memory stats fallback to /proc", "error", memErr)
			if mem, mErr := system.ReadMemoryInfo(); mErr == nil {
				ramUsedBytes = mem.UsedBytes
				if ramTotalBytes == 0 {
					ramTotalBytes = mem.TotalBytes
				}
			}
		}
	}
	if ramTotalBytes == 0 {
		ramTotalBytes = memoryKiB * 1024
	}
	if ramUsedBytes == 0 && memInfo.AvailableBytes > 0 && ramTotalBytes > memInfo.AvailableBytes {
		ramUsedBytes = ramTotalBytes - memInfo.AvailableBytes
	}
	if memInfo.TotalBytes == 0 {
		memInfo.TotalBytes = ramTotalBytes
	}
	if memInfo.UsedBytes == 0 {
		memInfo.UsedBytes = ramUsedBytes
	}

	load := readLoadAvg()
	if load.RunQueueDepth == 0 && snapshot.ProcsRunning > 0 {
		load.RunQueueDepth = snapshot.ProcsRunning
	}

	staticInfo := r.loadCPUStaticInfo(uint64(mhz), uint64(cpus))
	if staticInfo.ThreadCountLogical == 0 {
		staticInfo.ThreadCountLogical = uint64(cpus)
	}

	processCount, threadCount := readProcessThreadCounts()

	if hostname == "" {
		hostname = os.Getenv("HOSTNAME")
	}

	cpuCores := staticInfo.ThreadCountLogical
	if cpuCores == 0 {
		cpuCores = 1
	}
	systemLoadPercent := clampPercent((load.Load1 / float64(cpuCores)) * 100)

	return model.NodeMetrics{
		NodeID:               nodeID,
		Hostname:             hostname,
		TimestampUnix:        now.Unix(),
		CPUUsagePercent:      cpuRealtime.TotalPercent,
		CPULoad1:             load.Load1,
		CPULoad5:             load.Load5,
		CPULoad15:            load.Load15,
		CPUCores:             cpuCores,
		CPURunQueue:          float64(load.RunQueueDepth),
		RAMUsagePercent:      percentOf(memInfo.UsedBytes, memInfo.TotalBytes),
		RAMUsedBytes:         memInfo.UsedBytes,
		RAMTotalBytes:        memInfo.TotalBytes,
		DiskReadMBS:          diskAgg.ReadMBS,
		DiskWriteMBS:         diskAgg.WriteMBS,
		DiskTotalMBS:         diskAgg.TotalMBS,
		DiskUtilPercent:      diskAgg.UtilPercent,
		DiskIOPS:             diskAgg.IOPS,
		NetRxMBS:             netAgg.RxMBS,
		NetTxMBS:             netAgg.TxMBS,
		NetTotalMBS:          netAgg.TotalMBS,
		NetPacketRate:        netAgg.PacketRate,
		GPUCount:             gpuAgg.Count,
		GPUTotalUtilPercent:  gpuAgg.TotalUtil,
		GPUMemoryUtilPercent: percentOf(gpuAgg.MemoryUsedBytes, gpuAgg.MemoryTotalByte),
		GPUMemoryUsedBytes:   gpuAgg.MemoryUsedBytes,
		GPUMemoryTotalBytes:  gpuAgg.MemoryTotalByte,
		ProcessCount:         processCount,
		ThreadCount:          threadCount,
		SystemLoadPercent:    systemLoadPercent,
	}, nil
}
