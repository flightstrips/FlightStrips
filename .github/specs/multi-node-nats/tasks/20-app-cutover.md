# Task 20 — Application runtime and CI cutover

**Depends on:** 04–19, 15a, 15b, 18a–18c and 19a–19c, now complete on the integration base. **Outcome:** the FlightStrips application builds and runs with NATS as its sole operational store.

**Release boundary:** merge only with the coordinated integration PR after local qualification. This task removes the current production storage path and cannot be released or deployed against the current infrastructure; see [release-safety.md](../release-safety.md).

**Operator decision:** qualification uses locally built applications on the user's machine. No release-candidate builds, artifact promotion or mandatory staging deployment are part of this plan. Ordinary releases remain the publication path after local testing.

## Implementation split

Task 20 is a parent acceptance gate, implemented by three independently reviewable integration-base PRs:

| Child | Owns | Dependencies |
| --- | --- | --- |
| [20a — Runtime and API assembly](20a-runtime-assembly.md) | Concrete NATS application constructor, every enabled HTTP/socket/worker binding, readiness and shutdown | Completed 04–19 and follow-ups |
| [20b — Ordinary release workflow cleanup](20b-release-gates.md) | Remove migrator build/publication/bump references; preserve existing ordinary releases | Completed 14–19 and follow-ups; can run alongside 20a |
| [20c — Final integration and SQL retirement](20c-sql-retirement.md) | Sole-default NATS entrypoint, SQL removal, local two-backend Compose, documentation and combined acceptance | 20a and 20b merged and accepted |

All children are held from `main` and release under this parent's boundary. Temporary isolated construction in 20a ends in 20c; it is not a released fallback. Task 20 implementation requires all three children and automated parent acceptance checks below. Tasks 21/22 may prepare infrastructure and local fault evidence after that implementation is merged; the operator's manual acceptance and release decision remain pending gates for Task 24 and for merging held work to `main`. Missing business adapters discovered during assembly belong to 20a and block its acceptance; they cannot be deferred as unbound hooks to 20c.

**Integration status:** 20a/#827, 20b/#826 and 20c/#828 are merged with passing CI. [20c's binding audit and parent acceptance evidence](20c-sql-retirement-evidence.md) record complete NATS-only startup, SQL retirement and automated local tests. The user's complete-system manual acceptance is not inferred from PR merges and remains pending; no production release or deployment is authorized.

## Work

- Remove PostgreSQL pool/repository/migrator runtime wiring, SQL-dependent service calls, `DATABASE_CONNECTIONSTRING`, and migration image build/publish after every adapter above is converted. Remove `docker-compose.prod.yml`; update local Compose and test harness to use three NATS nodes and two backends.
- Preserve ALB source, `/albEvents`, configuration and wire behavior while changing shared application construction.
- Wire NATS configuration, `/readyz`, projection/lease/effect metrics and safe command logs into the application. Keep `/healthz` as process liveness. Update `backend/Architecture.md`, development instructions, release workflow and storage-specific text in `docs/position-performance.md`.
- Remove migrator release workflow dependencies through 20b and retain the existing ordinary release process. Keep infrastructure image-bump automation as a PR, never a direct stack update. Hold the integration work and actual release PR until the user finishes local qualification.
- Provide exact local build/start/test commands for two backends, three NATS nodes, the frontend and revision-2 plugin. Record tested source revision, configuration, commands and results; preserve current component/development build identity. Local test results are not a claim about multi-host production performance.
- Use stop-first for the initial PostgreSQL-to-NATS deployment. Later NATS-only start-first updates require task 22's overlapping-version test and the internal event-reader rule in [operations.md](../operations.md).

## Done when

- Build, tests and local two-backend startup need no PostgreSQL or Redis. A code scan finds no runtime `pgx`/SQL transaction/connection-string path; archived fixtures are not linked into the new application.
- `/readyz` fails during replay, quorum loss, resource drift or projection stall and recovers after catch-up. Logs/metrics carry command ID, sequence and epochs without token or message-body leakage.
- Release CI no longer builds or bumps `backend-migrate`.
- The user can build and test the complete system locally without any release-candidate or staging artifacts. Ordinary release behavior is preserved, migrator release paths are removed, and the integration/release PR remains held until local qualification and the operator's release decision.

**Starting points:** `backend/internal/app/app.go`, `backend/cmd/server/main.go`, `.github/workflows/release-please.yml`, `.github/workflows/build-backend.yml`, and local Compose.
