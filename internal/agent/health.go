package agent

import (
	"sync/atomic"
	"time"
)

type HealthStatus struct {
	libvirtConnected atomic.Bool
	streamConnected  atomic.Bool
	lastNodeSampleAt atomic.Int64
	lastVMSampleAt   atomic.Int64
}

func NewHealthStatus() *HealthStatus {
	h := &HealthStatus{}
	h.libvirtConnected.Store(false)
	h.streamConnected.Store(false)
	return h
}

func (h *HealthStatus) SetLibvirtConnected(ok bool) {
	h.libvirtConnected.Store(ok)
}

func (h *HealthStatus) SetStreamConnected(ok bool) {
	h.streamConnected.Store(ok)
}

func (h *HealthStatus) MarkNodeSample(ts time.Time) {
	h.lastNodeSampleAt.Store(ts.UnixNano())
}

func (h *HealthStatus) MarkVMSample(ts time.Time) {
	h.lastVMSampleAt.Store(ts.UnixNano())
}

func (h *HealthStatus) Snapshot() map[string]any {
	out := map[string]any{
		"libvirt_connected": h.libvirtConnected.Load(),
		"stream_connected":  h.streamConnected.Load(),
	}
	if v := h.lastNodeSampleAt.Load(); v > 0 {
		out["last_node_sample_at"] = time.Unix(0, v).UTC()
	}
	if v := h.lastVMSampleAt.Load(); v > 0 {
		out["last_vm_sample_at"] = time.Unix(0, v).UTC()
	}
	return out
}
