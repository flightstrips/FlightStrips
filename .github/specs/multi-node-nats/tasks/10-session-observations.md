# Task 10 — Position observations and shared hub state

**Depends on:** 03, 05 and 06. **Outcome:** all session state required by a different backend is replicated.

## Work

- Move latest aircraft position/disconnect observations to epoch-keyed `FS_POSITIONS`. Serialize writes per aircraft, use KV revision preconditions, retain current per-aircraft order/backpressure, and drain accepted positions before a disconnect tombstone. An observation never increments a user-edit strip version.
- Move frontend messages, METAR/ATIS presentation, CLX overrides, EuroScope sync/runway data and delayed disconnect state from hub maps to session entities/deadlines. Leave only physical socket maps, queues and diagnostic recorders local.
- Build existing initial snapshots from one session projection revision plus tagged position/presence observations. Derived strip/stand/AMAN transitions remain committed domain commands and recheck their source observation.

## Done when

- Two backend processes produce equal initial state and live deltas for messages, ATIS, CLX and aircraft observations. Full restart restores latest values but marks them stale until fresh master sync.
- Previous-owner-epoch writes cannot replace the new epoch's value. Disconnect does not race an older position back into existence.
- The existing 100/200-per-second position-load input and assertion coverage still run; performance gate is task 23.

**Starting points:** `backend/internal/frontend/hub_messages.go`, `hub_metar.go`, `backend/internal/euroscope/hub.go`, and `backend/internal/shared/position_dispatcher.go`.
