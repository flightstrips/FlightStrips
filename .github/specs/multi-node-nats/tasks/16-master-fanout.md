# Task 16 — Cluster-wide EuroScope master and client fanout

**Depends on:** 05, 10, 13 and 14. **Outcome:** clients on either backend share one master and one committed live view.

**Release boundary:** merge with task 14 in the coordinated candidate PR and release at task 24; see [release-safety.md](../release-safety.md).

## Work

- Publish per-socket `FS_PRESENCE` records with connection generation, session/CID, position, observer and establishment time. Session owner elects master using existing airport priority, then establishment time and connection ID; commit `master_changed` and increase master epoch.
- Send `SessionInfoEvent` role and epochs to plugins. Validate socket generation, session, CID and master epoch on master-only inbound events; require new sync after master change or reconnect. Durable controller metadata is shown online only while matching presence is fresh.
- Fan out applied session/airport deltas on every backend to its local sockets. CID-targeted delivery finds the node and connection generation through presence; local hubs do not decide commit success.

## Done when

- Split frontend/EuroScope clients across two backends; both see the same state, one master exists, and one CID gets a targeted command.
- A stale master still sending after takeover is rejected. Reconnect creates a new generation and cannot inherit the old synced flag.
- Initial snapshot plus buffered deltas has no gap or duplicate state change.

**Starting points:** `backend/internal/euroscope/hub.go`, `client.go`, `backend/internal/frontend/hub.go`, and `proto/euroscope.proto`.
