# Task 18a — PDC polling and clearance lifecycle

**Depends on:** 09, 15, 15b, 17 and the merged Task 18/19 candidates. The frequency-reader interface can be implemented/tested in parallel with 19c; Task 20 binds its concrete accepted transceiver reader after 19c merges.

**Outcome:** concrete session-owner PDC polling, incoming-message processing, clearance composition and outbound provider calls work without SQL. Exports the `SessionWork.PDC` callback and candidate action bindings for Task 20.

**Release boundary:** held under the Task 18 rule; candidate remains dormant until Task 20 and releases only at Task 24.

**Implementation status:** the concrete `pdc.NewCandidate` constructor,
`Candidate.Bind` owner action planner and `Candidate.PDC` session callback are
implemented. Runtime assembly remains Task 20; legacy SQL startup is unchanged.
The constructor accepts injected `TransceiverLookup` sources; Task 19c's
`cluster.NewTransceiverSource` is exercised against its real HTTP provider,
accepted global checkpoint and both independent backend projections.

- [x] Exact additive parsed-message/page/effect schemas and regenerated Go,
  TypeScript and C++ bindings; compatibility baseline preserved.
- [x] Production request validation and clearance helpers shared by browser
  controller actions, HTTP Web PDC and authenticated Hoppie processing.
- [x] Accepted transceiver frequency reader, operational-controller eligibility,
  existing airborne priority and sector fallback used by clearance composition.
- [x] Stable provider message identity, committed checkpoint replay, durable
  poll slots, one-shot outbound intents, accepted results and uncertainty.
- [x] Pilot CID/sequence correlation, WILCO/UNABLE, ten-minute timeout recovery,
  stale-response rejection, confirmation/revert and Task 17 typed effects.
- [x] Real two-replica NATS/provider-fixture acceptance tests, including pending
  work, owner death around intent/result commits, snapshot/full restart,
  mandatory routes, actual browser remarks, deleted-strip cancellation and
  Web HTTP JSON without Hoppie.

Evidence: `NATS_INTEGRATION=1 NATS_TEST_PORT_BASE=6422 go test ./internal/pdc
-run 'TestNATSPdc|TestParsedHoppie' -count=1 -timeout 8m` against three
`nats:2.15.0` nodes. The ordinary backend suites, binary effect renderer tests,
schema/coverage checks and 75 browser command-mapping tests also pass. On
Windows, Docker-dependent service/testtool suites require
`DOCKER_HOST=npipe:////./pipe/dockerDesktopLinuxEngine`; they passed with that
endpoint after the default discovery path rejected rootless Docker.
The final gate on a clean fixture and the Task 19c base passed in 190.730
seconds, including deleted-strip cancellation and the accepted secondary
radio frequency in a production Web clearance.
`go vet -copylocks=false` passes for the changed packages. Default vet reports
existing copylock warnings in `cluster/ecfmp_test.go`,
`cluster/vatsim_reconcile_test.go` and `pdc/mock_hubs_test.go`.

## Contracts

