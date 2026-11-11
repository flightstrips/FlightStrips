# vIFF routes during the AIRAC.NET outage

Set `NAVIGATION_SOURCE=viff` on the backend to use
`GET https://api.viffsys.com/ifps/arrAirport?airport=EKCH&path=true` for live
arrival routes. The existing `CDM_KEY` is attached when configured; no controller
subscription or plugin update is required. `VIFF_API_BASE_URL` optionally
overrides the default API origin for isolated replay/testing.

Production is deployed by Portainer from
[`flightstrips/infrastructure`'s `stacks/flightstrips.yml`](https://github.com/flightstrips/infrastructure/blob/main/stacks/flightstrips.yml),
not this repository's legacy `docker-compose.prod.yml`. Set the provider in that
stack and deploy a released backend image containing this implementation. Keep
the API and migrator image versions in lockstep; a configuration-only change
against an older backend image cannot enable vIFF support.

AMAN and the existing CDM inbound-arrivals poller share the same bulk response.
There is at most one request per airport per 30 seconds **per backend process**,
including concurrent flights and sessions. Requests are demand-driven, failed
requests are paced too, and longer `Retry-After` values are respected by both
consumers. There are no callsign/profile requests or flight-plan creation calls.
The existing departures polling remains separate.

This mode needs an existing, validated terminal manifest in the backend's
PostgreSQL navigation cache, compatible with the terminal configuration and
currently effective. It validates/rebuilds the terminal fragment locally on
startup and does **not** refresh fixes, procedures or cycles from AIRAC.NET.
Preserve the existing database when deploying; do not deploy against an empty
navigation cache. An empty, corrupt, expired or incompatible cache leaves
navigation unready with a diagnostic, rather than fabricating terminal geometry.
The bundled configuration's cycle 2611 expires on 26 November 2026; this mode
does not obtain the next terminal dataset automatically.

Each route must match the flight's callsign, origin and destination, and reach
a configured arrival entry at the cached coordinates. A filed downstream arrival
fix such as TNO can be the join point instead of the STAR's first fix; vIFF's
compatible path is retained up to that join.
Parenthetical provider qualifiers are stripped from all waypoint identifiers,
so `ERNOV(AR2)` becomes `ERNOV`. The arrival join point must still pass the
cached-coordinate check. vIFF supplies the enroute path; differences from the
filed waypoints do not reject otherwise usable geometry. If the controller
assigns a STAR with a different entry in EuroScope, the last enroute
point is joined directly to the new entry using cached local coordinates; vIFF
does not need to update its previous STAR selection first. The old entry and
terminal continuation are discarded. AMAN retains its selected
terminal continuation, aircraft/wind model and timing calculations; vIFF's speed,
altitude and ETA are not prediction inputs. `T_O_D` and `T_O_C` profile markers
are filtered out before coordinate validation, co-located points are collapsed,
and unusable paths are rejected per flight. Flights absent from the bulk feed
wait for vIFF processing. Usable geometry for the same filed route can
continue from the canonical cache during a vIFF outage.

For controller flight-plan amendments that vIFF has not processed yet, configure
`NAVIGATION_SECTOR_FILE` alongside `NAVIGATION_SOURCE=viff`. This enables a local
route fallback even when the bulk endpoint is healthy: a path that conflicts
with the current filed geometry uses the sector resolution until vIFF matches
again. The filed-route acceptance described above applies to vIFF without this
fallback; with the sector file configured, available local geometry is used to
check it. See [sector.md](sector.md) for coverage limits and setup.

Bulk arrivals does not supply the filed route revision or airway labels, and
vIFF may lag the VATSIM/EuroScope flight plan. Live routes are checked again every
30 seconds; TMA-frozen routes retain their existing geometry.

Before enabling this mode in production, verify the current EKCH manifest is
readable, deploy with `NAVIGATION_SOURCE=viff`, and confirm a newly observed
arrival obtains a route with `viff` provenance. The navigation integration test
`TestVIFFStartupRoutesAndRestartWithoutAIRACNet` exercises cached startup, shared
CDM/AMAN arrivals, canonical route persistence, terminal continuation and restart
while all external HTTP except the mock bulk endpoint is blocked.
