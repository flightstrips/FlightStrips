# Multi-airport support

Status: requirements discovery recorded; architecture proposal drafted, airport
values deferred; coded-handler/shared-service directions selected, no formal ADR accepted.

Started: 2026-10-29.

Current planning status: the [consistency review and decision register](multi-airport-support/planning-readiness.md)
is the current assessment. Discovery rounds below preserve the discussion at
that time; superseded statements about an unchosen schema/rule engine or pending
handler selection are historical, not current decisions.

## Purpose

Capture the operational requirements and decisions needed to plan support for
multiple airports across FlightStrips, including the backend, web application,
and EuroScope plugin. This document is the working record for the discussion and
will provide the basis for ADRs and one or more implementation epics once the
requirements are sufficiently understood.

Implementation planning has not started. Questions and candidate approaches in
this document are not agreed requirements or architecture decisions.

The [architecture proposal](multi-airport-support/architecture-proposal.md)
develops these requirements into a proposed session/membership model,
configuration/workflow boundary, service isolation, protocol migration, and
acceptance scenarios. It remains a draft for review.

A self-contained [HTML reading edition with diagrams](multi-airport-support/architecture-proposal.html)
is generated from the Markdown proposal.

## Requirements stated so far

- EKCH is the currently supported airport.
- Support must expand to airports with different operational flows, ownership
  requirements, and allowed or prohibited actions.
- ENGM is a concrete example of an airport with substantially different needs.
- EKBI and EKYT must also be supported and have their own requirements.
- One controller may need to interact with multiple airports and may be the
  master for multiple airports.
- The requirements discussion must be recorded in the repository before an
  implementation plan is developed.

### Current agreed constraints

This summary reflects the latest answers; the discovery rounds below retain the
context of how they were established.

| Concern | Agreed requirement |
| --- | --- |
| Initial delivery | EKBI and EKYT with RADIS; use ENGM as a future design input. |
| Airport state | Independent strips and workflow/board state per airport, including flights between supported airports. Flight-plan facts synchronize through EuroScope. |
| RADIS collaboration | One shared airport board, shared placement/order, no strip or sector ownership; anyone working the airport may edit/move strips. |
| EuroScope writes | RADIS bay moves and relevant field edits update EuroScope for now. Use the initiating controller's own plugin; block edits when it is disconnected. |
| Mastership | Independent per airport/session; a plugin may be master for several airports and non-master for others. |
| Coverage policy | Configurable per airport and must consider the controller's tuned radio frequencies. Coverage is needed even where sector ownership does not exist. |
| Coverage changes | Remove an airport association only when the controller no longer covers required responsibilities there. APP taking over may remove EKDK; TWR alone leaves EKDK covering approach. Eligible EKDK instances automatically return when APP leaves. |
| Relinquishment timing | Leave when the replacement connects to the backend, or after a bounded wait if it does not. Nearly immediate relinquishment is acceptable initially; respect the known intentional plugin startup delay. |
| Offline behavior | Reuse the application's current controller-offline/read-only behavior, retaining the board and blocking edits. |
| Frontend navigation | Airport switcher and separate tabs. Show an initial airport/session choice only when more than one currently eligible session exists; enter directly when exactly one exists. Preserve existing behavior when none exists. |
| Configuration | One combined repository file per airport; restarts are acceptable. Select a registered coded handler with concrete typed settings; RADIS code can be shared without a universal rule language. |
| Services | Existing services should be reusable at any airport through airport-specific configuration/data. EKBI/EKYT initially use CDM, stand assignment, and text-pilot support; PDC and AMAN remain disabled there without a committed activation date. Preserve existing EKCH services. |
| Shared VATSIM sources | Fetching, parsing, and caching of the network-data and transceiver feeds stay shared at application scope, rather than being replicated per session. Sessions/services derive scoped projections from the same read-only source. |
| Shared PDC | One application service/client, deployment logon, and polling coordinator; distinct airport mailboxes, per-airport availability/validation, and session-scoped clearances. |
| Strip lifecycle and bays | Automatic appearance, removal, bays, transitions, and their effects are configurable per airport, using EKCH as an existing behavior reference and ENGM as a broader workflow example. |
| Frontend extensibility | Airport-specific views may require frontend code. New airports are expected to involve code changes for the foreseeable future; a code-free airport onboarding system is not a requirement. |

