# Controller, sector and runway candidate mapping

The current PostgreSQL repositories stay wired until the coordinated cutover.
The opt-in `cluster.ControllerSector` adapter reads typed session and airport
projections; writes use `PlanControllerSector` and the session subject CAS.

| Current call | Candidate replacement |
| --- | --- |
| Controller `Create`, `Get`, `GetByCid`, `GetByCallsign`, `List`, `ListBySession`, `GetByPosition` | `PutController`, `ControllerByCID`, `ControllerByCallsign`, `Controllers`, `ControllersByPosition` |
| Controller `Delete`, `SetPosition`, `SetObserver` | `RemoveController` or revision-checked `PutController` of a complete record |
| Controller `SetCid` | CID is the candidate entity key and is fixed on creation; a changed authenticated identity removes the old key and creates the new identity through the owner workflow |
| Controller `SetLayout` | `SetPositionLayout`, one typed event for all controllers on the position |
| Controller `SetEuroscopeSeen`, `SetFrontendSeen` | `FS_PRESENCE` client observations and TTL; `OperationalControllers` joins the fresh view with durable metadata |
| Sector owner `CreateBulk`, `ListBySession`, `DeleteAllBySession`, `RemoveBySession` | `ReplaceSectorOwners`, `SectorOwners`; an empty replacement removes all |
| Sector owner `GetByID`, `Delete` | Legacy methods have no SQL implementation; candidate identity is the uppercase sector key, read from `SectorOwners` or removed with `RemoveSectorOwner` |
| Session `UpdateActiveRunways`, runway pair status | `ChangeRunways`, `UpdateRunwayStatus`, revision checked against the typed `Session` |
| Airport master position order | `MasterPositionOrder` from typed airport `AirportPolicy` |

The candidate stores one sector key per legacy sector array entry. Session
runways, sector routing and controller metadata survive replay. Operational
controllers require fresh client and node presence, and the separate session
sync marker still requires its current live master epoch and connection.
