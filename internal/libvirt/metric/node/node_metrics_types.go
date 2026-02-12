package node

import (
	"log/slog"
	"sync"
	"time"

	"aurora-kvm-agent/internal/libvirt/metric"
	"aurora-kvm-agent/internal/system"
)

type cpuStaticInfo struct {
	ModelName             string
	Vendor                string
	Architecture          string
	VirtualizationSupport string
	CacheL1               string
	CacheL2               string
	CacheL3               string
	FreqMinMhz            uint64
	FreqMaxMhz            uint64
	CoreCountPhysical     uint64
	ThreadCountLogical    uint64
	SocketCount           uint64
}

type procCPUSnapshot struct {
	CapturedAt      time.Time
	Aggregate       system.CPUCounters
	PerCore         []system.CPUCounters
	ContextSwitches uint64
	Interrupts      uint64
	ProcsRunning    uint64
}

type cpuRealtimeMetrics struct {
	TotalPercent       float64
	UserPercent        float64
	SystemPercent      float64
	IdlePercent        float64
	IOWaitPercent      float64
	StealPercent       float64
	IRQPercent         float64
	SoftIRQPercent     float64
	NicePercent        float64
	PerCoreUsagePct    []float64
	ContextSwitchesSec float64
	InterruptsSec      float64
}

type loadAverage struct {
	Load1         float64
	Load5         float64
	Load15        float64
	RunQueueDepth uint64
}

type memoryInfoProc struct {
	TotalBytes       uint64
	UsedBytes        uint64
	FreeBytes        uint64
	AvailableBytes   uint64
	CachedBytes      uint64
	BufferBytes      uint64
	SharedBytes      uint64
	SlabBytes        uint64
	ActiveBytes      uint64
	InactiveBytes    uint64
	DirtyBytes       uint64
	WritebackBytes   uint64
	SwapTotalBytes   uint64
	SwapUsedBytes    uint64
	SwapFreeBytes    uint64
	HugePagesTotal   uint64
	HugePagesUsed    uint64
	HugePagesFree    uint64
	HugePageSizeByte uint64
}

type vmStatSnapshot struct {
	CollectedAt     time.Time
	PageFaults      uint64
	MajorPageFaults uint64
	SwapInPages     uint64
	SwapOutPages    uint64
	OOMKillCount    uint64
}

type memoryRateMetrics struct {
	SwapInBytesPerSec     float64
	SwapOutBytesPerSec    float64
	PageFaultsPerSec      float64
	MajorPageFaultsPerSec float64
	OOMKillCount          uint64
}

type memoryModuleInfo struct {
	Slot         string
	SizeBytes    uint64
	SpeedMhz     uint64
	Type         string
	Model        string
	Manufacturer string
	PartNumber   string
	Serial       string
}

type memoryHardwareInfo struct {
	StickCount         uint64
	StickSlot          []string
	StickSizeBytes     []uint64
	StickSpeedMhz      []uint64
	StickType          []string
	StickModel         []string
	StickManufacturer  []string
	StickPartNumber    []string
	StickSerial        []string
	TotalPhysicalBytes uint64
	ECCEnabled         bool
}

type netIfCounterSnapshot struct {
	RxBytes   uint64
	TxBytes   uint64
	RxPackets uint64
	TxPackets uint64
}

type NodeMetricsReader struct {
	conn   *metric.ConnManager
	logger *slog.Logger

	mu                 sync.Mutex
	prevSnapshot       procCPUSnapshot
	hasPrevSnapshot    bool
	prevVMStatSnapshot vmStatSnapshot
	hasPrevVMStat      bool
	prevDiskStats      map[string]diskStatSnapshot
	prevDiskAt         time.Time
	hasPrevDiskStats   bool
	prevNetStats       map[string]netIfCounterSnapshot
	prevNetAt          time.Time
	hasPrevNetStats    bool
	cpuInfoLoaded      bool
	cpuStatic          cpuStaticInfo
	memInfoLoaded      bool
	memHardware        memoryHardwareInfo
}
