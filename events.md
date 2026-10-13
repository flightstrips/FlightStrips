# Events

## Frontend messages

`send_message` carries `text`, `recipients` (configured area names; an empty
array means broadcast), and optional `format`: `AUTO`, `SMALL`, or `LARGE`.
`message_received` carries the same fields plus `id`, `sender`, and
`is_broadcast`. Format is retained in the session message history and initial
snapshot. Missing format is equivalent to `AUTO`: a single targeted recipient
uses large text; broadcast and multiple recipients use small text. Explicit
`SMALL` or `LARGE` overrides recipient-based sizing.

Targeted areas resolve to an online operational controller at the area's primary
configured frequency, or otherwise to the current owner of its sector. Ground
west coverage includes both `GWA` and `GWD`. Static fallback positions without
the selected area's coverage do not receive the message. Observers are excluded;
the sender also receives a copy.

## Frontend next-controller selection

`next_controller_select` accepts `callsign` and `sector` (`AA`, `GE`, `GW`,
`TE`, or `TW`). The strip must be assumed by the sender, unlocked, and have
no pending coordination. The backend resolves the selected sector through
normal routing and radio coverage, persists it, and broadcasts `owners_update`
with the updated SI target and frequency. This does not create coordination,
change ownership, move the strip, or initiate a EuroScope handover.
The selection survives route recalculation and reconnects, and clears when
ownership changes. Selecting an unavailable target is rejected without
changing the existing SI. If a selected target later goes offline, SI becomes
blank until that sector is available again.

## Frontend flight-plan REA

`cdm_ready` accepts optional `rea: true` for the flight-plan REA control. This
uses the normal persisted ready/status flow, but changes TOBT to current UTC
only if the effective TOBT is more than six minutes in the past or future.
TOBT within six minutes of now is unchanged. Without `rea`, the
existing READY timing policy is unchanged. The returned CDM `status: "REA"`
drives the flight-plan indication.

## Frontend CDM de-icing platform

Sent by: Frontend

Selects the remote de-icing platform used for the flight's EXOT calculation.
`platform` must be `A`, `B`, `V`, or an empty string to clear the selection.

```json
{
    "type": "cdm_deice_platform_update",
    "callsign": "SAS123",
    "platform": "B"
}
```

The server persists the selection and includes it as `deice_platform` in
`cdm_data`, `cdm_data_batch`, and initial strip payloads. The accompanying
`deice_platform_acknowledged` flag becomes `true` when the strip enters a
de-icing bay and returns to `false` whenever a platform is selected again.
The values after
the base taxi time in each taxi-zone row correspond to platforms A, B, and V:

```text
EKCH:22R:<polygon coordinates>:11,2,3,5
```

Here the base taxi time is 11 minutes; the extra times are 2 minutes for A,
3 minutes for B, and 5 minutes for V.

## Authentication event

1. Server and client connects
1. Client sends auth token (max 2 seconds after connection is open)
1. Server validates, if not valid disconnect

```json
{
    "type": "token",
    "token": ""
}
```

## EuroScope

### Login event

Sent by: EuroScope

`connection` can be `LIVE`, `SWEATBOX` or `PLAYBACK_<id>`. The ES plugin creates
a unique ID for each playback and keeps it until playback stops, including across
backend reconnects. The backend uses this name to resume the persisted session.
Plain `PLAYBACK` from older plugins still creates a new session on each login.

```json
{
    "type": "login",
    "connection": "LIVE",
    "airport": "EKCH",
    "position": "118.105",
    "callsign": "EKCH_A_TWR",
    "range": 50, // EuroScope range useful for selecting master
    "local_ip": "192.168.1.25" // optional private IPv4 reported by the plugin host
}
```


### Controller online

Sent by: EuroScope

```json
{
    "type": "controller_online",
    "position": "118.105",
    "callsign": "EKCH_A_TWR"
}
```

### Controller offline

Sent by: EuroScope

```json
{
    "type": "controller_offline",
    "callsign": "EKCH_A_TWR"
}
```

### Sync

Sent by: EuroScope

```json
{
    "type": "sync",
    "controllers": [
        {
            "position": "118.105",
            "callsign": "EKCH_A_TWR"
        }
    ],
    "strips": [
        {
            "callsign": "",
            "origin": "",
            "destination": "",
            "alternate": "",
            "route": "",
            "remarks": "",
            "runway": "",
            "squawk": "1234",
            "assigned_squawk": "1234",
            "sid": "",
            "cleared": false,
            "ground_state": "PUSH",
            "cleared_altitude": 1234,
            "requested_altitude": 1234,
            "heading": 0, // 0 - not set
            "aircraft_type": "A320",
            "aircraft_category": "M",
            "position": { "lat": 1111, "lon": 1111, "altitude": 123 },
            "stand": "A12",
            "capabilities": "G",
            "communication_type": "V",
            "eobt": "1200", // nullable
            "eldt": "1200", // nullable
            "hold": "OLPIB", // empty when not holding
            "hold_type": "enroute", // "enroute" | "tsa", empty when not holding
            "hold_eat": "1422" // empty when not given
        }
    ]
}
```

### Assigned squawk

Sent by: EuroScope and Server

```json
{
    "type": "assigned_squawk",
    "callsign": "SAS123",
    "squawk": "1111"
}
```

### Squawk

Sent by: EuroScope

Squawk set by pilot

```json
{
    "type": "squawk",
    "callsign": "SAS123",
    "squawk": "1111"
}
```

### Requested altitude

Sent by: EuroScope and Server

Requested altitude set by controller

```json
{
    "type": "requested_altitude",
    "callsign": "SAS123",
    "altitude": 1111
}
```

### Cleared altitude

Sent by: EuroScope and Server

