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
| `scenery` | no | Which add-on the pilot is running. Unknown or missing, the answer uses the controller's own stand name and the bare pushback point, with no scenery labels. |

```
GET /api/gsx/stand?callsign=SAS1401&icao=EKCH&scenery=Simnord-Sonnich
```

```json
// arriving at EKCH
{ "stand": "Gate A31", "pushback": null, "point": null, "target": null,
  "revision": "stand:Gate A31" }

// departing EKCH: a named point
{ "stand": null, "pushback": ["Z2 Face E"], "point": "Z2",
  "target": { "lat": 55.627587, "lon": 12.632347, "heading": null, "radius_m": 40 },
  "revision": "push:Z2:Z2 Face E" }

// departing EKCH: a direction button (a bare taxiway)
{ "stand": null, "pushback": null, "point": "W",
  "target": { "lat": null, "lon": null, "heading": 270, "radius_m": 40,
              "avoid": [{ "lat": 55.627813, "lon": 12.631736 }, ...] },
  "revision": "push:W" }
```

| Field | Departures | What it is for |
| --- | --- | --- |
| `point` | always | The release point as FlightStrips stores it: a named point (`Z2`) or a direction (`N`, `E`, `S`, `W`). |
| `target` | when the point has a position, or is a direction | Where the push should end. Handlers match it against their own routes' end positions. See [Release points](#release-points). |
| `pushback` | mapped sceneries only | The point in that scenery's labels. Kept for handlers older than `target`. |

`400` for a missing or malformed `callsign` or `icao`. `503` when the lookup
fails, which tells the script to keep its current stand and retry. Everything
else is `200`.

## revision and ETag

`revision` is the value the handler script compares between polls, and the
response carries it as an `ETag` so the script can poll with
`fetchJson(url, etag=True)` and get a zero-byte `304 Not Modified` when nothing
has changed.

`revision` is the assignment itself, deliberately, not `strips.version`. A
strip's version changes for reasons that have nothing to do with parking, and
each of those would otherwise look like a reassignment and make the script
re-apply what is already applied.

## Arrivals and departures

The two halves are never both live. A **stand** is where arriving traffic is
told to park; a **pushback point** is how departing traffic comes off the stand
it is already sitting on. They belong to opposite ends of a turnaround — often
to two different callsigns, since the inbound SAS1401 leaves again as SAS1402.

The endpoint decides which applies, using the same rule as the PDC lookup —
`strip.Origin == session.Airport` means the flight is leaving:

| Leg | `stand` | `pushback` |
| --- | --- | --- |
| arriving here | the assigned stand | `null` |
| departing here | `null` | the assigned route |

So a release point sitting on an inbound strip is never published: it belongs to
that aircraft's later departure. And a departing aircraft is never sent a stand,
because it is already parked and loading — `selectGate` would refuse anyway
while ground services are active, and moving it would be wrong even if it
worked.

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

## Release points

Every GSX profile names its pushback routes its own way, and some name them
wrongly. Two profiles for the same EKCH scenery call one route `S1 Face N` and
`Release Point S1 (NORTH)`, and between them three routes are named for a
point they don't end at. Where a route ends doesn't vary: across both
profiles, the routes to one point end within a few metres of each other.

So FlightStrips places each release point on the ground, and the handler
picks the route that ends there. This lives in `gsx_sceneries.json`, beside
the per-scenery mapping:

```json
{
  "icao": "EKCH",
  "release_point_radius_m": 40,
  "release_points": {
    "S1": { "lat": 55.626033, "lon": 12.653220 }
  },
  "facings": {
    "A14": { "Y1": "E" }
  }
}
```

| Field | Meaning |
| --- | --- |
| `release_points` | Each named point's position. Seeded from the median end position of every route to it in both EKCH Simnord profiles. |
| `release_point_radius_m` | How far from the point a route may end. It must stay below the distance between the two closest points (Z1 and Z2, 46 m), so a stand without the assigned point is never sent to its neighbour. A test enforces this. |
| `facings` | Gate, then point, then compass direction (`N` to `NW`), for a stand that leaves one point in two facings. It's an airport rule, so it applies to every profile. |
| `release_points_review` | Free-text notes on points without a position or with doubtful data. Ignored by the server. |

