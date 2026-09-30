# Merge and release safety contract

## Answer and terms

**No: the current tasks are not all independently releasable.** A task is a reviewable unit of implementation, not necessarily a production release unit. This migration has one coordinated activation of backend, frontend, EuroScope plugin, and infrastructure. A task marked **preparatory** below may merge to `main` and be included in ordinary releases only while its new behavior is dormant. A task marked **held** is developed on the integration branch and merges to `main` only in one coordinated candidate PR; it is not individually released. A task marked **staging** can complete before activation without merging a production stack change. Task 24 is the production activation itself.

There are three distinct events:

1. **Merge to application `main`:** Release Please runs on that push and may prepare a release PR. A merge does not by itself deploy the production stack.
2. **Publish a release:** under today's workflow, merging the Release Please PR can publish backend/frontend images, including mutable `latest` tags, and attach a plugin DLL to a GitHub release. The frontend release also triggers a Discord announcement. Publication can expose an incompatible client even while the production stack remains old. Task 20 changes this workflow before activation: omit `latest` for the candidate images, retain pinned version tags, stage the plugin DLL as a verified CI artifact, and move DLL attachment and frontend announcement to a manually dispatched cutover job.
3. **Deploy production:** the release workflow opens a `flightstrips/infrastructure` image-bump PR; its merge changes the Portainer-watched stack. Do not merge an image-bump PR for a held candidate. Production must use pinned version tags, never `latest`.

The release workflow currently has no migration-aware publish gate. Task 20 must add that gate before the coordinated candidate PR merges to application `main`. After that merge, hold the Release Please PR until staging gates pass; no other release PR or production deployment may merge during this window. In particular, do not publish a revision-2 plugin DLL or deploy the binary-only frontend while the PostgreSQL/JSON deployment is live. Ordinary releases of preparatory work are safe only if their built artifacts still behave like the current deployment.

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
| 14 | No; held | No | Replacing active EuroScope schema, token and plugin behavior is part of the coordinated candidate PR. |
| 15 | No; held | No | Binary-only browser/backend transport is part of the coordinated candidate PR. |
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
| 20 | No; held | No | Removing SQL/migrator and wiring NATS-only startup is activation code. |
| 21 | Infrastructure PR only | No | Prepare and validate the production stack PR; deploy an equivalent isolated staging stack. Do not merge the production PR yet. |
| 22 | Held test evidence | No | Run against the integrated candidate and staging stack; no production stack change. |
| 23 | Held test evidence | No | Same candidate and production-equivalent staging placement; record exact versions. |
| 24 | Release PR merge | Coordinated deployment only | Promote the tested backend/frontend candidate, then merge the pinned production infrastructure PR during the stop-first cutover and distribute the tested plugin. |

“Yes” is conditional on the preparatory merge invariant and normal CI evidence, not an automatic approval for a behavioral replacement. Task 22/23 test harness code that needs held implementation lands in the coordinated candidate PR before qualification; later harness fixes change the candidate and invalidate affected gates. Their pass results are gates for task 24.

## Integration and cutover sequence

1. Land preparatory tasks in dependency order, each with a current-production-mode regression check and isolated NATS test. Keep held task branches based on a common integration branch and regularly bring in `main`.
2. Complete the held code on that branch and pass its pre-merge integration checks. Merge it as one coordinated candidate PR after task 20's publication gate is reviewed. Freeze other application releases and production deployments. This main merge only runs build/test CI and causes Release Please to open the unified version/release PR; it does not deploy production.
3. On the Release Please PR head, build backend/frontend images under immutable candidate tags and build the revision-2 plugin DLL as a hash-checked CI artifact. Run tasks 21–23 against those exact candidate image digests, DLL hash, and reviewed staging stack. The task 21 production infrastructure PR remains open. Any source, release PR tree, image digest, DLL hash, or staging stack change after a gate requires the affected gate to rerun.
4. After all gates pass, merge the unified Release Please PR with no intervening `main` changes. CI verifies that the merged release tree equals the tested PR tree and **promotes** those same OCI image digests to pinned version tags without rebuilding. It does not update `latest`, attach the plugin DLL, or announce the frontend. A tree/digest mismatch fails publication and returns to staging tests. Hold any automatically created infrastructure image-bump PR until the reviewed task 21 stack PR has the matching pinned versions.
5. Perform task 24's stop-first cutover. Merge the reviewed infrastructure PR, then manually attach/distribute the verified plugin DLL and send the frontend announcement in the cutover window. Verify readiness and protocol rejection. Do not let an image-bump automation replace the task 21 stack review.

This deliberately permits ordinary releases throughout preparatory work while preventing a partial protocol or storage cutover. There is no old-client adapter or PostgreSQL fallback in the activated NATS-only runtime. ALB remains outside the project.
