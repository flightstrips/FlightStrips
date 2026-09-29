# Task 21 — Production Swarm infrastructure

**Depends on:** 01, 03, 13 and 20. **Outcome:** a reviewed production stack PR is ready, and an equivalent isolated staging stack deploys the fixed three-node topology. The production PR stays unmerged until task 24.

## Work

- Prepare an unmerged PR for `flightstrips/infrastructure/stacks/flightstrips.yml` that replaces PostgreSQL/migrator with three pinned `nats:2.15.0` services on distinct labeled hosts and separate persistent volumes. Configure routes, client/route TLS, JetStream encryption key, internal networking, health checks, and scoped versioned Swarm secrets. Add one-shot resource bootstrap; backend tasks only verify resources. Deploy equivalent configuration only in isolated staging before task 24.
- Run two backend replicas with NATS URLs/credentials and Traefik `/readyz` service health checks at two-second interval/one-second timeout. Preserve provider, OIDC, aircraft config, frontend and docs settings. Use stop-first for the initial fresh cutover.
- Remove infrastructure `.github/workflows/check-drift.yml` and migrator tag coupling. Validate rendered stack, placement labels, three file-backed replicas, external secrets/configs, pinned tags, and backup/restore instructions in its README.

## Done when

- In staging, three NATS tasks run on three hosts with independent disks; no NATS port is public. The stack starts two ready backends without database services. The production stack PR remains unmerged.
- CI rejects a floating tag, missing secret/volume, wrong NATS placement or missing `/readyz` routing. One lost NATS node retains quorum; backup restores to an isolated cluster.
- The stack PR links the tested application candidate digests and task 20 evidence. After tasks 22 and 23 pass and matching release tags are pinned, task 24 merges this PR as the production deployment action.

**Starting points:** [`flightstrips/infrastructure`](https://github.com/flightstrips/infrastructure), `stacks/flightstrips.yml`, `README.md`, and `.github/workflows/validate-stack.yml`.
