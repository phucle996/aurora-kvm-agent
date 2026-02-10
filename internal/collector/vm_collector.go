package collector

import (
	"context"

	"aurora-kvm-agent/internal/libvirt"
	"aurora-kvm-agent/internal/model"
)

type VMCollector struct {
	reader *libvirt.VMMetricsReader
	nodeID string
}

func NewVMCollector(reader *libvirt.VMMetricsReader, nodeID string) *VMCollector {
	return &VMCollector{reader: reader, nodeID: nodeID}
}

func (c *VMCollector) Collect(ctx context.Context) ([]model.VMMetrics, error) {
	return c.reader.Collect(ctx, c.nodeID)
}
