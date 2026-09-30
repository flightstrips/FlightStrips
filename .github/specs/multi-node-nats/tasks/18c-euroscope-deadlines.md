# Task 18c — EuroScope deadline scheduling, retention and squawk throttle

**Depends on:** 05, 13, 16, 17, the Task 18 candidate and 19a.

**Outcome:** actual candidate socket events schedule/cancel durable session work, use shared aircraft retention, and generate squawks through the durable effect path. Candidate remains dormant until Task 20 and held until Task 24 under Task 18's release boundary.

## Contracts

- Socket nodes submit typed owner-routed events after authenticated connection/generation checks. Session owners schedule/rearm/cancel `SessionDeadline` records for controller-offline, aircraft-disconnect, session-update and session-disconnect. Reuse existing deadline kinds and source-revision meanings; reconnect/newer entity or observation revisions invalidate old deadlines. Cross-node coverage and shared observations determine outcomes, not a node's local socket count.
- Preserve current grace/debounce defaults and behavior from EuroScope offline/disconnect code. Provide concrete session-update/disconnect sector/layout/route reconciliation callbacks and a shared `SessionWorkerPlanner.RetainedAircraft` implementation. Read accepted VATSIM/lifecycle/position state; stale or unavailable data fails closed on deletion. Match Task 19a retention/protected occupancy and preserve newer controller edits.
- Sector frequency lookup uses the existing read-only `GetFrequencies(callsign) []string` source port. Task 19c supplies its concrete typed reader at Task 20. Reconcile accepted transceiver checkpoint revisions as well as socket changes; do not start a local provider poller or allocate `ProviderPage` field 13.
- Squawk generation uses the existing typed browser `GenerateSquawk` action and revision-2 `GenerateSquawkEvent`. Add `GenerateSquawkEffect { string callsign = 1; }` at `EffectRecord.generate_squawk` oneof field 17. Do not encode the operation into a flight-plan value string. Select a live operational target CID when accepting the request and retain it immutably; delivery requires the exact claimed socket generation.
- Add `SessionSquawkThrottle { int32 session_id = 1; google.protobuf.Timestamp next_allowed_at = 2; }`, `EntityKind.SESSION_SQUAWK_THROTTLE` value 32 and matching `EntityRecord.session_squawk_throttle` field 32. Canonical entity key is the decimal session ID. No separately serialized queue is needed: WAITING squawk effects form the queue, ordered by their accepted command outcome's committed stream sequence, then UUID.
- Reserve `SystemCommand.request_squawk` field 15, carrying `RequestSquawk { string callsign = 1; string target_cid = 2; }`, for the authenticated automatic adapter. Validate the strip and target CID at the owner. Manual controller authorization follows the existing client command rules. An identical UUID is an idempotent retry; a different UUID for a callsign already waiting returns `SQUAWK_ALREADY_PENDING` and cannot retarget the pending effect.
- Admit at most one squawk dispatch claim per session every five seconds. Commit the throttle change and effect DISPATCH_CLAIMED transition in the same accepted event. Derive `next_allowed_at` from that event's JetStream server time plus five seconds in replay/snapshot state; proposer clocks cannot shorten it. Recheck the current strip immediately before claiming; a removed strip or valid assigned squawk cancels waiting work without sending. Lost PubAck/owner takeover cannot issue a second send under a claim. Claimed uncertainty follows Task 17's UNKNOWN behavior; never requeue a claimed request.
- Schema ownership: this task owns enum/record slot 32, SystemCommand slot 15 and EffectRecord slot 17. Task 18a owns enum/record slot 31, SystemCommand slot 14 and ProviderPage slot 12. Preserve all existing wire/storage numbers and regenerate bindings without replacing the compatibility baseline.

## Work

- Connect concrete scheduling/cancellation to binary EuroScope login, controller changes, disconnect, aircraft observations and master sync. An admitted state change must either commit its required deadline in the same owner transition or be recoverably reconciled from accepted state after a crash; a best-effort socket callback alone is insufficient.
- Implement the shared retention decision and the real sector/layout/route outcomes. Expose constructor/binding dependencies for Task 20. Landing-clearance delayed validation remains disabled; do not create an unowned enabled timer.
- Implement durable automatic/manual squawk queuing, claim throttling, renderer and plugin outcome handling. The current in-memory SQL throttle stays exclusive to the old runtime until Task 20.
- Update the session inventory and document any retained delivery-only timers. Do not change ALB.

## Done when

- Real two-replica NATS tests use actual binary socket event adapters: cross-node controller coverage, reconnect invalidation, deadline rearm, master-generation changes, shared aircraft retention and takeover before/after scheduling and expiry.
- Actual squawk requests share one five-second dispatch rate across replicas and takeover; duplicate UUID, already-pending callsign, valid assigned squawk, lost claim acknowledgement, owner death and plugin result uncertainty cannot duplicate the send.
- Concrete production callbacks satisfy the tests; no injected retention policy or no-op reconciliation satisfies completion. Only runtime assembly remains for Task 20.
