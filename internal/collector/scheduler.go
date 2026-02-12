package collector

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"log/slog"
	"sort"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"aurora-kvm-agent/internal/model"
	"aurora-kvm-agent/internal/stream"
)

type Scheduler struct {
	logger            *slog.Logger
	node              *NodeCollector
	vm                *VMCollector
	vmRuntime         *VMRuntimeCollector
	sink              stream.Sink
	nodeInterval      time.Duration
	infoSyncInterval  time.Duration
	vmInterval        time.Duration
	vmRuntimeInterval time.Duration
	errorBackoff      time.Duration
	agentVersion      string
	capabilities      []string

	mu sync.Mutex

	latestNodeStatic *model.NodeStaticMetrics
	lastNodeHash     uint64
	nodeSynced       bool

	latestVMInfoByID map[string]model.VMInfo
	lastVMInfoByID   map[string]model.VMInfo
	vmInfoSynced     bool
}

func NewScheduler(
	logger *slog.Logger,
	node *NodeCollector,
	vm *VMCollector,
	vmRuntime *VMRuntimeCollector,
	sink stream.Sink,
	nodeInterval, infoSyncInterval, vmInterval, vmRuntimeInterval, errorBackoff time.Duration,
	agentVersion string,
) *Scheduler {
	if errorBackoff <= 0 {
		errorBackoff = time.Second
	}
	if infoSyncInterval <= 0 {
		infoSyncInterval = 10 * time.Minute
	}
	if vmRuntimeInterval <= 0 {
		vmRuntimeInterval = 5 * time.Second
	}
	return &Scheduler{
		logger:            logger,
		node:              node,
		vm:                vm,
		vmRuntime:         vmRuntime,
		sink:              sink,
		nodeInterval:      nodeInterval,
		infoSyncInterval:  infoSyncInterval,
		vmInterval:        vmInterval,
		vmRuntimeInterval: vmRuntimeInterval,
		errorBackoff:      errorBackoff,
		agentVersion:      agentVersion,
		capabilities: []string{
			"node_metrics_stream",
			"vm_metrics_stream",
			"vm_runtime_metrics_stream",
			"node_info_sync",
			"vm_info_sync",
		},
		latestVMInfoByID: map[string]model.VMInfo{},
		lastVMInfoByID:   map[string]model.VMInfo{},
	}
}

func (s *Scheduler) Run(ctx context.Context) error {
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return s.runNodeLoop(gctx)
	})
	g.Go(func() error {
		return s.runVMLoop(gctx)
	})
	g.Go(func() error {
		return s.runVMRuntimeLoop(gctx)
	})
	g.Go(func() error {
		return s.runInfoPeriodicLoop(gctx)
	})
	return g.Wait()
}

func (s *Scheduler) runNodeLoop(ctx context.Context) error {
	ticker := time.NewTicker(s.nodeInterval)
	defer ticker.Stop()

	if err := s.collectAndSendNode(ctx); err != nil {
		s.logger.Warn("initial node collect failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.collectAndSendNode(ctx); err != nil {
				s.logger.Error("node collect/send failed", "error", err)
				s.sleepWithContext(ctx, s.errorBackoff)
			}
		}
	}
}

func (s *Scheduler) runVMLoop(ctx context.Context) error {
	ticker := time.NewTicker(s.vmInterval)
	defer ticker.Stop()

	if err := s.collectAndSendVM(ctx); err != nil {
		s.logger.Warn("initial vm collect failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.collectAndSendVM(ctx); err != nil {
				s.logger.Error("vm collect/send failed", "error", err)
				s.sleepWithContext(ctx, s.errorBackoff)
			}
		}
	}
}

func (s *Scheduler) runVMRuntimeLoop(ctx context.Context) error {
	if s.vmRuntime == nil {
		return nil
	}
	ticker := time.NewTicker(s.vmRuntimeInterval)
	defer ticker.Stop()

	if err := s.collectAndSendVMRuntime(ctx); err != nil {
		s.logger.Warn("initial vm runtime collect failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.collectAndSendVMRuntime(ctx); err != nil {
				s.logger.Error("vm runtime collect/send failed", "error", err)
				s.sleepWithContext(ctx, s.errorBackoff)
			}
		}
	}
}

func (s *Scheduler) runInfoPeriodicLoop(ctx context.Context) error {
	ticker := time.NewTicker(s.infoSyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.syncNodeInfoPeriodic(ctx); err != nil {
				s.logger.Error("node info periodic sync failed", "error", err)
			}
			if err := s.syncVMInfoPeriodic(ctx); err != nil {
				s.logger.Error("vm info periodic sync failed", "error", err)
			}
		}
	}
}

