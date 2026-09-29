# Task 15a — Browser command identity and durable outcomes

**Depends on:** 13 and 15. **Outcome:** browser users can distinguish durable acceptance, execution and uncertainty across reconnects.

**Release boundary:** merge with task 15 in the coordinated candidate PR; release at task 24; see [release-safety.md](../release-safety.md).

## Work

- Require a UUID `FrontendCommand.request_id` for every mutation; translate it to the internal `CommandRequest.command_id`. Persist a typed `CommandOutcome` for authenticated domain results, including outcome-only rejections. Return `SUCCEEDED` only after a backend-only commit, `ACCEPTED` while an effect is pending, and the exact terminal states from [contracts.md](../contracts.md).
- Implement generated `FrontendActionResult`, `ActionStatusQuery` and `ActionStatusMissing` handling. Only the authenticated actor can query an outcome. Reusing an ID with different normalized Protobuf command content fails.
- Keep pending IDs and non-sensitive labels in browser session storage; keep action bodies only in live memory. On reconnect query at most 100 IDs, retry only an identical in-memory action with the original ID, and show a manual retry message after reload if the body was lost. Display failed, expired and unknown separately.

## Done when

- Lost WebSocket reply, backend restart and cross-node reconnect return the same durable outcome for one ID; another actor cannot query it.
- A duplicate identical command has one outcome; changed content under one ID fails. Private-message text never enters browser session storage.
- The UI never reports a plugin effect as executed merely because a socket write completed.

**Starting points:** `frontend/src/api/websocket.ts`, `frontend/src/store/store.ts`, backend frontend handlers and typed command ledger.
