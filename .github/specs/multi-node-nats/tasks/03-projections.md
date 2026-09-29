# Task 03 — Replay, projections, snapshots and readiness

**Depends on:** 02. **Outcome:** every backend can reconstruct and serve the same committed state.

## Work

- Give each backend an independent `FS_STATE` consumer, immutable in-memory aggregate projections, reducer-maintained indexes, and a barrier that waits for a committed stream sequence. Buffer live events while forming an initial snapshot, then deliver later deltas without a gap.
- Write a snapshot after five minutes or 10,000 events, whichever comes first. Verify the immutable Object Store object before changing `FS_SNAPSHOT_INDEX`; retain two verified snapshots and the full event log. Replay from the proper checkpoint and fall back to older snapshot or full log on corruption.
- Expose projection health to `/readyz`: quorum/resource access, current high-water check at most two seconds old, and replay caught up. Unknown event versions fail readiness.
- Keep NATS projection startup and its readiness dependency in the isolated NATS test runtime until task 20. Current production mode must still start without NATS and retain its existing routing behavior.

## Done when

- Two independent processes replay identical projections from different checkpoints. Restart and corrupt-newest-snapshot tests recover without partial reads.
- A local write does not report success before its projection applies the PubAck sequence. A stalled consumer or lost NATS quorum makes `/readyz` return 503.
- No SQL query is replaced by an ad hoc cross-node read.

**Starting points:** `backend/internal/app/app.go`, `backend/internal/frontend/hub.go`, and the cluster code from task 02.
