# Task 19c — VATSIM transceiver frequency feed

**Depends on:** 12, 13 and the Task 19 provider candidate (#816).

**Outcome:** the global owner fetches and commits the VATSIM transceiver feed once per accepted refresh slot; PDC and sector policy read the same accepted typed frequency generation on every backend. Candidate remains dormant until Task 20 and held until Task 24 under Task 19's release boundary.

## Contracts

- Preserve provider parsing and frequency normalization from `vatsim/transceivers.go`, including flat/nested provider response shapes, invalid/empty-response rejection, callsign normalization, sorting and deduplication. External JSON is parsed only at the HTTP boundary.
- Add `TransceiverFeedPage { google.protobuf.Timestamp fetched_at = 1; repeated TransceiverFeedClient clients = 2; }` and `TransceiverFeedClient { string callsign = 1; repeated uint64 frequencies_hz = 2; }`. Callsigns and frequencies are canonical, sorted and unique; units are whole hertz. Store only accepted normalized radio frequencies. Reserve `ProviderPage.transceivers` oneof field 13. No additional entity or SystemCommand field is allocated.
- Provider is `vatsim`, global checkpoint resource is `transceivers/v3`. Reuse `ProviderPage`, Object Store content addressing, `ProviderCheckpoint`, `WorkflowRecord` and `ExternalCallWorker`. Persist an owner-fenced intent before each provider attempt and commit a validated typed result/checkpoint using its stable result UUID. An uncertain attempt cannot be repeated under the same intent; the next refresh slot has a distinct persisted identity. Preserve existing refresh-interval configuration.
- Export a concrete candidate source/reader with `GetFrequencies(callsign) []string`, compatible with both PDC and server source ports. It formats the accepted hertz values with current `NormalizeFrequency` semantics. Rebuild any local lookup map from the accepted checkpoint; it is never authoritative and never starts another provider poller. Missing/unready source state cannot invent frequencies.
- A changed committed checkpoint triggers session-owner reconciliation of frequency-dependent sector ownership through the Task 18c source port; owner takeover can recover a missed notification by comparing accepted source revisions. The provider worker must not directly run the legacy SQL sector refresh callback. Task 20 binds the concrete reader and callback constructors after both tasks merge.
- Parallel schema reservations: 18a owns ProviderPage field 12 and entity/command slots 31/14; 18c owns entity/command/effect slots 32/15/17. This task owns only ProviderPage field 13. Preserve the compatibility baseline and regenerate Go, TypeScript and C++ bindings.

## Work

- Implement the actual provider adapter and global-owned candidate refresh/recovery path using the existing HTTP client/normalization policy. Add a typed projection-backed lookup usable by PDC and sector candidate constructors.
- Audit the old transceiver update callback and expose a durable revision-aware reconciliation handoff; only runtime startup binding remains for Task 20.
- Update the worker inventory and Task 19 checklist with constructor and evidence links. No SQL, Redis, new persistence format, hidden JSON or ALB changes.

## Done when

- Real two-replica NATS tests use the actual parser/normalizer with HTTP fixtures and prove one provider call per intent/slot, nonowner rejection, owner failure around intent/call/result commit, accepted result replay and no repeat of an uncertain attempt.
- Both backend readers expose identical canonical frequencies after replay/takeover; malformed/empty responses preserve the last accepted generation. Frequency changes reach owner-routed sector reconciliation or remain recoverably pending by source revision.
- PDC/server source-port compatibility is verified without starting the legacy cache. Candidate construction needs no SQL or fake provider policy. Only Task 20 assembly remains.

## Candidate implementation and evidence

The dormant adapter, verified typed reader, and durable source-revision handoff
are implemented on `codex/multi-node-nats-19c-transceiver-feed` from `1cb4ea17`.
Draft [PR #821](https://github.com/flightstrips/FlightStrips/pull/821) targets the
integration base and remains unmerged.
See [Task 19c evidence](19c-transceiver-feed-evidence.md) for constructors,
contract details, two-replica HTTP/NATS failure tests and remaining Task 20
assembly. Integration merge remains pending; release stays held until Task 24.