Exact coverage lists, master priorities, the final EKBI/EKYT service subset,
RADIS inclusion/removal values, and bay-to-EuroScope mappings are deliberately
deferred. They are inputs needed before the affected airport is released, not
prerequisites for discussing the architecture or a reason to invent defaults.

See [configuration design requirements](multi-airport-support/configuration-design-requirements.md)
for the typed configuration and coded-handler/core boundaries. The [concrete schema contract](multi-airport-support/schema-contract.md)
now defines the proposed format and links a validated, lossless worked EKCH
configuration example. Backend loader/adapter integration remains future work.

### Clarifications from discovery round 1

- The first delivery targets EKBI and EKYT with RADIS. The precise RADIS feature
  set and acceptance scenarios remain to be defined; ENGM is a future design
  input rather than the first delivery target.
- An EKDK controller working traffic into both EKYT and EKCH is a representative
  multi-airport scenario. Exact coverage and interaction rules remain open.
- Views are airport-specific. Multiple airports can share views: EKBI and EKYT
  are expected to share their layout with few differences. Their backend and
  frontend behavior will differ substantially from EKCH.
- Prefer configurable backend behavior to airport-specific code changes where
  possible. The boundary between supported configuration and new code remains
  open. Shared views do not establish that all operational rules are identical.
- Master means the EuroScope synchronization master. One plugin must be able to
  be master for one airport and non-master for another, with mastership changing
  independently for each airport. One plugin may be master for multiple airports.
- Ownership refers to strip ownership and sector ownership. Either concept may
  be absent for an airport; the required combinations and consequences remain
  open. Administrative or organizational ownership was not the intended meaning.
- The supplied `ENGM EFS.md` is the reference for ENGM differences. See
  [ENGM reference notes](multi-airport-support/engm-reference-notes.md) for the
  extracted behavioral examples, source status, and unresolved points.

### Clarifications from discovery round 2

- RADIS is intended to be a simple interface that mimics paper strips, with few
  operational rules. EKCH's ownership and automation must not be assumed to
  apply to it.
- At this point the user's recollection was that RADIS has no ownership.
  Round 3 subsequently confirmed the absence of ownership and the shared board.
- The RADIS reference is the selected Figma design, rather than a written manual.
  The inspected section is `FlightStrips for RADIS/EKBI` (`2676:1558`) in
  `Project FS / Flightstrps by VATSCA`. See
  [RADIS reference notes](multi-airport-support/radis-reference-notes.md).
- Airport participation is still undecided. A candidate is configuration listing
  the airports an individual EuroScope instance is allowed to connect for.
- Configured eligibility alone is insufficient: if EKDK serves EKBI and
  `EKBI_APP` comes online, EKDK no longer needs the EKBI association and should
  not remain its master. The exact coverage rules and readiness condition for
  that transition remain open.
- Configurable workflow states/bays, permitted actions, ownership rules,
  validations, and automatic transitions are within the intended configuration
  scope. A shared RADIS definition with airport-specific values and exceptions
  is an accepted direction to investigate; no schema or engine is chosen yet.

### Clarifications from discovery round 3

- RADIS has one shared board per airport, with no strip or sector ownership.
  Anyone working that airport may edit and move strips. Strip placement and
  ordering are shared rather than arranged independently by each controller.
- Moving between bays must update EuroScope. Editing fields such as assigned
  squawk and altitude must also update EuroScope for the initial delivery. The
  user explicitly allows this coupling to change later; exact bay-to-EuroScope
  state mappings are still to be specified.
- Relevant flights should appear automatically. Inclusion criteria, initial bay,
  removal rules, and the additional roles of NEW/FIND remain open.
