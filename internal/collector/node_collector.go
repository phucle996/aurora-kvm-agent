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

func NewNodeCollector(reader *libvirtnode.NodeMetricsReader, nodeID, hostname string) *NodeCollector {
	return &NodeCollector{reader: reader, nodeID: nodeID, hostname: hostname}
}

func (c *NodeCollector) Collect(ctx context.Context) (model.NodeMetrics, error) {
	return c.reader.Collect(ctx, c.nodeID, c.hostname)
}
