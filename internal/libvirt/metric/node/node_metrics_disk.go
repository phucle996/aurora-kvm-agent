package node

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
	"syscall"
	"time"

	"aurora-kvm-agent/internal/model"
)

type diskStatSnapshot struct {
	Name            string
	ReadsCompleted  uint64
	SectorsRead     uint64
	TimeReadingMs   uint64
	WritesCompleted uint64
	SectorsWritten  uint64
	TimeWritingMs   uint64
	IOInProgress    uint64
	TimeDoingIOms   uint64
}

type diskMountUsage struct {
	Mountpoints []string
	Filesystems []string
	FSTotal     uint64
	FSUsed      uint64
	FSFree      uint64
	InodeTotal  uint64
	InodeUsed   uint64
	InodeFree   uint64
}

func (r *NodeMetricsReader) collectNodeDiskMetrics(now time.Time) ([]model.NodeDiskMetrics, uint64, uint64) {
	current, err := readDiskStatSnapshots()
	if err != nil {
		r.logger.Warn("read per-disk stats failed", "error", err)
		return nil, 0, 0
	}

	mountUsages := readDiskMountUsages()
	names := make([]string, 0, len(current))
	for name := range current {
		names = append(names, name)
	}
	sort.Strings(names)

	var (
		prevByName map[string]diskStatSnapshot
		elapsedSec float64
	)
	r.mu.Lock()
	if r.hasPrevDiskStats {
		prevByName = r.prevDiskStats
		elapsedSec = now.Sub(r.prevDiskAt).Seconds()
	}
	r.prevDiskStats = current
	r.prevDiskAt = now
	r.hasPrevDiskStats = true
	r.mu.Unlock()

	disks := make([]model.NodeDiskMetrics, 0, len(names))
	var totalReadBytes uint64
	var totalWriteBytes uint64

	for _, name := range names {
		cur := current[name]
		sysInfo := readDiskSysInfo(name)
		mountUsage := mountUsages[name]

		disk := model.NodeDiskMetrics{
			Name:            name,
			Model:           sysInfo.Model,
			Serial:          sysInfo.Serial,
			Type:            sysInfo.Type,
			SizeBytes:       sysInfo.SizeBytes,
			QueueLength:     cur.IOInProgress,
			ReadBytesTotal:  cur.SectorsRead * 512,
			WriteBytesTotal: cur.SectorsWritten * 512,
			ReadOpsTotal:    cur.ReadsCompleted,
			WriteOpsTotal:   cur.WritesCompleted,
		}

		if mountUsage != nil {
			disk.Mountpoint = strings.Join(mountUsage.Mountpoints, ",")
			disk.Filesystem = strings.Join(mountUsage.Filesystems, ",")
			disk.FSTotalBytes = mountUsage.FSTotal
			disk.FSUsedBytes = mountUsage.FSUsed
			disk.FSFreeBytes = mountUsage.FSFree
			disk.FSUsagePercent = percentOf(mountUsage.FSUsed, mountUsage.FSTotal)
			disk.InodeTotal = mountUsage.InodeTotal
			disk.InodeUsed = mountUsage.InodeUsed
			disk.InodeFree = mountUsage.InodeFree
		}

		if elapsedSec > 0 && prevByName != nil {
			if prev, ok := prevByName[name]; ok {
				readBytesDelta := deltaCounter(cur.SectorsRead, prev.SectorsRead) * 512
				writeBytesDelta := deltaCounter(cur.SectorsWritten, prev.SectorsWritten) * 512
				readOpsDelta := deltaCounter(cur.ReadsCompleted, prev.ReadsCompleted)
				writeOpsDelta := deltaCounter(cur.WritesCompleted, prev.WritesCompleted)
				readTimeDelta := deltaCounter(cur.TimeReadingMs, prev.TimeReadingMs)
				writeTimeDelta := deltaCounter(cur.TimeWritingMs, prev.TimeWritingMs)
				busyMsDelta := deltaCounter(cur.TimeDoingIOms, prev.TimeDoingIOms)

				disk.ReadBytesPerSec = float64(readBytesDelta) / elapsedSec
				disk.WriteBytesPerSec = float64(writeBytesDelta) / elapsedSec
				disk.ReadIOPS = float64(readOpsDelta) / elapsedSec
				disk.WriteIOPS = float64(writeOpsDelta) / elapsedSec
				if readOpsDelta > 0 {
					disk.ReadLatencyMs = float64(readTimeDelta) / float64(readOpsDelta)
				}
				if writeOpsDelta > 0 {
					disk.WriteLatencyMs = float64(writeTimeDelta) / float64(writeOpsDelta)
				}
				disk.UtilPercent = clampPercent(float64(busyMsDelta) / (elapsedSec * 10))
			}
		}

		totalReadBytes += disk.ReadBytesTotal
		totalWriteBytes += disk.WriteBytesTotal
		disks = append(disks, disk)
	}

	return disks, totalReadBytes, totalWriteBytes
}