func (s *Scheduler) collectAndSendNode(ctx context.Context) error {
	m, err := s.node.Collect(ctx)
	if err != nil {
		return err
	}
	if err := s.sink.SendNodeMetrics(ctx, m); err != nil {
		return err
	}
	return s.syncNodeInfoFromMetric(ctx, m, "runtime_change")
}

func (s *Scheduler) collectAndSendVM(ctx context.Context) error {
	metrics, err := s.vm.Collect(ctx)
	if err != nil {
		return err
	}
	if len(metrics) == 0 {
		if s.vmRuntime == nil {
			return s.syncVMInfoFromMap(ctx, map[string]model.VMInfo{}, "", time.Now().UTC(), "runtime_change")
		}
		return nil
	}
	if err := s.sink.SendVMMetrics(ctx, metrics); err != nil {
		return err
	}
	if s.vmRuntime == nil {
		return s.syncVMInfoFromVMMetrics(ctx, metrics, "runtime_change")
	}
	return nil
}

func (s *Scheduler) collectAndSendVMRuntime(ctx context.Context) error {
	metrics, err := s.vmRuntime.Collect(ctx)
	if err != nil {
		return err
	}
	if len(metrics) == 0 {
		return s.syncVMInfoFromMap(ctx, map[string]model.VMInfo{}, "", time.Now().UTC(), "runtime_change")
	}
	if err := s.sink.SendVMRuntimeMetrics(ctx, metrics); err != nil {
		return err
	}
	return s.syncVMInfoFromRuntimeMetrics(ctx, metrics, "runtime_change")
}

func (s *Scheduler) syncNodeInfoFromMetric(ctx context.Context, m model.NodeMetrics, reason string) error {
	staticMetric := model.BuildNodeStaticMetrics(m)
	hash := hashNodeStatic(staticMetric)

	s.mu.Lock()
	s.latestNodeStatic = &staticMetric
	syncMode := model.SyncModeDelta
	shouldSync := false
	if !s.nodeSynced {
		syncMode = model.SyncModeFull
		shouldSync = true
		reason = "agent_boot"
	} else if s.lastNodeHash != hash {
		shouldSync = true
	}
	s.mu.Unlock()

	if !shouldSync {
		return nil
	}

	info := model.NodeInfoSync{
		NodeID:       staticMetric.NodeID,
		Timestamp:    staticMetric.Timestamp,
		SyncMode:     syncMode,
		Reason:       reason,
		Info:         staticMetric,
		AgentVersion: s.agentVersion,
		Capabilities: append([]string(nil), s.capabilities...),
	}
	if err := s.sink.SendNodeInfoSync(ctx, info); err != nil {
		return err
	}

	s.mu.Lock()
	s.nodeSynced = true
	s.lastNodeHash = hash
	s.mu.Unlock()
	return nil
}

func (s *Scheduler) syncVMInfoFromVMMetrics(ctx context.Context, metrics []model.VMMetrics, reason string) error {
	currentMap := model.BuildVMInfoMap(metrics)
	ts := time.Now().UTC()
	nodeID := ""
	if len(metrics) > 0 {
		ts = metrics[0].Timestamp
		nodeID = metrics[0].NodeID
	}
	return s.syncVMInfoFromMap(ctx, currentMap, nodeID, ts, reason)
}

func (s *Scheduler) syncVMInfoFromRuntimeMetrics(ctx context.Context, metrics []model.VMRuntimeMetrics, reason string) error {
	currentMap := model.BuildVMInfoMapFromRuntime(metrics)
	ts := time.Now().UTC()
	nodeID := ""
	if len(metrics) > 0 {
		ts = metrics[0].TS
		nodeID = metrics[0].NodeID
	}
	return s.syncVMInfoFromMap(ctx, currentMap, nodeID, ts, reason)
}

func (s *Scheduler) syncVMInfoFromMap(ctx context.Context, currentMap map[string]model.VMInfo, nodeID string, ts time.Time, reason string) error {
	if nodeID == "" {
		nodeID = s.resolveVMNodeID(currentMap)
	}
	upserts, removed, syncMode, shouldSync := s.computeVMInfoDiff(currentMap)
	if !shouldSync {
		return nil
	}

	info := model.VMInfoSync{
		NodeID:       nodeID,
		Timestamp:    ts,
		SyncMode:     syncMode,
		Reason:       reason,
		Upserts:      upserts,
		RemovedVMIDs: removed,
	}
	if syncMode == model.SyncModeFull {
		info.Reason = "agent_boot"
	}

	if err := s.sink.SendVMInfoSync(ctx, info); err != nil {
		return err
	}

	s.mu.Lock()
	s.vmInfoSynced = true
	s.lastVMInfoByID = cloneVMInfoMap(currentMap)
	s.latestVMInfoByID = cloneVMInfoMap(currentMap)
	s.mu.Unlock()
	return nil
}

