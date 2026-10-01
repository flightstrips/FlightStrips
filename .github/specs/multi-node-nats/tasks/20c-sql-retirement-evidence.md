# Task 20c — SQL retirement and final integration evidence

Implementation is on `codex/multi-node-nats-20c-sql-retirement`, based on
`origin/codex/multi-node-nats-base` at `7936899e` (20a/#827 and 20b/#826 merged).
The implementation commit is recorded in the source-revision section below.
All work remains held from `main`, release and deployment. Task 20 is **not marked
complete**: this child is a draft, and review/operator acceptance remains pending.
Tasks 21–24 are separate. No production infrastructure, release, activation,
promotion, staging or announcement was performed.

## Final constructor and binding audit

`cmd/server` unconditionally constructs `app.BuildNATS`. The SQL constructor,
pool, concrete repositories, query generation, migrations, migrator executable/
image, seeders, SQL test runners and obsolete JSON socket hubs are removed.
`DATABASE_CONNECTIONSTRING`, Docker database credentials and SQL fallback are
absent. The obsolete root `docker-compose.prod.yml` is removed. Retired recordings
and dated performance records are historical data and are not executable paths.

The final startup retains the complete [20a HTTP, planner and worker matrix](20a-runtime-assembly-evidence.md).
The accepted-state implementations remain bound; the change does not substitute
injected policy, SQL calls or no-op business hooks. Pure shared SAT, PDC, CDM,
controller, ECFMP, validation and AMAN policy remains compiled and tested. Pure
AIRAC adapter tests are retained; only SQL checkpoint/cache tests are retired.

| Final binding | Construction / accepted source | Rechecked evidence |
| --- | --- | --- |
| Session/airport/global owners, command router, projection and observations | `BuildNATS`, owner runtime, typed `FS_STATE`/KV replay | two-app takeover; unbounded-context compiled entrypoint; drift/stall/restart |
| Frontend and EuroScope binary transports | projection/router, registry/controller sectors, deadlines, position writer, effect renderer | binary clients on different compiled processes; mutation plus JSON outcome and accepted strip |
| HTTP JSON APIs, command outcomes, EFB/PDC/pilot/stand/CDM/AMAN/GSX | existing APIs through projection/typed command adapters | assembled HTTP tests; backend unit API tests; contract boundary check |
| Traffic/CDM/PDC, SAT arrival/departure, deadlines and cleanup | `SessionWork` with concrete candidate policy, durable workflows/effects | actual provider fixture, CDM action, LIVE stand lifecycle, Hoppie poll/send, durable plugin result |
| Navigation/AIRAC, AMAN, Open-Meteo/weather | concrete source adapters, typed provider objects, operational policy | `TestBuildNATSAIRACWindAndAMANPolicy` |
| Global/airport provider feeds and rate quota | VATSIM/transceivers, METAR/ATIS, ECFMP, CDM configuration, Hoppie and accepted checkpoints | `TestBuildNATSProductionProvidersCDMSATAndPDC`, duplicate global quota refusal |
| ALB | existing `alb.NewHub`, `Run`, `/albEvents`, `EnableALB=true` | ALB source has zero diff; existing NATS ALB construction unchanged; no protocol/config change |
| Disabled features | `ENABLE_TEST_TOOLS=false`; constructor rejects enabled SAT scenario/replay; landing creation switch remains false | configuration and source audit; no replay compatibility runner |

Real executable startup found a worker-context bug: NATS `FlushWithContext`
requires a deadline, while normal server workers use an unbounded context.
Router/quota/socket/effect subscription registration now uses bounded attempts
and tolerates transient transport loss. The stalled-delivery and real executable
checks verify recovery. Runtime socket errors log types rather than error text.
The provider fixture now imports CDM configuration once; concurrent refresh
fencing remains covered by the existing CDM tests, avoiding a one-second refresh
race with that fixture's unrelated initial operational sync.

## Parent Task 20 acceptance record

| Parent acceptance item | Result and command / evidence |
| --- | --- |
| Build/test and two real backends without PostgreSQL/Redis | PASS: `go build ./...`, `go build -o bin/api.exe ./cmd/server`, `go test ./...`; native pinned three-broker `TestServerNATSBinaryCrossNodeRestartAndReadiness` builds and runs two servers from the real entrypoint |
| No active SQL runtime, driver, transaction or connection-string fallback | PASS: Go source/config/module and `go list -deps ./cmd/server` scans; `go version -m bin/api.exe`; no pgx/Postgres/sqlc/Redis implementation/module. Standard-library `database/sql/driver` interfaces remain transitively through `google/uuid` Scanner/Valuer support; this is not a linked SQL driver or database runtime. `database/sql` is absent |
| Replay, resource drift and projection-stall readiness failure/recovery | PASS: initial entrypoint admission waits on replay; `TestBuildNATSReadinessResourceDriftAndFailedConstruction` and `TestBuildNATSReadinessProjectionStallAndRecovery` against real brokers. `/healthz` remains live; operational admissions fail closed |
| Quorum loss and recovery | PASS: native executable test stops two of its own brokers, checks both readiness endpoints return 503 and both liveness endpoints remain 200, restores quorum/catch-up. Docker-specific `TestBuildNATSReadinessQuorumLossAndRecovery` also PASS after the operator restored the engine; CI review remains pending |
| Backend death, accepted outcomes and effective-work safety | PASS: two-app takeover and compiled-entrypoint death/restart retain acknowledged state; full broker/backend restart replays the same outcome. Assembled deadline/squawk plugin dispatch/results, duplicate quota refusal and existing owner/effect policy tests pass. Broader fault/version qualification remains Task 22 |
| Typed NATS/storage/frontend/EuroScope; JSON first-party HTTP | PASS: `python scripts/cluster_proto.py --check` (300 binary oneof + 291 optional-zero fixtures), `python scripts/check_cluster_contract.py`; generated schemas unchanged; real binary clients and JSON command-result reads on both compiled processes |
| Safe logs and metrics | PASS: measured PubAck/store + projection/lease/effect metrics remain wired; metadata logging retains command ID, stream sequence, owner/master epoch. Entry-point test checks every backend generation for absence of token/credential strings; runtime socket errors log type only. No payload logging added |
| Ordinary release behavior; no migrator build/bump | PASS: 20b merged (#826); workflows have no `backend-migrate`, `Dockerfile.Migrate` or `cmd/migrate`; ordinary component/release/version paths have zero change in 20c. Normal build CI now runs Go tests and sequential real-NATS suites |
| Repeatable local setup without published artifacts | Implementation PASS: `backend/local.ps1`, `backend/Readme.md`, architecture and position-performance updates. PowerShell parse/init/idempotent key preservation and Compose resolution PASS. Docker image build, empty three-broker bootstrap, two-backend readiness, restart, stop/start and full down/up with retained volumes PASS |
| User's own complete-system local acceptance and release decision | PENDING: exact frontend/plugin/backend commands and cross-node setup are documented. Automated builds/tests do not claim the user has loaded EuroScope or performed manual acceptance. Integration remains held |
| All three children merged and parent gate complete | PENDING: 20a/#827 and 20b/#826 merged; 20c draft must pass review/CI and outstanding review/operator checks. Task 21 does not start on the strength of this evidence |

## Machine, configuration and executed commands

2026-10-01, Windows on the user's machine, PowerShell, Go 1.25.7 application
toolchain, Node/npm, Python 3.11.9, CMake/VS 2022 Win32 compiler 19.44 and Windows
SDK 10.0.26100. No component version changes or candidate artifacts.

- Backend `go build ./...` and `go test ./...`: PASS after SQL retirement and restored pure policy tests. SQL fixtures are not required; integration tests skip unless explicitly enabled.
- Frontend `npm ci`, `npm run build`, `npm test`: PASS, 66 files / 544 tests.
- Plugin `cmake -S euroscope-plugin -B euroscope-plugin/build-local -G 'Visual Studio 17 2022' -A Win32`; default Release build; `ctest --test-dir euroscope-plugin/build-local -C Release --output-on-failure`: PASS, 440/440 tests. `cluster_proto_contract` Release target: PASS.
- Both Python protocol/coverage checks: PASS. ALB and generated schema source diffs: empty. `git diff --check`: PASS.
- `docker compose -f backend/docker-compose.yaml -p flightstrips-local config --services`: PASS, exactly three brokers and two complete backends; administrator bootstrap is separately profiled. No SQL/Redis service. Image is pinned to NATS 2.15.0.
- `backend/local.ps1 init` twice plus PowerShell parser: PASS, one shared 32-byte key preserved; secret path is ignored by Git.
- Native NATS built with `go install github.com/nats-io/nats-server/v2@v2.15.0` (automatic Go 1.26.8 broker toolchain). Three independent processes use the committed local authorization rules, ports 5322–5324, cluster routes 6322–6324 and separate file stores. Administrator bootstrap is separate from backend credential verification.
- With fresh disposable native stores and `NATS_INTEGRATION=1`, `NATS_TEST_PORT_BASE=5322`: `go test ./internal/app -run '^TestBuildNATS' -count=1 -timeout=10m -v`: PASS, six scenarios; one Docker-only quorum scenario skipped. Fresh stores matter: retained provider checkpoints intentionally suppress repeated HTTP fetches and are unsuitable for fresh-import assertions.
- `NATS_INTEGRATION=1`, `NATS_SERVER_BINARY=<absolute backend/bin/nats-server.exe>`; `go test ./cmd/server -run '^TestServerNATS' -count=1 -timeout=5m -v`: PASS. Test owns empty brokers, temporary effect ring and actual OIDC signing fixture; binary frontend/EuroScope on different nodes mutate state, HTTP outcomes agree, backend death/restart, quorum loss/recovery and full retained-store restart pass. Broker readiness is awaited before restarting backends; startup fails closed while quorum is unavailable.

After the operator restored Docker Desktop, the engine reported version 29.4.2
with Linux containers. The default `local.ps1 build/brokers/bootstrap/start/status`
sequence passed from empty resources; both backend images reported stand
assignment ready and readiness 200. `restart`, `stop/start`, `down` then
`brokers/start` passed with retained volumes and key, without another bootstrap.
HTTP liveness was 200; the empty shadow-mode AMAN health body reported degraded
components before accepted provider/board inputs. No manual AMAN acceptance is
claimed from that startup; its operational policy is exercised by the fixtures.
The image is `flightstrips-backend:development`, image ID
`b0f2edd458b45d60b5d61d25e82b03273b7ae3940184fe26741ab608533a2e72`.
Local secrets and native binaries are excluded from the build context; final
image inspection confirms no `.local-secrets` or `.env` file.

The fresh Docker `fs20a` fixture ran all **seven** app assembly tests, including
the label-checked broker-quorum fault/recovery test: PASS (142.28 s).
The sequential compiled-server test against Docker brokers also PASS (8.05 s).
Native evidence above remains separately identified. The default SAT aircraft
reference is explicitly the three-type local fixture; full-fleet manual testing
requires the operator's own GRPlugin reference in ignored `config/data`, mounted
read-only. The stale development terminal geometry path was corrected to 2609.
No result claims operator manual acceptance, multi-host or capacity qualification.

## Source revision and handoff

Implementation revision: recorded after the implementation commit. The subsequent
evidence-only commit identifies that exact tested source revision. See the draft
PR for current head and CI results. Rebuild/retest on the operator's chosen head;
retain the shared effect ring and broker volumes across normal restarts.
