# Multi-airport configuration design requirements

Status: current coded-handler and shared-service requirements, with concrete
combined EKCH/fictional examples. Runtime integration remains implementation work.

This document complements the [multi-airport discovery record](../multi-airport-support.md).
It distinguishes confirmed requirements from engineering recommendations and
examples drawn from the unreviewed [ENGM reference](engm-reference-notes.md).
It is not an implementation plan or an executable configuration.

The [draft architecture proposal](architecture-proposal.md) recommends a concrete
direction based on these requirements. Coded handlers, combined files, shared
VATSIM acquisition, and shared PDC are selected directions. The
[planning register](planning-readiness.md) records D1–D5 as confirmed on
2026-11-01. The proposal is not a formally accepted ADR or completed integration.

The [schema contract](schema-contract.md),
[JSON Schema](schema/airport-configuration.schema.json), and
[complete EKCH example](examples/ekch.airport.yaml) now make the authoring format
concrete. The validator checks structural/reference correctness and reconstructs
all current EKCH YAML settings; this does not claim runtime behavior parity.

## Confirmed configuration boundary

- Keep airport configuration in the repository; restarts are acceptable.
- Backend airport behavior should be configurable where practical. Airports
  can share a coded handler, such as RADIS, with concrete typed airport values.
  Custom behavior may be code rather than an expanded configuration language.
- Frontend views remain application code. Multiple airports may reuse a view,
  and new airports may require frontend changes for the foreseeable future.
- Coverage is configurable per airport and must consider tuned radio
  frequencies. Master selection is configurable independently per airport.
- Support airport-specific bays/workflow states, actions, ownership, validation,
  automatic transitions, and appearance/removal through typed settings plus
  coded handlers. This does not require all behavior to be YAML rules.
- Existing services should be reusable across airports through configuration
  and data, rather than separate implementations of CDM, AMAN, stand assignment,
  or text-pilot support for each airport.
- VATSIM network-data and transceiver feed acquisition/caches remain shared at
  application scope. Session-specific rules select relevant data and freshness
  requirements; they must not create an upstream poller or full feed replica
  per airport session.
- RADIS has no ownership. Its shared board, EuroScope writes, and participation
  requirements still apply without strip or sector ownership.
- Controller writes always use the initiating controller's own plugin; an
  airport handler must not override this by sending through the master. This
  human-action rule is distinct from D4: automatic work may use the session
  master or the controller who has assumed the aircraft.
- Use the current offline/read-only mechanism. Configuration does not introduce
  a new disconnected-user workflow.

## Configuration areas to represent

These are required capabilities of the combined configuration plus coded
handler/core contract, not a requirement to encode each behavior as a YAML rule.
The concrete field contract is defined in the linked schema.

| Area | What it needs to express |
| --- | --- |
| Airport identity and geography | Airport identifier, coordinates/elevation, runways, regions, relevant procedure/stand references. |
| Workflow reuse | Selection of a registered coded handler with concrete typed settings, without assuming all airports sharing a view have identical behavior. |
| Coverage | Operational responsibilities, matching positions/frequencies, tuned radio coverage, staffing-dependent fallback, and when no responsibility remains for an airport. |
| Master policy | Per-airport eligible positions/priorities and applicable election/replacement policy; independent of strip ownership and the identity of a command's sender. |
| Ownership and coordination | Whether strip/sector ownership applies, who may act, available handoffs, target resolution, pending state, and acceptance/cancellation where the workflow uses them. |
| Board/workflow state | Bay identifiers, initial/terminal states, shared ordering, relevant flight types, and movement rules. A displayed bay need not determine every aircraft or clearance state. |
| Actions and event transitions | Controller actions and external observations, conditions for applying them, resulting state, validation, and EuroScope/coordination effects. |
| Appearance and removal | Flight relevance, time/distance/position conditions, initial placement, completion/disconnect handling, retention, and manual remove/restore when supported. |
| Operational modes | Runway configuration, staffing, winter/deicing or other modes that influence applicable rules. |
| Services | Per-airport availability plus parameters and data references for the same reusable service implementations. |
| Frontend contract | The identifiers/capabilities/actions an airport's coded view consumes. Backend policy must remain authoritative for action validation and effects. |