func (s *Scheduler) syncNodeInfoPeriodic(ctx context.Context) error {
	s.mu.Lock()
	if s.latestNodeStatic == nil {
		s.mu.Unlock()
		return nil
	}
	staticMetric := *s.latestNodeStatic
	s.mu.Unlock()

	info := model.NodeInfoSync{
		NodeID:       staticMetric.NodeID,
		Timestamp:    time.Now().UTC(),
		SyncMode:     model.SyncModeFull,
		Reason:       "periodic_sync",
		Info:         staticMetric,
		AgentVersion: s.agentVersion,
		Capabilities: append([]string(nil), s.capabilities...),
	}
	if err := s.sink.SendNodeInfoSync(ctx, info); err != nil {
		return err
	}

	s.mu.Lock()
	s.nodeSynced = true
	s.lastNodeHash = hashNodeStatic(staticMetric)
	s.mu.Unlock()
	return nil
}

func (s *Scheduler) syncVMInfoPeriodic(ctx context.Context) error {
	s.mu.Lock()
	if len(s.latestVMInfoByID) == 0 {
		s.mu.Unlock()
		return nil
	}
	full := model.SortedVMInfosByID(s.latestVMInfoByID)
	nodeID := ""
	if len(full) > 0 {
		nodeID = full[0].NodeID
	}
	s.mu.Unlock()

	info := model.VMInfoSync{
		NodeID:       nodeID,
		Timestamp:    time.Now().UTC(),
		SyncMode:     model.SyncModeFull,
		Reason:       "periodic_sync",
		Upserts:      full,
		RemovedVMIDs: []string{},
	}
	if err := s.sink.SendVMInfoSync(ctx, info); err != nil {
		return err
	}

	s.mu.Lock()
	s.vmInfoSynced = true
	if len(s.latestVMInfoByID) > 0 {
		s.lastVMInfoByID = cloneVMInfoMap(s.latestVMInfoByID)
	}
	s.mu.Unlock()
	return nil
}

func (s *Scheduler) computeVMInfoDiff(current map[string]model.VMInfo) ([]model.VMInfo, []string, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.latestVMInfoByID = cloneVMInfoMap(current)

	if !s.vmInfoSynced {
		return model.SortedVMInfosByID(current), []string{}, model.SyncModeFull, true
	}

	upserts := make([]model.VMInfo, 0)
	for vmID, info := range current {
		prev, exists := s.lastVMInfoByID[vmID]
		if !exists || prev != info {
			upserts = append(upserts, info)
		}
	}
	sort.Slice(upserts, func(i, j int) bool {
		return upserts[i].VMID < upserts[j].VMID
	})

	removed := make([]string, 0)
	for vmID := range s.lastVMInfoByID {
		if _, exists := current[vmID]; !exists {
			removed = append(removed, vmID)
		}
	}
	sort.Strings(removed)

	if len(upserts) == 0 && len(removed) == 0 {
		return nil, nil, "", false
	}
	return upserts, removed, model.SyncModeDelta, true
}

func (s *Scheduler) sleepWithContext(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func hashNodeStatic(m model.NodeStaticMetrics) uint64 {
	copyMetric := m
	copyMetric.Timestamp = time.Time{}
	data, _ := json.Marshal(copyMetric)
	h := fnv.New64a()
	_, _ = h.Write(data)
	return h.Sum64()
}

func cloneVMInfoMap(in map[string]model.VMInfo) map[string]model.VMInfo {
	out := make(map[string]model.VMInfo, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (s *Scheduler) resolveVMNodeID(current map[string]model.VMInfo) string {
	for _, info := range current {
		if info.NodeID != "" {
			return info.NodeID
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, info := range s.latestVMInfoByID {
		if info.NodeID != "" {
			return info.NodeID
		}
	}
	for _, info := range s.lastVMInfoByID {
		if info.NodeID != "" {
			return info.NodeID
		}
	}
	return ""
}

func AsNodeEnvelope(m model.NodeMetrics) model.Envelope {
	return model.Envelope{Type: model.MetricTypeNode, NodeID: m.NodeID, Timestamp: m.Timestamp, Payload: m}
}

func AsVMEnvelope(nodeID string, payload []model.VMMetrics, at time.Time) model.Envelope {
	return model.Envelope{Type: model.MetricTypeVM, NodeID: nodeID, Timestamp: at, Payload: payload}
}
