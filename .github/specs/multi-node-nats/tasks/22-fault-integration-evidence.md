# Task 22 — Local cross-node fault and restore evidence

**PASS, 2026-10-01:** the complete sequential runner finished in 828.113 seconds:
10 top-level tests and all eight fault-boundary subcases passed, with no skips.
Run ID: `20261001T184758Z-218cf03d13ff4742abeedb0d4b9e89e4`.
Implementation and evidence are in [draft PR #829](https://github.com/flightstrips/FlightStrips/pull/829).

The subsequent [idle CPU investigation](22-idle-cpu-evidence.md) records the
retained-history copy fix and its qualification against a newer source revision.
The results below remain the historical fault qualification for `9aecd076`.

This is local qualification for the held NATS integration branch. It does not
authorize a main merge, release, deployment or rolling production update.
Task 20 automated acceptance remains complete; operator browser/EuroScope
acceptance remains pending. Task 21 owns production infrastructure and Traefik
health checks. Task 23 capacity and Task 24 cutover gates remain pending.

## Source, binaries and machine

The complete run uses source `9aecd076ce6fac9da73474c48cc9a98a1042e363`, based on
`db662817b8191c160e5d35fc39f07a8db88f91ca`. Its source metadata records a clean
tracked runtime/harness and the SHA-256 of an empty tracked diff; the only
untracked file is this evidence draft. The compatibility test builds
the actual committed `db662817` executable from a temporary `git archive` and
runs it alongside the current executable. The documentation follow-up does not
change the tested runtime or harness.

| Item | Actual local environment |
| --- | --- |
| Host | Windows 11 Pro, 10.0.26200; AMD Ryzen 7 7700X; 16 logical processors; 33,462,480,896 bytes RAM |
| Physical failure domains | **One**; three independent broker processes on the same machine |
| Backend | Go 1.25.7 windows/amd64; real `cmd/server` builds and two separate processes per fixture |
| Ordinary backend binary | SHA-256 `6b7b3bac3c3f4c3023b871626f209cee232e5254bec924964530f875cc99d79a` |
| Tagged fault backend binary | SHA-256 `7b90401fef616d2a59761006e766dfaa0e561dc21e4593ddbe0525512c9d5800` |
| Actual base executable in overlap | SHA-256 `b86c5fa562e9e6ebac49976c46814604b336b80b948529979fc9099f7a51d449` |
| NATS | Native 2.15.0; SHA-256 `caed81357cbddb36c579220f8c35cead62063cf6a8de9d126f0424f50c04d986` |
| Frontend toolchain | Node 22.16.0, npm 10.9.2 |
| EuroScope plugin | Visual Studio 2022, MSVC 19.44, Windows SDK 10.0.26100.0, Win32 Release |
| Plugin DLL | SHA-256 `168c835ef784b084aef8162c164555ce8c044657ad03525ac0428979af8b825e` |
| Docker | Desktop engine 29.4.2; `docker run --rm nats:2.15.0 --version` returns 2.15.0 |

The frontend and plugin sources are unchanged from the integration base. The
clients in the fault suite are automated binary WebSocket clients using the real
frontend/EuroScope schemas, authentication, handlers and command results. The
compiled browser bundle and DLL were built and tested locally; this does not
claim interactive browser or installed EuroScope acceptance.

## Reproduce in PowerShell

Run from the repository root with Go, Python, Node 22.16, Git and Windows `tar`
available. Install the pinned broker if it is absent:

```powershell
go install github.com/nats-io/nats-server/v2@v2.15.0
& backend/testdata/nats/task22.ps1
```

Go selected its required 1.26.8 toolchain to build the broker. Application builds
use the repository's Go 1.25.7 toolchain. The runner checks the broker version,
records machine/source/broker hashes, restores its process environment, and runs:

```powershell
$env:NATS_INTEGRATION = '1'
$env:NATS_TASK22 = '1'
$env:NATS_TASK22_LONG = '1'
$env:NATS_SERVER_BINARY = 'C:\Users\fsr19\go\bin\nats-server.exe'
Push-Location backend
go test ./cmd/server -run '^TestServerNATS' -count=1 -timeout=25m -json
Pop-Location
```

Prefer the runner: it fails on any failed **or skipped** acceptance case. Its
artifacts are under ignored `backend/.task22/runs/<UTC>-<UUID>/`: machine-source
JSON, complete `tests.jsonl`, and filtered `evidence.txt`. The committed evidence
artifacts include [machine/source metadata](22-artifacts/machine-source.json),
[fault/restore checkpoints](22-artifacts/fault-restore.txt), and the
[complete successful run](22-artifacts/tests.jsonl). They retain timings,
command IDs, stream sequences, process IDs, build and
configuration hashes, snapshot and restore comparisons. No credentials, keys,
tokens or private-message bodies are included.

The final long-outage checkpoint records 5m29.995s from broker stop through
recovery, with an actual 305-second sleep while all three brokers are stopped.
The no-controller marker moved from `18:49:09.4779764Z` to
`18:54:29.4780289Z` on 2026-10-01; persisted pause time is
`18:54:30.4781011Z`, at stream sequence 43. The session remained active.

Additional checks executed locally:

```powershell
Push-Location backend
go build ./...
go test ./...
Pop-Location
python scripts/cluster_proto.py --check
python scripts/check_cluster_contract.py
Push-Location frontend
npm ci
npm run build
npm test
Pop-Location
cmake -S euroscope-plugin -B euroscope-plugin/build-local -G 'Visual Studio 17 2022' -A Win32
cmake --build euroscope-plugin/build-local --config Release
ctest --test-dir euroscope-plugin/build-local -C Release --output-on-failure
cmake --build euroscope-plugin/build-local --config Release --target cluster_proto_contract
docker version --format '{{.Server.Version}}'
docker run --rm nats:2.15.0 --version
```

Go build/tests, protocol generation/contract checks, frontend build and 544 tests
in 66 files, plugin Release build and 440/440 CTest cases, and C++ cluster
Protobuf contract compilation passed. No protocol definitions or ALB paths change.
The Docker commands above inspect the engine and run a disposable version-only
container with no mounts or published ports. Fault qualification itself uses the
native brokers; it is not a Docker production topology qualification.

## Fixture isolation and fault method

Each case creates a fresh temporary directory, random loopback client/route/API
ports, a unique cluster name, three separate broker stores, a random store key,
a separate random effect key and fixture-only authorization. Fault fixtures use
AES-encrypted file stores and the fixed R3 durable resources; presence remains
disposable memory R3. No default development process, Compose service, volume,
key or secret file is modified. Stops use only the exact child process handle
and PID created by that fixture, verified before termination. The official run
executes cases sequentially and never kills by process name or discovered PID.

API clients connect directly to the two fixture ingress addresses. This checkout
does not provide a local HTTP load-balancer fixture; production Traefik routing
and health-check configuration belong to Task 21. The TCP PubAck gate is on the
backend-to-NATS transport.

The `task22fault` build tag enables metadata-only barriers immediately before
publish, after the actual successful PubAck, and before/after the actual socket
write. It also permits loopback HTTP origins for real AIRAC/wind adapters. Normal
builds select no-op implementations and exclude those environment/file controls.
The independent PubAck-loss test pauses **incoming TCP bytes**, lets the actual
outbound publish reach JetStream, proves its commit through a separate observer,
then kills the owner before its PublishMsg timeout. It does not fabricate a
PubAck or replace the store/router with an in-process mock.

HTTP outcome APIs retain JSON. Application commands, events, snapshots, objects,
positions and WebSocket frames remain the strict typed binary contract. Backup
uses NATS's JSON administrative snapshot/restore API and a local encrypted
artifact container; those are administration boundaries, not application wire
or state format changes.

## Assertions and exact outcomes

| Scenario | Required assertion |
| --- | --- |
| Concurrent session creation | Two authenticated CIDs log into different processes concurrently; one global registry entry, two controllers and exactly one binary `master` role at the accepted epochs; accepted master sync seeds state |
| Strip race | Same entity revision on both ingress nodes yields one `SUCCEEDED`, one durable `FAILED/REVISION_CONFLICT` |
| Stand race | Real SAT registry/policy; competing A17 assignment yields one success, one `FAILED/INVALID_ARGUMENT`; exactly one assignment; explicit override semantics are not mistaken for a reservation conflict |
| AMAN race | Real AIRAC/wind adapters and operational AMAN policy with local provider fixtures; same airport revision yields one success and one revision conflict; audits persist |
| Reconnect/order/parity | First binary frame is initial state; subsequent session/airport deltas increase their respective revisions strictly; normalized initial entities and revisions match across both processes |
| Death before domain publish | No ledger outcome, no commit, no automatic replay after restart (`NOT_FOUND`) |
| Death after domain PubAck | Durable `SUCCEEDED` survives takeover and restart |
| PubAck lost before owner death | Independent observer sees commit; incoming ack is blocked; surviving/restarted servers return the same success; same command ID returns the original committed sequence |
| Death before effect claim publish | A successor may dispatch once; absent a result, visible terminal `UNKNOWN`; no duplicate |
| Death after claim PubAck / before socket write | Committed claim, zero deliveries, visible terminal `UNKNOWN`; no automatic resend |
| Death after socket write | Exactly one delivery to the original target CID, no delivery to the other CID; terminal `UNKNOWN`; no automatic resend |
| Death before executed-result publish | One delivery; terminal `UNKNOWN` because the result is not durable |
| Death after executed-result PubAck | One delivery; durable effect `EXECUTED` and command `SUCCEEDED` |
| Old master/owner input | Old master and current-epoch non-master frames change no strip and create no ledger outcome; stale owner event advances replay only, preserving the entity and omitting ledger/effect mutation |
| Corrupt snapshot | Replace fixture object bytes; both real processes restart using retained history and preserve exact command outcomes |
| One broker loss | Remaining quorum retains readiness and acknowledged outcomes; broker rejoins |
| Two broker loss | Both readiness endpoints return 503 while liveness stays 200; restored quorum recovers acknowledged outcomes |
| Full shutdown/restart | Stop both backends and all brokers; restart the same isolated stores; acknowledged outcome remains present |
| Controller-free outage | Actual 305-second all-broker outage, backends remain running; after recovery the session is not tombstoned and the controller-free deadline shifts by paused outage time |
| Compatible version overlap | Actual base/current executables both read schema v1, accept writes entering each process and expose equal durable results |
| Unknown writer version | Append schema v2; both actual readers return readiness 503 and reject WebSocket admission with 503; liveness remains 200 |

Same-ID terminal effect replays return their original durable status and cannot
create another delivery. The target CID never changes during takeover. Claimed
uncertainty is recorded visibly as `unknown`; no automatic resend is introduced.
The overlap qualification is for these two concrete schema-v1 versions only.
Future version pairs still need their own compatibility evidence.

## Separate-cluster encrypted restore

The restore case freezes source writers at an observed stream high-water mark,
saves verified aggregate snapshots, and snapshots `FS_STATE`, `KV_FS_POSITIONS`,
`KV_FS_SNAPSHOT_INDEX`, and `OBJ_FS_OBJECTS`. An AES-GCM off-cluster artifact
contains the snapshot bytes, full fixture authorization configurations, store
key and effect key; its backup key is kept separately. Authenticated tampering
must fail. The source brokers are then stopped completely.

Restore creates three **new** broker processes, stores, ports and cluster identity.
Only fixture listen/routes/store identities are rewritten; authorization and
store cipher configuration come from the backup. Each durable stream restores
its exact message count, last sequence and R3 configuration. Bootstrap recreates
disposable presence and verifies the durable configuration.

Before backend writers restart, an independent restored projection compares
exact typed snapshots for global, airport and session aggregates: entities,
revisions, ledger outcomes, effects, workflows, audits, nav manifests and pending
deadlines. Counts, sequences and SHA-256 digests are recorded. Latest-position
bytes and KV revision match exactly; nav object digest matches; the restored
effect key decrypts the original encrypted body. Some audit/nav/deadline records
are explicit typed fixture seeds through the real owner command route, rather
than claims of live operational data collection.

Two real backends then start against the new cluster. A reconnecting original
CID receives no automatic resend of the restored dispatch claim; both JSON APIs
report `unknown`, and the pending deadline remains present. This is an actual
local off-cluster backup/restore exercise, not an offsite or multi-host disaster
recovery claim.

## Failures discovered and runtime corrections

The original longer outage exposed a warm-replica recovery deadlock: after every
lease expired, each former owner advertised itself unready and waited for a
ready candidate before claiming its expired aggregate. Both `/readyz` endpoints
remained 503 with `owner renewal unavailable` after broker quorum returned.

Ownership maintenance now permits a connected, caught-up local replica to
attempt a claim when the ready-candidate set is empty. This permits recovery
only: subject CAS, server-time lease validation and accepted-term checks still
fence ownership; command and effect work remain gated. The real 15-second lease
expiry regression and the full 305-second outage exercise this correction.
Earlier failure logs remain local under ignored `.task22/exploration`; they are
not represented as passing acceptance. The final complete run is the source of
the committed acceptance artifact.

After ownership recovered, a full-duration case also exposed an interrupted
cleanup recovery: metadata could briefly become healthy before the pause was
persisted, freezing an early `recoveredAt`. A renewed outage retained that short
interval and counted the rest of the outage toward deletion. Cleanup now resets
the provisional recovery time on renewed failure and extends only the portion
not already persisted. Unrelated reconciler errors no longer retain a pause
already durable for every session. Focused regressions reproduce the original
failure and assert that extending a partially recorded pause cannot double
count it. The full suite was rerun after both runtime corrections.
