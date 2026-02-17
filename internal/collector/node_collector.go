package collector

import (
	"context"

	libvirtnode "aurora-kvm-agent/internal/libvirt/metric/node"
	"aurora-kvm-agent/internal/model"
)

type NodeCollector struct {
	reader   *libvirtnode.NodeMetricsReader
	nodeID   string
	hostname string
}

// NewNodeCollector creates a collector bound to one node identity.
func NewNodeCollector(reader *libvirtnode.NodeMetricsReader, nodeID, hostname string) *NodeCollector {
	return &NodeCollector{reader: reader, nodeID: nodeID, hostname: hostname}
}

// Collect reads the latest node metrics snapshot from the node metrics reader.
func (c *NodeCollector) Collect(ctx context.Context) (model.NodeMetrics, error) {
	return c.reader.Collect(ctx, c.nodeID, c.hostname)
}
