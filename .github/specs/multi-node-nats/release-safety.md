# Merge and release safety contract

## Answer and terms

**No: the current tasks are not all independently releasable.** A task is a reviewable unit of implementation, not necessarily a production release unit. This migration has one coordinated activation of backend, frontend, EuroScope plugin, and infrastructure. A task marked **preparatory** below may merge to `main` and be included in ordinary releases only while its new behavior is dormant. A task marked **held** is developed on the integration branch and merges to `main` in one coordinated integration PR after local qualification; it is not individually released. Infrastructure preparation and local test work can complete without deploying production. Task 24 is the production activation itself.

**Operator decision (2026-09-30):** the user will test local builds on their machine. There are no release-candidate builds, candidate artifacts/manifests, digest promotion, custom manual-publication framework or mandatory staging deployment. This supersedes the earlier candidate release procedure. Normal build/test CI and the existing ordinary release process remain.

There are three distinct events:

1. **Merge to application `main`:** Release Please runs on that push and may prepare a release PR. A merge does not by itself deploy the production stack.
2. **Publish a release:** merging the ordinary Release Please PR publishes backend/frontend images, attaches the plugin DLL and may trigger release announcements under the existing workflows. Task 20b removes migrator publication references and preserves this normal release behavior. Hold the actual release PR until local checks are complete and the operator is ready for the matched protocol/storage cutover; local testing itself requires no publication.
3. **Deploy production:** the release workflow opens a `flightstrips/infrastructure` image-bump PR; its merge changes the Portainer-watched stack. Keep this as a PR and hold it until Task 24 incorporates matching component versions into the reviewed production stack. Production uses pinned version tags, never `latest`.

The migration is coordinated through held integration/release/stack PRs, without a new publication gate framework. Complete the integrated local checks before the held changes reach `main`; then hold the ordinary Release Please PR until the operator chooses the cutover window. Freeze other application releases and production deployments during that window. Ordinary releases of preparatory work are safe only if their built artifacts still behave like the current deployment. No actual merge, release, announcement or deployment is authorized merely by finishing an implementation task.

## Preparatory merge invariant

For every preparatory task, CI must run the current production-mode build and tests plus that task's applicable schema or isolated NATS tests. The candidate main-branch artifacts must:

- start with the existing production configuration, PostgreSQL and current secrets, without NATS or new required environment variables;
- retain current HTTP behavior, frontend JSON WebSocket protocol, EuroScope protocol, endpoint availability, and current worker ownership;
- neither connect to NATS at startup nor start a new worker, owner loop, or side effect in current production mode;
- leave existing `/healthz` and production routing behavior intact; `/readyz` may exist for the isolated NATS test runtime but must not make the current production task unready;
- keep the current root EuroScope generated schema and plugin artifact active; generated revision-2 code may be added under a separate package or candidate path;
- never dual-write PostgreSQL and NATS. The isolated NATS runtime may be selected explicitly by the test harness, but only one store and one authority are active in a process.

An implementation that cannot maintain these conditions is reclassified **held** before merging. In preparatory task files, words such as “replace SQL” and “no active SQL” refer to the explicitly selected NATS test runtime; they do not authorize replacing the current production wiring before task 20. The final NATS-only runtime removes PostgreSQL and old client protocol code at activation; preparatory coexistence is temporary source isolation, not a compatibility mode in the released NATS system.

## Per-task decision