The exact fallback lists, service subset, bay mappings, and thresholds for
EKBI/EKYT are intentionally deferred. Do not insert sample operating values as
accepted requirements.

## ENGM cases that constrain the format

These cases come from a draft whose review has not started. They are evidence
for design flexibility, not a commitment to implement every ENGM function in
the RADIS release.

| Reference case | Implication for configuration |
| --- | --- |
| Drag a departure from TAXI LINE to RUNWAY LINE to indicate lineup; use a clearance-box or runway-line-bar action to enter the same RUNWAY LINE with takeoff clearance. | A destination bay alone cannot determine the effect. Distinguish the action, direction/current context, destination placement, and clearance/EuroScope effects. |
| Three arrival interactions move a flight to the runway line with landing clearance. | Different interface gestures may invoke the same semantic action, while departure actions in a similar visual area have different effects. |
| West ground sends a departure for 01L to west tower, but a departure for 01R passes through east ground before east tower. | Route/target selection needs runway, current responsibility, and relevant staffing context. |
| SUCC HAND chooses an approach target using SID/runway; omnidirectional departures require manual TMA selection. | An action may use a configured target resolver or require an explicit target. Missing automatic routing must not silently select an arbitrary controller. |
| A transferred departure remains grey until approach accepts in EuroScope, then disappears; an inbound transfer can be accepted from either interface. | Pending coordination and external acceptance are separate events from initiating a handoff. Configured completion/visibility effects may depend on an external event. |
| Deicing/sequencing transfer targets depend on those positions being online. | Action availability and target resolution can depend on coverage, staffing, and optional capabilities. |
| Closed runways mark clearance boxes; the draft describes preventing departures. | The configuration needs conditions/validation, but whether a rule warns, blocks, or permits an override requires operational confirmation. |
| GATE/CNL remove strips; a regret action restores recently removed strips within fifteen minutes. | Removal can have reasons and retained state, with workflow-specific recovery policy; it need not mean immediate database deletion. |
| Local strips can be created without a filed flight plan, and obstacle strips represent runway obstructions. | Not every strip follows the same aircraft-backed lifecycle or can produce the same EuroScope writes. |
| West tower is a SUP role for shared settings. | An operational supervisor role is distinct from synchronization mastership. Do not overload the master flag as permission to change settings. |

One especially important distinction is that coverage responsibilities must be
available even for RADIS, which has no sector ownership. Reusing EKCH's
ownership machinery as the only way to determine airport participation would
encode an unwanted dependency.

## Coded decision and execution model

The selected model is a compiled handler with immutable typed settings. It
receives normalized context/actions/observations and returns a typed decision:

1. Validate workflow-specific conditions for the semantic action or event.
2. Describe local strip/clearance/coordination/visibility changes.
3. Describe any required core/domain-service effect as data.
4. Let the common executor recheck authority/revisions, persist according to the
   action's completion policy, dispatch effects, and reconcile results.

Handlers own no provider client, polling loop, database, or mutable airport
switch. Shared services perform I/O through the executor outside its critical
section. Controller effects retain their exact initiating plugin binding;
genuine automatic service effects may use the same session master or assuming
controller under D4.

ENGM's lineup and takeoff gestures can be separate coded actions reaching one
bay with different clearance decisions. RADIS can use simpler decisions without
ownership. Neither requires a general rule interpreter. ES observations
reconcile local state without repeating controller write effects; preserve
specific controller placement where generic ground state cannot describe it.

Independent airport strips can observe the same flight facts, but commands do
not copy board order/bays between airports. Source precedence and stable identity
remain explicit production-contract work (E3). The sample's immediate commit
does not replace the current production path. D3 chooses the existing EKCH
execution/error/reconciliation behavior without a new pending framework.

