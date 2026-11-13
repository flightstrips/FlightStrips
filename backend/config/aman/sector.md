# Local EuroScope sector-file navigation

The backend can read a local EuroScope `.sct` file to supply AMAN navigation
when an online provider cannot resolve the current flight plan. This includes
vIFF returning an older path after a controller amendment, and supports local
Sweatbox and Playback callsigns absent from vIFF. EuroScope still supplies the
flight plans and positions through `AMAN_SOURCE_MODE=euroscope`.

For automatic fallback, keep `NAVIGATION_SOURCE=airacnet` or
`NAVIGATION_SOURCE=viff` and set `NAVIGATION_SECTOR_FILE` to an absolute path
readable by the backend process. The configured provider is tried first. If its
catalog is unavailable, an existing current terminal manifest is retained. With
an empty database, the sector file and bundled terminal configuration can build
the manifest locally. Failed route resolution falls back to the sector file,
with `sector` provenance persisted on the route.

With vIFF as the primary, a successful HTTP response can still contain an old
route. When the sector file covers the current filed route, AMAN compares its
geometry with vIFF's selected path, allowing equivalent airway expansion and
provider departure prefixes. A changed waypoint, airway path, or DCT shortcut
uses locally resolved geometry for that flight until vIFF catches up. Other
flights continue using vIFF. The comparison runs on every normal vIFF route
refresh, so repeated stale responses cannot restore the old route. A current
flight-plan change requests its own route immediately, using the existing
paced bulk response; it does not create extra polling or change CDM's feed.

If the file cannot resolve the entire route, explicit filed waypoints and DCT
segments are still checked against the online path. A proven mismatch with no
usable local resolution fails for that flight. Geometry outside the file's
coverage cannot be fully verified against airway labels alone.

For the primary-plus-fallback setup:

```powershell
$env:NAVIGATION_SOURCE = "viff"
$env:NAVIGATION_SECTOR_FILE = "C:/path/to/Copenhagen.sct"
```

Catalog refresh runs at startup and every six hours. A successful AIRAC.NET
refresh restores its manifest. While the active manifest comes from a sector
file, AIRAC.NET route parsing is skipped because that provider cannot serve a
local dataset revision. vIFF can use either manifest and continues checking its
live routes at the usual interval.

To use local navigation immediately, without online navigation requests, run
from `backend/` with these PowerShell variables, replacing the example path:

```powershell
$env:AMAN_SOURCE_MODE = "euroscope"
$env:NAVIGATION_SOURCE = "sector"
$env:NAVIGATION_SECTOR_FILE = "C:/path/to/Copenhagen.sct"
$env:NAVIGATION_TERMINAL_GEOMETRY_PATH = "config/aman/ekch-terminal-2611.json"
go run ./cmd/server
```

The existing AMAN mode, enabled-airport and plugin connection settings still
apply. Other enabled features, such as live weather, have their own settings.
Restart the backend after changing the sector file or its configuration.

## Geometry and coverage

The reader imports named fixes, VORs, NDBs, airport reference positions and
segments in `HIGH AIRWAY` and `LOW AIRWAY`. It reads hemisphere-prefixed SCT
degrees/minutes/seconds coordinates and supports named airway endpoints.
Multi-word visual reporting labels are ignored instead of being truncated into
flight-plan waypoint identifiers. Map and SID/STAR drawing layers are not
treated as published procedure semantics. The existing terminal configuration
supplies STAR continuations, runway thresholds, approaches and holding data.

The file's declared AIRAC must match the terminal configuration. Its effective
dates come from that configuration and must cover the calculation time. Expired,
future or mismatched data cannot supply fallback geometry. A fingerprint of the
file identifies its dataset revision. The file stays local and is not uploaded
or included in the repository or release image.

Routes must contain a continuous section of at least two resolved points ending
at the selected terminal join. DCT and consecutive named fixes, airway segments
present in the file, and ITEM15 speed/level tokens are supported. Unknown or
ambiguous fixes and procedures, unsupported route tokens, and disconnected or
ambiguous airway paths remain explicit gaps. Known sections after a gap are
retained with partial coverage; no leg crosses the missing span. Airway
expansion resumes only from a known filed entry to a known filed exit with one
verified path through actual file segments.

Using a section after a gap requires three distinct, fresh position reports
consistent with that section, track and plausible motion. A gap ahead, ambiguous
position or aircraft outside the route corridor blocks recovery. An explicit
active direct-to can bypass a gap only when its downstream continuation is
verified. Merely being near a later fix does not authorize a direct clearance.
If there is no verified inbound filed leg at the terminal join, or its coordinates
disagree with the cached terminal, sector fallback remains unavailable. The
existing basic terminal estimate may still be used while route recovery is
pending or unavailable.

When online geometry matches all locally resolved sections and the explicit
filed waypoint order, it remains preferred despite gaps in local coverage.
Coverage is regional; this is not a worldwide database or an airway restriction
validator.

## Validation and deployment

Synthetic fixtures cover offline startup, provider failure, stale successful
vIFF responses during controller amendments, primary recovery, route expansion,
terminal composition, cache retention and restart. To check a locally installed
file against the bundled Copenhagen terminal configuration:

```powershell
$env:FLIGHTSTRIPS_TEST_SECTOR_FILE = "C:/path/to/Copenhagen.sct"
go test ./internal/aman/navdata/sector ./internal/navigation -count=1
```

Integration tests require Docker for their isolated PostgreSQL database. The
local-file acceptance tests are skipped when the variable is unset.

Production uses `flightstrips/infrastructure`'s `stacks/flightstrips.yml`.
Enabling fallback there requires a released backend image with this adapter,
the environment variables in that stack, and a readable sector-file mount on
Swarm nodes eligible to run the backend. A local Windows path is not accessible
inside a production container. Supply the file separately rather than bundling
it in this repository or the container image.
