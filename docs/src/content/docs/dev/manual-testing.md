---
title: Manual testing launcher
description: Launch a worktree-local backend, frontend, development plugin, and EuroScope/FSD test session on Windows.
---

Run these commands from the repository root in **PowerShell 7**:

```powershell
pwsh ./scripts/manual-test.ps1 init
# Edit scripts/manual-test.local.json for this machine.
pwsh ./scripts/manual-test.ps1 start -Scenario ekch-twr-22 -OpenBrowser
```

You can also pass a scenario file directly, without adding a preset to the JSON:

```powershell
pwsh ./scripts/manual-test.ps1 start -Scenario "./scenarios/ekch-twr-Rwy22.txt"
pwsh ./scripts/manual-test.ps1 reset -Scenario "./scenarios/ekch-twr-Rwy04.txt"
```

`-ScenarioFile` is an alias for `-Scenario`. Direct relative paths resolve from
your current directory; relative file paths in presets resolve from the JSON
config's directory. A filename containing `Rwy22` selects arrival runway **22L**
and departure runway **22R**; `Rwy04` selects **04L** and **04R**. Explicit runway
settings in a preset take precedence. Other filenames keep the copied profile's
runway selections unless a preset supplies them. Both FSD and EuroScope receive
the same absolute file path. Missing files are rejected before `reset` stops
an existing test session.

The example leaves installation paths empty. Fill them in only in the
gitignored `scripts/manual-test.local.json`; do not put personal paths,
container names, or credentials in the shared example or documentation.
Set the EuroScope and FSD executable paths,
the complete EuroScope data folder, the current EuroScope `.prf`, the GRplugin stand and aircraft references, the ICAO
airlines file, and the paths to your installed scenarios. `DefaultScenario`
selects the scenario used when `-Scenario` is omitted. Set `Callsign` and
`Facility` to the controller position being tested. `PluginUserConfig` can be
empty for a fresh development login, or point to an existing **development**
`flightstrips_user.ini`; do not use production authentication tokens.
Each preset can specify `File`, `ArrivalRunways`, and `DepartureRunways` for
EKCH, as in the example config. A plain file-path string is also accepted;
runway direction is inferred from `Rwy04`/`Rwy22` when present in the filename.

Prerequisites: Docker Desktop running, Go (automatic toolchain selection enabled),
Node.js as specified by `frontend/package.json`, CMake, and Visual Studio 2022
with the C++ workload and Windows SDK. The launcher selects the Win32 compiler
itself; an x86 developer shell is not required. EuroScope's FSD licence must be
configured once; a first-run licence dialog is shown in the FSD window.
Optionally set `NodeDirectory` to a portable Node installation directory; it is
prepended to PATH only for the launcher and its workers.

Every new launch copies the **complete EuroScope data folder** (including all
sector files, settings, plugins, and data) and the **complete installation
folder** into `.manual-test/euroscope/current/data/` and `current/app/`. It launches
the copied executable with the copied current profile. `EuroScopeDataDirectory`
is the source data folder, and `BaseProfile` must be inside it. If FSD is
installed separately, its entire installation folder is copied too. Existing
snapshots are retained under `euroscope/history/`; `reset` archives the entire
previous copy and creates a fresh whole-folder copy at the same `current/` path.
Sector updates are picked up automatically. The stable executable paths let
Windows reuse its existing firewall decisions instead of treating each launch
as a new application. The first launch from this new path may still prompt;
the launcher does not change firewall rules or approve network permissions.

The launcher then applies the local test settings **inside that copy**: localhost
connection, non-VATSIM simulation, synthetic FSD credentials, selected runways,
and the current development plugin/config. Absolute references to the source
data and installation directories are rebased to the copied directories. The
selected scenario is referenced by its exact canonical absolute path in both
EuroScope and FSD; the launcher never gives them separate scenario copies.
Startup and status compare both configured paths and the selected file hash,
and readiness fails if either path differs or the file changes. The local
config selects the TWR intermediate runway-22 or runway-04 scenario. Supporting plugins
use their copied files. The original folders are not changed.
Path rebasing handles Windows capitalization and separator differences. Runway
writes must resolve inside the copied data folder and cannot cross symlinks or
junctions. If a profile references an external sector file, put its sector files
inside `EuroScopeDataDirectory` and update the source profile before launching
with scenario runway overrides; the launcher rejects external runway writes.

