# Task 22 — Cross-node fault and recovery suite

**Depends on:** 17–20, including the Task 20c local setup. Can run alongside Task 21's production stack preparation. **Outcome:** repeatable local fault tests and recorded proof that coordination and persistence hold under the tested failures.

**Release boundary:** harness code lands on the held integration branch; tests run against local builds on the user's machine. No release-candidate or staging artifacts are required. Behavior/configuration/harness changes require affected local checks to rerun. The operator's own acceptance is recorded only after they report it.

## Work

- Run three NATS nodes, two backends, and frontend/EuroScope clients split across both. Cover concurrent session creation, strip/stand/AMAN races, one master, CID-targeted effects, projection parity and initial snapshot/delta ordering.
- Inject backend death before/after PubAck, effect claim, socket write and plugin result; old-master input; one and two NATS-node loss; snapshot corruption; full cluster shutdown/restart; and a controller-free outage longer than five minutes. Assert the exact outcomes in [contracts.md](../contracts.md), [state-map.md](../state-map.md), and this task, not merely process survival.
- Restore an off-cluster backup into an isolated three-node cluster and compare entity counts/revisions, command outcomes, audits, nav digests, latest positions and pending deadlines. Confirm NATS credentials and encrypted-store key restoration.
- Record integrated source revision, local backend/frontend/plugin build commands, DLL hash where available, NATS/configuration versions, machine resources and test commands/results. Rerun affected cases after changes before release. The local fixture uses distinct NATS processes/volumes; do not describe them as separate physical hosts.

## Done when

- No acknowledged state is lost, no stale epoch changes state, no claimed effect is automatically resent, and all uncertain effects end visibly `unknown`.
- `/readyz` rejects unready backends; a surviving replica serves reconnecting clients with equivalent state. Exercise routing through the local proxy where provided; validate production Traefik health-check configuration in Task 21.
- Fault timings, command IDs, stream sequences and restore checksums are attached as test artifacts; failures are reproducible.

**Starting points:** existing backend e2e harness, `backend/internal/testing`, and Task 20c's local three-NATS/two-backend setup.
