---
title: Local development (Windows)
description: Toolchain, run commands, and wiring for backend, frontend, EuroScope plugin, and docs on Windows.
---

| Part | Platform |
| --- | --- |
| Backend | Windows — Docker and/or Go; Postgres required |
| EuroScope plugin | Windows x86 (Win32) — MSVC |
| Frontend | Node 22.x wherever Node runs (`frontend/package.json` `engines`) |
| Docs | Node (Starlight / Astro), same as frontend |

Install **Docker Desktop**, **Go** (matching `backend/go.mod`), **Node 22**, and **Git**. For the plugin, use **Visual Studio 2022** (Desktop development with C++) or **Build Tools** plus **CMake** and **Ninja**. CI builds **Win32** (`cmake` fails on x64 targets); open an **x86** MSVC dev shell when building so the toolchain matches EuroScope’s 32-bit host.

## Backend

From `backend/`, bring up the full stack (API on **8090**, Postgres, migrator, Aspire dashboard):

```sh
docker compose --profile all up --build -d
```

For API work on the host binary, run only Postgres (and the one-shot migrator via compose):

```sh
docker compose --profile database up --build -d
```

Point `DATABASE_CONNECTIONSTRING` at `localhost:5432` (see `.env` / `Readme.md`), then from `backend/`:

```sh
go run ./cmd/server
```

The HTTP/WebSocket surface matches what the frontend and plugin expect on **8090** in dev.

## Frontend

From `frontend/`:

```sh
npm ci
npm run dev
```

Vite loads `public/config.js`; default `wsUrl` is `ws://localhost:8090/frontEndEvents`. Change it if the API listens elsewhere.

## EuroScope plugin

Code lives in `euroscope-plugin/`. The **loader** target produces `build/bin/FlightStripsPlugin.dll`; **Debug** post-build copies `src/config_dev.ini` to `build/bin/flightstrips_config.ini` (dev defaults include `ws://localhost:8090/euroscopeEvents`). **Release** follows the CI layout (`flightstrips_config.ini` from prod `config.ini`, plus a `flightstrips_config_dev.ini` sidecar).

From `euroscope-plugin/` in an **x86** MSVC environment:

```sh
cmake -DCMAKE_BUILD_TYPE=Debug -DCMAKE_EXPORT_COMPILE_COMMANDS=ON -G Ninja -B build
cmake --build build
```

Swap `Debug` for `Release` to mirror `.github/workflows/build-plugin.yml`.

Copy `FlightStripsPlugin.dll` and `flightstrips_config.ini` from `build/bin/` into your sector package’s EuroScope **Plugins** folder (same pattern as production: `%AppData%\EuroScope\<ICAO>\Plugins\`, or your pack’s equivalent). Keep any EuroScope-supplied dependency DLLs beside the plugin if your install requires it. Edit `flightstrips_config.ini` if the API host, OIDC, or airport sections differ from your machine.

Load the DLL from EuroScope’s plugin dialog. With the backend up, the plugin should negotiate the WebSocket defined under `[api] baseurl`.

## Docs

From `docs/`:

```sh
npm ci
npm run dev
```

## Wiring checks

With **backend** listening on **8090**, **frontend** dev server should show live strips once Auth0 (or your dev identity setup) and WebSockets succeed. **Plugin** traffic is independent of the Vite origin; it only needs the backend `euroscopeEvents` endpoint and matching auth config in the ini file.