It builds both FlightStrips DLLs from the current checkout, including local
changes, stages them in `.manual-test/plugin/`, then deploys them and the runtime
DLLs to the copied data folder's `EKDK/Plugins/FlightStrips-Manual-Test/`. The
copied profile loads the core DLL directly so the production updater cannot
replace the build being tested. `status` prints the exact copied profile path
and verifies the deployed files against their recorded hashes. Change the local
JSON to make repeatable test adjustments; manual changes made inside a snapshot
stay in that snapshot and are not carried into the next fresh copy.

An isolated Docker Compose project and database volume are named for the
worktree. The database listens on loopback port **55432** by default; existing
development databases are not migrated or reset. Migrations and the backend
run from this checkout. Backend and frontend default to **8090** and **8080**;
change these in the local JSON if occupied. The frontend refuses a busy port
and exposes a development-only run identity checked by the launcher.
Authentication callback URLs must allow the chosen frontend port. If another
Docker application occupies the required port, `SuspendContainers` can list
the exact container names you explicitly want stopped during testing. Only
containers that were running are recorded and restarted by `stop`/`reset`.

The launcher starts FSD with the selected scenario, waits for its listener,
then opens the test EuroScope profile. In EuroScope, click **CONNECT**, confirm
**localhost**, **Connect to VATSIM unchecked**, and **Start simulator session**,
then click **Connect**. The scenario field must show the same full path as the
FSD Scenario tab. Select the primary frequency for the configured position
(for example **118.105** for `EKCH_A_TWR`). Sign in to the development FlightStrips plugin
and frontend when needed. Authentication remains the normal development flow.

The launcher waits up to 120 seconds for the configured controller in the EKCH
SWEATBOX session and strips observed by EuroScope in that session since this
launch. The observation boundary comes from the database clock immediately
before EuroScope starts. Retained strips from earlier launches and traffic in
other sessions do not count toward readiness. It prints **READY FOR MANUAL TESTING** only when
those checks, the backend HTTP check, the frontend run identity, and deployed
plugin hashes succeed. Readiness does not assert that every optional service
(for example AMAN navigation) is healthy, or that the frontend user has signed
in. Inspect `/healthz` for optional service readiness. A connection timeout exits
with code 2 and leaves the environment running so setup can be completed.

```powershell
pwsh ./scripts/manual-test.ps1 status
pwsh ./scripts/manual-test.ps1 stop
pwsh ./scripts/manual-test.ps1 reset -Scenario ekch-twr-22
```

`start` reuses a launch owned by this worktree. If tracked source changed, it
requires `reset` to build and deploy again. `reset` restarts the scenario and
services with a fresh whole-folder copy but preserves the test database and
cached development login in `.manual-test/plugin/`. It does
not delete database contents. `stop` only stops recorded processes with matching
start times and this worktree's Compose project; it restores the FSD scenario
selection that existed before launch. It terminates the owned test application
processes, so unsaved changes in the test EuroScope window are discarded.

Unowned EuroScope/FSD instances and occupied application ports cause a clear
failure instead of changing a running environment. Failure preserves logs and
partial startup state in `.manual-test/`; run `status` to inspect it and `stop`
to clean up. Retrying `start` preserves pending container and FSD-setting
restoration records from an earlier failure; `stop`/`reset` clears them only
after successful restoration. Launch state created before observation tracking
was added requires `reset` before it can report ready.
`BackendEnvironment` adds test-specific backend feature settings;
the launcher always uses its own database, normal authentication, loopback HTTP,
and disables the public VATSIM and transceiver feeds. Other external services,
such as authentication and navigation, can still use the network.

Run the focused launcher regressions without starting any services:

```powershell
pwsh ./scripts/tests/manual-test.Tests.ps1
```

Optionally add `-DatabaseContainer NAME` to exercise readiness SQL against an
existing PostgreSQL container with the local `fs` user and `fsdb` database.
These checks use temporary tables inside a rolled-back transaction; application
data is unchanged.
