package stream

import (
	"encoding/json"
	"time"

	"aurora-kvm-agent/internal/model"
)

type Sink interface {
	SendNodeMetrics(ctx Context, m model.NodeMetrics) error
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
	NodeID        string            `json:"node_id"`
	TimestampUnix int64             `json:"timestamp_unix"`
	Metrics       model.NodeMetrics `json:"metrics"`
}

type VMFrame struct {
	NodeID        string            `json:"node_id"`
	TimestampUnix int64             `json:"timestamp_unix"`
	Metrics       []model.VMMetrics `json:"metrics"`
}

type VMInfoSyncFrame struct {
	NodeID        string         `json:"node_id"`
	TimestampUnix int64          `json:"timestamp_unix"`
	SyncMode      string         `json:"sync_mode"`
	Reason        string         `json:"reason"`
	Upserts       []model.VMInfo `json:"upserts"`
	RemovedVMIDs  []string       `json:"removed_vm_ids"`
}

type VMRuntimeFrame struct {
	NodeID        string                   `json:"node_id"`
	TimestampUnix int64                    `json:"timestamp_unix"`
	Metrics       []model.VMRuntimeMetrics `json:"metrics"`
}

func EncodeEnvelope(e model.Envelope) ([]byte, error) {
	return json.Marshal(e)
}

func NewNodeFrame(m model.NodeMetrics) NodeFrame {
	return NodeFrame{NodeID: m.NodeID, TimestampUnix: m.TimestampUnix, Metrics: m}
}

func NewVMFrame(metrics []model.VMMetrics) VMFrame {
	nodeID := ""
	at := time.Now().UTC().Unix()
	if len(metrics) > 0 {
		nodeID = metrics[0].NodeID
		at = metrics[0].TimestampUnix
	}
	return VMFrame{NodeID: nodeID, TimestampUnix: at, Metrics: metrics}
}

func NewVMRuntimeFrame(metrics []model.VMRuntimeMetrics) VMRuntimeFrame {
	nodeID := ""
	at := time.Now().UTC().Unix()
	if len(metrics) > 0 {
		nodeID = metrics[0].NodeID
		at = metrics[0].TimestampUnix
	}
	return VMRuntimeFrame{NodeID: nodeID, TimestampUnix: at, Metrics: metrics}
}

func NewVMInfoSyncFrame(info model.VMInfoSync) VMInfoSyncFrame {
	return VMInfoSyncFrame{
		NodeID:        info.NodeID,
		TimestampUnix: info.TimestampUnix,
		SyncMode:      info.SyncMode,
		Reason:        info.Reason,
		Upserts:       append([]model.VMInfo(nil), info.Upserts...),
		RemovedVMIDs:  append([]string(nil), info.RemovedVMIDs...),
	}
}
