# Windows local development

Run these commands from the repository root in PowerShell. Prerequisites are
Docker Desktop with its Linux engine running, Compose with `include` and optional
`env_file` support, Go (automatic Go 1.25.7 toolchain download enabled), Node/npm,
Python 3, CMake, and Visual Studio 2022 C++ tools with Win32/Windows SDK support.
This setup uses local component builds and the committed revision-2 clients.

## Configure and start two backends

The tracked `backend/.env` contains development defaults. Put personal overrides
in `backend/.env.dev`; do not commit provider credentials. The existing development
OIDC authority/audience and frontend/plugin client IDs must agree.
If an existing `.env.dev` sets `NAVIGATION_TERMINAL_GEOMETRY_PATH` to the retired
AIRAC 2609 file, update it to `config/aman/ekch-terminal-2610.json` before startup.
The default local SAT aircraft source is the committed three-type fixture (A320/B738/C172).
For full-fleet manual testing, copy your GRPlugin `ICAO_Aircraft.json` to ignored
`backend/config/data/ICAO_Aircraft.json`, then set
`GRPLUGIN_ICAO_AIRCRAFT_JSON=config/data/ICAO_Aircraft.json` in `.env.dev`.
Compose mounts `config/data` read-only; the same path works natively. Unknown
aircraft fail the existing reference checks. This fixture is not a full-fleet
qualification result. Local builds
use the existing development identity; no release version bump is required.

```powershell
.\backend\local.ps1 init
.\backend\local.ps1 build
.\backend\local.ps1 brokers
.\backend\local.ps1 bootstrap
.\backend\local.ps1 start
.\backend\local.ps1 status
```

`init` creates one random **32 raw byte** key at
`backend/.local-secrets/effects-v1.key`, only if absent. Both backends mount that
same file. Preserve it while retaining broker volumes; do not regenerate it on
restart. `NATS_EFFECT_ACTIVE_KEY_ID=v1` and
`NATS_EFFECT_KEY_FILES=v1=/run/secrets/nats_effect_v1` select the shared ring in
Compose. Native builds use `v1=.local-secrets/effects-v1.key`. Retain old IDs and
files when rotating keys until all retained encrypted effects are readable.

The default Compose starts three `nats:2.15.0` nodes, each with a separate
persistent volume, and two instances of the real backend image. Only the explicit
`bootstrap` action uses administrator credentials. Backends always verify and
use existing resources; they cannot create or repair resource configuration.
The fixture credentials in `testdata/nats` are local-only.

| Client | Node A | Node B |
| --- | --- | --- |
| JSON HTTP | `http://localhost:8090` | `http://localhost:8091` |
| Frontend binary | `ws://localhost:8090/frontEndEvents` | `ws://localhost:8091/frontEndEvents` |
| EuroScope revision 2 | `ws://localhost:8090/euroscopeEvents` | `ws://localhost:8091/euroscopeEvents` |
| Liveness/readiness/metrics | `/healthz`, `/readyz`, `/metrics` | same paths |

Broker client ports are 4222–4224 on loopback. Native backend development after
bootstrap can run `go build -o bin/api.exe ./cmd/server` from `backend`, then
`./bin/api.exe -addr 127.0.0.1:8090` and, in another terminal,
`./bin/api.exe -addr 127.0.0.1:8091`. Stop Compose backends first to release ports.
Environment variables override `.env`; `.env.dev` loads after `.env` for native
development. Set `OTEL_EXPORTER_OTLP_ENDPOINT=''` to disable the optional local
collector. Configuration files resolve relative to `backend`, including
`config/aman/ekch-terminal-2610.json` when terminal navigation is enabled.

## Frontend and EuroScope plugin

```powershell
Push-Location frontend
npm ci
npm run build
npm test
npm run dev -- --host 127.0.0.1
Pop-Location
```

Open the Vite URL printed by npm (normally `http://localhost:5173`).
`frontend/public/config.js` defaults both API and socket to the failover proxy on
`localhost:8092`. To connect directly to node B,
change its `wsUrl` to `ws://localhost:8091/frontEndEvents` and `apiBaseUrl` to
`http://localhost:8091`, then reload. These are temporary local configuration
edits; retain the existing OIDC values. Use a frontend on B and EuroScope on A to
check shared strips, controller state, CDM/PDC and JSON read outcomes.

```powershell
cmake -S euroscope-plugin -B euroscope-plugin/build-local -G 'Visual Studio 17 2022' -A Win32
cmake --build euroscope-plugin/build-local --config Release --parallel 4
ctest --test-dir euroscope-plugin/build-local -C Release --output-on-failure
cmake --build euroscope-plugin/build-local --target cluster_proto_contract --config Release --parallel 4
```

The outputs are in `euroscope-plugin/build-local/bin`. For local qualification,
copy the development configuration and load the **core DLL directly** using
EuroScope's plugin dialog (its exported `EuroScopePlugInInit` supports this):

```powershell
Copy-Item euroscope-plugin/src/config_dev.ini euroscope-plugin/build-local/bin/flightstrips_config.ini
```

Load `euroscope-plugin/build-local/bin/FlightStripsPluginCore.dll` with the runtime
DLLs in that directory. `[api] baseurl` in `flightstrips_config.ini` selects A or
B's `/euroscopeEvents`; keep its development authentication identity. The direct
core loading path uses the locally built artifact and avoids the release loader's
update/download path. Normal published plugin loader behavior is unchanged.
Connect with your normal development login; unload the plugin before replacing
DLLs. Automated plugin tests do not claim that the operator has loaded/tested it
in EuroScope.

## Stop, restart and replay retained data

### Manual failover through one client address