- In the EKDK-to-EKBI_APP coverage example, EKDK should leave the airport
  association when the replacement connects to the backend, or after a bounded
  wait if it does not connect. It must not remain indefinitely merely because
  the replacement does not run FlightStrips.
- For the initial delivery, leaving nearly immediately on coverage change is
  acceptable. No exact timeout has been selected. The plugin intentionally
  delays connecting, so absence during that startup interval must not be
  mistaken for an unexpected failure. Whether to wait for a complete initial
  synchronization, rather than backend connection alone, is not established.
- EKBI_TWR alone does not remove EKDK's EKBI association: EKDK still provides
  approach service. Participation must reflect responsibilities still covered,
  not just the presence of any local controller.
- Behavior when APP leaves again and the interface during a gap were subsequently
  resolved in round 4. Per-airport master priority while several covering
  controllers remain connected is still unresolved.

### Clarifications from discovery round 4

- Flights between supported airports have independent strips per airport,
  including independent bay placement, order, and workflow state. Flight-plan
  facts such as squawk and cleared altitude continue to follow EuroScope.
- All controller-initiated EuroScope writes use that controller's own plugin,
  even when another client is the airport master. Block edits when the
  controller's plugin is disconnected; do not fall back to the master.
- Eligible EKDK instances automatically reconnect for an airport when the local
  APP controller leaves and coverage returns to EKDK.
- Keep the board read-only during disconnection using the existing
  controller-offline behavior. Reuse that mechanism rather than introducing a
  new waiting/offline interaction.
- Use an airport switcher and separate tabs for working multiple airports.
  The initial screen should ask which airport/session to load only if the
  connected controller can participate in multiple sessions. Do not insert a
  chooser into the single-session path.
- Keep configuration in the repository. Restarting for changes is acceptable.
- New airports are expected to require custom frontend code for quite a while.
  Backend configurability is not a requirement to make every new airport work
  without any application code changes.

### Clarifications from discovery round 5

- The user does not currently have exact operational values for the round 5
  questions. Defer those values rather than repeating the same questions.
- Coverage is configurable per airport and must account for tuned radio
  frequencies. The specific matching/fallback rules are not supplied yet.
- Master selection is configurable per airport. Exact position priorities are
  not supplied yet.
- In principle all existing services should support multiple airports through
  airport-specific data/configuration, without airport-specific copies of the
  service. CDM needs its own airport parameters; AMAN needs airport-specific
  arrival/terminal configuration.
- EKBI/EKYT will most likely initially use CDM, stand assignment, and text-pilot
  workflows. This is a provisional release scope, not a permanent limitation on
  RADIS or the other services.
- Bays and their behavior are configurable. Use ENGM's different workflows to
  inform what the format can represent; do not select a format based solely on
  RADIS's three visible areas.
- Automatic appearance and removal are configurable per airport, as with EKCH.
  No EKBI/EKYT thresholds or initial/terminal state mappings were supplied.

### Architecture review: shared VATSIM data

- The user explicitly requires VATSIM feed acquisition/cache data to stay shared,
  rather than being replicated into a separate feed/cache for each session.
- Current application assembly already shares a VATSIM source among consumers
  and a transceiver cache among radio lookups. Preserve that ownership boundary.
- Shared source data is distinct from airport-specific strip records, service
  projections, board state, and operational results, which remain isolated.
- The proposal now explicitly includes shared acquisition, consistent snapshots,
  aggregate source demand, environment isolation, and acceptance scenarios.

### Architecture review: concrete configuration schema

- The user requires a defined schema and a worked EKCH migration, rather than
  placeholder configuration sections.
- The proposal now includes a closed JSON Schema 2020-12 contract, airport and
  workflow examples, and executable structural/reference/roundtrip checks.
- All 20 current EKCH YAML settings are represented; selected code constants
  and the current master-priority policy are explicit in the example.
- This validates configuration representability, not runtime behavior. Current
  EKCH code-owned transitions remain behind a proposed compatibility boundary.
- Unknown EKBI/EKYT values remain open and application behavior is unchanged.

## Current implementation: initial findings

