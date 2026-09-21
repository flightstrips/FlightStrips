# Event update fixes — 16 September 2026

> This is a historical measurement record. Database batching was promoted to
> the default on 21 September 2026; the current rollout contract is documented
> in [the position performance runbook](../../position-performance.md).

The changes following [the audit](event-update-audit.md) keep synchronous PostgreSQL persistence and one position execution worker. No dashboard or production configuration was changed.

## Delivered changes

| Area | Change | Boundaries retained |
| --- | --- | --- |
| Full strip publication | One typed SQL snapshot reads the strip, session, controllers, sector owners, pending coordination and applicable stand assignments | Fresh snapshot for every publication; full frontend/CLX payload; pending disconnect suppression; no cross-message cache |
| Stand blocking | A single-strip publication compares that strip against other assignments, instead of evaluating all pairs | Same conflict rules, expected departure exception and blocker order |
| Operational validation | Read reuse lasts for one validation pass where no audited message state already exists | Later mutations read fresh state; existing transaction locks/revalidation remain |
| Squawk changes | Capture old code under the same row lock as the write; validate target and old/new peers | Full-session membership is retained so a peer's second code can still conflict outside the affected group |
| AMAN holds | Narrow read of relevant holding facts skips missing, unchanged or stale observations | Actual changes still reload/validate/commit the complete aggregate with bounded revision retries |
| AMAN route facts | Narrow active identity lookup for speed reports and missing-flight checks | Direct-to mutation still uses full aggregate revision checks, authority revalidation and reconciliation |
| PDC issuance | Save message state, timestamp, sequence, issuer and web clearance in one read/write pair | Delivery and successful persistence still precede successful completion |
| Attribution | Physical query counts for every websocket event; preserve ground-state context through bay moves; PDC preparation/delivery/persistence/move/publication spans | Does not enable a mutable message cache for every handler; no dashboard changes |
| Replay | Operational event query counts and processing P95/P99 | Sender-deadline position completion remains the acceptance metric |

### Implementation choices

The publication fix uses a combined fresh read after the field write instead of UPDATE RETURNING followed by separate enrichment queries. This removes the enrichment round trips too and avoids introducing version/ownership caching in the field handlers.

Publication inputs use a private context value that exists only while building that publication. It does not seed the caller's mutable message state. The combined read supports transaction-bound repositories and preserves controller/assignment ordering.

Validation reuse is deliberately narrower than caching an entire frontend move or coordination handler: several operational mutations still have independent reload requirements. Remaining reads must be profiled and removed with explicit invalidation, not by enabling the message cache globally.

Squawk validation still reads session strips once to calculate accurate membership; this change reduces validation work, not the entire session read's data volume. It captures the old code without adding a separate read.

A real changed hold/direct-to can still cost more than a few milliseconds. Narrow reads remove unnecessary aggregate decoding for the no-change/missing/identity-only paths; they do not eliminate the required aggregate commit.

## Verification

- Full PostgreSQL-backed Go suite passed after the publication, AMAN and validation changes.
- PDC PostgreSQL suite passed after the combined issuance write.
- Race-enabled PostgreSQL tests passed for repository/postgres, AMAN holdingclearance/routefact, services, frontend, server, shared, websocket and PDC.
- The final publication ordering/linear blocking follow-up passed targeted PostgreSQL-backed race tests.
- Focused coverage checks publication snapshot parity/one SQL statement/session isolation/transaction-local changes, pending coordination display, frontend assignment payload, old/new squawk peers and outside duplicate membership, fresh subsequent validation passes, missing/landed AMAN identities, unchanged/stale holding facts, changed holding aggregate commits, speed correlation and atomic PDC metadata assembly.

### Stand-blocking microbenchmark

Windows amd64, Go 1.25.7, AMD Ryzen 7 7700X, 200 synthetic assignments, no database:

| Algorithm | Time per publication calculation |
| --- | ---: |
| Previous all-pairs calculation | 756,055 ns (0.756 ms) |
| Single-strip calculation | 5,625 ns (0.0056 ms) |

