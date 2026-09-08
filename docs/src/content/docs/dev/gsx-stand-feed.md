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

- It answers only `{ "stand": ..., "pushback": ..., "revision": ... }`. No route,
  no CID, no PDC state, no flight plan, no aircraft type.
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
| `scenery` | no | Which add-on the pilot is running. Without it the answer uses the controller's own stand name and carries no pushback point. |

```
GET /api/gsx/stand?callsign=SAS1401&icao=EKCH&scenery=Simnord-Sonnich
```

```json
{ "stand": "Gate A31", "pushback": "Z2 Face E", "revision": "Gate A31|Z2 Face E" }
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

## Gate and scenery configuration

The same physical gate is not the same object in every add-on. Developers number
stands differently and each authors their own pushback route names, so what a
controller calls `F91` is `Parking 91` in Simnord's EKCH, and "push onto Z2
facing east" is a string that only exists in that one profile. Nothing resolves
this centrally, so it is declared per airport in
`backend/config/<icao>/gsx_sceneries.json`:

```json
{
  "icao": "EKCH",
  "gates": {
    "A31": {
      "Simnord-Sonnich": {
        "stand": "Gate A31",
        "points": { "Z/L": "Z2 Face E", "Y/L": "Z3 Face W", "K/J": "J1 Face S" }
      }
    }
  }
}
```

Read it as **gate → scenery → what that scenery calls it**:

- The **gate key** is the stand as controllers know it — the value in
  `strips.stand`.
- The **scenery key** is whatever the handler script sends as `scenery`.
- `stand` is the name handed to `selectGate()`. Omit it when the scenery uses
  the controller's own name.
- `points` maps a FlightStrips release point to a GSX pushback label.

Everything is optional and everything degrades quietly. A missing file, an
unknown scenery, an unmapped release point: the feed still publishes the stand,
just without translation or a pushback point. Lookups ignore case and extra
spacing, which matters because scenery labels are hand-typed — one EKCH profile
contains both `Y1 Face E` and `Y1  Face E`.

### Generating a starting file

`tools/gsx-scenery-skeleton.mjs` reads a GSX `.ini` and emits the gates, the
stand names, and the pushback labels that profile actually offers:

```bash
node tools/gsx-scenery-skeleton.mjs \
  "%APPDATA%/Virtuali/GSX/MSFS/EKCH-Simnord-Sonnich.ini" \
  Simnord-Sonnich EKCH \
  backend/config/ekch/GRpluginStands.txt \
  > backend/config/ekch/gsx_sceneries.json
```

Given the SAT stand list it also reconciles the two vocabularies, rewriting the
scenery's `[parking 89]` to the controller's ident where that is unambiguous. It
never guesses: gates it cannot resolve are written with a `review` note and
listed on stderr, and `points` is always left empty. Which physical route a
controller means by `R/W` is local knowledge no file on disk contains.

For the shipped EKCH profile that is 119 gates, 87 reconciled automatically and
32 needing a decision.

## Pushback points

`getGate().pushback`, `pushbackLabels` and `pushbackAddPos` are all writable at
any time, so the handler can narrow the pushback menu to the assigned route:

- An **extra slot** defined by the profile: keep only that entry in
  `pushbackAddPos` and set `pushback = 0`.
- One of the **two defaults**: `pushbackLabels` is left then right, and the
  direction enum is `1` for left, `2` for right.

**GSX has no `selectPushback()`.** A script cannot answer the menu on the
pilot's behalf; it can only remove the routes that were not assigned, leaving
the assigned one as the only routed choice. GSX always offers Straight Pushback
and Pull Straight regardless of the parking preference, so the menu does not
disappear — the pilot still confirms.

The route only means anything once the aircraft is on the assigned stand, and
`selectGate` is deferred by a cycle, so the script applies the stand first and
the pushback on a later poll.

## Client

The handler script lives in `gsx-client/`. It is named after the GSX `.ini`
profile it accompanies, which binds it to one airport and one scenery, and is
distributed alongside that profile. Set `SCENERY` in it to match the scenery key
in `gsx_sceneries.json`.

## Adding this to your GSX profile

If you build and publish GSX profiles, you can ship stand assignment with the
profile you already distribute. Pilots install it the same way they install
everything else — copy the files into `%APPDATA%\Virtuali\GSX\MSFS\` — and there
is nothing for them to configure.

### Naming

GSX loads an airport handler only when its filename is the active `.ini`
profile's name plus `_handler`. The stem must match exactly:

| Your profile | Your handler script |
| --- | --- |
| `EKCH-Simnord-Sonnich.ini` | `EKCH-Simnord-Sonnich_handler.py` |
| `ekbi-simnord24.ini` | `ekbi-simnord24_handler.py` |
| `EGKK-MyScenery.ini` | `EGKK-MyScenery_handler.py` |

That binding is what makes the script scenery-specific. It loads only for the
scenery whose profile is active, so the stand names it resolves are the ones
that scenery actually has — you do not need to worry about another developer's
numbering for the same airport.

### A complete working script

Copy this, rename it to match your profile, and set `API_BASE`. It is the whole
feature in about fifty lines; the script in `gsx-client/` is the same thing with
more logging and edge-case handling.

```python
# -- coding: utf-8 --
# Stand assignment from FlightStrips. Python 3.7.
# Save next to your .ini as <profile name>_handler.py

