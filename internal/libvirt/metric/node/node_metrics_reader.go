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
	memRates := r.computeMemoryRates(vmStat)
	disks, diskReadTotal, diskWriteTotal := r.collectNodeDiskMetrics(now)

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
	if memInfo.FreeBytes == 0 && ramTotalBytes > ramUsedBytes {
		memInfo.FreeBytes = ramTotalBytes - ramUsedBytes
	}
	if memInfo.AvailableBytes == 0 && ramTotalBytes > ramUsedBytes {
		memInfo.AvailableBytes = ramTotalBytes - ramUsedBytes
	}
	if memInfo.UsedBytes == 0 {
		memInfo.UsedBytes = ramUsedBytes
	}
	if memInfo.TotalBytes == 0 {
		memInfo.TotalBytes = ramTotalBytes
	}
	if memInfo.SwapTotalBytes > 0 && memInfo.SwapUsedBytes == 0 {
		memInfo.SwapUsedBytes = memInfo.SwapTotalBytes - memInfo.SwapFreeBytes
	}
	ramUsagePercent := percentOf(memInfo.UsedBytes, memInfo.TotalBytes)
	ramAvailablePercent := percentOf(memInfo.AvailableBytes, memInfo.TotalBytes)
	swapUsagePercent := percentOf(memInfo.SwapUsedBytes, memInfo.SwapTotalBytes)

	if diskReadTotal == 0 && diskWriteTotal == 0 {
		diskCounters, diskErr := system.ReadDiskCounters()
		if diskErr == nil {
			diskReadTotal = diskCounters.ReadBytes
			diskWriteTotal = diskCounters.WriteBytes
		} else {
			r.logger.Warn("read aggregate disk counters failed", "error", diskErr)
		}
	}
	netifs, netGlobal := r.collectNodeNetworkMetrics(now)
	load := readLoadAvg()
	if load.RunQueueDepth == 0 && snapshot.ProcsRunning > 0 {
		load.RunQueueDepth = snapshot.ProcsRunning
	}

	staticInfo := r.loadCPUStaticInfo(uint64(mhz), uint64(cpus))
	currentMhz := readCPUCurrentMHz(uint64(mhz))
	if currentMhz == 0 {
		currentMhz = uint64(mhz)
	}
	if staticInfo.FreqMinMhz == 0 {
		staticInfo.FreqMinMhz = currentMhz
	}
	if staticInfo.FreqMaxMhz == 0 {
		staticInfo.FreqMaxMhz = currentMhz
	}
	if staticInfo.FreqMaxMhz < staticInfo.FreqMinMhz {
		staticInfo.FreqMaxMhz = staticInfo.FreqMinMhz
	}
	if staticInfo.ThreadCountLogical == 0 {
		staticInfo.ThreadCountLogical = uint64(cpus)
	}
	if staticInfo.CoreCountPhysical == 0 {
		staticInfo.CoreCountPhysical = staticInfo.ThreadCountLogical
	}

	gpuMetrics := r.collectGPUMetrics(ctx)
	processCount, threadCount := readProcessThreadCounts()
	cpuTemperature := readCPUTemperatureCelsius()
	memoryPressurePercent := readMemoryPressurePercent()
	memHardware := r.loadMemoryHardwareInfo(ramTotalBytes)

	if hostname == "" {
		hostname = os.Getenv("HOSTNAME")
	}

	return model.NodeMetrics{
		NodeID:    nodeID,
		Hostname:  hostname,
		Timestamp: now,
		NodeCPUMetrics: model.NodeCPUMetrics{
			CPUUsagePct:              cpuRealtime.TotalPercent,
			CPUUsageTotalPercent:     cpuRealtime.TotalPercent,
			CPUUserPercent:           cpuRealtime.UserPercent,
			CPUSystemPercent:         cpuRealtime.SystemPercent,
			CPUIdlePercent:           cpuRealtime.IdlePercent,
			CPUIOWaitPercent:         cpuRealtime.IOWaitPercent,
			CPUStealPercent:          cpuRealtime.StealPercent,
			CPUIRQPercent:            cpuRealtime.IRQPercent,
			CPUSoftIRQPercent:        cpuRealtime.SoftIRQPercent,
			CPUNicePercent:           cpuRealtime.NicePercent,
			CPUCores:                 staticInfo.ThreadCountLogical,
			CPUMhz:                   currentMhz,
			CPUFreqCurrentMhz:        currentMhz,
			CPUFreqMinMhz:            staticInfo.FreqMinMhz,
			CPUFreqMaxMhz:            staticInfo.FreqMaxMhz,
			CPUCoreCountPhysical:     staticInfo.CoreCountPhysical,
			CPUThreadCountLogical:    staticInfo.ThreadCountLogical,
			CPUSocketCount:           staticInfo.SocketCount,
			CPUCoreUsagePercent:      cpuRealtime.PerCoreUsagePct,
			ContextSwitchesPerSec:    cpuRealtime.ContextSwitchesSec,
			InterruptsPerSec:         cpuRealtime.InterruptsSec,
			CPUTemperatureCelsius:    cpuTemperature,
			CPUModel:                 staticInfo.ModelName,
			CPUVendor:                staticInfo.Vendor,
			CPUModelName:             staticInfo.ModelName,
			CPUArchitecture:          staticInfo.Architecture,
			CPUVirtualizationSupport: staticInfo.VirtualizationSupport,
			CPUCacheL1:               staticInfo.CacheL1,
			CPUCacheL2:               staticInfo.CacheL2,
			CPUCacheL3:               staticInfo.CacheL3,
			CPUBaseMhz:               staticInfo.FreqMinMhz,
			CPUBurstMhz:              staticInfo.FreqMaxMhz,
		},
		NodeMemoryMetrics: model.NodeMemoryMetrics{
			RAMTotalBytes:         memInfo.TotalBytes,
			RAMUsedBytes:          memInfo.UsedBytes,
			RAMFreeBytes:          memInfo.FreeBytes,
			RAMAvailableBytes:     memInfo.AvailableBytes,
			RAMCachedBytes:        memInfo.CachedBytes,
			RAMBufferBytes:        memInfo.BufferBytes,
			RAMSharedBytes:        memInfo.SharedBytes,
			RAMSlabBytes:          memInfo.SlabBytes,
			RAMActiveBytes:        memInfo.ActiveBytes,
			RAMInactiveBytes:      memInfo.InactiveBytes,
			RAMDirtyBytes:         memInfo.DirtyBytes,
			RAMWritebackBytes:     memInfo.WritebackBytes,
			RAMUsagePercent:       ramUsagePercent,
			RAMAvailablePercent:   ramAvailablePercent,
			SwapTotalBytes:        memInfo.SwapTotalBytes,
			SwapUsedBytes:         memInfo.SwapUsedBytes,
			SwapFreeBytes:         memInfo.SwapFreeBytes,
			SwapUsagePercent:      swapUsagePercent,
			SwapInBytesPerSec:     memRates.SwapInBytesPerSec,
			SwapOutBytesPerSec:    memRates.SwapOutBytesPerSec,
			PageFaultsPerSec:      memRates.PageFaultsPerSec,
			MajorPageFaultsPerSec: memRates.MajorPageFaultsPerSec,
			OOMKillCount:          memRates.OOMKillCount,
			RAMStickCount:         memHardware.StickCount,
			RAMStickSlot:          memHardware.StickSlot,
			RAMStickSizeBytes:     memHardware.StickSizeBytes,
			RAMStickSpeedMhz:      memHardware.StickSpeedMhz,
			RAMStickType:          memHardware.StickType,
			RAMStickModel:         memHardware.StickModel,
			RAMStickManufacturer:  memHardware.StickManufacturer,
			RAMStickPartNumber:    memHardware.StickPartNumber,
			RAMStickSerial:        memHardware.StickSerial,
			RAMTotalPhysicalBytes: memHardware.TotalPhysicalBytes,
			RAMECCEnabled:         memHardware.ECCEnabled,
			HugePagesTotal:        memInfo.HugePagesTotal,
			HugePagesUsed:         memInfo.HugePagesUsed,
			HugePagesFree:         memInfo.HugePagesFree,
			MemoryPressurePercent: memoryPressurePercent,
		},
		NodeLoadMetrics: model.NodeLoadMetrics{
			Load1:          load.Load1,
			Load5:          load.Load5,
			Load15:         load.Load15,
			RunQueueLength: load.RunQueueDepth,
		},
		NodeIOMetrics: model.NodeIOMetrics{
			DiskReadBytes:           diskReadTotal,
			DiskWriteBytes:          diskWriteTotal,
			NetRxBytes:              netGlobal.NetRxBytes,
			NetTxBytes:              netGlobal.NetTxBytes,
			Disks:                   disks,
			Netifs:                  netifs,
			NetTotalRxBytesPerSec:   netGlobal.NetTotalRxBytesPerSec,
			NetTotalTxBytesPerSec:   netGlobal.NetTotalTxBytesPerSec,
			NetTotalRxPacketsPerSec: netGlobal.NetTotalRxPacketsPerSec,
			NetTotalTxPacketsPerSec: netGlobal.NetTotalTxPacketsPerSec,
			NetTotalConnections:     netGlobal.NetTotalConnections,
			NetConntrackCount:       netGlobal.NetConntrackCount,
			NetConntrackMax:         netGlobal.NetConntrackMax,
		},
		NodeProcessMetrics: model.NodeProcessMetrics{
			ProcessTotal: processCount,
			ThreadTotal:  threadCount,
			ProcessCount: processCount,
			ThreadCount:  threadCount,
		},
		NodeGPUMetrics: model.NodeGPUMetrics{
			GPUCount:                gpuMetrics.GPUCount,
			GPUModel:                gpuMetrics.GPUModel,
			GPUUsagePct:             gpuMetrics.GPUUsagePct,
			GPUMemoryUsedBytes:      gpuMetrics.GPUMemoryUsedBytes,
			GPUMemoryTotalBytes:     gpuMetrics.GPUMemoryTotalBytes,
			GPUs:                    gpuMetrics.GPUs,
			GPUTotalCount:           gpuMetrics.GPUTotalCount,
			GPUTotalMemoryBytes:     gpuMetrics.GPUTotalMemoryBytes,
			GPUTotalMemoryUsedBytes: gpuMetrics.GPUTotalMemoryUsedBytes,
			GPUTotalUtilPercent:     gpuMetrics.GPUTotalUtilPercent,
			GPUTotalPowerWatts:      gpuMetrics.GPUTotalPowerWatts,
		},
	}, nil
}
