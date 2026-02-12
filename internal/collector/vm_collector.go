package collector

import (
	"context"

	libvirtvm "aurora-kvm-agent/internal/libvirt/metric/vm"
	"aurora-kvm-agent/internal/model"
)

type VMCollector struct {
	reader *libvirtvm.VMMetricsReader
	nodeID string
}

func NewVMCollector(reader *libvirtvm.VMMetricsReader, nodeID string) *VMCollector {
	return &VMCollector{reader: reader, nodeID: nodeID}
}

func (c *VMCollector) Collect(ctx context.Context) ([]model.VMMetrics, error) {
	return c.reader.Collect(ctx, c.nodeID)
}
