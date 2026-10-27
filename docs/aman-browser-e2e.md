# AMAN running-system regression gate

`AMAN running-system E2E` is a pre-merge GitHub Actions check. It runs on
every pull request (including stacked PRs), main push and merge queue entry,
without path filters, conditional job skipping or a release-stage dependency.
The [main-branch ruleset](https://github.com/flightstrips/FlightStrips/rules/300063)
requires this exact check name alongside **AMAN publisher-consumer contract**.
The existing fast contract, unit,
transport and component tests remain useful and mandatory; they do not replace
this browser suite.

## Run locally

Use Docker with Linux containers, Go 1.27, Node 26.11, CMake 3.24+, Ninja,
Git and a C++20 compiler. On Windows, run from an x64 Visual Studio Developer
PowerShell or Developer Command Prompt. Ports 4180 and 4181 must be free.
The suite deliberately refuses to reuse a running server.

```sh
cd frontend
npm ci
npx playwright install --with-deps chromium
npm run test:e2e:aman:mutation
npm run test:e2e:aman
```

The mutation run is expected to **report a killed mutation and exit zero**.
An unrelated startup, compilation, native decoder or browser failure does not
count as detecting the regression. The normal run must then pass.

The first native build fetches the same protobuf v26.1 runtime as the plugin;
subsequent builds reuse it. Dependency installation can need network access.
Scenario execution uses local fixtures and has no live aviation, weather,
identity or font dependencies. Do not run the mutation and normal commands
concurrently: the mutation temporarily edits the consumer source and restores
its original bytes in a `finally` block.

## What runs

Playwright owns two child processes: a Go executable that assembles the real
application with `app.Build`, and a server for a fresh production frontend build.
PostgreSQL 16 runs in a disposable Testcontainers container. The existing
test-database helper applies all real migrations and clones an isolated database.
Only the airport bootstrap row is inserted directly; controller identity,
session, active runways and observations enter through their real protocols.
The `/e2e/persisted` endpoint reads the production PostgreSQL repository; it
cannot set flight state or command outcomes. Test control endpoints are compiled
only into `internal/testing/amanbrowser`, never `cmd/server`.

The suite uses the real VATSIM HTTP decoder and observation worker, AIRAC.NET
adapter and navigation materializer, prediction/filter/lifecycle/sequence
services, repositories, frontend and EuroScope hubs, WebSocket serializers,
browser WebSocket client, Zustand store, application router and AMAN/TMT
components. A EuroScope client authenticates, logs in, waits for the actual
master-role message and sends its binary protobuf sync. A second real protocol
connection establishes an observer. Identity verification and the Auth0 SDK
cache are fixtures at the external identity boundary; server-derived FMP and
observer authorization are exercised normally.

External HTTP calls are redirected to a local fixture server by a transport
installed only in the test executable. Unknown live hosts are rejected.
Synthetic route/fix responses pass the production materialization and activation
checks against the committed EKCH terminal geometry. Open-Meteo returns a
controlled unavailable response, exercising supported weather fallback. The
application, navigation adapter/materializer, weather adapter and command
handler share a replay clock fixed to 26 October 2026. The browser clock uses
the same date. Timeout transitions advance that clock; assertions poll actual
events or UI conditions, with no hard sleeps or retry-to-green setting.

## Assertions and evidence

The scenario checks:

- Three actual visible aircraft targets, populated TMT buckets, weather
  warnings, healthy shared readiness and allocated runway slots.
- A speed discontinuity in CCA877Q's inbound observations that the real
  prediction filter rejects. Advancing past its two-minute timeout generates
  `prediction_unreliable`; SAS101 and SAS202 remain visible with slots and
  commands remain available.
- A browser arrival-rate action that changes actual slot spacing and persists
  the runway policy, plus a server-confirmed persisted EAT-writeback setting;
  stale revisions and observer commands are rejected without effects.
- A real HTTP source outage that retains targets/TMT while disabling commands;
  attempts to bypass the disabled UI are also rejected by the backend.
- Fresh source and aircraft observations restoring readiness and guidance.
- Unsupported warning entries and a malformed warning collection injected into
  otherwise genuine backend frames by a transparent WebSocket proxy. The
  browser keeps valid targets and TMT, shows an unavailable-diagnostics notice,
  and clears it after a valid replacement.
- Older genuine frames delivered after a new revision cannot undo the confirmed
  setting. A real socket closure triggers reconnection and repairs the view from
  persisted state.
- Binary EuroScope gain/loss frames captured from the live socket are read by
  the production C++ `AMANGainLossStore` and generated protobuf decoder.
  Healthy neighbours keep complete native guidance, the unreliable aircraft
  loses its fresh guidance, recovery restores it, older revisions are ignored,
  and the decoder accepts a new revision stream after reconnect.

`frontend/aman-e2e-artifacts/` contains backend/frontend logs, an HTML report,
captured WebSocket JSON and protobuf frames, native decoder output, and browser
console errors. Failures retain screenshots, video and traces. CI uploads these
even after a failing step. To inspect a failure:

```sh
npx playwright show-trace aman-e2e-artifacts/results/<test-directory>/trace.zip
```

The controlled mutation reproduces the original outage's observable behavior:
a valid per-flight warning makes the consumer reject the complete operational
replacement. The browser must fail specifically because the healthy SAS101
target disappeared. The original logged incident fixture
`backend/pkg/events/frontend/testdata/aman-filter-2026-10-26.json` remains in
the fast regressions. It records prediction outputs rather than all original
inbound aircraft facts; this E2E uses controlled physical observations to create
the same failure condition through the running engine.

## Coverage limits

This is a Chromium, single-backend, single-session regression using synthetic
aviation inputs and the real bundled EKCH geometry. It does not validate live
provider availability/data quality, Auth0 login/refresh, multi-node failover,
every aircraft/performance model, every runway/holding, or every AMAN command.
It checks the EAT-writeback setting's persistence, not TopSky's transient EAT
pulse inside a controller installation. The production C++ gain/loss decoder
runs as a standalone executable: EuroScope itself, its SDK callbacks, tag
painting, tracking ownership and native TopSky runtime are outside this test.
The existing Windows plugin tests cover additional native behavior.
