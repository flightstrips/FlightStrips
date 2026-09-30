# Task 21 — Production Swarm infrastructure

**Depends on:** 01, 03, 13 and 20. **Outcome:** a reviewed production stack PR is ready. The production PR stays unmerged until Task 24; the user qualifies local builds on their machine, with no required staging deployment or release-candidate artifacts.

## Work

- Prepare an unmerged PR for `flightstrips/infrastructure/stacks/flightstrips.yml` that replaces PostgreSQL/migrator with three pinned `nats:2.15.0` services on distinct labeled hosts and separate persistent volumes. Configure routes, client/route TLS, JetStream encryption key, internal networking, health checks, and scoped versioned Swarm secrets. Add one-shot resource bootstrap; backend tasks only verify resources. Validate the rendered configuration and use the Task 20c local setup for rehearsals; do not deploy staging or production to complete this task.
- Run two backend replicas with NATS URLs/credentials and Traefik `/readyz` service health checks at two-second interval/one-second timeout. Preserve provider, OIDC, aircraft config, frontend and docs settings. Use stop-first for the initial fresh cutover.
- Remove infrastructure `.github/workflows/check-drift.yml` and migrator tag coupling. Validate rendered stack, placement labels, three file-backed replicas, external secrets/configs, pinned tags, and backup/restore instructions in its README.

## Done when

- Rendered configuration requires three NATS tasks on distinct labeled production hosts with independent volumes and internal-only NATS ports, plus two backends without database services. Local rehearsal verifies startup/readiness/bootstrap. The production stack PR remains unmerged; actual production placement is verified at Task 24, not claimed from one-machine testing.
- CI rejects a floating tag, missing secret/volume, wrong NATS placement or missing `/readyz` routing. One lost NATS node retains quorum; backup restores to an isolated cluster.
- The stack PR links Task 20/local testing evidence. After local qualification and the operator's release decision, Task 24 inserts the matching ordinary released component versions and merges the reviewed PR as the production deployment action. No candidate digest/promotion or staging dependency is required.

**Starting points:** [`flightstrips/infrastructure`](https://github.com/flightstrips/infrastructure), `stacks/flightstrips.yml`, `README.md`, and `.github/workflows/validate-stack.yml`.
