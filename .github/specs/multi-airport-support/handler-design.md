# Coded workflow handlers: contract and executable proof

Status: selected direction for the architecture draft, with an isolated Go
example. This is not application activation or a complete EKCH migration.

The [consistency review and planning register](planning-readiness.md) separates
the D1–D5 decisions confirmed on 2026-11-01, proportionate engineering work
(E1–E7), and airport rollout inputs. The user selected existing EKCH execution
and a coordinated offline upgrade; the proof does not establish full runtime parity.

## Boundaries and dispatch

Use one combined airport YAML file. It selects a registered workflow handler and
contains that handler's concrete typed settings, bays, and declared annotations,
alongside coverage, master policy, ownership, views, services, and validation.
Large domain datasets may remain referenced files. There is no separate workflow
profile, inherited rule list, embedded script, or generic interpreter.

At startup, validate the full document and resource references, resolve the
registered implementation, decode/validate its settings, and construct an
immutable definition. Select that definition when creating a session. Every
event then calls the already bound handler directly. No per-event config
decoding, file access, registry scan, handler discovery, or airport conditionals
are required in the common dispatcher.

| Part | Owns |
| --- | --- |
| Airport definition | Immutable settings, handler instance, known bays/annotations, service dependencies, and revision. |
| Handler | Workflow decisions for typed actions and observations. It is stateless and can be shared by all sessions using the definition. |
| Session/core | Mutable strip state, isolation, synchronization, revisions, permissions, persistence/broadcasts, and execution of validated effects; shared services may own session-keyed partitions without separate clients/workers per session. |
| Services | Existing CDM/stand/AMAN algorithms with explicit scoped dependencies; shared VATSIM acquisition stays outside handlers and sessions. |

PDC is an application-level shared service too: one provider client, transport,
polling coordinator, and deployment logon serve all enabled airports. A handler
requests operations through the core with explicit airport/session context; it does not own a PDC
client or credentials. Airport validation and clearance/timeout state remain
scoped. The isolated handler example does not instantiate external services.

Production hooks should cover controller actions, normalized observations,
admission/lifecycle evaluation, and coordination where required. The executable
proof implements only bay movement, full-flight observations, and one custom
action. Those hooks are small enough to exercise the boundary without claiming
the rest of the application has been migrated.

The handler receives a session snapshot and a typed event and returns a decision
containing state changes and effect intents. It cannot access the database,
transport, VATSIM feed, or a mutable current-airport global. The common executor
checks the complete decision before applying any changes. The production
executor must add transactional persistence, broadcasts, and reliable dispatch;
an emitted ES intent is not an acknowledgement.

Production domain-service requests are typed intents executed by the core;
handlers do not synchronously invoke service I/O. Normalize service results back
into decision inputs and release session/strip locks or database transactions
before network calls. Human deferred actions retain their own-plugin authority;
D4 permits genuine automatic effects through the same session master or the
controller who has assumed the aircraft, using the existing suitable resolver.
Human-originated work retains its own plugin. PDC and AMAN stay disabled at
EKBI/EKYT, while current EKCH services are preserved.

Use one explicit owner for each event. Avoid chains of generic, EKCH, and airport
fallback handlers. Share helpers or a whole workflow implementation where
behavior matches: EKBI/EKYT can share RADIS, while retaining separate definitions
and session state. A distinct feature can be implemented in a dedicated handler
without adding a configuration language construct or branching throughout the
shared services.

## Concrete implementations in the proof

The opt-in [handler proof patch](examples/handler-proof.patch) contains the
example package, handlers, in-memory executor, tests, and demo command. It is a
specification artifact; application files are not added by this spec PR.

| Config and handler | Demonstrated behavior |
| --- | --- |
| [EKCH](examples/ekch.airport.yaml), `ekch/v1` | Wraps the actual `shared.GetDepartureBay` and `shared.GetGroundState` functions. A TAXI observation selects TAXI or TAXI_LWR depending on ground staffing. Moving to DEPART produces a LINEUP intent. Existing specific taxi placement and forward-only departure behavior remain delegated to the real helper. |
| [Fictional ZZZZ](examples/zzzz.airport.yaml), `example-paper/v1` | A first TAXI observation selects HOLD. Later observations preserve manual board placement. Moving to RUNWAY emits the configured LINEUP intent and sets a shared `review_required` flag. The coded `review.clear` action clears that flag only in the runway bay; it is not supported by EKCH. |

ZZZZ, its geography, frequencies, thresholds, and operating values are invented
demonstration data. It is not an airport to deploy and does not establish RADIS
requirements. It has no ownership and disables all services. No coded frontend
for this fictional view is included.

EKCH's combined example preserves all 20 existing YAML settings, all 18 bays,
and selected code-derived values. Its helper still contains geography/distance
constants: the example adapter explicitly rejects different values rather than
claiming the helper is fully configurable. Its positive airborne threshold is
passed from typed settings into the existing function.

## Configuration and extension contract