How the handler uses `target`:

- **Named point:** of the stand's routes ending within `radius_m` of the
  point, the nearest. With a `heading`, it takes the nearest of those within
  90° of that heading when there are any. A route is never chosen by its name
  while `target` is present. When the chosen route's name says another point,
  the pilot is shown the controller's point instead.
- **Direction:** the route whose end heading is closest, within 90°. It prefers
  routes that don't end at any point in `avoid`, so `W` means the taxiway, not
  W2.
- **No `target`** (a point with no position yet, or an older server): the
  scenery's `pushback` labels if they exist at the stand, else routes whose
  names contain the point as a word.

`gsx-client/tools/audit.js` checks a profile's `.ini` against these positions,
and reports every route whose name and end point disagree.

### Still to place

K1, L2, L3, Z8, R3 and S4 have no route in either Simnord profile, so they
have no position yet. Y3 has only two routes, 46 m apart, and its position is
one of them. Both are recorded under `release_points_review`.

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
        "points": { "Z/L": ["Z2 Face E"], "Y/L": ["Z3 Face W"], "K/J": ["J1 Face S"] }
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
- `points` maps a FlightStrips release point to every GSX route that reaches
  it. It is a list because a stand often offers the same taxiway in two
  facings; order is preferred-first, so a known default can lead. It only feeds
  the `pushback` field, for handlers older than `target`. New work belongs in
  [Release points](#release-points): one position serves every profile, where
  a label serves one. Once older handlers have been replaced, `points` can go.

Everything is optional and everything degrades quietly. A missing file, an
unknown scenery, an unmapped release point: the feed still publishes the stand
and the bare point, just without translation. Lookups ignore case and extra
spacing, which matters because scenery labels are hand-typed — one EKCH profile
contains both `Y1 Face E` and `Y1  Face E`.

### Maintaining the file

It is edited by hand. There is deliberately no generator: the file is the source
of truth rather than a derivation of one, so hand-tuned entries cannot be lost to
a regeneration.

The EKCH file was seeded by matching GSX labels against the frontend's
`RELEASE_POINTS` — the set `PushbackMapDialog` writes to `strips.release_point`.
The two vocabularies share a token, since a GSX label is
`<release point> Face <direction>`, so `Z2 Face E` is the route for a controller
who assigned `Z2`. That relationship is still the rule to follow when adding a
stand by hand.

Two fields exist only to help whoever edits it, and are ignored by the loader:

- `available` lists every route the scenery offers at that stand, so you can see
  what there is to map without opening the `.ini`.
- `review` records what is still unresolved there.

To add a stand, find its section in the GSX `.ini`, take `pushbacklabels` (the
two defaults, left then right) plus the `label` of each `pushbackaddpos` entry,
and map each to the release point a controller would assign for it. Where a
stand offers the same taxiway in two facings, list both unless local knowledge
says the stand only ever leaves one way — the pilot then picks the facing, and
cannot leave via a taxiway the controller did not name.

### What is still unresolved at EKCH

| Category | Count | What it needs |
| --- | --- | --- |
| Facing rule leaves no route | 1 | A17 always faces east, but its only `Y0` route faces west |
| Two facings published | 2 | A34 `Z1`, E20 `S1` — a local rule would narrow each to one |
| Bare taxiway | 22 stands | scenery offers "onto J/V/W"; controllers only have numbered points there |
| Stand ident unresolved | 32 | mostly E110–E137 and E15–E19, which FlightStrips does not track |
| No named routes | 1 | F95 offers only generic Nose/Tail — nothing to map |

Each one is recorded in that stand's `review` note.

## Pushback points

For departing traffic only. The handler **adds one entry** to the pushback menu,
named for the assigned point (`ATC Assigned: Z2 Face E`), and leaves every
scenery route on offer. The pilot's side:

1. Select **Prepare for Pushback and Departure**. The entry goes on the menu
   now, assigned or not. With nothing assigned yet it is a bare `ATC Assigned`
   leading to the scenery's first route.
2. Wait for the tug. GSX asks which way to push once the tug is connected.
   Calling ATC for pushback only now, with the menu on screen, is fine.
3. Pick the **ATC Assigned** entry and push.

The entry is a copy of the scenery route that reaches the point: same `pos`,
same `approach`, and the extra fields (`wws`, `snap`, `startEngines`) of a
slot the profile already defines. GSX drives geometry it already knows under
another name. When the server lists two facings of one taxiway, the entry
takes the first, and its label names that facing.

The assigned point reaches the pilot in three ways, because `showMessage` only
shows while the GSX menu is open:

| Where | When |
| --- | --- |
| Menu entry label | only if assigned before the menu is built (state 5) |
| Stand display (VDGS) | `PUSH Z2 FACE E`, from the moment it is assigned |
| GSX message | on assignment, and again once as the route menu opens (state 5) |

**Reads of `pushbackAddPos` are copies.** Changing a dict read out of it does
nothing. Flown at D2: the entry kept its old name and the push went to its old
route. So every change rewrites the whole list, with only the entry replaced,
same length and same order. Appending the entry at the request is the same kind
of write, and pushes normally.

**GSX reads the routes again when the tug drives.** The menu's names are fixed
at state 5, but where each entry leads is not. Also flown at D2: a point
assigned with the route menu already on screen, by rewriting the list, and the
bare "ATC Assigned" entry pushed there. This is the usual case, because most
pilots call for pushback only once GSX shows the menu. After state 5 the
handler leaves the label alone and only moves the route. The messages quote
the entry by the name the menu actually shows.

Two writes are never made:

- **Before the request.** A parked, boarding aircraft keeps the scenery's own
  menu. An early assignment is only announced, on the VDGS and in a message.
- **Once the aircraft moves.** The tug is driving the entry's geometry. The
  pilot is told to cancel and request again.

`onAirportBypassPinConnected` re-checks the feed, because the pin goes in
about twenty seconds before GSX builds the menu. A clearance that arrives just
then reaches the menu without waiting up to five seconds for the poll.

*Where the window closes*, below, describes the marking mode. That cutoff
applies to labels, not to where an entry leads.

`ATC_ENTRY = False` falls back to the earlier modes described below: marking a
scenery route in place, and optionally narrowing the menu until state 5.

### Why it does not simply narrow

`getGate().pushback`, `pushbackLabels` and `pushbackAddPos` are all writable,
and the obvious thing to do is drop the routes ATC did not assign: keep the
wanted entries in `pushbackAddPos`, and set the direction bitmask (`1` left,
`2` right, `3` both, `0` neither) to hide the unwanted defaults.

**That breaks the pushback.** GSX reaches "release parking brakes" and then
never moves. Measured at EKCH B8, one flight per combination:

| `pushback` | `pushbackAddPos` | Result |
| --- | --- | --- |
| untouched | untouched | pushes normally |
| written | untouched | hangs at "release parking brakes" |
| untouched | written | hangs at "release parking brakes" |
| written | written | hangs at "release parking brakes" |

GSX builds the tug's path from those two properties when it prepares the
parking. Writing either afterwards leaves the menu offering routes whose
geometry no longer exists, so the push has nowhere to go — that is the
`[workZoneGate] PushBack: NOT GATED - pose not computable` line present in every
failing log. `pushbackLabels` has no such effect and is safe to write at any
time, including after the menu has been built.

Narrowing does work if the scenery's own values are written back at departure
state 5, before the push runs (`SHOW_ONLY_ASSIGNED`). But it only helps when the
point is known before state 5, and it rewrites both properties twice per push.
The added entry rewrites the list once and needs no restore. The pilot can still
pick a route ATC did not give them; what they cannot do is miss which one was
assigned. That is why the label carries the point's name: the alternatives stay
visible, so a bare "ATC Assigned" would not say *which* push among four.

`SET_DIRECTION` and `SET_ADDPOS` in the script remain as switches, both off, so
the finding can be re-checked against a future GSX build.

**GSX has no `selectPushback()`.** A script cannot answer the menu on the
pilot's behalf. It always offers Straight Pushback and Pull Straight regardless
of the parking preference, so the pilot always confirms.

The route applies to the stand the aircraft is already parked on, which is why
this is a departure-only concern and why the script never pairs it with a stand
selection.

### The pilot does not have to wait for clearance

Requesting pushback and choosing a route are two separate moments in GSX, and
they are far apart. Timed from a departure at EKCH Gate A32:

| T+ | |
| --- | --- |
| 0:16 | pilot requests departure — `onAirportDepartureRequested` fires |
| 0:18 | handling operator chosen |
| 0:28 | tug operator reaches the aircraft |
| 1:13 | nose gear raised, aircraft frozen |
| 1:25 | tug opens its arms — `FSDT_GSX_DEPARTURE_STATE` goes **4 → 5** |
| 1:45 | nose gear raised, and **the route menu is built and shown** |

So a pilot can call the tug the moment they are ready and pick up their
clearance while it drives out, instead of waiting for ATC and then waiting again
for a tug. `onAirportDepartureRequested` is no longer the last chance to narrow
anything; it only applies whatever is already assigned and marks where the
aircraft was standing. The 5-second poll carries the rest.

### Where the window closes

**GSX takes its own copy of the parking's pushback configuration when it builds
that menu.** The properties stay writable afterwards and GSX stops reading them,
so a late write changes nothing the pilot can see.

This was measured at B8, not assumed. `Y1 Face E` written at T+1:20 (state 4)
reached the menu; `Y2 Face W` written at T+2:52 (state 5, menu already up) did
not, and the menu went on offering `Y1` until the pilot cancelled the request and
made it again — at which point GSX rebuilt the parking, re-read it, and the new
route took.

The script therefore treats **departure state 5 as the cutoff**. That is twenty
seconds earlier than the true boundary at the nose gear, but it is the last
marker the script can observe, and erring early costs a pilot one honest "cancel
and request again" where erring late costs them a push in the wrong direction
that the script claimed to have fixed. Movement is kept as a backstop, for a sim
where the LVar cannot be read at all.

`FSDT_VAR_Frozen` is not used for any of this. It was the original cutoff and it
is wrong in both directions: it goes to 1 at the nose-gear raise on an aircraft
configured for full freeze, half a minute before the pilot is asked anything,
and on the B8 test aircraft it stayed at 0 for the entire pushback.

### Marking mode (`ATC_ENTRY = False`)

- **The mark moves; it does not accumulate.** Marking renames in place, so the
  previously marked route is given its own name back before the new one is
  marked. Otherwise a reassignment would leave two entries both claiming to be
  the ATC route. The script snapshots the scenery's own names — for the two
  defaults and for every extra slot — and restores from that.
- **The mark is re-asserted, not written once.** The labels are read back on
  each poll and re-applied silently if GSX has restored its own, so a rebuild
  cannot quietly leave the pilot with an unmarked menu.
- **An extra slot was renamed by mutating its dict.** That was later shown to
  have no effect: reads of `pushbackAddPos` are copies (see above).

An assignment that lands before the cutoff is applied and the pilot is told to
re-open the menu, rather than left waiting for GSX's own refresh. One that lands
after it is still applied — renaming works at any time — but whether GSX rebuilds
the menu from the labels at that point is not something the script can observe,
so the pilot is told to cancel the pushback and request it again if the mark has
not appeared.

Calling the tug with nothing assigned says nothing at all. It is tempting to
answer with "waiting for ATC", and `ANNOUNCE_WAITING` in the script turns that
on, but it is off by default: the feed cannot tell "on a strip, nothing assigned
yet" from "nobody is controlling", because it answers both with an empty payload
so as never to disclose whether a callsign is known to FlightStrips. A pilot
flying offline with the profile installed would be promised a narrowing that is
never coming.

## Client

The handler script lives in `gsx-client/`. It is named after the GSX `.ini`
profile it accompanies, which binds it to one airport and one scenery, and is
distributed alongside that profile. Set `SCENERY` in it to match the scenery key
in `gsx_sceneries.json`.

## Adding this to your GSX profile

The step-by-step guides live in the GSX section:

- [GSX for pilots](/gsx/pilots/): installing the script and using
  **ATC Assigned**.
- [GSX for profile makers](/gsx/profile-makers/): mapping a scenery in
  `gsx_sceneries.json`, preparing and testing the script, and the constraints
  any change to it has to respect.
