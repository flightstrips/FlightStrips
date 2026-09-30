# Task 20 — Application runtime and CI cutover

**Depends on:** 04–19, 15a, 15b, 18a–18c and 19a–19c. Tasks 18 and 19 must satisfy their complete acceptance criteria; partial candidate PRs do not satisfy these dependencies. Tasks 19a/19b are complete, but 19c supplies the remaining global VATSIM transceiver feed. **Outcome:** the FlightStrips application builds and runs with NATS as its sole operational store.

**Release boundary:** merge only with the coordinated candidate PR. This task removes the current production storage path and cannot be released or deployed against the current infrastructure; see [release-safety.md](../release-safety.md).

## Work

- Remove PostgreSQL pool/repository/migrator runtime wiring, SQL-dependent service calls, `DATABASE_CONNECTIONSTRING`, and migration image build/publish after every adapter above is converted. Remove `docker-compose.prod.yml`; update local Compose and test harness to use three NATS nodes and two backends.
- Preserve ALB source, `/albEvents`, configuration and wire behavior while changing shared application construction.
- Wire NATS configuration, `/readyz`, projection/lease/effect metrics and safe command logs into the application. Keep `/healthz` as process liveness. Update `backend/Architecture.md`, development instructions, release workflow and storage-specific text in `docs/position-performance.md`.
- Change the release workflow before merging the coordinated candidate PR: build backend/frontend images from the unified Release Please PR head under immutable candidate tags, and build the revision-2 plugin DLL as a hash-checked CI artifact. Record the release PR Git tree, OCI digests and DLL hash for staging tasks 21–23. On release PR merge, verify the merged Git tree equals the tested tree and promote the same image digests to pinned version tags without rebuilding. Never update `latest` for the candidate. Move GitHub release DLL attachment and the frontend Discord announcement into a manually dispatched cutover job. That job takes the exact release tag and expected DLL hash as inputs and refuses a mismatch. Keep the infrastructure image-bump automation as a PR, never a direct stack update.
- Set embedded build identity from the release version and tested Git tree hash, rather than the eventual merge commit SHA, so the promoted image still reports the source that was exercised in staging.
- Use stop-first for the initial PostgreSQL-to-NATS deployment. Later NATS-only start-first updates require task 22's overlapping-version test and the internal event-reader rule in [operations.md](../operations.md).

## Done when

- Build, tests and local two-backend startup need no PostgreSQL or Redis. A code scan finds no runtime `pgx`/SQL transaction/connection-string path; archived fixtures are not linked into the new application.
- `/readyz` fails during replay, quorum loss, resource drift or projection stall and recovers after catch-up. Logs/metrics carry command ID, sequence and epochs without token or message-body leakage.
- Release CI no longer builds or bumps `backend-migrate`.
- A release PR merge alone cannot publish the incompatible plugin DLL, announce the new frontend, or update mutable `latest` image tags. The release promotion job fails on Git tree or digest drift instead of silently rebuilding. The manual cutover job is tested with a dry run against the staged artifacts.

**Starting points:** `backend/internal/app/app.go`, `backend/cmd/server/main.go`, `.github/workflows/release-please.yml`, `.github/workflows/build-backend.yml`, and local Compose.