The included local HAProxy serves `http://localhost:8092` and the binary socket
paths on `ws://localhost:8092`. It checks each backend's `/readyz` every second
and routes new connections to ready nodes. See the [HAProxy health-check contract](https://docs.haproxy.org/3.2/configuration.html#option%20httpchk).
Direct node ports remain 8090 and 8091 for diagnostics and cross-node checks.
Both the frontend development config and copied plugin `config_dev.ini` use the
proxy by default. Existing WebSockets reconnect after their backend stops;
an established socket is not transferred between processes.

After the initial build/bootstrap/start commands above, log in, open a session
and make a strip change. Run these commands from the repository root:

```powershell
# Hold A down; observe reconnect and retained accepted state through port 8092.
docker compose -f backend/docker-compose.yaml -p flightstrips-local stop backend-a
Invoke-WebRequest http://localhost:8092/readyz -UseBasicParsing
docker compose -f backend/docker-compose.yaml -p flightstrips-local start backend-a
.\backend\local.ps1 status

# Repeat for B; verify changes from a client on each node before killing it.
docker compose -f backend/docker-compose.yaml -p flightstrips-local stop backend-b
docker compose -f backend/docker-compose.yaml -p flightstrips-local start backend-b
.\backend\local.ps1 status

# One broker down should retain quorum and both backend readiness endpoints.
docker compose -f backend/docker-compose.yaml -p flightstrips-local stop nats-1
.\backend\local.ps1 status
docker compose -f backend/docker-compose.yaml -p flightstrips-local start nats-1
```

Allow the readiness check and ownership takeover to finish before testing new
commands. `/readyz` can briefly fail while the proxy detects a stopped node.
Use `kill backend-a` (or `kill backend-b`) instead of `stop` to simulate an abrupt
process exit, then inspect `ps` and explicitly restore it with `start`.
Do not run both backend stop tests at once. Preserve broker volumes and the shared
effect key; these commands do not reset data. Logs are available with
`docker compose -f backend/docker-compose.yaml -p flightstrips-local logs --tail 100 backend-proxy backend-a backend-b`.

```powershell
.\backend\local.ps1 stop       # stop both backends
.\backend\local.ps1 start      # replay retained accepted state
.\backend\local.ps1 restart    # restart both backend processes
.\backend\local.ps1 down       # remove containers/network, retain volumes and key
.\backend\local.ps1 brokers
.\backend\local.ps1 start      # existing resources, no bootstrap needed
```

Use `docker compose -f backend/docker-compose.yaml -p flightstrips-local logs
--tail 100 backend-a backend-b` for startup failures. `/readyz` returns 503 during
replay, quorum loss, resource drift or projection stall. `/healthz` remains
process liveness. Wait for both readiness endpoints before opening clients.
Its JSON body can report degraded domain health while an empty/shadow AMAN board
has not yet received provider inputs; that is separate from HTTP liveness and
accepted-state readiness.
Never use `down -v` on data you intend to retain. Initial empty startup requires
bootstrap; normal restart does not. Logs expose command IDs, sequence/epochs and
error types, without credentials or message bodies.

## Automated validation (disposable fixture)

```powershell
python scripts/cluster_proto.py --check
python scripts/check_cluster_contract.py
Push-Location backend
go build ./...
go test ./...
$env:NATS_PORT_1='5322'; $env:NATS_PORT_2='5323'; $env:NATS_PORT_3='5324'
docker compose -f docker-compose.nats.yaml -p fs20a up -d
$env:NATS_INTEGRATION='1'; $env:NATS_TEST_PORT_BASE='5322'; $env:NATS_FAULT_PROJECT='fs20a'
go test ./internal/app -run '^TestBuildNATS' -count=1 -timeout=10m -v
go test ./cmd/server -run '^TestServerNATS' -count=1 -timeout=5m -v
# Fixture is disposable; remove only this project's volumes after testing.
docker compose -f docker-compose.nats.yaml -p fs20a down -v
Remove-Item Env:NATS_INTEGRATION, Env:NATS_TEST_PORT_BASE, Env:NATS_FAULT_PROJECT
Remove-Item Env:NATS_PORT_1, Env:NATS_PORT_2, Env:NATS_PORT_3
Pop-Location
```

Run these integration packages sequentially because they deliberately change
shared provider generations and cluster availability. The app suite exercises
actual provider policy, effects, AMAN, stalled replay, drift and backend death.
The entrypoint test builds and launches two real servers with fixture OIDC,
revision-2 EuroScope, binary frontend and JSON HTTP command-result checks. Its
optional `NATS_SERVER_BINARY` path lets it own three native pinned NATS 2.15.0
processes and verify quorum loss plus a full broker/backend restart. Without that
path the Docker app fault suite supplies broker-quorum coverage. To run this
native entrypoint alternative from `backend`:

```powershell
$env:GOBIN=(Join-Path (Get-Location) 'bin')
go install github.com/nats-io/nats-server/v2@v2.15.0
Remove-Item Env:GOBIN
$env:NATS_INTEGRATION='1'; $env:NATS_SERVER_BINARY=(Resolve-Path bin/nats-server.exe).Path
go test ./cmd/server -run '^TestServerNATS' -count=1 -timeout=5m -v
Remove-Item Env:NATS_INTEGRATION, Env:NATS_SERVER_BINARY
```

The pinned broker build may automatically download its required Go 1.26 toolchain.
Record your source revision (`git rev-parse HEAD`), local config, commands and
results. The operator's manual acceptance remains pending until supplied.
[Task 20c evidence](../.github/specs/multi-node-nats/tasks/20c-sql-retirement-evidence.md)
separates automated results from pending operator acceptance. SQL and Redis are
not needed. SAT scenario/replay tools and landing validation remain disabled.
Production infrastructure, version overlap/capacity qualification and activation
remain Tasks 21–24; initial activation is stop-first and is held from release.
