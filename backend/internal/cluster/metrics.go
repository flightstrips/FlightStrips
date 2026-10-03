package cluster

import (
	"fmt"
	"io"
	"runtime"
	"sort"
	"strings"
	"time"
)

// WriteMetrics reports bounded labels from verified resources and accepted
// state. Readiness-age lag also exposes a stalled metadata/replay loop.
func (p *Projection) WriteMetrics(out io.Writer, node string) {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	fmt.Fprintf(out, "fs_go_heap_alloc_bytes %d\nfs_go_heap_sys_bytes %d\nfs_go_heap_idle_bytes %d\nfs_go_heap_released_bytes %d\n", memory.HeapAlloc, memory.HeapSys, memory.HeapIdle, memory.HeapReleased)
	p.mu.RLock()
	defer p.mu.RUnlock()
	behind := uint64(0)
	if p.highWater > p.applied {
		behind = p.highWater - p.applied
	}
	lag := time.Since(p.checked).Seconds()
	if p.checked.IsZero() {
		lag = time.Since(p.startedAt).Seconds()
	}
	if behind > 0 && !p.lastAppliedServerTime.IsZero() {
		if replayLag := p.highWaterServerTime.Sub(p.lastAppliedServerTime).Seconds(); replayLag > lag {
			lag = replayLag
		}
	}
	if lag < 0 {
		lag = 0
	}
	fmt.Fprintf(out, "fs_projection_lag_seconds{node=%q} %g\nfs_projection_behind_messages{node=%q} %d\n", node, lag, node, behind)
	fmt.Fprintf(out, "fs_owner_takeovers_total %d\nfs_stale_epoch_rejections_total %d\nfs_snapshot_verify_failures_total %d\n", p.takeovers.Load(), p.staleEpochs.Load(), p.snapshotFailures.Load())
	fmt.Fprintf(out, "fs_snapshot_size_skips_total %d\nfs_snapshot_index_contentions_total %d\n", p.snapshotSizeSkips.Load(), p.snapshotIndexContentions.Load())
	fmt.Fprintln(out, "fs_history_cache_bytes 0")
	hot := map[string]int{"outcome": 0, "workflow": 0, "effect": 0}
	leases := map[string]float64{"global": 0, "airport": 0, "session": 0}
	seen := map[string]bool{}
	effects := map[string]int{}
	for _, state := range p.states {
		hot["outcome"] += len(state.Ledger)
		hot["workflow"] += len(state.Workflows)
		hot["effect"] += len(state.Effects)
		kind := "session"
		if state.Ref.GetGlobal() != nil {
			kind = "global"
		} else if state.Ref.GetAirport() != nil {
			kind = "airport"
		}
		if state.Owner.GetNodeId() == node {
			remaining := time.Until(state.Owner.LeaseUntil.AsTime()).Seconds()
			if remaining < 0 {
				remaining = 0
			}
			if !seen[kind] || remaining < leases[kind] {
				leases[kind] = remaining
				seen[kind] = true
			}
		}
		for _, effect := range state.Effects {
			field := effect.ProtoReflect().WhichOneof(effect.ProtoReflect().Descriptor().Oneofs().ByName("payload"))
			if field != nil {
				effects[effect.Status.String()+"/"+string(field.Name())]++
			}
		}
	}
	for _, kind := range []string{"global", "airport", "session"} {
		fmt.Fprintf(out, "fs_owner_lease_remaining_seconds{aggregate_kind=%q} %g\n", kind, leases[kind])
	}
	for _, kind := range []string{"outcome", "workflow", "effect"} {
		fmt.Fprintf(out, "fs_projection_hot_history_records{kind=%q} %d\n", kind, hot[kind])
	}
	keys := make([]string, 0, len(effects))
	for key := range effects {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts := strings.SplitN(key, "/", 2)
		fmt.Fprintf(out, "fs_effects{status=%q,type=%q} %d\n", parts[0], parts[1], effects[key])
	}
}
