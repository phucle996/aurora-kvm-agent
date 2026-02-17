package collector

import (
	"context"
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

	latestVMInfoByID map[string]model.VMInfo
	lastVMInfoByID   map[string]model.VMInfo
	vmInfoSynced     bool
}

// NewScheduler wires collectors, sink, and runtime intervals for all metric loops.
func NewScheduler(
	logger *slog.Logger,
	node *NodeCollector,
	vm *VMCollector,
	vmRuntime *VMRuntimeCollector,
	sink stream.Sink,
	nodeInterval, infoSyncInterval, vmInterval, vmRuntimeInterval, errorBackoff time.Duration,
	agentVersion string,
) *Scheduler {
	if nodeInterval <= 0 {
		nodeInterval = 1 * time.Second
	}
	if vmInterval <= 0 {
		vmInterval = 1 * time.Second
	}
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
			"vm_info_sync",
		},
		latestVMInfoByID: map[string]model.VMInfo{},
		lastVMInfoByID:   map[string]model.VMInfo{},
	}
}

// Run starts all periodic loops and stops when context is cancelled or any loop fails.
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

// runNodeLoop periodically collects node metrics and pushes them to sink.
func (s *Scheduler) runNodeLoop(ctx context.Context) error {
	return s.runTickerLoop(ctx, "node", s.nodeInterval, func(runCtx context.Context) error {
		return s.collectAndSendNode(runCtx)
	})
}

// runVMLoop periodically collects VM lite metrics and pushes them to sink.
func (s *Scheduler) runVMLoop(ctx context.Context) error {
	return s.runTickerLoop(ctx, "vm", s.vmInterval, func(runCtx context.Context) error {
		return s.collectAndSendVM(runCtx)
	})
}

// runVMRuntimeLoop periodically collects VM runtime detail metrics when runtime collector is enabled.
func (s *Scheduler) runVMRuntimeLoop(ctx context.Context) error {
	if s.vmRuntime == nil {
		return nil
	}
	return s.runTickerLoop(ctx, "vm runtime", s.vmRuntimeInterval, func(runCtx context.Context) error {
		return s.collectAndSendVMRuntime(runCtx)
	})
}

// runInfoPeriodicLoop periodically sends full VM info sync snapshots.
func (s *Scheduler) runInfoPeriodicLoop(ctx context.Context) error {
	ticker := time.NewTicker(s.infoSyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.syncVMInfoPeriodic(ctx); err != nil {
				s.logger.Error("vm info periodic sync failed", "error", err)
			}
		}
	}
}

// runTickerLoop executes one immediate run, then keeps running by interval with backoff on errors.
func (s *Scheduler) runTickerLoop(ctx context.Context, name string, interval time.Duration, fn func(context.Context) error) error {
	if err := fn(ctx); err != nil {
		s.logger.Warn("initial collect failed", "loop", name, "error", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := fn(ctx); err != nil {
				s.logger.Error("collect/send failed", "loop", name, "error", err)
				s.sleepWithContext(ctx, s.errorBackoff)
			}
		}
	}
}

// collectAndSendNode executes one node metrics cycle.
func (s *Scheduler) collectAndSendNode(ctx context.Context) error {
	m, err := s.node.Collect(ctx)
	if err != nil {
		return err
	}
	return s.sink.SendNodeMetrics(ctx, m)
}

// collectAndSendVM executes one VM lite metrics cycle and updates VM info sync when runtime stream is disabled.
func (s *Scheduler) collectAndSendVM(ctx context.Context) error {
	metrics, err := s.vm.Collect(ctx)
	if err != nil {
		return err
	}
	if len(metrics) == 0 {
		if s.vmRuntime == nil {
			return s.syncVMInfoFromMap(ctx, map[string]model.VMInfo{}, "", time.Now().UTC().Unix(), "runtime_change")
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

// collectAndSendVMRuntime executes one VM runtime detail cycle and updates VM info sync.
func (s *Scheduler) collectAndSendVMRuntime(ctx context.Context) error {
	metrics, err := s.vmRuntime.Collect(ctx)
	if err != nil {
		return err
	}
	if len(metrics) == 0 {
		return s.syncVMInfoFromMap(ctx, map[string]model.VMInfo{}, "", time.Now().UTC().Unix(), "runtime_change")
	}
	if err := s.sink.SendVMRuntimeMetrics(ctx, metrics); err != nil {
		return err
	}
	return s.syncVMInfoFromRuntimeMetrics(ctx, metrics, "runtime_change")
}

// syncVMInfoFromVMMetrics converts VM lite metrics into VM info state and sends sync payload.
func (s *Scheduler) syncVMInfoFromVMMetrics(ctx context.Context, metrics []model.VMMetrics, reason string) error {
	currentMap := model.BuildVMInfoMap(metrics)
	tsUnix := time.Now().UTC().Unix()
	nodeID := ""
	if len(metrics) > 0 {
		tsUnix = metrics[0].TimestampUnix
		nodeID = metrics[0].NodeID
	}
	return s.syncVMInfoFromMap(ctx, currentMap, nodeID, tsUnix, reason)
}

// syncVMInfoFromRuntimeMetrics converts VM runtime detail metrics into VM info state and sends sync payload.
func (s *Scheduler) syncVMInfoFromRuntimeMetrics(ctx context.Context, metrics []model.VMRuntimeMetrics, reason string) error {
	currentMap := model.BuildVMInfoMapFromRuntime(metrics)
	tsUnix := time.Now().UTC().Unix()
	nodeID := ""
	if len(metrics) > 0 {
		tsUnix = metrics[0].TimestampUnix
		nodeID = metrics[0].NodeID
	}
	return s.syncVMInfoFromMap(ctx, currentMap, nodeID, tsUnix, reason)
}

// syncVMInfoFromMap computes full/delta VM info payload and sends it through sink.
func (s *Scheduler) syncVMInfoFromMap(ctx context.Context, currentMap map[string]model.VMInfo, nodeID string, tsUnix int64, reason string) error {
	if nodeID == "" {
		nodeID = s.resolveVMNodeID(currentMap)
	}
	upserts, removed, syncMode, shouldSync := s.computeVMInfoDiff(currentMap)
	if !shouldSync {
		return nil
	}

	info := model.VMInfoSync{
		NodeID:        nodeID,
		TimestampUnix: tsUnix,
		SyncMode:      syncMode,
		Reason:        reason,
		Upserts:       upserts,
		RemovedVMIDs:  removed,
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

// syncVMInfoPeriodic sends a full VM info snapshot on periodic timer.
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
		NodeID:        nodeID,
		TimestampUnix: time.Now().UTC().Unix(),
		SyncMode:      model.SyncModeFull,
		Reason:        "periodic_sync",
		Upserts:       full,
		RemovedVMIDs:  []string{},
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
	return model.Envelope{Type: model.MetricTypeNodeLite, NodeID: m.NodeID, TimestampUnix: m.TimestampUnix, Payload: m}
}

func AsVMEnvelope(nodeID string, payload []model.VMMetrics, atUnix int64) model.Envelope {
	if atUnix == 0 {
		atUnix = time.Now().UTC().Unix()
	}
	return model.Envelope{Type: model.MetricTypeVMLite, NodeID: nodeID, TimestampUnix: atUnix, Payload: payload}
}
