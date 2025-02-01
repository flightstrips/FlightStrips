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

```json
{
    "type": "login",
    "airport": "EKCH",
    "position": "118.105",
    "callsign": "EKCH_A_TWR",
    "range": 50 // EuroScope range useful for selecting master
}
```


### Controller online

```json
{
    "type": "controller_online",
    "position": "118.105",
    "callsign": "EKCH_A_TWR"
}
```

### Controller offline

```json
{
    "type": "controller_offline",
    "position": "118.105",
    "callsign": "EKCH_A_TWR"
}
```

### Sync

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
            "position": { "lat": 1111, "long": 1111, "altitude": 123 },
            "stand": "A12",
            "capabilities": "G",
            "communication_type": "V",
            "eobt": "1200", // nullable
            "eldt": "1200" // nullable
        }
    ]
}
```

### Assigned squawk

```json
{
    "type": "assigned_squawk",
    "callsign": "SAS123",
    "squawk": "1111"
}
```

### Squawk

Squawk set by pilot

```json
{
    "type": "squawk",
    "callsign": "SAS123",
    "squawk": "1111"
}
```

### Requested altitude

Requested altitude set by controller

```json
{
    "type": "requested_altitude",
    "callsign": "SAS123",
    "altitude": 1111
}
```

### Cleared altitude

Cleared altitude set by controller

```json
{
    "type": "cleared_altitude",
    "callsign": "SAS123",
    "altitude": 1111
}
```

### Communication type

Communication type set by controller

```json
{
    "type": "communication_type",
    "callsign": "SAS123",
    "communication_type": "V"
}
```

### Ground state

Ground state set by controller

```json
{
    "type": "ground_state",
    "callsign": "SAS123",
    "ground_state": "TAXI"
}
```

### Cleared flag

Cleared flag set by controller

```json
{
    "type": "cleared_flag",
    "callsign": "SAS123",
    "cleared": false
}
```

### Position update

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

Set heading. Heading 0 means not set.

```json
{
    "type": "heading",
    "callsign": "SAS123",
    "heading": 0
}
```

### Disconnect

Flight disconnected

```json
{
    "type": "aircraft_disconnect",
    "callsign": "SAS123",
}
```

### Stand

Stand update

```json
{
    "type": "stand",
    "callsign": "SAS123",
    "stand": "A12"
}
```

### Strip update

Strip update

```json
{
    "type": "strip_update",
    "callsign": "",
    "origin": "",
    "destination": "",
    "alternate": "",
    "route": "",
    "remarks": "",
    "runway": "",
    "sid": "",
    "aircraft_type": "A320",
    "aircraft_category": "M",
    "capabilities": "G",
    "eobt": "1200", // nullable
    "eldt": "1200" // nullable
}
```

### Runway

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

