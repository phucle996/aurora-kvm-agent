package model

import "time"

// NodeMetrics represents host-level measurements sampled from libvirt + kernel counters.
type NodeMetrics struct {
	NodeID    string    `json:"node_id"`
	Hostname  string    `json:"hostname"`
	Timestamp time.Time `json:"timestamp"`

	NodeCPUMetrics
	NodeMemoryMetrics
	NodeLoadMetrics
	NodeIOMetrics
	NodeProcessMetrics
	NodeGPUMetrics
}
