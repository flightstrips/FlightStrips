# Task 23 — Position load and capacity gate

**Depends on:** 22. **Outcome:** measured capacity and release evidence, without deploying production.

**Release boundary:** test harness code that requires held implementation lands in the coordinated candidate PR before qualification; tests run against its Release Please PR artifacts in staging. A later harness fix invalidates affected gates.

## Work

- Adapt `backend/internal/testing/positionload` to NATS/two backends without weakening traffic or assertions. Run 15-minute mixed, arrivals-heavy and departures-heavy 100-report/second mixes, each with ten-second 200-report/second overload; exercise even and once-per-second burst patterns, five operational EuroScope messages and one frontend action per second.
- Enforce `docs/position-performance.md`: receipt-to-completion p95 ≤20 ms, p99 ≤50 ms, exact sent/completed counts, zero unexpected errors, stable backlog and overload drain within two seconds. Keep sender-deadline checks enabled. Record CPU, memory, NATS disk/network placement, PubAck latency and projection lag. Under steady load, backend-kill recovery targets p95 ≤15 seconds including client reconnect.
- Verify off-cluster backup/restore rehearsal, disk alerts at 70%, and release block at 85%. Produce the evidence bundle consumed by task 24; do not alter the production stack here.
- Use the same candidate digests, plugin hash and staging stack revision as task 22, or rerun affected fault cases before declaring the gate passed.

## Done when

- All load and task 22 fault gates pass on production-equivalent placement. A completed run with failed latency assertions is recorded as a failure, not a pass.
- Release record contains commands, revisions, image tags, hardware placement, benchmark measurements, fault timeline, backup manifest and reconnection times.
- Capacity results include a definite pass/fail judgment for each traffic mix and pattern. Failed gates stop task 24 until repaired and rerun.

**Starting points:** `docs/position-performance.md`, `backend/internal/testing/positionload`, and the infrastructure deployment procedure.
