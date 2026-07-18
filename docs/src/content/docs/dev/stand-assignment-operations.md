---
title: Stand assignment operations
description: Production readiness, observability, enablement, and rollback for SAT.
---

The Stand Assignment Tool (SAT) is release-safe and disabled by default. Deploy
the application and its database migrations with `ENABLE_STAND_ASSIGNMENT=false`.
In this mode SAT does not load its configuration, start its VATSIM reconciliation
or lifecycle workers, expose controller actions, or change the controller UI.
Existing manual stand handling remains available.

SAT can remain active without sending its automated stand-assignment private
messages to pilots through EuroScope. Set `ENABLE_STAND_ASSIGNMENT_ES_MESSAGES=true`
to enable those messages. The default is `false`; SAT allocation, persistence,
stand synchronization, controller notifications, and the frontend continue to
operate regardless of this flag.

## Readiness

`GET /healthz` remains an HTTP 200 liveness endpoint so a SAT input failure does
not restart or remove otherwise healthy FlightStrips instances. Inspect the JSON
body before enabling SAT:

```json
{
  "status": "ok",
  "stand_assignment": {
    "enabled": true,
    "ready": true,
    "status": "ready",
    "snapshot_age_seconds": 4.2
  }
}
```

SAT can report `disabled`, `invalid_config`, `feed_unavailable`, `feed_failed`,
or `feed_stale`. Do not enable production controller access unless the status is
`ready`. A stale or failed feed degrades SAT readiness but does not take unrelated
FlightStrips features down.

Useful OpenTelemetry instruments are:

- `sat.vatsim.snapshot.age` and `sat.vatsim.records` for feed freshness and relevant online/prefile volume;
- `sat.assignments`, labelled by stage, source, category, and tier;
- `sat.allocation.outcomes` for assigned, override, no-compatible-stand, and contention results;
- `sat.allocation.conflicts` and `sat.assignments.expired`.

SAT logs use callsign and session where operational correlation is required.
They do not log pilot names, and metric dimensions never contain callsigns or CIDs.

## Enablement and rollback

1. Deploy with `ENABLE_STAND_ASSIGNMENT=false`; allow migrations to complete.
2. Validate the SAT configuration files in the mounted configuration directory.
3. Set the flag to `true` on one instance and restart it.
4. Wait for `/healthz` to report `stand_assignment.status=ready`, then roll the same change to the remaining instances.
5. Watch feed age, allocation outcomes, contention, expiry, and structured SAT errors during the rollout.

To roll back, set `ENABLE_STAND_ASSIGNMENT=false` and restart all backend
instances. Do not revert the SAT migrations: disabled code ignores the SAT tables,
and existing manual stand behavior remains available.
