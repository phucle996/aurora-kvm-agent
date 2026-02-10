package stream

import (
	"encoding/json"
	"time"

	"aurora-kvm-agent/internal/model"
)

type Sink interface {
	SendNodeMetrics(ctx Context, m model.NodeMetrics) error
	SendVMMetrics(ctx Context, metrics []model.VMMetrics) error
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
