package collector

import (
	"context"

	"aurora-kvm-agent/internal/libvirt"
	"aurora-kvm-agent/internal/model"
)

type NodeCollector struct {
	reader   *libvirt.NodeMetricsReader
	nodeID   string
	hostname string
}

func NewNodeCollector(reader *libvirt.NodeMetricsReader, nodeID, hostname string) *NodeCollector {
	return &NodeCollector{reader: reader, nodeID: nodeID, hostname: hostname}
}

func (c *NodeCollector) Collect(ctx context.Context) (model.NodeMetrics, error) {
	return c.reader.Collect(ctx, c.nodeID, c.hostname)
}