func readDiskStatSnapshots() (map[string]diskStatSnapshot, error) {
	f, err := os.Open("/proc/diskstats")
	if err != nil {
		return nil, fmt.Errorf("open /proc/diskstats: %w", err)
	}
	defer f.Close()

	out := make(map[string]diskStatSnapshot)
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 14 {
			continue
		}

		name := fields[2]
		if !isWholeBlockDevice(name) {
			continue
		}

		out[name] = diskStatSnapshot{
			Name:            name,
			ReadsCompleted:  parseUintFlexible(fields[3]),
			SectorsRead:     parseUintFlexible(fields[5]),
			TimeReadingMs:   parseUintFlexible(fields[6]),
			WritesCompleted: parseUintFlexible(fields[7]),
			SectorsWritten:  parseUintFlexible(fields[9]),
			TimeWritingMs:   parseUintFlexible(fields[10]),
			IOInProgress:    parseUintFlexible(fields[11]),
			TimeDoingIOms:   parseUintFlexible(fields[12]),
		}
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("scan /proc/diskstats: %w", err)
	}
	return out, nil
}

func readDiskMountUsages() map[string]*diskMountUsage {
	raw, err := os.ReadFile("/proc/self/mounts")
	if err != nil {
		return map[string]*diskMountUsage{}
	}

	out := make(map[string]*diskMountUsage)
	seenMount := make(map[string]map[string]struct{})
	seenFS := make(map[string]map[string]struct{})

	lines := strings.Split(string(raw), "\n")
	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 3 {
			continue
		}
		source := fields[0]
		mountpoint := fields[1]
		filesystem := fields[2]

		if !strings.HasPrefix(source, "/dev/") {
			continue
		}

		deviceName := strings.TrimPrefix(source, "/dev/")
		parentDisk := resolveParentBlockDevice(deviceName)
		if parentDisk == "" || !isWholeBlockDevice(parentDisk) {
			continue
		}

		stat := syscall.Statfs_t{}
		if err := syscall.Statfs(mountpoint, &stat); err != nil {
			continue
		}

		usage := out[parentDisk]
		if usage == nil {
			usage = &diskMountUsage{}
			out[parentDisk] = usage
		}

		if seenMount[parentDisk] == nil {
			seenMount[parentDisk] = make(map[string]struct{})
		}
		if _, ok := seenMount[parentDisk][mountpoint]; !ok {
			seenMount[parentDisk][mountpoint] = struct{}{}
			usage.Mountpoints = append(usage.Mountpoints, mountpoint)

			totalBytes := stat.Blocks * uint64(stat.Bsize)
			freeBytes := stat.Bavail * uint64(stat.Bsize)
			usedBytes := uint64(0)
			if totalBytes >= freeBytes {
				usedBytes = totalBytes - freeBytes
			}
			usage.FSTotal += totalBytes
			usage.FSFree += freeBytes
			usage.FSUsed += usedBytes

			inodeTotal := stat.Files
			inodeFree := stat.Ffree
			inodeUsed := uint64(0)
			if inodeTotal >= inodeFree {
				inodeUsed = inodeTotal - inodeFree
			}
			usage.InodeTotal += inodeTotal
			usage.InodeFree += inodeFree
			usage.InodeUsed += inodeUsed
		}

		if seenFS[parentDisk] == nil {
			seenFS[parentDisk] = make(map[string]struct{})
		}
		if filesystem != "" {
			if _, ok := seenFS[parentDisk][filesystem]; !ok {
				seenFS[parentDisk][filesystem] = struct{}{}
				usage.Filesystems = append(usage.Filesystems, filesystem)
			}
		}
	}

	for _, usage := range out {
		sort.Strings(usage.Mountpoints)
		sort.Strings(usage.Filesystems)
	}
	return out
}