These findings describe the inspected code, not constraints on the new design.
The inventory is preliminary rather than an exhaustive dependency audit.

| Area | Observed behavior | Evidence |
| --- | --- | --- |
| Session data | Each session has one airport and a name. Queries distinguish sessions by airport and name. | [Session model](../../backend/internal/models/session.go), [session queries](../../backend/queries/sessions.sql), [architecture](../../backend/Architecture.md) |
| Controller and connections | A controller record belongs to one session. Each inspected EuroScope and frontend client has one session and airport context. | [Controller model](../../backend/internal/models/controller.go), [EuroScope client](../../backend/internal/euroscope/client.go), [frontend client](../../backend/internal/frontend/client.go) |
| Airport configuration | Airport configuration is loaded into package-level state for positions, routes, sectors, layouts, runways, validation, coordinates, and CDM defaults. | [Configuration loader](../../backend/internal/config/config.go), [EKCH configuration](../../backend/config/ekch.yaml) |
| Bay lifecycle | Bay/ground-state and position transitions include hardcoded EKCH coordinates/elevation and state-preservation rules. Moving this behavior into airport-aware configuration is broader than loading more YAML files. | [Bay lifecycle helpers](../../backend/internal/shared/bays.go) |
| Radio coverage | Existing coverage calculation combines primary position/frequency with frequencies returned by transceiver providers. This provides behavior to reuse, but currently reads controllers from a single session. | [Controller coverage collection](../../backend/internal/server/sectors.go), [coverage resolution](../../backend/internal/config/sectors.go) |
| Service configuration | CDM and AMAN have existing configuration models; AMAN terminal configuration is already versioned and airport-specific. Stand-assignment loading still contains an explicit EKCH directory and global readiness/registry state. | [CDM configuration](../../backend/internal/cdm/config.go), [AMAN terminal configuration](../../backend/internal/aman/terminal/config.go), [stand-assignment configuration](../../backend/internal/config/stand_assignment.go) |
| Master selection | Masters are tracked per session. The current priority policy names EKDK and EKCH positions. | [EuroScope hub](../../backend/internal/euroscope/hub.go) |
| Plugin airport resolution | Callsign mapping and a geographic fallback resolve a single airport; the default callsign map is EKCH. | [Airport resolution](../../euroscope-plugin/src/plugin/plugin/AirportResolution.cpp), [plugin configuration](../../euroscope-plugin/src/plugin/configuration/AppConfig.cpp) |
| Plugin connection delay | The current plugin has 5-second fast-connect and 30-second online-transition delays. These are existing client behavior, not the selected airport relinquishment timeout. | [WebSocket service](../../euroscope-plugin/src/plugin/websocket/WebSocketService.cpp), [delay constants](../../euroscope-plugin/src/plugin/websocket/WebSocketService.h) |
| Interface | The application router maps layouts to EKCH screen components. | [Application router](../../frontend/src/routes/AppRouter.tsx) |
| Read-only/offline state | Frontend events and the store already carry read-only state, and the hub handles controller disconnect and reassociation. The user requires reuse of this behavior. | [Frontend hub](../../backend/internal/frontend/hub.go), [event definitions](../../backend/pkg/events/frontend/events.go), [frontend store](../../frontend/src/store/store.ts) |
| Strip ownership | The documented model distinguishes owner, next controllers, previous controllers, and pending handoffs. | [Ownership guide](../../docs/src/content/docs/concepts/ownership.mdx) |

## Terms to clarify

- **Airport:** the operational aerodrome; whether it is also the unit of isolation
  remains undecided.
- **Session:** currently an airport and environment/name combination; the future
  relationship between sessions and multi-airport work remains undecided.
- **Master:** the selected EuroScope client supplying most strip and controller
  events, currently scoped to a session. The requested master has this same
  synchronization meaning and must support independent roles across airports.
- **Coverage:** which airport responsibilities a controller performs, including
  combined positions and top-down service. This must be distinguished from data
  synchronization responsibilities.
