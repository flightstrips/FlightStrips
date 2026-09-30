# Task 20c — Final integration and SQL retirement

**Depends on:** 20a and 20b merged and accepted on the integration base.

**Outcome:** the sole application runtime uses NATS, local development runs three NATS nodes and two complete backends, and every parent Task 20 acceptance gate has recorded evidence.

**Release boundary:** held under [Task 20](20-app-cutover.md). No release/deployment or production stack changes. Task 21 starts only after this task completes the parent gate.

## Work and contracts

- Install `BuildNATS` as the sole default server construction. Remove the temporary SQL/candidate selector, SQL pool/repository/migrator wiring, database connection environment/secrets, SQL-dependent runtime calls and migration executable/image. Retire obsolete SQL code/fixtures from the active application/build graph; no `pgx` or SQL driver is imported/linked into the new runtime. Preserve operational policy extracted/shared by candidates rather than deleting it based on an old directory name.
- There is no SQL fallback, dual write or automatic data migration. Audit the completed 20a binding matrix against final startup: every enabled route/action/worker uses its accepted-state adapter. A missing business adapter blocks completion and returns to 20a; do not fill it with SQL or a no-op. All NATS/storage/frontend/EuroScope payloads remain typed Protobuf; first-party HTTP remains JSON. No Redis. **Do not change ALB source, endpoint, protocol, configuration or enablement**; preserve existing construction only.
- Remove obsolete `docker-compose.prod.yml`. Replace the default development Compose path with three pinned NATS nodes and two actual backend processes/images. Bootstrap resources separately with administrator credentials; backend runtime credentials verify/use them without resource creation. Expose paths for cross-node binary clients and unchanged JSON HTTP requests. Document the empty startup, bootstrap, normal restart and full shutdown/replay sequence.
- Update `backend/Architecture.md`, development instructions and storage-specific portions of `docs/position-performance.md`, including new configuration, liveness/readiness, metrics and safe logs. Provide exact local backend/frontend/plugin build, bootstrap/start/stop/restart, cross-node client and test commands for the user's machine, including prerequisites and secret/configuration paths. Production Swarm stack work remains Task 21; activation remains Task 24 with an initial stop-first deployment.
- Recheck the parent readiness/log/metric gates and 20b migrator release cleanup on the combined source tree. Preserve normal component/development build identity and ordinary releases. No candidate artifact, promotion or staging workflow is required.

## Done when

- Starting an empty fixture with two complete backend instances needs no PostgreSQL or Redis. Real binary frontend/EuroScope clients and JSON HTTP fixtures perform mutations/read outcomes on different nodes. Provider fixtures exercise assembled PDC/CDM/AMAN, VATSIM lifecycle and durable effects rather than injected policy. All required workers are present in the final binding audit.
- Killing one backend and separately removing NATS quorum demonstrates accepted-state recovery, readiness failure/recovery and no duplicate effective work. Reuse relevant existing integration tests, adding combined-startup evidence for remaining risks. Broader cross-version/recovery and capacity qualification stay in Tasks 22/23.
- Backend/frontend/plugin contract checks and builds pass on the final source tree. Source/dependency/binary scans show no active SQL runtime, driver, connection-string fallback, Redis or hidden JSON payload path. HTTP behavior and ALB preservation checks pass.
- A repeatable local setup lets the user test both backends, frontend and plugin without a release-candidate build or published artifact. Record source revision, local build/configuration and checks performed; leave the operator's own results pending until provided. Do not publish, announce or deploy to demonstrate acceptance, or claim multi-host production qualification from local tests.
- Record every parent Task 20 acceptance item with commands/results and constructor/route/worker matrix links. Mark 20 complete only when all three child PRs and the original parent acceptance gates are satisfied. Open and attach a draft PR into `codex/multi-node-nats-base`; do not merge or release it.

**Starting points:** 20a runtime/binding evidence, 20b release helper/workflows, `backend/cmd/server/main.go`, local Compose, backend dependency graph and development/architecture documents.
