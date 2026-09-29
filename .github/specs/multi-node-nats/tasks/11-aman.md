# Task 11 — AMAN operational state and audit

**Depends on:** 03 and 04. **Outcome:** airport AMAN results and explanations are durable and identical on all nodes.

## Work

- Move airport state, flights, VATSIM observation identities, command outcomes, audit records, validation evidence and AMAN coordination requests to typed airport entities. Preserve current airport revision, active CID/callsign uniqueness, ordering, source provenance and command rejection behavior from `.github/specs/aman-cph.md`.
- Convert AMAN materializers and operational read models to projection reads. A transition that currently updates several AMAN tables emits one airport event. Airport-to-session changes use the durable intent contract, with source revision recorded and stale intents superseded.

## Done when

- Existing AMAN operational, replay, command, audit and coordination tests run against the NATS adapter. Two backends rebuild the same board and audit trail after total process restart.
- A repeated source observation or command ID adds no extra revision. A crash after intent but before session update resumes without duplicate strip change.
- No AMAN operational result depends on one process's materialized cache or a PostgreSQL transaction.

**Starting points:** `backend/internal/aman/persistence.go`, `backend/internal/aman/materializer`, and `backend/internal/repository/postgres/aman.go`.