- **Ownership:** strip responsibility and sector ownership, each potentially
  absent depending on the airport. Airport administration and configuration
  maintenance are separate concerns.
- **View:** an airport-specific presentation that may be shared by several
  airports. View sharing does not imply shared operational state or mastership.
- **RADIS:** the paper-strip-like system/workflow requested for EKBI and EKYT,
  represented by the selected Figma design. Its operational semantics remain
  partially unspecified.
- **Eligible airport:** an airport a EuroScope instance may serve, potentially
  defined by instance configuration; this is a candidate concept.
- **Current airport participation:** whether an eligible instance currently needs
  to serve that airport given staffing and coverage. Its relationship to viewing,
  editing, subscriptions, and master candidacy remains to be defined.

## Discovery round 1: operating model

1. What is the intended order of introducing ENGM, EKBI, and EKYT, and what must
   the first release enable?
2. What is a concrete example of one controller working multiple airports:
   callsign/position, airports, responsibilities, and whether they must see and
   act on those airports simultaneously?
3. Does master mean the EuroScope synchronization master? Can one plugin be
   master for one airport and non-master for another, and can mastership change
   independently for each airport?
4. What are two or three concrete ENGM workflow/action differences from EKCH,
   including who may act and what should happen instead?
5. Which kinds of ownership differ between airports: strip responsibility,
   sector/position coverage, airport/configuration administration, or something
   else?

Answers: recorded in the clarifications above. The example position and scope of
mastership are understood; detailed coverage, workflows, and release acceptance
criteria require further discovery.

## Discovery round 2: RADIS and controller coverage

1. Is there a RADIS manual, design, or existing implementation to use as the
   reference? If not, describe a normal departure and arrival, including the
   lists/bays used, controller actions, and automatic transitions.
2. For EKBI and EKYT separately, should strip ownership and sector ownership
   exist? If either is absent, what determines who can change a strip?
3. How should a plugin/controller become associated with airports: automatically
   from configured callsigns, positions or frequencies; explicit selection; or a
   combination? Distinguish viewing, acting, and eligibility to become master.
4. When an EKDK controller covers EKYT and a local EKYT controller connects,
   which responsibilities move to the local controller, and what should happen
   to existing strip ownership? Does responsibility transfer automatically or
   require acceptance?
5. What must be configurable in the backend: positions and routing only, or also
   workflow states/bays, permitted actions, ownership rules, validations, and
   automatic transitions? Is a shared RADIS definition with airport-specific
   values and exceptions an acceptable direction to investigate?

Answers: recorded in the round 2 clarifications above, with the provisional
ownership answer resolved by round 3. Configuration scope is confirmed and
participation/coverage policy is partially defined. The Figma inspection
establishes visible controls but not a complete interaction model.

## Discovery round 3: board semantics and participation changes

1. For RADIS, do all controllers working an airport share the same board,
   including strip placement and order, with any participating non-observer
   able to edit it? Or is some placement local to a controller or position?
2. Does moving a strip among ACTIVE, PASSIVE, and RUNWAY only change the board,
   or should some moves also update EuroScope clearance or ground state? Which
   field edits must write back to EuroScope?
3. How do strips enter and leave RADIS: automatic inclusion of relevant flights,
   manual NEW/FIND, or both? Which movements/removals are automatic, if any?
4. In the EKDK-to-EKBI_APP example, should EKDK stop serving EKBI when APP is
   merely online on VATSIM, or only once its FlightStrips plugin is connected
   and synchronized? What should happen when that replacement disconnects?
5. Does a local position remove EKDK's whole airport association or only the
   responsibilities it actually covers? For example, if only EKBI_TWR connects,
   does EKDK retain approach responsibilities and its EKBI association?

Answers: recorded in the round 3 clarifications above. Shared board, no ownership,
EuroScope writes, automatic inclusion, and partial coverage behavior are
confirmed. Detailed mappings, lifecycle rules, and restoration of coverage remain
open.

## Discovery round 4: state, command routing, and configuration

