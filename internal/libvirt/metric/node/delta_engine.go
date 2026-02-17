package node

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type deltaSample struct {
	value uint64
	at    time.Time
}

type deltaEngine struct {
	mu      sync.Mutex
	samples map[string]deltaSample
}

func newDeltaEngine() *deltaEngine {
	return &deltaEngine{samples: make(map[string]deltaSample)}
}

// ObserveCounter stores current counter and returns delta + elapsed seconds from previous sample.
func (e *deltaEngine) ObserveCounter(key string, now time.Time, cur uint64) (delta uint64, seconds float64, ok bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	prev, exists := e.samples[key]
	e.samples[key] = deltaSample{value: cur, at: now}
	if !exists {
		return 0, 0, false
	}

	seconds = now.Sub(prev.at).Seconds()
	if seconds <= 0 {
		return 0, 0, false
	}
	if cur < prev.value {
		// counter reset/overflow/restart
		return 0, seconds, false
	}
	return cur - prev.value, seconds, true
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
