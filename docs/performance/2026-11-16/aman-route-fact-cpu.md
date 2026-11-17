# AMAN route-fact CPU follow-up

The eight-statement path still spent most of its time on CPU sequencing work.
A profile restricted to the changed 132-arrival event measured 1,214.99 ms wall
time, with 214.76 ms in SQL calls including result handling. Slot placement
accounted for 870 of 1,130 ms of sampled CPU time (76.99%). Repeated STAR-order
checks dominated both candidate generation/optimization and queue offers.
The final follow-up profile measured 760.95 ms wall time and 350 ms of sampled
placement CPU time, compared with the original 870 ms. These CPU samples and
SQL timings overlap; they are not additive categories.

## Safe change

The placement loop previously evaluated expensive flight ordering before
checking whether the candidate timestamp was on the relevant side of the
reservation. Reversing these pure boolean operands avoids ordering work when
the time comparison already makes the condition false. Equal timestamps still
reach the same checks; all rejection conditions and earlier/later bounds are
unchanged.

The comparison chain also repeatedly copied large `preparedFlight` structs.
Its private, read-only helpers now take pointers without retaining or mutating
them. The placement scan reads each reservation through its slice element
instead of copying the whole allocated entry. No result is cached across
candidates or calculations.

This changes execution cost, not scheduling policy. STAR order, manual and
Stable boundaries, holding priorities, WTC/rate/same-STAR spacing, optimization
trial limits, vacancy handling and queue-offer generation remain active. There
are no changes to authorization, SQL, persistence, publication, locks or the
synchronous scope of reconciliation in this follow-up.

## Measurements

Same worktree and database harness as the SQL investigation, with three serial
runs per workload before and after this CPU change. Setup and verification
reads are excluded; the full changed event, both publications, synchronous
reconciliation and coordination are included.

| Arrivals | Before median | After median | SQL before / after |
| --- | ---: | ---: | ---: |
| 1 | 18.748 ms | 17.220 ms | 8 / 8 |
| 132 | 1,166.295 ms | 764.706 ms | 8 / 8 |
| 2, including removal/expiry/audits | 101.107 ms | 101.849 ms | 8 / 8 |

The 132-arrival median improves by approximately **34.4%**. Its before samples
were 1,276.391, 1,166.295 and 1,064.567 ms; after samples were 764.706,
775.240 and 718.357 ms. The smaller workloads remain dominated by database
round trips. This is a synthetic local workload, not deployed production
latency. Samples and full published-state fingerprints are recorded in
[aman-route-fact-cpu.json](aman-route-fact-cpu.json).

Both complete published airport states have identical SHA256 fingerprints
before and after for all three workload sizes and all repetitions. For the
132-arrival case this is
`f135b2b2c19f77d1b5d7d5fbfb74ca45db7fbc780c8fb1e5cdc0d3e4a94a36d0`.
The existing behavioral assertions also check durable fact/progress identity,
prediction, revision, publication count, coordination correlation, expiry,
audits and session isolation. Fingerprints are comparative evidence, not a
replacement for those behavioral checks.

## Verification and reproduction

The sequence suite passed after each implementation step. All AMAN, event,
application assembly and PostgreSQL regression packages passed. The first broad
run could not initialize Docker for the coordination package; rerunning that
package succeeded without code changes. Race checks passed for sequence,
operational, route-fact and PostgreSQL packages.

```powershell
go test ./internal/aman/... ./internal/coordinationrequest ./internal/repository/postgres ./internal/app ./pkg/events/... -count=1
go test -race ./internal/aman/sequence ./internal/aman/operational ./internal/aman/routefact ./internal/repository/postgres -count=1
go test ./internal/aman/operational -run '^TestRouteFactFullEventSQLBudget' -count=3 -v
$env:FLIGHTSTRIPS_ROUTEFACT_CPU_PROFILE = Join-Path $env:TEMP 'aman-routefact.cpu'
go test ./internal/aman/operational -run '^TestRouteFactFullEventSQLBudget/132_arrivals$' -count=1 -v
go tool pprof -top -cum $env:FLIGHTSTRIPS_ROUTEFACT_CPU_PROFILE
```

The profiling switch is test-only and records only the changed-event call,
not fixture setup, repeat callbacks or clears. Profile shutdown occurs after
the event timer stops. SQL timing includes pgx result handling and does not
replace existing production telemetry.

All changes remain in `C:/Users/fsr19/.codex/worktrees/aman-route-fact-sql` on
`codex/aman-route-fact-sql`. Production was not changed during profiling or validation.
