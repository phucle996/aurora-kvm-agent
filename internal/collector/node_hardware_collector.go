package collector

import (
	"context"

	libvirtnode "aurora-kvm-agent/internal/libvirt/metric/node"
	"aurora-kvm-agent/internal/model"
)

// NodeHardwareCollector collects static hardware/node-info payloads.
type NodeHardwareCollector struct {
	reader   *libvirtnode.NodeMetricsReader
	nodeID   string
	hostname string
}

// NewNodeHardwareCollector creates a hardware collector bound to one node identity.
func NewNodeHardwareCollector(reader *libvirtnode.NodeMetricsReader, nodeID, hostname string) *NodeHardwareCollector {
	return &NodeHardwareCollector{reader: reader, nodeID: nodeID, hostname: hostname}
}

// Collect reads static hardware information from the node metrics reader.
func (c *NodeHardwareCollector) Collect(ctx context.Context) (model.NodeHardwareInfo, error) {
	return c.reader.CollectHardware(ctx, c.nodeID, c.hostname)
}
