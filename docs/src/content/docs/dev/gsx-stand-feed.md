---
title: GSX stand feed
description: The unauthenticated GET endpoint that publishes a callsign's stand to a pilot's simulator.
---

`GET /api/gsx/stand` publishes the stand a callsign currently holds so that GSX,
running in the pilot's simulator, can select that parking without the pilot
doing anything. It is disabled by default; set `ENABLE_GSX_STAND_FEED=true` to
expose it.

## Why it is unauthenticated

GSX's scripting API can only issue a plain HTTP GET. Its one network call is
`fetchJson(url, timeout, etag)`, which sends no headers, so the
`Authorization: Bearer` token every other web API depends on cannot reach it.
Requiring a token would mean every pilot pasting one into a Python file before
their first flight.

The endpoint is therefore public, and kept safe by disclosing as little as
possible rather than by authenticating:

- It answers only `{ "stand": ..., "revision": ... }`. No route, no CID, no PDC
  state, no flight plan, no aircraft type.
- It reads `LIVE` sessions only. A sweatbox or playback stand never reaches a
  pilot's simulator.
- An unknown callsign, a callsign with no strip, and a strip with no stand are
  all the same answer: `{ "stand": null }`. The endpoint does not reveal whether
  a callsign is known to FlightStrips.

Callsign and position are already published on the VATSIM datafeed, and the
stand is passed to the pilot over the air, so the feed discloses nothing that is
not already available.

## Request

| Parameter | Required | Notes |
| --- | --- | --- |
| `callsign` | yes | Alphanumeric, up to 12 characters. Normalised to upper case. |
| `icao` | no | 3-4 characters. Restricts the lookup to the LIVE session at that airport. |

```
GET /api/gsx/stand?callsign=SAS1401&icao=EKCH
```

```json
{ "stand": "A12", "revision": "A12" }
```

`400` for a missing or malformed `callsign` or `icao`. `503` when the lookup
fails, which tells the script to keep its current stand and retry. Everything
else is `200`.

## revision and ETag

`revision` is the value the handler script compares between polls, and the
response carries it as an `ETag` so the script can poll with
`fetchJson(url, etag=True)` and get a zero-byte `304 Not Modified` when nothing
has changed.

`revision` is the stand itself, deliberately, not `strips.version`. A strip's
version changes for reasons that have nothing to do with parking, and each of
those would otherwise look like a reassignment and make the script re-select the
stand it is already on.

## Source of truth

The endpoint reads `strips.stand`, which is the authoritative operational value:
SAT's automatic allocation and a controller's manual override both write there.
It does not read `stand_assignments`, so it needs no knowledge of SAT stages,
provenance or acknowledgement, and it keeps working when
`ENABLE_STAND_ASSIGNMENT` is false and stands are being set by hand.

Nothing is written back. A pilot who parks somewhere else is something the
controller should see and resolve on the board, not something the simulator
silently overwrites.

## Identity

FlightStrips keys strips on `(session_id, callsign)` and holds no registration
used for matching, so the script has to reproduce the callsign the pilot
connected to the network with. It tries three sources in order:

1. **SimBrief** — `getSimbrief().callsign` is the ATC callsign from the filed
   plan, the same string the pilot gives vPilot. GSX itself falls back to
   `icao_airline + flight_number` when the plan omits it. This is the reliable
   path, and most pilots on the network file a SimBrief plan.
2. The livery's `aircraft.icaoAirline` joined to the `ATC FLIGHT NUMBER` SimVar.
3. `ATC ID`, the tail number, for anyone flying without either.

A pilot the script cannot identify gets `{ "stand": null }` and no stand — the
same as not installing the script at all. It fails quiet, never wrong.

GSX loads SimBrief data when the aircraft is parked with engines off and the GSX
menu is opened, and the plan stays loaded for the session, so it is available in
`onEnterAirport` on arrival. Note that GSX only accepts a plan whose aircraft
type matches the loaded aircraft and whose ETD has not passed, unless the pilot
enables **Simbrief Ignore Time** in GSX settings.

## Client

The handler script lives in `gsx-client/`. It is named after the GSX `.ini`
profile it accompanies, which binds it to one airport and one scenery, and is
distributed alongside that profile.
