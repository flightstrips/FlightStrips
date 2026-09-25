---
title: Local development
description: Toolchain, run commands, and wiring for backend, frontend, EuroScope plugin, and docs on Windows.
---

| Part | Platform |
| --- | --- |
| Backend | Windows |
| EuroScope plugin | Windows x86 (Win32) — MSVC |
| Frontend | Windows, Linux & Mac |
| Docs | Windows, Linux & Mac |

## Host requirements

| Requirement | Used for | Notes |
| --- | --- | --- |
| Docker Desktop | Backend | Compose for Postgres, migrator, optional full API image |
| Go | Backend | Version per `backend/go.mod` |
| Node.js 22.x | Frontend, docs | Version locked in `frontend/package.json` `engines` |
| Visual Studio 2022 **or** Build Tools (C++ workload) | EuroScope plugin | Must use an **x86 Native Tools** MSVC prompt — x64 produces the wrong architecture |
| CMake ≥ 3.15 | EuroScope plugin | |
| Ninja | EuroScope plugin | Matches CI |

## Backend

From `backend/`, bring up the full stack (API on **8090**, Postgres, migrator):

```sh
docker compose --profile all up --build -d
```

To run the Go binary on the host instead, start only Postgres then:

```sh
docker compose --profile database up --build -d
go run ./cmd/server
```

Set `DATABASE_CONNECTIONSTRING` to `localhost:5432` (see `backend/.env`).

## Frontend

From `frontend/`:

```sh
npm ci
npm run dev
```

Default `wsUrl` in `public/config.js` is `ws://localhost:8090/frontEndEvents`.

## EuroScope plugin

From `euroscope-plugin/` in an **x86** MSVC environment:

```sh
cmake -DCMAKE_BUILD_TYPE=Debug -DCMAKE_EXPORT_COMPILE_COMMANDS=ON -G Ninja -B build
cmake --build build
```

Swap `Debug` for `Release` to mirror `.github/workflows/build-plugin.yml`.

To install:

