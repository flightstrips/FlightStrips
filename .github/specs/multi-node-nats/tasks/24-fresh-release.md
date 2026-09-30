# Task 24 — Fresh NATS-only production cutover

**Depends on:** 20–23. **Outcome:** merge, publish and deploy the matched new system empty under the operator's infrastructure repository.

## Work

- Confirm all prior implementation evidence and the user's reported local acceptance, including source revision, configuration, fault/load results and restore rehearsal. Verify three labeled production NATS hosts, secrets, encryption key, backup destination and the reviewed infrastructure stack PR. No release-candidate builds, promotion or staging evidence is required. Release/deployment actions require the operator's decision for this cutover; task completion alone does not authorize them.
- After local qualification, merge held changes as one coordinated integration PR and review/merge the unified ordinary Release Please PR when the operator is ready. Normal CI builds/publishes matched component releases, DLL/config attachments and announcements under the existing workflows. Check the resulting versions and actual plugin hash; there is no candidate artifact to promote. Keep automatic infrastructure image-bump PRs unmerged and incorporate matching pinned normal versions into the reviewed Task 21 stack PR. Retest affected local cases if behavior changes after local acceptance; version-only metadata changes are reviewed separately.
- Execute the stop-first cutover in [operations.md](../operations.md): merge the reviewed Task 21 stack PR to remove PostgreSQL/migrator, bootstrap empty NATS resources, deploy two backend replicas and the matching frontend, and require EuroScope protocol revision 2. Distribute the matching ordinary released plugin and coordinate existing release announcements in the cutover window. Keep ALB code, endpoint and protocol unchanged. Do not copy existing data or run both old and new backends on the production route.
- Verify both `/readyz` endpoints, NATS quorum and disk, no SQL connection attempts, a cross-node live session, one master, command acknowledgment, and backend failover. Record deployment revision and observed results.

## Done when

- Production has three healthy NATS storage nodes and two ready backend replicas; no PostgreSQL or Redis runtime remains. New operational state survives a controlled backend restart.
- Users receive explicit results for plugin effects, and unsupported client protocol revisions are rejected.
- If the new deployment has accepted writes, recovery is fix-forward from retained NATS data and backup; the procedure does not suggest rolling back to the old PostgreSQL state.

**Starting points:** `flightstrips/infrastructure/stacks/flightstrips.yml`, the release record from task 23, and [operations.md](../operations.md).