1. For a flight between two supported airports, should each airport have its own
   strip placement/order and workflow state while flight-plan facts such as
   squawk and cleared altitude follow EuroScope? For example, departure-board
   movements at EKBI must not also move the arrival strip at EKYT unless that
   coupling is explicitly desired.
2. Should EuroScope writes continue through the initiating controller's plugin
   even if a different plugin is the airport master? If the initiating plugin
   is unavailable, should the action be rejected or routed through the master?
3. When APP leaves, should eligible EKDK instances automatically resume serving
   the airport? During a gap without an active plugin/master, should the frontend
   retain the last board as read-only or show a waiting screen?
4. How should a controller use different airport views: an airport switcher with
   support for separate windows/tabs, a combined view, or another arrangement?
5. Should airport workflows, coverage rules, and master priorities be centrally
   maintained in repository configuration, with only the list of eligible
   airports configured per EuroScope instance? Or should another mechanism own
   these settings, and do changes need to take effect without a deployment?

Answers: recorded in the round 4 clarifications and current agreed constraints.
Command routing, automatic return, offline behavior, navigation, and repository
configuration are confirmed. No replacement offline workflow or code-free
frontend configuration system is required.

## Discovery round 5: coverage policy and initial RADIS behavior

1. What positions cover EKBI/EKYT and in what fallback order, including relevant
   EKDK sectors and cross-coupled positions? Should these use the current
   primary-frequency/cross-coupling matching rules, with airport-specific data?
2. What should the master priority be at EKBI and EKYT when several eligible
   controllers remain? Keep the existing preference/preemption behavior but
   provide an ordered position list per airport, or use a different policy?
3. Which services are needed in the first RADIS release: PDC, CDM/timing, AMAN,
   stand assignment, deicing, text-pilot workflows, or others? Which should be
   disabled for these airports?
4. What does moving a departure or arrival into PASSIVE, ACTIVE, or RUNWAY mean
   in EuroScope? Include whether moving backwards should undo a state and
   whether landing/takeoff clearance needs a separate action.
5. Which flights should appear automatically, in which initial bay, and when
   should they disappear? Specify departure inclusion, arrival time/distance
   thresholds, and manual removal/restoration where needed.

Answers: configurability and tuned-radio inputs are confirmed in the round 5
clarifications. EKBI/EKYT most likely initially use CDM, stand assignment, and
text-pilot workflows. Exact priorities, mappings, thresholds, and the final
service subset are deferred until operational input is available.

## Working model to evaluate

The requirements suggest separating airport behavior, airport presentation,
controller participation, and per-airport synchronization mastership. This is an
inference to evaluate, not an accepted architecture decision.

In particular, a configured list of eligible airports and the currently required
set of airports may be different. A controller leaving one airport must still
be able to serve another. Paper-like strip placement also needs to be
distinguished from aircraft/clearance state in EuroScope. RADIS currently requires
bay movements and relevant field edits to write to EuroScope. Each airport's
strip placement/workflow is independent, while shared flight-plan changes follow
EuroScope. The exact bay mappings and treatment of incoming EuroScope ground-state
changes still need definition.

## ENGM as a future design input

The supplied draft describes behavior that cannot be represented solely by
changing screen layouts: manual activation through pending/active lists,
runway-dependent handoff routes, explicit deicing/sequencer transfers,
EuroScope handoff acceptance, and different clearance effects for different
ways of entering a runway line. These are candidate cases for evaluating the
configuration boundary, not accepted implementation requirements yet.

The source identifies itself as in progress with review not started and includes
older screenshots and open questions. Its SUP position is an operational role;
equating that role with the synchronization master has not been agreed.

## Subsequent discovery areas

Follow-up questions will be based on the answers and further inspection of the
relevant implementation. These topics are unresolved, not assumed scope.

- **Coverage and identity:** callsign/frequency matching, cross-coupled positions,
  top-down rules, local positions taking over, observers, instructors, and
  multiple connections or browser windows for the same controller.
- **Sessions and isolation:** live, training, and playback environments; joining
  and leaving airports; lifecycle and cleanup; whether independent airport
  sessions are sufficient or an operational grouping is required.
