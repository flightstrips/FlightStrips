# Task 20a — Runtime and API assembly

**Depends on:** completed 04–19, including 15a/15b, 18a–18c and 19a–19c.

**Outcome:** a complete, testable NATS application constructor binds every enabled API, socket and worker to concrete accepted-state adapters.

**Release boundary:** held under [Task 20](20-app-cutover.md); merge only to the integration base. Task 20c installs this runtime as the sole default. No publication or deployment.

## Contracts and ownership

- Export `app.BuildNATS(ctx, cfg, deps) (*App, error)`, using the existing `app.Config` and `app.Dependencies` with typed extensions where necessary. It owns the NATS connection, resource verification, projections/replay, leases/router, complete planner composition, position/presence dispatchers, snapshots/recovery and effects. Backend startup verifies resources; only the separate administrator bootstrap creates them. Partial construction unwinds every acquired resource. Shutdown stops admissions, cancels and joins workers, drains sockets/NATS, and releases resources in a documented order.
- Admit authoritative commands and start owner workers only after their required projections/resources are ready. One effective supervisor exists for each session, airport and global aggregate. Provider work, effect dispatch/GC, master reconciliation and cleanup have stable identities, accepted checkpoints/deadlines, takeover recovery and explicit shutdown. Any retained local timer must match the non-authoritative timers in the worker inventory.
- Compose the actual candidate constructors from Tasks 18/19 and their evidence files. In particular bind `pdc.NewCandidate`, `services.NewCdmCandidate`, `services.NewCdmActionService`, `services.NewTrafficCandidate`, `euroscopebinary.NewDeadlineCandidate`, `services.NewVatsimLifecycleCandidate`, `amancandidate.New`, and `cluster.NewTransceiverSource`. Use the shared position dispatcher and barriers required by 18c/19a. Bind every callback listed in [session-worker-inventory.md](../session-worker-inventory.md) and the Task 19 checklist. Empty callbacks, injected policy decisions and legacy SQL workers cannot satisfy this task.
- Serve binary frontend and EuroScope transports at the existing `/frontEndEvents` and `/euroscopeEvents` paths using the fixed revision-2 contracts/subprotocols. Bind accepted snapshots/deltas, durable outcomes, controller presence, master election, effect claims/results and all planner actions. Preserve actor, revision, connection-generation and owner/master fencing checks. Handshake failures cannot register an operational connection.
- First-party HTTP stays JSON with existing paths, response shapes, authentication and feature flags. Bind stand status/preview, CDM, ECFMP, AMAN, pilot/EFB/PDC/GSX, command outcomes and enabled test tools. Reads use accepted typed projections; writes route to the aggregate owner with the existing HTTP idempotency contract. Port any remaining SQL-dependent HTTP handler/service adapter here. Task 20c must not inherit unconverted business behavior.
- **ALB is out of scope:** do not modify ALB source, protocol, endpoint, configuration or enablement. Preserve its existing construction when assembling shared services. No Redis or hidden JSON in NATS/storage/socket payloads; external provider formats remain boundary parsing only.
- `/healthz` is process liveness. `/readyz` fails during initial replay, lost quorum, resource drift or projection stall and recovers after verified catch-up; use the existing readiness contract rather than a constant response. Expose the specified projection/lease/effect metrics and safe logs with command ID, sequence and epochs, without tokens, private message bodies or provider secrets.
- Preserve the existing `main.buildVersion` entrypoint and component/development version behavior. The user tests local builds; no tested-tree build identity or release-candidate integration is required.

## Scope boundaries

- Own runtime code, handler adapters, startup supervisors, readiness/shutdown and real application tests. The old constructor may remain temporarily isolated for 20c; a process runs exactly one constructor and worker set. An explicit isolated candidate executable or selection is acceptable for tests, documented for 20c removal.
- Do not edit release workflows: 20b owns the minimal migrator cleanup. Do not perform final default Compose conversion or SQL deletion: 20c owns those. Preserve current Dockerfile/frontend build identity; no release-candidate tooling is required. Extract/share existing operational policy only when needed by the candidate; preserve behavior.
- Check existing handoff evidence before inventing ports. Resolve missing implementation within this PR; update contract/evidence documents when an actual incompatibility is discovered.

## Done when

- Two independent real `BuildNATS` applications start against the three-node fixture without a database. Tests connect actual binary socket clients and use unchanged JSON HTTP boundaries on different nodes; mutations/outcomes/reads agree after replay and owner takeover.
- Production PDC clearance/polling, CDM/traffic, AMAN, VATSIM lifecycle, EuroScope deadlines and squawk work through the assembled runtime with provider HTTP fixtures. No injected policy shortcuts, no-op planner or mock persistence replaces these bindings.
- Fault tests exercise readiness during replay/quorum loss/drift/stall and recovery; construction failure and shutdown leave no running owner/provider workers or admitted writes. Focus tests on combined assembly risks, reusing existing adapter fault evidence where applicable.
- A checked binding matrix records every enabled route, planner action and worker, its concrete constructor/source and acceptance test. Only features already disabled in current production may remain disabled, explicitly documented. Remaining final-entrypoint/SQL-deletion work is listed precisely for 20c.
- Backend build and applicable protocol/frontend/plugin checks pass. Open and attach a draft PR into `codex/multi-node-nats-base`, with evidence and exact remaining 20c bindings. Do not merge or release it.

**Starting points:** `backend/internal/app/app.go`, `backend/cmd/server/main.go`, candidate constructors and 18a/18b/18c/19a/19b/19c evidence files.
