# Task 01 — NATS resources and local fixture

**Depends on:** 00. **Outcome:** a reproducible, verified NATS substrate; no domain data moved yet.

## Work

- Add the Go NATS dependency and typed client configuration for URLs, credentials/TLS, timeouts, and resource names. Add an idempotent one-shot bootstrap for the exact `FS_STATE`, `FS_POSITIONS`, `FS_PRESENCE`, `FS_SNAPSHOT_INDEX`, and `FS_OBJECTS` settings in [contracts.md](../contracts.md). Normal backend startup verifies settings and never silently rewrites drifted resources.
- Add a three-node `nats:2.15.0` local/integration fixture with separate file stores and a test helper that waits for quorum. Keep the bootstrap credential distinct from the backend application credential.

## Done when

- Bootstrap twice leaves identical resources; a fresh three-node cluster and a restarted cluster both verify successfully.
- Wrong subjects, replicas, storage, history, or retention make verification fail with the resource named. One NATS node loss still permits file-backed writes; two-node loss prevents durable writes.
- No PostgreSQL runtime path is removed by this task.

**Starting points:** `backend/go.mod`, `backend/cmd/server/main.go`, local Compose, and `backend/internal/testing`.