- Reuse typed `PdcSequence`, `Strip`, `SessionDeadline`, actor-scoped command outcomes and owner CAS. Preserve request/issue/acknowledgement/timeout/revert semantics and the ten-minute response timeout. Compose clearance from accepted session, strip, runway, SID, route, ATIS and frequency inputs; browser ISSUE remarks are additional remarks, never a substitute for the clearance. Retrying a command UUID cannot change its clearance or target CID.
- Frequency lookup retains `GetFrequencies(callsign) []string` through an injected read-only source port. Task 19c provides the concrete typed projection reader for Task 20. The PDC adapter never starts the legacy transceiver cache or allocates `ProviderPage` field 13, which belongs to 19c.
- Hoppie response text is parsed at the provider boundary. Derive incoming identity with existing `HoppieMessageCommandID`; persist the UUID/hash and parsed fields, never `Raw`, `Packet`, JSON, or a serialized map. A malformed/unsupported message becomes a typed reason, without its raw body.
- Required additive storage shapes: `PdcProviderMessage` has `message_id` string field 1 (UUID), `from` string 2, `to` string 3, `transport` enum 4 (UNSPECIFIED=0, CPDLC=1, TELEX=2), `sequence` uint64 5, optional `response_to` uint64 6, `kind` enum 7, optional `HoppiePdcRequest request` 8, optional `clearance_text` string 9, `reason_code` string 10, optional Timestamp `provider_accepted_at` 11. Its kind enum is UNSPECIFIED=0, REQUEST=1, STATUS=2, CLEARANCE=3, WILCO=4, UNABLE=5, CONFIRMED=6, NO_RESPONSE=7, REVERT_TO_VOICE=8, FLIGHT_PLAN_NOT_HELD=9, UNAVAILABLE=10, INVALID_AIRCRAFT_TYPE=11, NOT_SUPPORTED=12, MALFORMED=13. Use nested `Transport`/`Kind` enums with `TRANSPORT_`/`KIND_` identifier prefixes to avoid Protobuf enum symbol collisions. Clearance text is radio clearance prose, not encoded data.
- `HoppiePdcRequest` fields are callsign=1, aircraft_type=2, departure=3, destination=4, stand=5, atis=6, remarks=7, all strings. `HoppiePollPage` fields are station=1 string, poll_id=2 UUID string, observed_at=3 Timestamp, messages=4 repeated PdcProviderMessage. Reserve `ProviderPage.hoppie` oneof field 12, `EntityKind.PDC_PROVIDER_MESSAGE` value 31 and `EntityRecord.pdc_provider_message` field 31. Entity key is message UUID within the session aggregate. Provider is `hoppie`, checkpoint resource is `station/<uppercase callsign>` within that session aggregate. Bound message/page sizes with the existing frame/object limits.
- Reserve `SystemCommand.apply_pdc_provider_message` field 14, carrying `ApplyPdcProviderMessage { PdcProviderMessage message = 1; }`. Only the authenticated provider/system adapter may submit it. The session owner validates identity, addressed station, request fields, sequence correlation and current PDC state before producing one atomic transition. The same incoming message UUID retains the same typed body across retries.
- Outbound messages persist the typed message plus a `WorkflowRecord` before a fenced provider attempt, then use `ExternalCallWorker` and stable result IDs. An uncertain poll/send is not repeated under the same intent; a later accepted poll slot is a distinct workflow. Commit a successful poll page/checkpoint before processing its messages, so takeover resumes accepted results. Persist the next poll deadline, choosing the existing 25–45 second interval once per accepted slot. Disabled Hoppie leaves Web PDC functional. Do not store the Hoppie logon secret.
- Plugin actions use Task 17 effects. If needed, append optional bool `cleared`=4, optional string `state`=5 and optional string `remarks`=6 to `PdcEffect`; actions `SET_CLEARED_FLAG` and `STATE_CHANGE` render those typed fields into existing revision-2 events. Existing ISSUE and REVERT_TO_VOICE actions remain valid. Route/SID changes use their existing typed effects.

## Work and scope

- Implement the actual request parsing/validation, fault acknowledgements, issue, pilot response correlation, confirmation, no-response and revert behavior using existing PDC policy/helpers. Bind frontend controller actions and candidate HTTP/Web PDC to the same owner path. Close the Task 15 clearance-composition gap.
- Supply a concrete candidate constructor and `func(context.Context, int32) error` polling callback. No SQL repositories, local authoritative timeouts, legacy hub sends or second polling loop in candidate construction.
- Own PDC files and the extensions reserved above. Tasks 18b/18c run in parallel; do not allocate their field numbers. Regenerate all bindings from the canonical schema and preserve the compatibility baseline. Update the task checklist/inventory with construction and evidence.

## Done when

- Real two-replica NATS tests exercise production PDC policy with a provider fixture: one poll per slot, committed result replay, owner death around poll/send/result commit, stable message IDs, no duplicate clearance send, correlated WILCO/UNABLE, timeout recovery and stale-response rejection.
- Actual browser/controller ISSUE composes a valid clearance; Web PDC works without Hoppie; pilot/EFB HTTP JSON contracts remain valid. Tests include restart with pending work and uncertain sends. Only runtime assembly remains for Task 20; no fake policy or stub callback can satisfy completion.
