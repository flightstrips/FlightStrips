# Task 22 — Cross-node fault and recovery suite

**Depends on:** 17–21, with task 21's staging deployment and unmerged production PR. **Outcome:** recorded proof that coordination and persistence hold under real failures.

**Release boundary:** test harness code that requires held implementation lands in the coordinated candidate PR before qualification; tests run against its Release Please PR artifacts in staging. A later harness fix invalidates affected gates.

## Work

- Run three NATS nodes, two backends, and frontend/EuroScope clients split across both. Cover concurrent session creation, strip/stand/AMAN races, one master, CID-targeted effects, projection parity and initial snapshot/delta ordering.
- Inject backend death before/after PubAck, effect claim, socket write and plugin result; old-master input; one and two NATS-node loss; snapshot corruption; full cluster shutdown/restart; and a controller-free outage longer than five minutes. Assert the exact outcomes in [contracts.md](../contracts.md), [state-map.md](../state-map.md), and this task, not merely process survival.
- Restore an off-cluster backup into an isolated three-node cluster and compare entity counts/revisions, command outcomes, audits, nav digests, latest positions and pending deadlines. Confirm NATS credentials and encrypted-store key restoration.
- Record the integrated candidate source revisions, image digests, plugin DLL hash and staging stack revision. Rerun affected cases if any candidate changes before release.

## Done when

- No acknowledged state is lost, no stale epoch changes state, no claimed effect is automatically resent, and all uncertain effects end visibly `unknown`.
- `/readyz` and Traefik remove unready backends; a surviving replica serves reconnecting clients with equivalent state.
- Fault timings, command IDs, stream sequences and restore checksums are attached as test artifacts; failures are reproducible.

**Starting points:** existing backend e2e harness, `backend/internal/testing`, and the staging Swarm stack from task 21.