`workflow.handler` is a versioned registered ID. Each implementation has a
closed settings object selected by JSON Schema `oneOf` and a corresponding Go
settings type decoded with unknown-field rejection. Unknown IDs/settings,
invalid references, incompatible ownership, and unsupported annotations fail
validation. The current registry and schema define exactly `ekch/v1` and
`example-paper/v1`.

Adding a real implementation requires its typed settings/schema branch, factory
registration, handler code, supported action contract, and behavioral tests.
Changing only airport values uses the same implementation without adding code.
Compiled handlers do not mean a separate copy of the shared backend per airport.

The sample executor still has an explicit allowlist for `strip.move` and
`review.clear`. A production dispatcher must obtain supported typed action
descriptors from handler registration, rather than add each custom action to
the common core. Likewise, the sample's callsign-keyed in-memory storage is not
the production identity contract for reused callsigns or manual strips (E2/E3).

Annotations belong to session strips. This proof exercises one declared boolean
flag, not a general implementation of every RADIS text/timestamp field. Future
RADIS settings must concretely declare those fields and their types. Actual ES
fields remain distinct. The shared whole-board read-only policy also applies to
annotation-only actions.

## Isolation, correctness, and performance checks

The example constructs EKCH and ZZZZ simultaneously from their complete combined
files and also exercises two sessions sharing one definition and two airport
definitions sharing the paper implementation with different settings. Tests use
the same callsign across sessions, concurrent conflicting edits, and independent
observations. Returned snapshots do not alias stored state.

Shared guards reject observer/disconnected/wrong-session edits, missing own
plugin binding, stale revisions, unknown bays, configured ownership violations,
invalid handler decisions, and observation-to-ES write loops. The executor
supplies the outbound plugin identity; the handler cannot choose another sender
or request master fallback. Binding and observation inputs in this proof are
trusted test/core inputs; production authentication and membership-generation
validation remain integration work.

One mutex protects each sample session. Different sessions do not share a lock;
immutable handler/config state requires no event-path lock. This is an example
of serialization boundaries, not a replacement decision for the current database
transaction model. Tests run under the Go race detector. Benchmarks measure the
complete in-memory session execution path, including guards, snapshots, and
result construction; they exclude YAML loading and do not establish production
latency or an end-to-end throughput guarantee.

A local Windows/amd64 run on 2026-10-30 (Go 1.26.0, Ryzen 7 7700X) measured:

| In-memory command path | Time/op | Allocated bytes/op | Allocations/op |
| --- | --- | --- | --- |
| EKCH | 256.2 ns | 272 | 8 |
| Fictional paper workflow | 633.9 ns | 1,152 | 13 |

These are single-run development measurements of the whole sample executor,
not a before/after production comparison or an operational latency target.

To run the isolated proof from a clean spec checkout, first apply its optional
artifact from the repository root. This creates demonstration files under
`backend` locally; it does not wire them into application startup:

```powershell
git apply --check .github/specs/multi-airport-support/examples/handler-proof.patch
git apply .github/specs/multi-airport-support/examples/handler-proof.patch
cd backend
```

Then run from `backend`:

```powershell
go test -race ./internal/airportworkflow ./internal/shared
go test ./internal/airportworkflow -run '^$' -bench BenchmarkSessionDispatch -benchmem
go run ./cmd/workflow-example ../.github/specs/multi-airport-support/examples/ekch.airport.yaml ../.github/specs/multi-airport-support/examples/zzzz.airport.yaml
```

The command prints both decisions, own-plugin command intents, and a blocked
disconnected edit. It starts no application services and performs no network or
database writes. Validate the full document/reference contract separately using
the [schema tool](schema/validate_examples.py), as described in the
[schema contract](schema-contract.md).

## What remains before real integration

Application startup still loads the existing EKCH YAML. No application route,
WebSocket, protocol, database, membership, or service assembly has been replaced.
The example loader reads/validates the workflow projection of the combined file;
unconsumed domain blocks are YAML nodes and are validated by the full schema tool,
not by a claimed complete production Go loader.

The sample immediately commits an in-memory move and returns an outbound intent;
it has no durable pending state, request deduplication, or ES acknowledgement
protocol. D3 selects reuse of the existing EKCH production execution/error path,
not a new confirmation gate or durable pending-move framework. Its trusted
binding, per-session mutex, and side-effect return values are evidence of the
boundary only, not a substitute for authenticated epochs, database transactions,
or reliable dispatch. The example is not a background-service executor.

Incremental ground-state/position messages, admission/removal/recovery,
coordination including EKCH SQ behavior, other field edits, CDM/stand/text-pilot
integration, and frontend actions still need adaptation. Preserve existing EKCH
PDC/AMAN; activation at EKBI/EKYT is deferred without a date. Backend/frontend/
plugin will be upgraded together while nobody is online, with no parallel legacy
protocol or extended support-window work. The tested EKCH paths
delegate to real functions; this is not proof that every existing path has moved
behind the interface or that full runtime parity is established.