type diskSysInfo struct {
	SizeBytes uint64
	Model     string
	Serial    string
	Type      string
}

func readDiskSysInfo(name string) diskSysInfo {
	basePath := "/sys/class/block/" + name
	sizeSectors := parseUintFlexible(readTextFile(basePath + "/size"))
	model := strings.TrimSpace(readTextFile(basePath + "/device/model"))
	serial := strings.TrimSpace(readTextFile(basePath + "/device/serial"))
	rotational := parseUintFlexible(readTextFile(basePath + "/queue/rotational"))

	diskType := "disk"
	switch {
	case strings.HasPrefix(name, "nvme"):
		diskType = "nvme"
	case rotational == 1:
		diskType = "hdd"
	case rotational == 0:
		diskType = "ssd"
	}

	return diskSysInfo{
		SizeBytes: sizeSectors * 512,
		Model:     model,
		Serial:    serial,
		Type:      diskType,
	}
}

func resolveParentBlockDevice(dev string) string {
	dev = strings.TrimPrefix(dev, "/dev/")
	dev = strings.TrimSpace(dev)
	if dev == "" {
		return ""
	}
	dev = strings.TrimPrefix(dev, "mapper/")
	dev = strings.TrimPrefix(dev, "disk/by-id/")
	dev = strings.TrimPrefix(dev, "disk/by-path/")
	dev = strings.TrimPrefix(dev, "disk/by-uuid/")
	dev = strings.TrimPrefix(dev, "disk/by-partuuid/")

	sysPath := "/sys/class/block/" + dev
	if _, err := os.Stat(sysPath); err != nil {
		return ""
	}

	if _, err := os.Stat(sysPath + "/partition"); err == nil {
		realPath, resolveErr := os.Readlink(sysPath)
		if resolveErr == nil {
			if !strings.HasPrefix(realPath, "/") {
				realPath = "/sys/class/block/" + realPath
			}
			parent := filepathBase(filepathDir(realPath))
			if parent != "" {
				return parent
			}
		}
		return guessParentFromPartitionName(dev)
	}
	return dev
}

func isWholeBlockDevice(name string) bool {
	if name == "" {
		return false
	}
	if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") || strings.HasPrefix(name, "fd") {
		return false
	}
	if strings.HasPrefix(name, "sr") {
		return false
	}
	switch {
	case strings.HasPrefix(name, "nvme"),
		strings.HasPrefix(name, "sd"),
		strings.HasPrefix(name, "vd"),
		strings.HasPrefix(name, "xvd"),
		strings.HasPrefix(name, "dm-"),
		strings.HasPrefix(name, "mmcblk"):
		// keep
	default:
		return false
	}
	if _, err := os.Stat("/sys/class/block/" + name + "/partition"); err == nil {
		return false
	}
	return true
}

func guessParentFromPartitionName(name string) string {
	if strings.HasPrefix(name, "nvme") {
		if idx := strings.LastIndex(name, "p"); idx > 0 {
			return name[:idx]
		}
	}
	if strings.HasPrefix(name, "mmcblk") {
		if idx := strings.LastIndex(name, "p"); idx > 0 {
			return name[:idx]
		}
	}
	trimmed := strings.TrimRight(name, "0123456789")
	if trimmed != "" && trimmed != name {
		return trimmed
	}
	return name
}

func readTextFile(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func filepathDir(path string) string {
	lastSlash := strings.LastIndex(path, "/")
	if lastSlash <= 0 {
		return ""
	}
	return path[:lastSlash]
}

func filepathBase(path string) string {
	if path == "" {
		return ""
	}
	lastSlash := strings.LastIndex(path, "/")
	if lastSlash < 0 || lastSlash == len(path)-1 {
		return path
	}
	return path[lastSlash+1:]
}
