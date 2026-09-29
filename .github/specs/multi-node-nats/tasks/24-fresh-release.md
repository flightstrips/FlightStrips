# Task 24 — Fresh NATS-only production cutover

**Depends on:** 20–23. **Outcome:** merge, publish and deploy the matched new system empty under the operator's infrastructure repository.

## Work

- Confirm all prior task evidence, staged plugin DLL hash, pinned backend/frontend image digests, three labeled NATS hosts, Swarm secrets, encryption key, backup destination and restore rehearsal. The infrastructure stack PR is the reviewed deployment artifact. Verify the release workflow's manual plugin/announcement gate and absence of candidate `latest` tags.
- Verify the held application changes already merged as one coordinated candidate PR, and tasks 22 and 23 passed on the Release Please PR head. Merge that release PR with no intervening `main` change; require CI to promote the tested image digests without rebuilding and verify the staged plugin hash. Keep the automatically generated infrastructure image-bump PR unmerged; incorporate only its matching pinned versions into the reviewed task 21 stack PR.
- Execute the stop-first cutover in [operations.md](../operations.md): merge the reviewed task 21 stack PR to remove PostgreSQL/migrator, bootstrap empty NATS resources, deploy two backend replicas and the matching frontend, and require EuroScope protocol revision 2. Manually attach/distribute the verified plugin DLL and send the frontend announcement in the cutover window. Keep ALB code, endpoint and protocol unchanged. Do not copy existing data or run both old and new backends on the production route.
- Verify both `/readyz` endpoints, NATS quorum and disk, no SQL connection attempts, a cross-node live session, one master, command acknowledgment, and backend failover. Record deployment revision and observed results.

## Done when

- Production has three healthy NATS storage nodes and two ready backend replicas; no PostgreSQL or Redis runtime remains. New operational state survives a controlled backend restart.
- Users receive explicit results for plugin effects, and unsupported client protocol revisions are rejected.
- If the new deployment has accepted writes, recovery is fix-forward from retained NATS data and backup; the procedure does not suggest rolling back to the old PostgreSQL state.

**Starting points:** `flightstrips/infrastructure/stacks/flightstrips.yml`, the release record from task 23, and [operations.md](../operations.md).