API_BASE = "https://flightstrips.example.org"
POLL_INTERVAL_MS = 30000
POLL_LIMIT = 240


def _standReadCallsign(self):
    """The callsign the pilot filed with, from their SimBrief plan."""
    try:
        sb = getSimbrief()
        if sb is not None and not sb.last_error and sb.callsign:
            clean = "".join(c for c in sb.callsign.upper() if c.isalnum())
            return clean[:12]
    except Exception as err:
        print("[stands] SimBrief unavailable: %s" % err)
    return ""


def _standCheck(self):
    """Fetch the current stand and select it if it has changed."""
    if not self._standCallsign:
        return

    airport = getAirport()
    payload = fetchJson("%s/api/gsx/stand?callsign=%s&icao=%s"
                        % (API_BASE, self._standCallsign,
                           airport.icao if airport else ""),
                        timeout=8, etag=True)

    # None is a network error, and an unchanged stand costs a 304. In both
    # cases there is nothing to do until the next poll.
    if not payload or payload.get("revision") == self._standRevision:
        return

    stand = payload.get("stand")
    if not stand:
        self._standRevision = payload.get("revision")
        return

    result = selectGate(stand)
    if isinstance(result, list) and result:
        result = selectGate(result[0])   # ambiguous name: take the first match
    if result is True:
        self._standRevision = payload.get("revision")
        showMessage("Stand %s assigned" % stand)   # visible with the menu open


def onEnterAirport(self):
    self._standCallsign = _standReadCallsign(self)
    self._standRevision = None
    self._standPoll = None

    if getGate() is not None:
        return          # the pilot already picked a stand - leave them alone

    _standCheck(self)

    def loop():
        for _ in range(POLL_LIMIT):
            truewait(POLL_INTERVAL_MS)   # wall clock, unaffected by sim rate
            _standCheck(self)

    self._standPoll = runAsync(loop)


def onGateReset(self, reason):
    """The pilot chose their own stand. Stop, for the rest of this visit."""
    if reason in ("user_changed", "user_revoked"):
        cancelAsync(getattr(self, "_standPoll", None))
        self._standPoll = None
```

### What to check before publishing

- **Set `API_BASE`** to the FlightStrips instance for your vACC, over HTTPS.
  Pilots never edit this — it ships already set.
- **Test with the profile active.** If the filename stem is wrong the script is
  silently not loaded; GSX will not warn you.
- **Watch the GSX Handler Editor output panel** on your first run. The script
  prints there, and `F5` reloads it without restarting the sim.
- **Confirm your stand names resolve.** `selectGate` matches the BGL name and
  number first (`Gate A12`), then the UI name, then a suffix (`A12` matches
  `Gate A12`). If your scenery numbers stands unusually, check that the strings
  controllers type in FlightStrips actually resolve in your profile.

### What pilots see

Nothing, unless they have a stand. A pilot with no strip, no assignment, or no
SimBrief plan gets one HTTP request on arrival and no further activity — GSX
behaves exactly as it does without the script. It never overrides a stand the
pilot picked themselves, and it stops for good the moment they change one.
