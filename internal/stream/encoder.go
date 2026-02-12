package stream

import (
	"encoding/json"
	"time"

	"aurora-kvm-agent/internal/model"
)

type Sink interface {
	SendNodeMetrics(ctx Context, m model.NodeMetrics) error
	SendNodeInfoSync(ctx Context, info model.NodeInfoSync) error
	SendVMMetrics(ctx Context, metrics []model.VMMetrics) error
	SendVMRuntimeMetrics(ctx Context, metrics []model.VMRuntimeMetrics) error
	SendVMInfoSync(ctx Context, info model.VMInfoSync) error
	Close(ctx Context) error
}

type Context interface {
	Done() <-chan struct{}
	Err() error
	Deadline() (time.Time, bool)
	Value(key any) any
}

type NodeFrame struct {
	NodeID    string            `json:"node_id"`
	Timestamp time.Time         `json:"timestamp"`
	Metrics   model.NodeMetrics `json:"metrics"`
}

type VMFrame struct {
	NodeID    string            `json:"node_id"`
	Timestamp time.Time         `json:"timestamp"`
	Metrics   []model.VMMetrics `json:"metrics"`
}

type NodeStaticFrame struct {
	NodeID    string                  `json:"node_id"`
	Timestamp time.Time               `json:"timestamp"`
	Metrics   model.NodeStaticMetrics `json:"metrics"`
}

type NodeInfoSyncFrame struct {
	NodeID       string                  `json:"node_id"`
	Timestamp    time.Time               `json:"timestamp"`
	SyncMode     string                  `json:"sync_mode"`
	Reason       string                  `json:"reason"`
	Info         model.NodeStaticMetrics `json:"info"`
	AgentVersion string                  `json:"agent_version"`
	Capabilities []string                `json:"capabilities"`
}

type VMInfoSyncFrame struct {
	NodeID       string         `json:"node_id"`
	Timestamp    time.Time      `json:"timestamp"`
	SyncMode     string         `json:"sync_mode"`
	Reason       string         `json:"reason"`
	Upserts      []model.VMInfo `json:"upserts"`
	RemovedVMIDs []string       `json:"removed_vm_ids"`
}

type VMRuntimeFrame struct {
	NodeID    string                   `json:"node_id"`
	Timestamp time.Time                `json:"timestamp"`
	Metrics   []model.VMRuntimeMetrics `json:"metrics"`
}

func EncodeEnvelope(e model.Envelope) ([]byte, error) {
	return json.Marshal(e)
}

func NewNodeFrame(m model.NodeMetrics) NodeFrame {
	return NodeFrame{NodeID: m.NodeID, Timestamp: m.Timestamp, Metrics: m}
}

func NewVMFrame(metrics []model.VMMetrics) VMFrame {
	nodeID := ""
	at := time.Now().UTC()
	if len(metrics) > 0 {
		nodeID = metrics[0].NodeID
		at = metrics[0].Timestamp
	}
	return VMFrame{NodeID: nodeID, Timestamp: at, Metrics: metrics}
}

func NewNodeStaticFrame(m model.NodeStaticMetrics) NodeStaticFrame {
	return NodeStaticFrame{NodeID: m.NodeID, Timestamp: m.Timestamp, Metrics: m}
}

func NewNodeInfoSyncFrame(info model.NodeInfoSync) NodeInfoSyncFrame {
	return NodeInfoSyncFrame{
		NodeID:       info.NodeID,
		Timestamp:    info.Timestamp,
		SyncMode:     info.SyncMode,
		Reason:       info.Reason,
		Info:         info.Info,
		AgentVersion: info.AgentVersion,
		Capabilities: append([]string(nil), info.Capabilities...),
	}
}

func NewVMRuntimeFrame(metrics []model.VMRuntimeMetrics) VMRuntimeFrame {
	nodeID := ""
	at := time.Now().UTC()
	if len(metrics) > 0 {
		nodeID = metrics[0].NodeID
		at = metrics[0].TS
	}
	return VMRuntimeFrame{NodeID: nodeID, Timestamp: at, Metrics: metrics}
}

func NewVMInfoSyncFrame(info model.VMInfoSync) VMInfoSyncFrame {
	return VMInfoSyncFrame{
		NodeID:       info.NodeID,
		Timestamp:    info.Timestamp,
		SyncMode:     info.SyncMode,
		Reason:       info.Reason,
		Upserts:      append([]model.VMInfo(nil), info.Upserts...),
		RemovedVMIDs: append([]string(nil), info.RemovedVMIDs...),
	}
}