## Reusable service configuration

| Service | Intended boundary | EKBI/EKYT expectation |
| --- | --- | --- |
| CDM | Reuse the service with airport-specific rates, taxi/routing data, SID intervals, deicing/timing parameters, and runtime state scope as appropriate. | Included in the initial delivery set. Exact airport data deferred. |
| Stand assignment | Reuse the allocator with airport-specific stands, capabilities, airline/scenery data, and lifecycle settings; do not load EKCH data implicitly for another airport. | Included in the initial delivery set. Exact airport data deferred. |
| Text-pilot workflows | Reuse transport, sender routing, and result handling; airport-specific views/phrases/actions must match that airport's workflow. | Included in the initial delivery set. Exact airport interactions/data deferred. |
| AMAN | Preserve existing EKCH implementation, terminal/navigation configuration, and shared inputs. | Disabled at EKBI/EKYT, with no committed activation date. New-airport activation may remain deferred for a long time. |
| PDC | One shared application service, provider client/transport, logon key, and polling coordinator. Per-airport enablement/validation and session clearance state remain separate; distinct airport callsigns use the same deployment key. | Disabled at EKBI/EKYT, with no committed activation date. Preserve existing EKCH PDC. |
| Other existing services | Allow airport-specific enablement/configuration using reusable implementations. Keep reusable implementations; additional airport activation is future scope. | Not part of the initial regional service set unless explicitly requested. |

Availability, parameters, data, and runtime state must not accidentally be
shared merely because multiple airports use the same service implementation.
This does not prohibit sharing the underlying read-only VATSIM source: its
fetch/parse/cache lifetime is application-scoped, while derived operational
results remain airport/session-scoped.
PDC also deliberately shares its transport and credentials at application scope,
while requests, responses, validation, and timeout state retain explicit context.
Airport files must not contain copies of the shared logon or independent polling
configuration.
Repository inspection already finds package-global airport configuration and
stand registries, EKCH paths in stand-assignment loading, and EKCH constants in
bay calculations. Configuration scoping and service lifecycle scoping therefore
need to be considered together.

## Schema design and remaining implementation decisions

The [PR #958 review](radis-prototype-review.md) adds implementation evidence for
RADIS. Shared typed annotations must represent paper-style data separately from
ES fields. Its four observed bay IDs remain evidence, while operating effects,
admission/removal thresholds, and timestamp semantics remain open. Feed-only
operation and browser polling remain prototype shortcuts.

The selected direction is now **coded handlers with typed configuration**,
detailed in the [handler design and executable proof](handler-design.md).
Behavior in the earlier tables may be implemented in a shared or airport-specific
handler; it does not all need to be encoded in a rule language. Its values and
supported mappings remain configuration. EKBI/EKYT may share RADIS without
sharing operational state.

The [combined schema](schema-contract.md) defines one file per airport, including
handler selection, concrete typed settings, bays, ownership, services, and the
other airport data. The [EKCH example](examples/ekch.airport.yaml) preserves
existing settings, and [fictional ZZZZ](examples/zzzz.airport.yaml) exercises a
second handler. No separate profile/rule-override file is required. A new handler
must supply its settings/schema, factory registration, action contract, and tests.

Remaining integration includes production domain loading/validation, scoped
services, the updated session/membership protocol, required persistence changes,
frontend wiring, and all workflow paths beyond the executable proof. The code
examples do not activate airports or establish complete EKCH runtime parity.
Use the [planning register](planning-readiness.md) for confirmed scope and
engineering work inside epics. Include all current prototype features, the
annotation controls, and restoration of deleted strips. NEW remains strip-only;
FIND remains connected airport flights only. Follow EKCH execution and upgrade
backend/frontend/plugin together while nobody is online; no dual-protocol
compatibility machinery is required. Airport values remain release inputs.
