# Task 18b CDM and traffic acceptance

Candidate branch: `codex/multi-node-nats-18b-cdm-traffic`, based on
`origin/codex/multi-node-nats-base` at `0e67b37f` (including the requested
`f5fa8bce`, Task 19c transceiver and merged Tasks 18a/18c). This is dormant candidate
work under [Task 18's release boundary](18-session-workers.md). App startup,
SQL runtime and ALB transport are unchanged. Task 20 binds the combined
candidates; release remains held until Task 24.

## Concrete binding checklist

| Operational path | SQL-free candidate | Acceptance evidence |
| --- | --- | --- |
| Initial/local/vIFF session sync and sequence | `NewCdmCandidate(...).CDM`, accepted Task 19 configuration/flights, actual `cdm.PlanSequence` | LocalSequenceDebounceAndTakeover, ViffPolicyAndUncertainActualTime |
| Periodic validation, READY, deice | Same callback and `Planner` | ReadyDeiceAndCtotValidation; activation acknowledgement survives periodic work |
| Browser READY, TOBT/deice/manual CTOT/removal | `NewCdmActionService`, `CdmCandidate.Planner`; browser uses CDM revision | OperationalActions, frontend command/projection tests |
| CLX TOBT, EOBT/logon, clearance TOBT, ASRT/TSAC | Same owner planner, including existing validation/update_tobt and combined strip/update_data EOBT commands | Production helper tests, OperationalActions, ReadyDeiceAndCtotValidation browser EOBT case; typed planner preserves original request UUID/actor and accompanying strip edits |
| Ground-state ASAT and transfer/takeoff actuals | Same planner; persisted AOBT/ATOT intent parameters | OperationalActions, ViffPolicyAndUncertainActualTime, ProviderCommitBoundaries |
| Better TOBT and direct TOBT push | Persisted DPI/SET_TOBT intents using Task 19 | OperationalActions; no export before pending local recalculation commits |
| Pushback assignment and vIFF verification | `prepare_pushback`, `PushbackResult`, persisted typed attempts/matches and callsign probe deadlines | PushbackVerificationTakeover; first match and next probe survive owner death |
| Session update/disconnect recalculation | `CdmCandidate.Recalculate` accepts/rearms a durable debounce from current aggregate revision | LocalSequenceDebounceAndTakeover, RestartPendingDebounce |
| Airport master registration/clearing | `CdmCandidate.ReconcileMaster` with accepted global registry and Task 19 master page | MasterRegistration; repeated observations yield one SET and one CLEAR |
| HTTP sequence presentation | `NewCdmCandidateWebAPI(auth, candidate)` and shared persisted-row presentation | CandidateSequenceHTTPUsesPersistedPolicyAndJSON; authorization, JSON fields and unready failure |
| Traffic counts and gauge publication | `NewTrafficCandidate(writer).Traffic`, existing bay/window classifier and metric labels | TrafficCandidateTwoReplicaNATS; boundary/midnight/future clocks, no domain writes, nonowner and closed owner stop |

No candidate constructor builds a SQL repository, legacy service, hub or local
outcome timer. Sequencing uses the extracted production policy with buffered
results; a single owner event commits all affected Strip/CDM replacements and
deadline changes. Complete aggregate, configuration, vIFF page and accepted
position inputs are checked again before CAS. Controller-owned fields and
unrelated validation remain intact. Overdue slots use current accepted-owner
time for policy while retaining the persisted polling/command identity.

Typed export intents precede all operational provider writes. READY exports
REA before its state and REQTOBT is acknowledged only after authoritative
state succeeds. Unchanged policy generations retain export identities;
failed/uncertain predecessors prevent successor dispatch. Task 19 records
CALL_UNCERTAIN when the provider result cannot be proved. ATOT is never
resent by retry, takeover or a repeated takeoff-clearance action.

## Verification

Tests use two independent projections/owner runtimes and the actual callbacks
against the pinned `nats:2.15.0` three-server fixture. Initial cases reused
ports 4222–4224; final acceptance after the base rebase uses an independent
`fs18b-cdm` Compose project on ports 7422–7424. Only provider
transport responses, clocks and crash boundaries are controlled by fixtures;
no sequence or CDM calculation is replaced. Existing fixture containers are
left untouched without resetting their data. The independent fixture uses
`backend/docker-compose.nats.yaml` with host ports remapped; no server, account
or stream configuration is changed. Sessions and CDM airport identities are
unique to each test.

PowerShell commands from the repository/backend directories:

```powershell
$env:NATS_INTEGRATION='1'
$env:NATS_TEST_PORT_BASE='7422'
go test ./internal/services -run 'Test(CdmCandidateTwoReplica|TrafficCandidate)' -v -count=1 -timeout=20m
$env:DOCKER_HOST='npipe:////./pipe/dockerDesktopLinuxEngine'
go test ./...
```

```powershell
python scripts/cluster_proto.py --check
```

From frontend:

```powershell
npm test -- src/api/projection.test.ts src/api/commands.test.ts
npx tsc -b --pretty false
```

The acceptance suite covers initial sync, independent sequence slots,
duplicate action UUIDs, owner loss around atomic multi-strip commit/PubAck,
configuration/controller changes during planning, pending debounce snapshot
restart, READY/deice ordering, CTOT acknowledgement, master reconciliation,
and owner loss around provider intent and result commits. The exact provider
matrix is: before intent -> one takeover call; after intent/before call ->
CALL_UNCERTAIN and zero calls; before result -> one call and uncertainty;
after result -> one call and proven completion. Repeated callbacks preserve
those outcomes.

Bindings remain uninstalled until Task 20: wrap the complete domain planner,
bind CDM/Traffic and recalculation callbacks, register candidate HTTP routes,
and schedule airport master reconciliation alongside Task 19's accepted master
observations. These are wiring steps, not SQL policy adapters. C++ bindings
are regenerated and contract-checked; a full EuroScope plugin build is not
claimed by this local acceptance run. CI and the combined 18a/18b/18c runtime
acceptance remain required before cutover.

Local checks passed: all backend Go packages (Docker Desktop endpoint supplied
for existing PostgreSQL tests); 81 browser command/projection tests;
TypeScript project compilation; 296 binary oneof and 288 optional-zero
cross-language contract fixtures. The first broad Go invocation used the
Windows default Docker discovery and failed with “rootless Docker is not
supported”; the configured Docker Desktop endpoint resolves that host setup
failure. Full EuroScope C++ compilation and live vIFF integration are not part
of these local checks; the transport fixture exercises actual owner policy and
durable adapters, without making operational provider calls.

After merging Task 18c at `0e67b37f`, the complete backend suite, 81 browser
tests, TypeScript compilation and regenerated contract checks passed again.
The independent three-server fixture also passed
`TestCdmCandidateTwoReplicaLocalSequenceDebounceAndTakeover`,
`TestTrafficCandidateTwoReplicaNATS` and
`TestDeadlineBinarySharedCoverageReconnectAndReconciliation` against the
combined branch. These focused checks preserve both tasks' deadline behavior;
Task 20 runtime binding remains separate.
