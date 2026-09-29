# Task 17 — Durable EuroScope effect delivery

**Depends on:** 09, 13–16 and 15a. **Outcome:** each plugin side effect has one durable request, one dispatch claim, and a visible outcome.

**Release boundary:** merge with the plugin and browser protocol tasks in the coordinated candidate PR and release at task 24; see [release-safety.md](../release-safety.md).

## Work

- Implement the `EffectRequested → EffectDispatchClaimed → succeeded/failed/expired/unknown` session state machine and separate 30-second dispatch/result deadlines in [contracts.md](../contracts.md). Resolve an immutable target CID; wait only for that CID before claim. Store private-message bodies as temporary encrypted Object Store objects before committing their reference.
- Route claimed commands to the backend holding the exact socket generation. That backend verifies the committed claim before write. Persist plugin result before sending `ResultRecordedEvent`; accept duplicate results idempotently. Never auto-resend a claimed effect, including PDC, private message, FPL and AMAN outputs.
- Reconcile desired state-setting effects from a later EuroScope sync without changing an original terminal `unknown` into a false execution acknowledgment. Garbage-collect effect objects only after terminal outcome plus 24 hours.

## Done when

- Offline-before-claim target waits for the same CID and expires after 30 seconds. Disconnect or crash after claim results in persisted result or `unknown`, with no second socket command.
- Kill backend after plugin execution but before result acknowledgment: plugin outbox resends one result; frontend sees one terminal outcome.
- Private-message `succeeded` is documented and displayed as local send acceptance, never pilot receipt.

**Starting points:** `backend/internal/euroscope/hub.go`, `backend/internal/frontend/handlers.go`, PDC send paths, and task 14 plugin result code.
