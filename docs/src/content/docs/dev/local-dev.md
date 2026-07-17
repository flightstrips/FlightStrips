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

All other keys (`authority`, `redirectPort`, `enabled`) stay the same as production. Full reference: `src/config_dev.ini` (dev), `src/config.ini` (prod).

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

`ENABLE_TEST_TOOLS` defaults to false. The backend refuses to start when it is
true in a `live`, `prod`, or `production` environment, and `/api/test/*` is not
registered while it is false.
