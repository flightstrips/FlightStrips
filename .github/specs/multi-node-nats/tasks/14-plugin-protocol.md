# Task 14 — EuroScope protobuf and local execution result

**Depends on:** 02. **Outcome:** the plugin can report local command execution precisely; backend delivery follows in task 17.

**Release boundary:** merge only with the coordinated candidate PR; publish the revision-2 DLL at task 24's cutover. Rejection of the current plugin protocol cannot be released independently; see [release-safety.md](../release-safety.md).

## Work

- Replace root `proto/euroscope.proto` with the field-numbered candidate [euroscope.proto](../proto/euroscope.proto) from the frozen contract, then regenerate Go and C++ code together. Reject plugin protocol revision other than 2 at authentication; no old-protocol path.
- Require `flightstrips.euroscope.pb.v2` WebSocket subprotocol. Refactor plugin command handlers to return `EXECUTED` or `FAILED` after their EuroScope API/UI operation runs on the correct thread. Include bounded reason codes, including partial execution. For private messages, report local send acceptance only.
- Keep unacknowledged results in an in-process outbox until `ResultRecordedEvent`; resend them after socket reconnect and deduplicate received command IDs within the plugin process.

## Done when

- Generated Go/C++ dispatch tests cover new fields and oneof cases; old plugin token is rejected.
- Each setter/send/UI failure maps to a reason; execution result is emitted after local operation, never merely after queueing an incoming WebSocket frame.
- Reconnect resends an unconfirmed result but does not execute the same command twice in the surviving plugin process.

**Starting points:** `proto/euroscope.proto`, `backend/pkg/events/euroscope`, and `euroscope-plugin/src/plugin/messages/MessageService.cpp`.