| Task | Individually merge to application `main`? | Independent ordinary release/deployment? | Required boundary |
| --- | --- | --- | --- |
| 00 | Yes | Yes | Schemas, generators and coverage only; do not replace active root EuroScope wire. |
| 01 | Yes | Yes | NATS fixture/configuration is opt-in; old startup needs no NATS. |
| 02 | Yes | Yes | Event writer is used only by isolated NATS tests. |
| 03 | Yes | Yes | Projection and NATS readiness are inactive in current production mode. |
| 04 | Yes | Yes | New session adapter is dormant; current SQL service stays wired. |
| 05 | Yes | Yes | New controller/sector adapter is dormant. |
| 06 | Yes | Yes | New strip adapter is dormant. |
| 07 | Yes | Yes | New coordination adapter is dormant. |
| 08 | Yes | Yes | New stand adapter is dormant. |
| 09 | Yes | Yes | New PDC/tactical adapter is dormant. |
| 10 | Yes | Yes | New position/presence adapter is dormant. |
| 11 | Yes | Yes | New AMAN adapter is dormant. |
| 12 | Yes | Yes | New navigation/weather adapter is dormant. |
| 13 | Yes | Yes | Owner leases and routing do not start in current production mode. |
| 14 | No; held | No | Replacing active EuroScope schema, token and plugin behavior is part of the coordinated integration PR. |
| 15 | No; held | No | Binary-only browser/backend transport is part of the coordinated integration PR. |
| 15a | No; held | No | Browser result UI depends on the revision-2 transport and complete effect lifecycle. |
| 15b | No; held | No | Mandatory `Idempotency-Key` would reject current HTTP clients; activate only with updated callers. |
| 16 | No; held | No | Depends on task 14's held revision-2 plugin protocol; election must never compete with the current master. |
| 17 | No; held | No | Depends on held browser/plugin outcomes; effect dispatch must never send alongside current dispatch. |
| 18 | No; held | No | Depends on held plugin/effect work; new session workers cannot start alongside current workers. |
| 18a | No; held | No | Completes PDC provider polling and clearance lifecycle; follows the Task 18 activation boundary. |
| 18b | No; held | No | Completes session CDM/traffic adapters; follows the Task 18 activation boundary. |
| 18c | No; held | No | Completes socket deadlines and authoritative squawk throttling; follows the Task 18 activation boundary. |
| 19 | No; held | No | Depends on held effect work; new airport/global workers cannot start alongside current workers. |
| 19a | No; held | No | Completes session-owned VATSIM lifecycle; follows the Task 18/19 activation boundary. |
| 19b | No; held | No | Completes operational AMAN policy and destination intents; follows the Task 19 activation boundary. |
| 19c | No; held | No | Completes the global VATSIM frequency feed; follows the Task 19 activation boundary. |
| 20 | No; held | No | Removing SQL/migrator and wiring NATS-only startup is activation code. |
| 20a | No; held | No | Concrete runtime/API assembly follows the Task 20 boundary; isolated construction only until 20c. |
| 20b | No; held | No | Minimal migrator release cleanup; preserve ordinary releases and do not introduce release candidates or promotion. |
| 20c | No; held | No | Installs the sole NATS runtime and removes SQL; completes Task 20 acceptance before Task 21. |
| 21 | Infrastructure PR only | No | Prepare and validate the production stack PR; local rehearsal is sufficient for this plan. Do not merge/deploy it yet. |
| 22 | Held test evidence | No | Run against local builds of the integrated application; no production stack change or mandatory staging. |
| 23 | Held test evidence | No | Same locally tested source/configuration; record hardware and limits without claiming production-equivalent performance. |
| 24 | Release PR merge | Coordinated deployment only | After local qualification, publish ordinary matched releases and merge the pinned production stack PR during the stop-first cutover. |

“Yes” is conditional on the preparatory merge invariant and normal CI evidence, not an automatic approval for a behavioral replacement. Task 22/23 harness code lands on the integration branch and runs locally before the coordinated integration PR merges to `main`. Behavior, configuration or harness changes require affected local cases to rerun. The operator's local results and release decision are recorded for Task 24.

## Integration and cutover sequence

1. Land preparatory tasks in dependency order, each with a current-production-mode regression check and isolated NATS test. Keep held task branches based on a common integration branch and regularly bring in `main`.
2. Complete Tasks 20a–20c on that branch. Provide the local build/start/test commands and run automated assembly checks. The user builds/tests the two-backend, three-NATS frontend/plugin system locally. Record the source revision, configuration, machine, commands and results; no published artifacts are required.
3. Run Tasks 22/23 fault and load checks on that local setup. Task 21 separately prepares and validates the unmerged production stack PR. Record local limitations; a one-machine run does not prove multiple physical failure domains or production capacity. Retest affected cases after behavioral/configuration changes. The operator's own acceptance stays pending until reported.
4. After local qualification and the operator's release decision, merge the held code as one coordinated integration PR. Freeze unrelated releases/deployments for cutover, review the unified Release Please version PR, and use the existing ordinary workflow to build/publish matched backend/frontend/plugin versions. No candidate build or promotion precedes this normal release. Keep automatically created infrastructure image-bump PRs held.
5. Perform Task 24's stop-first cutover with those pinned normal release versions in the reviewed Task 21 stack PR. Distribute the matching plugin and use existing release announcements in the coordinated window. Verify readiness and protocol rejection. Do not let an image-bump automation replace the stack review.

This deliberately permits ordinary releases throughout preparatory work while preventing a partial protocol or storage cutover. There is no old-client adapter or PostgreSQL fallback in the activated NATS-only runtime. ALB remains outside the project.