1. Copy `build/bin/FlightStripsPlugin.dll` and `build/bin/flightstrips_config.ini` into your EuroScope Plugins folder (`%AppData%\EuroScope\<ICAO>\Plugins\`).
2. Keep any EuroScope dependency DLLs beside the plugin.
3. Load the DLL from EuroScope's plugin dialog.

### `flightstrips_config.ini` for local development

:::note
Debug builds automatically deploy `src/config_dev.ini` as `flightstrips_config.ini` — skip this unless you are patching a Release build or an existing install.
:::

The values that differ from production:

```ini
[authentication]
audience = backend-dev
clientId = oPIlNgkBODM1OEFTrcKOZl9JavEives3

[api]
baseurl = ws://localhost:8090/euroscopeEvents

[logging]
level = DEBUG
```

All other keys (`authority`, `redirectPorts`, `enabled`) stay the same as production. Full reference: `src/config_dev.ini` (dev), `src/config.ini` (prod).

`userconfig.ini` holds personal tokens and is gitignored — do not commit it.

## Docs

From `docs/`:

```sh
npm ci
npm run dev
```

Dev server runs at `localhost:4321` by default.

## Wiring checks

With the backend on **8090**: the frontend shows live strips once auth and WebSockets succeed; the plugin negotiates the `euroscopeEvents` WebSocket independently of the frontend origin.

## Local SAT test console

The protected `/test` page can create and advance synthetic stand-assignment
scenarios without contacting the VATSIM data feed. Start the backend from
`backend/` with:

```powershell
$env:ENABLE_STAND_ASSIGNMENT = "true"
$env:ENABLE_TEST_TOOLS = "true"
go run ./cmd/server
```

When test tools are enabled, the backend uses its committed small aircraft
fixture unless `GRPLUGIN_ICAO_AIRCRAFT_JSON` explicitly points to a full
installed reference. The production VATSIM HTTP cache is replaced by an
in-memory source, and the VATSIM transceiver cache is disabled, so no VATSIM
network data is requested.

Sign in normally, connect the local EuroScope plugin to create an EKCH session,
then open `http://localhost:8080/test`. Select the session and use a departure,
arrival, or wrong-stand preset. `Next` drives the real reconciliation and SAT
lifecycle; manual time, position, block, remove, and reset controls are also
available.

### EuroScope-only AMAN for Sweatbox or Playback

AMAN can use EuroScope strips and position reports without a VATSIM feed or a
VATSIM CID. Start Postgres, then run the backend from `backend/` with:

```powershell
docker compose --profile database up --build -d
$env:ENVIRONMENT = "development"
$env:ENABLE_VATSIM = "false"
$env:ENABLE_VATSIM_TRANSCEIVERS = "false"
$env:AMAN_SOURCE_MODE = "euroscope"
$env:AMAN_MODE = "shadow"
$env:AMAN_ENABLED_AIRPORTS = "EKCH"
$env:NAVIGATION_SOURCE = "airacnet"
$env:ENABLE_TEST_TOOLS = "false"
$env:ENABLE_AMAN_HOLDING_EAT_WRITEBACK = "false"
go run ./cmd/server
```

`ENABLE_VATSIM=false` disables the public VATSIM data feed, and
`ENABLE_VATSIM_TRANSCEIVERS=false` disables the separate public transceiver
feed. `AMAN_SOURCE_MODE=euroscope` prevents AMAN startup, readiness, and
reconciliation from requiring either feed. `NAVIGATION_SOURCE=airacnet` is
still required for AMAN route geometry and is independent of VATSIM.

Build and connect the local EuroScope plugin as described above, then open a
Sweatbox or Playback session. Synchronize an arrival strip with `origin`,
`destination`, route/type fields when available, and no CID. The first
EuroScope position creates the surveillance fact; later reports derive track
and groundspeed for prediction. The TopSky fields `hold`, `hold_type`, and
`hold_eat` create or update the holding clearance. Clearing `hold` cancels it.

Use `AMAN_MODE=shadow` first to verify the computed state without AMAN-owned
writes. `read_only` enables AMAN ETA ownership but keeps controller mutations
disabled. `authoritative` enables AMAN commands after the normal technical and
rollout gates pass. Holding EAT writeback remains separately controlled by
`ENABLE_AMAN_HOLDING_EAT_WRITEBACK`.

The default `AMAN_SOURCE_MODE=hybrid` remains backwards compatible: EuroScope
observations work in every session, while VATSIM observations and strip
reconciliation are applied only to sessions named `LIVE`. The public VATSIM
HTTP cache also polls only while at least one `LIVE` session exists. AMAN identity is the
normalized callsign within each airport; CID is not part of AMAN state or
commands, and AMAN never creates a synthetic CID. A callsign change is therefore
treated as the old flight disappearing and a new callsign appearing. The
`/test` VATSIM scenarios below are a separate replay path; they are not needed
for EuroScope Sweatbox or Playback operation.

The callsign-identity database migration intentionally clears existing AMAN
airport state and coordination requests. Pre-migration AMAN sessions are not
restored; connected sources rebuild the current projection after startup.

### Offline VATSIM / AMAN replay

The same `/test` page can drive saved VATSIM v3 generation JSON files through
the real VATSIM normalizer, AMAN observation worker, reconciliation path, and
authenticated frontend WebSocket. It never starts the public VATSIM feed while
test tools are enabled. From a PowerShell terminal:

```powershell
Set-Location .\backend
$env:ENVIRONMENT = "development"
$env:ENABLE_TEST_TOOLS = "true"
$env:RECORDING_PATH = "C:\vatsim-data2"
$env:AMAN_MODE = "shadow"
$env:AMAN_SOURCE_MODE = "vatsim"
$env:AMAN_ENABLED_AIRPORTS = "EKCH"
$env:ENABLE_AMAN_HOLDING_EAT_WRITEBACK = "false"
go run ./cmd/server
```

`ENABLE_AMAN_HOLDING_EAT_WRITEBACK` is the backend-owned equivalent of ALB's
`HLW` permission. It defaults to `false`. Enable it only for an authoritative
AMAN deployment that should publish confirmed holding release times to TopSky.

Open `http://localhost:8080/test`, enter the absolute directory containing the
saved `*.json` VATSIM v3 generations (for example
`C:\vatsim-data2`), and click **Load**. Use **Play**, **Pause**, **Step**,
**Reset**, and the speed selector. The files are applied in filename order;
each file is validated as JSON before loading and the recorded feed timestamp
is used as simulated time. Keep the local EuroScope plugin connected to the
intended EKCH Sweatbox session when identity/tag testing is needed.

`ENABLE_TEST_TOOLS` defaults to false. The backend refuses to start when it is
true in a `live`, `prod`, or `production` environment, and `/api/test/*` is not
registered while it is false.