This isolated benchmark is approximately 134x faster. It is not an end-to-end latency or production capacity claim. Run with:

```powershell
go test ./internal/frontend -run '^$' -bench BenchmarkSingleStandPublication -benchmem -count=1
```

## Local replay conditions and limitations

One execution worker, batching enabled, local Docker PostgreSQL 16.11, pool 16, Windows loopback, 200 aircraft alternating at 100 reports/sec in once-per-second bursts; five interleaved heading messages/sec plus frontend marks and ground-state lifecycle actions. Ten-second warm-up, two-minute measurement, then 200/sec for ten seconds and recovery: 15,200 reports in each run.

These are not production-equivalent CPU/storage/network conditions. No PC CPU utilization or memory usage was sampled. The overload segment still sends positions only. This is not the required 15-minute/multiple-fleet-mix acceptance run.

- [publication-snapshot-1-worker.json](publication-snapshot-1-worker.json) is a diagnostic intermediate run; race/unit test activity overlapped it. Do not use it as an isolated timing comparison.
- [event-updates-1-worker.json](event-updates-1-worker.json) measures commit 3c5c16c5 plus replay instrumentation before the linear stand-blocking follow-up. No parallel test run overlapped this measurement.
- Final replay results are recorded below after the linear stand-blocking follow-up.

The implementation adds improvements in each identified area; it does not certify that every registered event now finishes within a few milliseconds. Rare events and real operational transitions still require dedicated latency scenarios.

## Final clean replay

[event-updates-linear-1-worker.json](event-updates-linear-1-worker.json) measures cc3dcb13, including linear stand blocking. No parallel unit/race/benchmark run overlapped this measurement.

| Event | Samples | Mean processing | P95 processing | P99 processing | Mean SQL statements |
| --- | ---: | ---: | ---: | ---: | ---: |
| heading | 600 | 2.782 ms | 4.178 ms | 5.217 ms | 2 |
| marked | 120 | 0.845 ms | 1.252 ms | 1.621 ms | 1 |
| ground_state (real lifecycle transitions) | 4 | 24.088 ms | 41.137 ms | 41.137 ms | 43.75 |

Four ground-state samples are insufficient for a stable tail-latency estimate. The high statement count identifies remaining transition work; these are not ordinary unchanged ground-state reports.

Positions: **15,200 sent / 15,200 completed, zero position or operational errors**, lifecycle assertions passed. Sender-deadline completion **P95 36.1046 ms / P99 60.2346 ms**, 0.27983 SQL operations per measured position report. Maximum steady outstanding reports: 100 (the burst size). Backlog was drained at the post-recovery checkpoint (0.5122 ms additional wait); that checkpoint value is not total burst processing time.

For comparison, the earlier AMAN-batch interleaved replay had heading mean 3.376 ms and ground-state mean 26.047 ms; position sender completion was 37.453 / 63.364 ms. These are separate local runs, not a controlled production A/B test.

**Acceptance gate still fails.** Neither the 20 ms P95 nor 50 ms P99 sender-completion target is met. Keep the position batching prototype disabled by default and the PR in draft. No longer or production-equivalent acceptance claim is made.

### Remaining limiting work

- Real ground-state transitions still execute many statements across bay ordering, CDM, validation and stand release/publication. The new physical counts expose work previously missing from event accounting. Further consolidation needs a transition-level trace and targeted invalidation tests.
- The interleaved one-second burst still incurs operational barriers and multiple position transaction groups. Improving heading publication alone does not remove that completion delay.
- Changed AMAN facts still use whole-airport aggregate commits/reconciliation. The narrow-read improvement deliberately only bypasses that work where it is unnecessary.
- Squawk membership still loads the full session once; narrowing that read requires preserving secondary-code membership and validation precedence.
- PDC delivery time requires deployed stage traces; its previously unattributed interval must not be presented as solved by the reduced persistence statements.

Publication/validation/AMAN/PDC database improvements are active code changes; only experimental position batching remains behind POSITION_DB_BATCHING_ENABLED. No production enablement or deployment was performed.

