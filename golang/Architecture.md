# Architecture

This document describes the general architecture of the backend, including the overall database structure and design.

## Communication

Both the frontend website and Euroscope communicate with the backend over WebSockets using plain JSON.

**IMPORTANT:** All events are unique between the frontend and Euroscope.

### Establishing a Connection

To establish a connection with the backend, the client must call the respective endpoint for either the frontend or 
Euroscope.

Once connected, the client **MUST** send a `token` event in the following format:

```json
{
    "type": "token",
    "token": ""
}
```

If the token is invalid, the backend disconnects the client.

For details on available events, see [events](../events.md).

### Pings

Each client will be sent a `ping` WebSocket protocol message and is expected to respond with a `pong` message. If the 
client fails to respond within a certain timeframe, it will be disconnected.

The `pong` message is used to update a database entry with a timestamp, allowing the system to clean up old sessions if
no activity is detected for a period of time. See [Invalidating an Old Session](#invalidating-an-old-session).

## Data

Data for strips, controllers, and other entities is stored in a PostgreSQL database. `sqlc` is used for generating code
from queries, and `TODO` is used for migrations.

A strip, controller, and related entities belong to a session. A session contains a name and an associated airport. The
name defines where the session is running, such as `PLAYBACK-xxx`, `Sweatbox`, or `LIVE`. This design ensures that each
piece of data belongs to a session tied to an airport and allows multiple sessions for the same airport on the same
backend server.

### Optimistic Concurrency

For data updates that depend on reading and then modifying database records, optimistic concurrency should be used to 
prevent overwriting with outdated data.

**Considerations:**

- Updates from Euroscope should **not** be affected by this (Euroscope is the source of truth). Instead, it should 
  increment the version.
- A position update for a strip **should not** increment the version, as only Euroscope updates it.

This approach ensures the frontend maintains the correct version, preventing multiple users from overwriting the same 
information simultaneously.

This issue is less concerning if only one person has access to modify strips based on who has the strip "assumed." 
However, under the current design, multiple people can be on the same position, leading to concurrent updates.

### Invalidating an Old Session

If no controllers are connected to the backend via Euroscope, session data should be cleaned up after approximately 
five minutes. This brief window allows old data to be used for syncing tags when a new controller logs in after another
has logged off (e.g., during a controller shift change).

### Ordering Strips

All controllers viewing the same data must see strips in the same order. Therefore, the ordering must be adjustable by
controllers, independent of the TSAT value.

Ordering is represented as an `INT32` in the database but is **not** stored in a fixed `order` field. Initially, strips
are spaced 100 positions apart. This provides flexibility for reordering without immediate conflicts.

#### Example

If we have three strips with the following order:

- A: `0`
- B: `100`
- C: `200`

If we move C between A and B, its new order will be `(100 - 0) / 2 = 50`.

If a new strip (D) is placed between A and C, its new order will be `(50 - 0) / 2 = 25`.

If there is no more room between two strips (which should be rare), ordering must be recalculated to maintain spacing.
One simple approach is resetting all order values with a 100-place gap.

As an alternative, [LexoRanks](https://medium.com/whisperarts/lexorank-what-are-they-and-how-to-use-them-for-efficient-list-sorting-a48fc4e7849f)
can be used to avoid these issues.

Given that a session resets after five minutes of inactivity, increasing the separation to 1000 should provide ample
flexibility before reaching the `INT32` limit.

## Support for Multiple Backend Servers

The backend architecture **MUST** support multiple backend servers to enable live updates and fault recovery without
disrupting sessions.

When running multiple backend servers, clients may connect to different servers. A server/client must be able to
broadcast messages to all clients. This requires a pub/sub system, such as **Redis**, for synchronization.

Backend servers must also communicate to determine the **master Euroscope client**. This communication must occur over
the pub/sub system.

### Starting point

For the first version this will not be supported but it should be built with multiple backend servers in mind. This
comes with the following considerations:

* No to very little state can be stored in memory.
* The communication with clients must be over well structured 'interfaces' which implementations can be changed to
  support multiple servers.

## Determining the Master Client

Due to the high-frequency communication from Euroscope, one Euroscope client must be designated as the
**master client**.

Non-master Euroscope clients will send only limited configuration-related events to the backend, while the master
client handles all events for strips and controllers.

Each airport should have a prioritized list of positions to determine the master client. For example, at **EKCH**:

1. EKCH_A_TWR
2. EKCH_D_TWR
3. EKCH_C_TWR
4. EKCH_A_GND (may not be used due to limited range)
5. EKCH_B_GND (may not be used due to limited range)
6. EKCH_S_GND (may not be used due to limited range)
7. EKCH_DEL (may not be used due to limited range)
8. EKCH_W_APP
9. EKCH_O_APP
10. EKCH_K_DEP
11. EKDH_CTR (and so on...)

The system must determine this priority based on the **primary frequency**, not callsigns.

## Updating a Flight Plan in Euroscope

If a flight plan is updated on the frontend, it must be synchronized with Euroscope.

It is **CRITICAL** that the Euroscope client associated with the user on the frontend is the one updating the flight
plan.

## Integration with vACDM

vACDM is the new system used for assigning startup times to pilots. Instead of having a ES plugin for using vACDM the
backend should instead act as the master and updating vACDM based on the events from ES.

Later if decided due to poor times from vACDM the backend may implement its own CDM system instead.