Cleared altitude set by controller

```json
{
    "type": "cleared_altitude",
    "callsign": "SAS123",
    "altitude": 1111
}
```

### Communication type

Sent by: EuroScope and Server

Communication type set by controller

```json
{
    "type": "communication_type",
    "callsign": "SAS123",
    "communication_type": "V"
}
```

### Ground state

Sent by: EuroScope and Server

Ground state set by controller

```json
{
    "type": "ground_state",
    "callsign": "SAS123",
    "ground_state": "TAXI"
}
```

### Cleared flag

Sent by: EuroScope and Server

Cleared flag set by controller

```json
{
    "type": "cleared_flag",
    "callsign": "SAS123",
    "cleared": false
}
```

### Position update

Sent by: EuroScope

Position changed for aircraft

```json
{
    "type": "aircraft_position_update",
    "callsign": "SAS123",
    "lat": 1111, 
    "lon": 1111, 
    "altitude": 123
}
```

### Set heading 

Sent by: EuroScope and Server

Set heading. Heading 0 means not set.

```json
{
    "type": "heading",
    "callsign": "SAS123",
    "heading": 0
}
```

### Disconnect

Sent by: EuroScope

Flight disconnected

```json
{
    "type": "aircraft_disconnect",
    "callsign": "SAS123",
}
```

### Stand

Sent by: EuroScope and Server

Stand update

```json
{
    "type": "stand",
    "callsign": "SAS123",
    "stand": "A12"
}
```

### Hold

Sent by: EuroScope and Server

A TopSky holding clearance, read from the live EuroScope scratch-pad protocol by
the plugin. EuroScope broadcasts `/HOLD/<point>/`, `/XHOLD/` and
`/HOLD_EAT/<HHMM>/` commands to every controller in range. The session master
publishes those commands once; a tracking controller may additionally reconcile
an active hold from TopSky annotation 6. An absent annotation is not a
cancellation because remote controllers do not reliably receive that local
strip state.

`hold` is the holding point (or the area name when `hold_type` is `tsa`). An
empty `hold` means the clearance was cancelled. `hold_eat` is the expect
approach time, empty when none was given.

For an authoritative AMAN holding plan, the server can send the same event to
EuroScope with the calculated release time in `hold_eat`. The tracking plugin
then writes the transient TopSky `/HOLD_EAT/HHMM/` command, provided the
tracking client has the matching en-route hold in TopSky. The backend can send
the estimated EAT before entry into that hold. Both live
publication and reconnect replay are controlled centrally by the backend
`ENABLE_AMAN_HOLDING_EAT_WRITEBACK` setting, which defaults to `true` in
authoritative mode and `false` otherwise. An explicit `false` disables writeback.
Operational clients retain the latest backend EAT even while they are not
tracking the aircraft, then reapply the transient command when tracking
ownership is confirmed so the new local TopSky holding list is populated.
When that authoritative holding projection disappears, the server sends the
same callsign, hold, and hold type with an empty `hold_eat`; clients use this as
a withdrawal of the retained backend EAT without changing the live clearance.

```json
{
    "type": "hold",
    "callsign": "SAS123",
    "hold": "OLPIB",
    "hold_type": "enroute", // "enroute" | "tsa"
    "hold_eat": "1422" // may be empty
}
```

### Strip update

Sent by: EuroScope

Strip update

The protobuf strip payload includes `flight_rules` (field 34), containing the filed
`I` (IFR) or `V` (VFR) value from EuroScope. The same field is included in frontend
strip snapshots and updates. CONTROLZONE FIND uses `V` to list filed VFR flights,
not squawk, route, or remarks. Older plugins omit this field; an existing stored
value is preserved until an authoritative flight-rules update is received.

If the stored value is still empty, VATSIM may backfill known flight rules even
when an older plugin has supplied a newer EuroScope observation. This does not
override known rules from a newer EuroScope observation or controller-protected
fields, and an empty VATSIM value never clears known rules.

```json
{
    "callsign": "",
    "origin": "",
    "destination": "",
    "alternate": "",
    "route": "",
    "remarks": "",
    "runway": "",
    "squawk": "1234",
    "assigned_squawk": "1234",
    "sid": "",
    "cleared": false,
    "ground_state": "PUSH",
    "cleared_altitude": 1234,
    "requested_altitude": 1234,
    "heading": 0, // 0 - not set
    "aircraft_type": "A320",
    "aircraft_category": "M",
    "position": { "lat": 1111, "lon": 1111, "altitude": 123 },
    "stand": "A12",
    "capabilities": "G",
    "communication_type": "V",
    "eobt": "1200", // nullable
    "eldt": "1200" // nullable
}
```

### Runway

Sent by: EuroScope

All clients will send this event

```json
{
    "type": "runway",
    "runways": [
        {
            "name": "22L",
            "departure": false,
            "arrival": true
        }
    ]
}
```

### Session info

Sent by: Server

Possible roles: `master` or `slave`

```json
{
    "type": "session_info",
    "role": "master",
}
```

### Generate squawk

Sent by: Server

```json
{
    "type": "generate_squawk",
    "callsign": "SAS123"
}
```

### Route

Sent by: Server

```json
{
    "type": "route",
    "callsign": "SAS123",
    "route": "ODDON LOKSA"
}
```

### Remarks

Sent by: Server

```json
{
    "type": "remarks",
    "callsign": "SAS123",
    "remarks": "remarks"
}
```

### Sid

Sent by: Server

```json
{
    "type": "sid",
    "callsign": "SAS123",
    "sid": "NEXEN2C"
}
```

### Runway

Sent by: Server

```json
{
    "type": "aircraft_runway",
    "callsign": "SAS123",
    "runway": "22L"
}
```
