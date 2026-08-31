# Events

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

`connection` can be `LIVE`, `SWEATBOX` or `PLAYBACK`

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

A TopSky holding clearance, read off the EuroScope scratch pad by the plugin.
EuroScope broadcasts the scratch pad to every controller in range, so a hold is
visible even for aircraft this controller is not tracking.

`hold` is the holding point (or the area name when `hold_type` is `tsa`). An
empty `hold` means the clearance was cancelled. `hold_eat` is the expect
approach time, empty when none was given.

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