- **Master lifecycle:** eligibility, priorities, per-airport data visibility,
  manual override, failure, reconnect, conflicting updates, and command routing
  through the appropriate controller's plugin.
- **Workflows and permissions:** departure/arrival states and bays; normal,
  prohibited, exceptional, and automatic actions; validation/override behavior;
  handoffs; combined positions; VFR, circuits, missed approaches, and local
  flights where relevant.
- **Flights involving multiple supported airports:** shared flight-plan data,
  airport-specific strip state and ownership, local flights, diversions,
  destination changes, and consistency between airport views.
- **Presentation:** separate or combined airport views, layouts and strip
  designs, airport context for commands, alerts and coordination, and which
  differences require distinct screens rather than configuration.
- **Airport capabilities and integrations:** CDM, AMAN, PDC/Hoppies, ECFMP,
  stands, taxiways, deicing, text-pilot workflows, EFB and external feeds;
  required behavior when a capability is absent.
- **Configuration and governance:** who supplies and approves airport rules,
  reuse between airports, validation, versioning, deployment versus runtime
  changes, and behavior when configuration is missing or incompatible.
- **Delivery constraints:** deployment boundaries, supported client versions,
  migration and EKCH compatibility, rollout, operational scale, and named
  airport representatives who can validate behavior.

## Decisions and readiness for planning

During D1 review the user confirmed NEW creates an airport-local shared strip
only for now. Actual ES flight-plan creation is deferred. FIND is restricted to
connected pilots whose flight origin or destination matches the selected airport;
prefiles are excluded. On 2026-11-01 the user confirmed all current prototype
features, annotation controls, and restoration of manually deleted strips.

The current [architecture proposal](multi-airport-support/architecture-proposal.md)
and [planning review](multi-airport-support/planning-readiness.md) supersede the
intermediate engineering options above. The user selected coded workflow
handlers, one combined airport file, concrete EKCH and fictional examples,
shared VATSIM acquisition, and shared PDC service/client/logon. These directions
are recorded without treating the entire proposal as an accepted ADR.

The [executable proof](multi-airport-support/handler-design.md) supplies two
registered Go handlers, combined configs, scoped in-memory execution/tests, and
benchmarks. It is not wired into application startup and does not establish
complete EKCH runtime parity or RADIS operational validation.

The [RADIS PR review](multi-airport-support/radis-prototype-review.md) records
PR #958 as the likely frontend starting point. Feed-only/no-ES-write behavior is
a demo shortcut, not the integrated target; its extra airport menu entries and
hard-coded limits do not expand delivery scope.

D1–D5 are now resolved: all current prototype features and restoration; PDC/AMAN
disabled at EKBI/EKYT; current EKCH execution/error behavior; automatic effects
through the same session master or assuming controller; coordinated deployment
while nobody is online. Human edits retain their own plugin. No concurrent
legacy/new client support or additional pending-move framework is required.
The proposal is ready to turn into epics without further product questions in
this register. The [implementation epic plan](multi-airport-support/epic-plan.md)
is now published as eight epics and 41 child issues, with native hierarchy and
prerequisites. See the [issue index](multi-airport-support/issue-index.md) and
[spec PR #972](https://github.com/flightstrips/FlightStrips/pull/972). Implementation
remains open; merging the spec does not close those issues.

Engineering E1–E7 items belong in the relevant epics with named design tasks:
production registry validation/activation, typed action/effect contracts,
protocol/identity/concurrency, evidence freshness, PDC reply correlation,
required offline state/config migrations, and representative performance
acceptance. New-airport PDC correlation and AMAN activation remain deferred,
not release gates for RADIS.

Coverage/master lists, bay/ES mappings, lifecycle/annotation values, service data,
and operational approval remain airport rollout inputs. They need not be guessed
to plan the foundations. Reuse existing offline/reconnection behavior; no new
unanswered question overrides the already agreed takeover timing or own-plugin
policy for human edits. Use the linked epic plan for scope, dependencies, child
issue candidates, acceptance criteria, and activation gates.
