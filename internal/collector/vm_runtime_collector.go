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

// NewVMRuntimeCollector creates a collector for VM runtime detail metrics.
func NewVMRuntimeCollector(reader *libvirtvm.VMMetricsReader, nodeID string) *VMRuntimeCollector {
	return &VMRuntimeCollector{reader: reader, nodeID: nodeID}
}

// Collect reads detailed runtime metrics for VMs on the configured node.
func (c *VMRuntimeCollector) Collect(ctx context.Context) ([]model.VMRuntimeMetrics, error) {
	return c.reader.CollectRuntime(ctx, c.nodeID)
}
