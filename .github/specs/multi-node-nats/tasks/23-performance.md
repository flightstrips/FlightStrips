# Task 23 — Position load and capacity gate

**Depends on:** 22. **Outcome:** repeatable local load checks and measurements on the user's machine, without building release candidates or deploying staging/production.

**Dependency status:** Task 22 merged as [PR #829](https://github.com/flightstrips/FlightStrips/pull/829), with passing CI and a complete recorded native fault/restore run. Use [the final memory/fault qualification](22-memory-evidence.md), including the bounded disposable history index and latest source, rather than claiming qualification from earlier pre-fix captures. Infrastructure PR #29 stays open; its failed key-rotation check is handled by 21a and does not prevent local load work, but remains a Task 24 gate. Changes to broker versions/configuration or runtime behavior require affected checks to rerun.

**Isolation:** build this task's source; do not benchmark the user's running default containers, which may contain an older executable. Use separately named disposable fixtures, verified ownership, isolated volumes/cache/key paths and non-conflicting ports. Preserve the user's development data/processes and confidential captures. Reuse Task 22's fixture/restore commands where applicable.

**Release boundary:** harness code lands on the held integration branch; tests run against local builds, using the Task 20c setup and Task 22 configuration. Later behavior/configuration/harness changes require affected checks to rerun. Record the operator's own results only when supplied.

## Work

- Adapt `backend/internal/testing/positionload` to NATS/two backends without weakening traffic or assertions. Run 15-minute mixed, arrivals-heavy and departures-heavy 100-report/second mixes, each with ten-second 200-report/second overload; exercise even and once-per-second burst patterns, five operational EuroScope messages and one frontend action per second.
- Enforce `docs/position-performance.md`: receipt-to-completion p95 ≤20 ms, p99 ≤50 ms, exact sent/completed counts, zero unexpected errors, stable backlog and overload drain within two seconds. Keep sender-deadline checks enabled. Record CPU, memory, NATS disk/network placement, PubAck latency and projection lag. Under steady load, backend-kill recovery targets p95 ≤15 seconds including client reconnect.
- Verify off-cluster backup/restore rehearsal, disk alerts at 70%, and release block at 85%. Produce the evidence bundle consumed by task 24; do not alter the production stack here.
- Use the same local source revision, build commands, plugin and configuration as Task 22, or rerun affected fault cases. Record machine limits; one-machine measurements do not certify production multi-host capacity.

## Done when

- All requested load patterns and Task 22 fault checks have recorded local pass/fail results. A completed run with failed latency assertions is recorded as a failure, not a pass. Repair/retest failed checks before acceptance unless the operator explicitly changes that requirement.
- Local test record contains commands, source revision, local build configuration, machine resources, benchmark measurements, fault timeline, backup manifest and reconnection times. No published images, release PR artifacts or staging deployment are needed.
- Capacity results include a definite pass/fail judgment for each traffic mix and pattern. Failed gates stop task 24 until repaired and rerun.

**Starting points:** `docs/position-performance.md`, `backend/internal/testing/positionload`, and Task 20c's local build/test procedure.
