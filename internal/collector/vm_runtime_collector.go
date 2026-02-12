package collector

import (
	"context"

	libvirtvm "aurora-kvm-agent/internal/libvirt/metric/vm"
	"aurora-kvm-agent/internal/model"
)

type VMRuntimeCollector struct {
	reader *libvirtvm.VMMetricsReader
	nodeID string
}

func NewVMRuntimeCollector(reader *libvirtvm.VMMetricsReader, nodeID string) *VMRuntimeCollector {
	return &VMRuntimeCollector{reader: reader, nodeID: nodeID}
}

func (c *VMRuntimeCollector) Collect(ctx context.Context) ([]model.VMRuntimeMetrics, error) {
	return c.reader.CollectRuntime(ctx, c.nodeID)
}
