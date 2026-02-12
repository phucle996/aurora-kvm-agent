package node

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func readGPUMetrics(ctx context.Context) (uint64, string, float64, uint64, uint64) {
	cmd := exec.CommandContext(
		ctx,
		"nvidia-smi",
		"--query-gpu=name,utilization.gpu,memory.used,memory.total",
		"--format=csv,noheader,nounits",
	)
	out, err := cmd.Output()
	if err != nil {
		return 0, "", 0, 0, 0
	}

	var count uint64
	var modelName string
	var usageTotal float64
	var memUsedTotal uint64
	var memTotalTotal uint64

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 4 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		utilStr := strings.TrimSpace(parts[1])
		memUsedStr := strings.TrimSpace(parts[2])
		memTotalStr := strings.TrimSpace(parts[3])

		if modelName == "" {
			modelName = name
		} else if modelName != name && name != "" {
			modelName = "mixed"
		}

		util, _ := strconv.ParseFloat(utilStr, 64)
		memUsedMB, _ := strconv.ParseUint(memUsedStr, 10, 64)
		memTotalMB, _ := strconv.ParseUint(memTotalStr, 10, 64)

		usageTotal += util
		memUsedTotal += memUsedMB * 1024 * 1024
		memTotalTotal += memTotalMB * 1024 * 1024
		count++
	}

	if count == 0 {
		return 0, "", 0, 0, 0
	}
	return count, modelName, usageTotal / float64(count), memUsedTotal, memTotalTotal
}

func readProcessThreadCounts() (uint64, uint64) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, 0
	}

	var processCount uint64
	var threadCount uint64

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "" {
			continue
		}
		isPID := true
		for i := 0; i < len(name); i++ {
			if name[i] < '0' || name[i] > '9' {
				isPID = false
				break
			}
		}
		if !isPID {
			continue
		}

		processCount++
		statusPath := "/proc/" + name + "/status"
		statusRaw, readErr := os.ReadFile(statusPath)
		if readErr != nil {
			continue
		}
		lines := strings.Split(string(statusRaw), "\n")
		for _, line := range lines {
			if !strings.HasPrefix(line, "Threads:") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 {
				break
			}
			v, parseErr := strconv.ParseUint(fields[1], 10, 64)
			if parseErr == nil {
				threadCount += v
			}
			break
		}
	}
	return processCount, threadCount
}

func deltaCounter(cur, prev uint64) uint64 {
	if cur < prev {
		return 0
	}
	return cur - prev
}

func percentDelta(cur, prev, totalDelta uint64) float64 {
	if totalDelta == 0 {
		return 0
	}
	return clampPercent((float64(deltaCounter(cur, prev)) / float64(totalDelta)) * 100)
}

func clampPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func percentOf(value, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return clampPercent((float64(value) / float64(total)) * 100)
}

func parseUintFlexible(raw string) uint64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	raw = strings.Fields(raw)[0]
	value, err := strconv.ParseUint(raw, 10, 64)
	if err == nil {
		return value
	}
	floatValue, err := strconv.ParseFloat(raw, 64)
	if err != nil || floatValue < 0 {
		return 0
	}
	return uint64(floatValue)
}

func parseMHzFlexible(raw string) uint64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return 0
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || value <= 0 {
		return 0
	}
	return uint64(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
