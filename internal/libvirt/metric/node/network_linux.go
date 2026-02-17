package node

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"aurora-kvm-agent/internal/model"
)

type netIfSample struct {
	Name      string
	SpeedMbps uint64
	LinkUp    bool

	RxBytesTotal   uint64
	TxBytesTotal   uint64
	RxPacketsTotal uint64
	TxPacketsTotal uint64

	RxErrors  uint64
	TxErrors  uint64
	RxDropped uint64
	TxDropped uint64
}

func (r *NodeMetricsReader) collectNodeNetworkMetrics(now time.Time, nodeID string) ([]model.NodeNetIfMetricLite, netAggregateLite) {
	samples := readNetworkInterfaceSamples()
	if len(samples) == 0 {
		return []model.NodeNetIfMetricLite{}, netAggregateLite{}
	}

	metrics := make([]model.NodeNetIfMetricLite, 0, len(samples))
	agg := netAggregateLite{}

	for _, sample := range samples {
		m := model.NodeNetIfMetricLite{
			NodeID:        nodeID,
			InterfaceName: sample.Name,
			TimestampUnix: now.Unix(),
			RxErrors:      sample.RxErrors,
			TxErrors:      sample.TxErrors,
			RxDropped:     sample.RxDropped,
			TxDropped:     sample.TxDropped,
			SpeedMbps:     sample.SpeedMbps,
			LinkUp:        sample.LinkUp,
		}

		if delta, sec, ok := r.delta.ObserveCounter("net:"+sample.Name+":rx_bytes", now, sample.RxBytesTotal); ok && sec > 0 {
			rxBps := float64(delta) / sec
			m.RxMBS = rxBps / (1024 * 1024)
		}
		if delta, sec, ok := r.delta.ObserveCounter("net:"+sample.Name+":tx_bytes", now, sample.TxBytesTotal); ok && sec > 0 {
			txBps := float64(delta) / sec
			m.TxMBS = txBps / (1024 * 1024)
		}
		m.TotalMBS = m.RxMBS + m.TxMBS

		if delta, sec, ok := r.delta.ObserveCounter("net:"+sample.Name+":rx_packets", now, sample.RxPacketsTotal); ok && sec > 0 {
			m.RxPacketsPerSec = float64(delta) / sec
		}
		if delta, sec, ok := r.delta.ObserveCounter("net:"+sample.Name+":tx_packets", now, sample.TxPacketsTotal); ok && sec > 0 {
			m.TxPacketsPerSec = float64(delta) / sec
		}

		m.BandwidthUtilPercent = computeBandwidthUtilPercent(sample.SpeedMbps, m.RxMBS, m.TxMBS)

		agg.RxMBS += m.RxMBS
		agg.TxMBS += m.TxMBS
		agg.PacketRate += m.RxPacketsPerSec + m.TxPacketsPerSec

		metrics = append(metrics, m)
	}

	agg.TotalMBS = agg.RxMBS + agg.TxMBS
	sort.Slice(metrics, func(i, j int) bool { return metrics[i].InterfaceName < metrics[j].InterfaceName })
	return metrics, agg
}

func readNetworkInterfaceSamples() []netIfSample {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return []netIfSample{}
	}

	out := make([]netIfSample, 0, len(entries))
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name())
		if shouldSkipNetworkInterface(name) {
			continue
		}

		base := filepath.Join("/sys/class/net", name)
		statsBase := filepath.Join(base, "statistics")

		sample := netIfSample{
			Name:           name,
			SpeedMbps:      readUintSignedFile(filepath.Join(base, "speed")),
			LinkUp:         readTextFile(filepath.Join(base, "carrier")) == "1" || strings.EqualFold(readTextFile(filepath.Join(base, "operstate")), "up"),
			RxBytesTotal:   readUintFile(filepath.Join(statsBase, "rx_bytes")),
			TxBytesTotal:   readUintFile(filepath.Join(statsBase, "tx_bytes")),
			RxPacketsTotal: readUintFile(filepath.Join(statsBase, "rx_packets")),
			TxPacketsTotal: readUintFile(filepath.Join(statsBase, "tx_packets")),
			RxErrors:       readUintFile(filepath.Join(statsBase, "rx_errors")),
			TxErrors:       readUintFile(filepath.Join(statsBase, "tx_errors")),
			RxDropped:      readUintFile(filepath.Join(statsBase, "rx_dropped")),
			TxDropped:      readUintFile(filepath.Join(statsBase, "tx_dropped")),
		}
		out = append(out, sample)
	}

	return out
}

func shouldSkipNetworkInterface(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return true
	}
	if name == "lo" {
		return true
	}
	if strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "veth") {
		return true
	}
	return false
}

func computeBandwidthUtilPercent(speedMbps uint64, rxMBS, txMBS float64) float64 {
	if speedMbps == 0 {
		return 0
	}
	capacityMBS := (float64(speedMbps) / 8)
	if capacityMBS <= 0 {
		return 0
	}
	return clampPercent(((rxMBS + txMBS) / capacityMBS) * 100)
}

func readUintFile(path string) uint64 {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return parseUintFlexible(string(raw))
}

func readUintSignedFile(path string) uint64 {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return 0
	}
	if strings.HasPrefix(text, "-") {
		return 0
	}
	return parseUintFlexible(text)
}
