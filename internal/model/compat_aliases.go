package model

// Compatibility aliases to keep old call sites compiling during proto-first migration.
type NodeMetrics = NodeMetricsLite
type NodeStaticMetrics = NodeHardwareInfo
type VMMetrics = VMLiteMetrics

func BuildVMInfoMap(metrics []VMMetrics) map[string]VMInfo {
	return BuildVMInfoMapFromLite(metrics)
}
