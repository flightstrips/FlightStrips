---
title: AMAN CPH feature specification
status: living-specification
audience: maintainers-and-coding-agents
source_revision: operator-frontend-review-2026-09-09
airport: EKCH
---

# AMAN CPH feature specification

## Purpose and maintenance

This is the internal, repository-owned description of AMAN CPH. It turns the original `AMAN-CPH V0.1` concept document, the `AMAN-CPH-FRONTEND` design, linked Figma frames, and later operator corrections into a single implementation reference.

Maintainers and coding agents should update this file when an operational decision changes. GitHub issues describe individual pieces of work; this file describes how the complete feature is expected to behave. Where this specification conflicts with an old planning issue or the original draft, this specification is the intended product behavior unless a newer recorded decision says otherwise.

## Product objective

AMAN CPH predicts when inbound aircraft can reach Copenhagen, assigns landing-runway slots, presents a stable arrival sequence, and tells controllers how much time each aircraft should gain or lose. The backend owns the prediction, lifecycle, sequence, persistence, and command validation. FlightStrips clients supply operational observations and present the resulting state.

The system must remain deterministic across restarts and replay. Navigation geometry, configuration, prediction inputs, manual actions, and degraded states must retain enough provenance to explain a result.

## Terminology

These concepts are deliberately distinct:

- **STAR entry family**: the outer arrival-route family, currently TESPI, TUDLO, MONAK, TIDVU, or ERNOV. It is used to select terminal geometry and may be used by STAR-family sequencing policy.
- **Feeder fix**: the downstream operational fix from which Stable and Superstable timing is measured. The configured EKCH feeder fixes are NEKSO, KOR, TNO, ERNOV, ESJAH, KUBIS, and WUPJA according to STAR family and runway direction. A feeder fix is not automatically the STAR entry family or holding fix.
- **Holding fix**: the fix of a published holding used to detect holding occupancy and calculate an expected release.
- **Merge fix**: the point at which configured terminal paths converge toward final approach.
- **RETA**: raw ETA supplied by or derived from an external source before the AMAN performance correction.
- **Raw TETA**: the latest physical trajectory prediction calculated by AMAN.
- **Operational TETA**: the accepted/smoothed/frozen TETA used for sequencing and controller presentation.
- **Landing slot**: a landing-runway time opportunity produced from arrival rate and separation policy.
- **GAP**: an intentionally unavailable landing-runway slot or interval inserted by an authorized FMP. It is not an aircraft.

## Inputs and ownership

### Backend-owned state

The backend owns:

- raw and operational TETA;
- lifecycle state and transition reasons;
- Stable protection and Superstable freeze;
- runway-group selection, slot assignment, queue opportunities, and sequence revision;
- manual ordering, manual ETA decisions, go-around effects, and runway GAPs;
- navigation/configuration versions and audit provenance.

### VATSIM inputs

VATSIM supplies planning and surveillance information where available, including flight identity, callsign, origin/destination, aircraft type and wake category, filed route, requested level, EOBT/EET, position, altitude, groundspeed, and track.

Planned and preliminary airborne predictions may use filed timing. Precise AMAN sequencing must use the route-aware trajectory prediction once sufficient operational data is available.

### FlightStrips EuroScope inputs

Controllers operating in the Danish FIR are expected to run the FlightStrips plugin. AMAN must therefore consume relevant controller-issued direct-to clearances from the plugin, not wait to infer them from aircraft track or assume that the filed route was rewritten.

An accepted direct-to fact must identify the aircraft and target fix, carry observation/source provenance, resolve through the active cached navigation dataset, and trigger a new trajectory, DTG, feeder-fix ETA, and raw TETA. Reports from multiple clients must be idempotent and ordered deterministically. Invalid or unresolved directs must degrade visibly without corrupting the last valid route.

Direct observations continue to update physical/raw prediction after a freeze but must not silently bypass Stable protection or Superstable/manual freeze policy.

### Frontend inputs

Authorized FMP controls include:

- selecting the landing runway group;
- setting and scheduling the arrival rate;
- reviewing exceptional TETA inputs;
- manually ordering or freezing a flight where permitted;
- reporting/confirming a go-around where required;
- inserting and removing a landing-runway GAP.

Commands must use the existing authenticated, idempotent, revision-checked command path and produce visible rejections on conflict.

## Frontend product design

This section describes the intended controller workspace rather than the smaller AMAN board that exists in the repository today. The linked Figma file is the visual reference. Domain behavior, authorization, and protection rules in this specification take precedence when an old mock-up label implies a conflicting implementation.

### Workspace composition

The reference desktop workspace is designed around a 3:2 viewport and contains two top-level regions:

- **MAESTRO**, pinned to the left, is the operational sequence display with a 2:3 portrait reference proportion. It contains the settings bar and the FMP work area. On a wider viewport it may consume additional space without stretching the timeline bars or aircraft targets out of proportion.
- **TMT (Traffic Management Tool)**, pinned to the right, retains a 3:4 reference aspect ratio and contains the traffic-prediction, holding-information, and warning tools. Additional wide-screen space primarily increases the separation between TMT and MAESTRO.

At the supported minimum desktop size, both regions must remain usable without overlap. Responsive implementation may use modern layout constraints instead of reproducing percentages literally, but must preserve the visual hierarchy and relative placement. Compact/mobile behavior is not defined by the source design.

The MAESTRO work area uses background `#555355`. A `#9C0000` line and a darker `#3F3F3F` region mark the final ten minutes of the visible timeline. This is a visual boundary; lifecycle state remains determined by authoritative backend state and configured feeder-fix timing, not by a frontend clock calculation.

### MAESTRO views

All MAESTRO views use the same time-axis and target components but select different lanes and emphasis:

- **FMP/ALL** is the overview of all five STAR entry families and uses three timelines. Each timeline identifies its configured feeder fix and runway in use. The mapping of families to timeline sides must come from versioned configuration rather than UI constants.
- **RWY** is runway-centric. The active runway with the most scheduled arrivals appears on the left side of the middle timeline. Additional runways use, in order, the right side of the middle timeline and then the left and right sides of the right timeline, stopping at four displayed runways.
- **ACC** retains the overall sequence but emphasizes the STAR family relevant to the controller. Aircraft on other families remain visible in `#686868` so the selected family can be understood in context.

The initial ACC position mapping is:

| Controller position | Emphasized view/family |
| --- | --- |
| `_FMP` | ALL |
| `_B_CTR` | MONAK |
| `_D_CTR` | TUDLO |
| `_E_CTR` | TESPI |
| `_K_CTR` | ERNOV |
| `ESMS_APP` | TIDVU |

Position-derived defaults are conveniences, not authorization. A locally selected view may override the default without changing shared AMAN state.

Until an operator-specific layout supersedes it, the initial versioned FMP/ALL lane mapping is:

| Timeline | Left side | Right side |
| --- | --- | --- |
| 1 | TESPI | TUDLO |
| 2 | MONAK | TIDVU |
| 3 | ERNOV | unused |

If one controller identity matches multiple ACC-family mappings, or no standard mapping matches, the initial view is ALL. The user may then select a local view without changing shared state. These defaults are deliberately configuration-owned so they can be adjusted without redesigning the timeline.

### Timeline behavior

Each vertical timeline:

- places current UTC time at the bottom and future time above it;
- labels every five minutes and marks every minute;
- advances in six-second increments, ten visual movements per minute;
- defaults to a 30-minute horizon and supports a continuous local zoom/scroll control up to 90 minutes;
- shows a rounded current-time box below the axis together with the applicable feeder fix and runway in use;
- keeps target placement as close as possible to the authoritative time while resolving collisions legibly;
- keeps the final-ten-minute boundary visible and synchronized with the chosen scale.

The left-side vertical control changes only the local time horizon. It does not change AMAN prediction or sequence state. The inspected Figma timeline frame is `412.53 × 1772`; its document annotations use approximately 2.7% screen width, a 7.5% top origin, and a 2.3% bottom reserve. These are reference proportions, not hard-coded browser pixels.

### Aircraft targets and local information selection

The inspected Figma aircraft target is `358 × 84` reference units. A target may contain these fields, ordered inward toward the timeline so hidden fields do not leave blank gaps:

1. feeder-fix ETA/STA;
2. callsign;
3. current delay still to be absorbed;
4. total assigned delay;
5. landing runway;
6. wake category;
7. aircraft type.

The default compact target shows callsign and current delay. The original reference widths are 1%, 4%, 1%, 1%, 1.5%, 1.5%, and 1.5% of the reference screen respectively, with about 7% usable on each side of a timeline. Implementations should preserve the priority and compactness rather than couple text size to viewport width.

Clicking the current-time box opens the Target Information dialog. It contains separate feeder-fix-side and runway-side toggles for STA FF, runway, aircraft type, WTC, feeder fix, total delay, and current delay. These display preferences are local to the signed-in user and must not be published as shared AMAN state. The inspected reference dialog is `493 × 497.08`.

Lifecycle colors are:

- Unstable: `#6E996E`;
- Stable: `#96D796`;
- Superstable: `#DCDCDC`.

Gain/delay emphasis uses:

- any gain or `=00`: `#96D796`;
- one through three displayed minutes to lose: `#F0E129`;
- four or more displayed minutes to lose: `#9C0000`.

Hovering an aircraft or menu button adds a rounded `#A3D5E8` fill with a one-pixel `#FFFFFF` edge and changes its text to `#FFFFFF`. Hover styling must not be the only indication of focus; keyboard focus requires equivalent visibility.

### Aircraft dialog and commands

Clicking a target opens the aircraft dialog. The Figma menu contains the following actions; every mutating action must use backend authorization, idempotency, revision checks, audit data, and the protection rules elsewhere in this specification:

- **Information** opens the read-only flight-information view.
- **Recompute** requests a new physical prediction without silently releasing Stable, Superstable, manual, or validated TMA protection.
- **Refresh Delay** re-renders/re-requests authoritative gain/lose information. It is a recovery action and must not create an independent frontend calculation.
- **Alternate Runway** selects the configured paired runway: 22L ↔ 22R and 04L ↔ 04R for the initial EKCH configuration.
- **Change Runway** assigns a chosen runway. For Stable/Superstable aircraft, the established sequence position remains protected while a valid target-runway slot is resolved; conflicts are rejected visibly.
- **Change ETA-FF** applies an explicit, audited manual feeder-fix ETA override and displays its provenance.
- **Maximum Delay** defines an operational upper bound for that aircraft. It may move the aircraft to the earliest legal position but never bypass wake separation or silently displace protected traffic.
- **Coordination** opens the tactical-request dialog for routing/direct or speed requests. A request is distinct from an accepted controller clearance and does not become a route fact until the authoritative workflow accepts it. The inspected dialog is `370 × 471`.
- **Missed Approach** opens a confirmation action and then uses the configured ten-minute go-around model.
- **De-sequence** moves the aircraft into DSEQ without deleting it. DSEQ shows a count and allows an authorized controller to resume or remove an entry.
- **Insert Closure** begins a runway-capacity closure either after a selected aircraft or at an explicit absolute UTC time and renders a red overlay across every visible lane for the affected runway.
- **Insert Gap** creates the first-class runway GAP defined elsewhere in this specification, using a requested duration after the selected aircraft.
- **Extra Flight** reserves one normal flight opportunity. In the domain this is a named capacity reservation/GAP with an optional display label (default `FLIGHT`), never a fabricated aircraft or callsign.
- **Remove** is restricted to cases such as diversion, requires explicit confirmation, removes the aircraft from the active sequence, and remains auditable even though the UI does not offer undo.

### MAESTRO settings bar

The settings bar spans the MAESTRO width and contains two rows. At the reference composition its height follows the inspected Figma annotation of 13⅓% of MAESTRO height. Figma is the visual source of truth where that proportion fits coherently with the rest of the responsive design; implementations may use equivalent grid/flex constraints rather than a hard-coded height.

The primary row contains:

| Control | Display | Action |
| --- | --- | --- |
| RIU | Selected runway(s) in use | Opens runway-selection dialog |
| Runway rate | Assigned arrival rate per runway | Opens arrival-rate dialog |
| Wind | Surface wind and 10,000-foot wind | Opens wind-data dialog when defined |
| Traffic load | Aircraft in the TMA above 1,500 feet and aircraft inside the selected MAESTRO horizon | Read-only summary |
| View | Current view | Opens view selection where applicable |
| UTC time | Current UTC time | No action |

The secondary row contains MAESTRO/view selection, ALL, RWY, ACC, and DSEQ. The selected view uses background `#86A4AF` with `#FFFFFF` text; the MAESTRO selector uses `#5174B8` with white text. RIU and runway-rate controls use `#F3D02E` with black text. The source calls these controls B1–B5 despite also referring to B1–B6; implementations use the five defined controls.

The reference horizontal placements are:

| Control | Left | Width | Row height/start |
| --- | ---: | ---: | --- |
| RIU | 3% | 5% | 50%, top 0.5% |
| Runway rate | 8.4% | 42.5% | 50%, top 0.5% |
| Wind | 51.3% | 11.25% | 50%, top 0.5% |
| Traffic load | 62.95% | 8% | 50%, top 0.5% |
| View | 71.35% | 15% | 50%, top 0.5% |
| UTC time | 86.85% | 15% | 50%, top 0.5% |
| MAESTRO selector | 3% | 12% | 23%, top 70% |
| ALL | 16% | 4.5% | 23%, top 70% |
| RWY | 20.9% | 4.5% | 23%, top 70% |
| ACC | 25.8% | 4.5% | 23%, top 70% |
| DSEQ | 31.1% | 7.5% | 23%, top 70% |

These percentages document the inspected reference composition. Responsive code may express them through grid/flex constraints provided visual-regression tests preserve the same grouping and emphasis.

Runway selection and arrival-rate changes remain independent commands as specified above.

### TMT traffic-prediction tool

The traffic-prediction tool shows 15-minute buckets from the nearest preceding quarter-hour through three hours ahead. For example, at 20:44 UTC the range begins at 20:30 and ends at 23:30.

Flights already classified by AMAN as Unstable, Stable, or Superstable use their authoritative AMAN landing time. Other flights use available VATSIM/API planning or airborne timing. A flight must appear exactly once.

Each bucket's displayed load factor is `aircraft count × 4`, expressing that quarter-hour at an equivalent hourly rate. Planned/not-airborne traffic is `#96D796`; airborne traffic is `#DCDCDC`.

Let `bucket_high` mean that the bucket's load factor exceeds the selected arrival rate by more than 10%. Let `window_high` mean that the total aircraft count in the preceding bucket, current bucket, and two following buckets exceeds 110% of the capacity for that one-hour window. A bucket is:

- yellow `#F0E129` when exactly one of `bucket_high` or `window_high` is true;
- red `#9C0000` when both are true;
- unalerted when neither is true.

Boundary handling and missing rate/planning data must be deterministic and visibly degraded rather than guessed.

### TMT holding-information tool

The holding tool shows aircraft that are authoritatively cleared into a holding, their callsign, cleared flight level, holding identifier, and expected approach time (EAT). It combines:

- a fixed one-hour time axis using the MAESTRO minute/five-minute convention without scroll;
- an altitude axis from FL090 through FL300;
- a callsign box centered on the cleared level;
- an EAT box and connector line from the aircraft to its EAT.

The EAT box is green when the EAT is within the next four minutes and yellow otherwise. The connector becomes more horizontal as the aircraft is both lower and closer to EAT, providing a visual comparison of stack position and expected release. The inspected Figma reference is `332.65 × 648.56`.

### TMT warning tool

The initial warning tool presents warnings that AMAN already derives authoritatively; it does not introduce a separate operator-authored warning system. Its initial sources are technical-health blocked/component states and sequence-engine warnings published in the full replacement state.

- Warning identity is deterministic from its source, stable code/component, runway group, flight, and related flight where applicable.
- Duplicate identities collapse to one item. Items sort by severity and then stable identity.
- Blocked, unavailable, and sequence-conflict conditions are errors. Degraded component/input conditions are warnings.
- Warnings are full-replacement current state, not retained history. An item disappears when its authoritative source no longer publishes it.
- The initial version has no acknowledgement workflow or warning persistence independent of its source state.
- All users who can view AMAN can view these warnings; warning visibility is not mutation authority.
- A stale or disconnected client presents its existing connection/data-state warning even when it cannot receive a newer replacement.
- Every warning uses text and a non-color cue in addition to severity color.

Additional warning sources, shared acknowledgement, retention, and role-specific audiences may be added later as explicit versioned product changes.

### Visual design references

The following Figma nodes were inspected on 2026-09-09 and remain the visual source:

- [Timeline](https://www.figma.com/design/csKJuv9WCgO36UjsE1lC2h/Project-FS---Flightstrps-by-VATSCA?node-id=3293-1078&m=dev)
- [Aircraft target](https://www.figma.com/design/csKJuv9WCgO36UjsE1lC2h/Project-FS---Flightstrps-by-VATSCA?node-id=3288-830&m=dev)
- [Target Information dialog](https://www.figma.com/design/csKJuv9WCgO36UjsE1lC2h/Project-FS---Flightstrps-by-VATSCA?node-id=3288-844&m=dev)
- [Aircraft dialog](https://www.figma.com/design/csKJuv9WCgO36UjsE1lC2h/Project-FS---Flightstrps-by-VATSCA?node-id=3288-845&m=dev)
- [Flight Information dialog](https://www.figma.com/design/csKJuv9WCgO36UjsE1lC2h/Project-FS---Flightstrps-by-VATSCA?node-id=3288-846&m=dev)
- [Coordination dialog](https://www.figma.com/design/csKJuv9WCgO36UjsE1lC2h/Project-FS---Flightstrps-by-VATSCA?node-id=3289-893&m=dev)
- [Runway closure](https://www.figma.com/design/csKJuv9WCgO36UjsE1lC2h/Project-FS---Flightstrps-by-VATSCA?node-id=3409-442&m=dev)
- [Runway GAP](https://www.figma.com/design/csKJuv9WCgO36UjsE1lC2h/Project-FS---Flightstrps-by-VATSCA?node-id=3409-443&m=dev)
- [RIU dialog](https://www.figma.com/design/csKJuv9WCgO36UjsE1lC2h/Project-FS---Flightstrps-by-VATSCA?node-id=3286-609&m=dev)
- [Arrival-rate dialog](https://www.figma.com/design/csKJuv9WCgO36UjsE1lC2h/Project-FS---Flightstrps-by-VATSCA?node-id=3286-644&m=dev)
- [MAESTRO dialog](https://www.figma.com/design/csKJuv9WCgO36UjsE1lC2h/Project-FS---Flightstrps-by-VATSCA?node-id=3287-662&m=dev)
- [DSEQ dialog](https://www.figma.com/design/csKJuv9WCgO36UjsE1lC2h/Project-FS---Flightstrps-by-VATSCA?node-id=3287-714&m=dev)
- [Holding information](https://www.figma.com/design/csKJuv9WCgO36UjsE1lC2h/Project-FS---Flightstrps-by-VATSCA?node-id=3291-1077&m=dev)

## Terminal route model

Every configured STAR-family/runway path must identify, separately:

1. STAR entry family;
2. feeder fix;
3. selected holding;
4. merge fix;
5. final approach and runway threshold.

The active configuration already contains the operational feeder fixes inside its paths, but currently names the outer TESPI/TUDLO-style family as the feeder. The model must be corrected without losing the outer family identity. Each configured feeder fix must resolve uniquely and occur on its associated path.

The approved mapping and nominal holding-to-feeder time are:

| STAR entry family | Runway groups | Holding fix | Feeder fix | Nominal time |
| --- | --- | --- | --- | --- |
| TESPI | all | ROSBI | TNO | 3:15 |
| TUDLO | all | LUGAS | KOR | 4:15 |
| MONAK | 04L, 04R, 22L, 22R, 12 | OLPIB | NEKSO | 3:20 |
| MONAK | 30 | OLPIB | KUBIS | unavailable |
| TIDVU | 04L, 04R, 22L, 22R | TIDVU | ESJAH | 2:20 |
| TIDVU | 12, 30 | TIDVU | WUPJA | unavailable |
| ERNOV | all | ERNOV | ERNOV | 0:00 |

`MONAK/30` and `TIDVU/12,30` deliberately keep their correct KUBIS and WUPJA feeder identities while their nominal times remain unavailable pending operator measurement. Configuration represents those durations as absent, never zero. Consumers that require the missing time degrade visibly and must not substitute landing ETA, holding ETA, or another runway's duration. TODO: add the approved OLPIB-KUBIS and TIDVU-WUPJA times when supplied.

## Flight lifecycle

Data freshness is separate from lifecycle state. A stale or disconnected source does not invent a lifecycle transition.

### Planned

A flight that has not become airborne may appear in traffic prediction using a preliminary landing time derived from EOBT, expected taxi-out, and filed or API flight duration. It is not part of the tactical arrival sequence.

### Airborne

After airborne detection, a preliminary arrival prediction may be based on the first airborne observation plus filed/API duration. This baseline is held for comparison until a route-aware prediction is accepted.

### Unstable

An airborne arrival becomes Unstable when it enters the configured AMAN planning horizon and has sufficient operational data. Its trajectory and TETA may change, and the sequence may be recalculated subject to protected aircraft and separation rules.

If the first route-aware TETA differs materially from the preliminary flight-plan ETA, AMAN must expose the existing review workflow rather than silently accepting suspicious route data.

A flight appearing close to EKCH must remain Unstable for at least two minutes before it can become Stable. This gives AMAN time to establish a usable trajectory and prevents a suddenly appearing aircraft from immediately taking a protected position.

### Stable

Stable is based on predicted time remaining to the configured feeder fix, normally about 20 minutes, and requires at least two minutes in Unstable.

On becoming Stable, the flight keeps its relative sequence and current slot protection. It may move into a legal vacancy under the defined queue/resequence rules but must not displace protected traffic merely because its TETA changes.

### Superstable

Superstable is the locked phase of a Stable flight. It is based on predicted time remaining to the same configured feeder fix, normally about 10 minutes. The current implementation represents this as a freeze reason rather than a separate lifecycle enum; that representation may remain as long as external behavior is unambiguous.

At the boundary, AMAN captures the operational TETA and landing slot. Raw prediction continues for drift monitoring. Only explicitly authorized exceptional behavior, such as the defined go-around or manual workflow, may change the protected result.

Stable and Superstable must not use landing TETA or a holding-fix ETA as a silent substitute when feeder-fix ETA is unavailable. Missing feeder-fix prediction is a visible degraded state.

## TETA calculation

Precise TETA is calculated from remaining route geometry and current aircraft state. The model includes:

- remaining distance to landing;
- groundspeed and current altitude;
- descent phase and top-of-descent assumptions;
- aircraft performance by wake/performance class;
- upper winds converted into segment groundspeeds;
- terminal route, valid controller-issued directs, approach geometry, and runway threshold.

The calculation retains leg/segment provenance so a controller or developer can inspect how the result was produced.

All WTC/L aircraft use the accepted RETA as their prediction basis rather than the normal performance/wind descent correction. They remain eligible for automatic sequencing regardless of engine type, subject to normal data-quality and lifecycle rules. The prediction source must be explicit in provenance, and the WTC/L follower separation below still applies.

Operational TETA is derived from raw TETA through the configured acceptance, smoothing, manual-review, and freeze policies. Landing-slot calculations use operational TETA.

## Landing slots and separation

The selected arrival rate defines the base landing grid. For an hourly rate `AR`, the nominal interval is `3600 / AR` seconds.

A flight may use a slot up to 30 seconds earlier than its operational TETA. Otherwise it receives the earliest later slot that satisfies all active constraints.

Minimum separation is the maximum of the base interval and applicable wake policy. The initial AMAN CPH policy is:

- a WTC/H leader requires at least 120 seconds behind it;
- a WTC/J leader requires at least 180 seconds behind it;
- a WTC/L follower requires at least 180 seconds ahead of it.

Unknown wake categories must use an explicit safe fallback and produce degraded-state visibility.

### Same-STAR-family spacing

The initial EKCH policy enables same-STAR-family spacing independently per STAR entry family at 20 arrivals per hour and above with one empty grid opportunity between aircraft from that family. Each STAR family has explicit versioned configuration, initially using the same retained values.

This is sometimes described as requiring an “alternating arrival,” but the policy does not require another aircraft to occupy the intervening opportunity; it can remain empty. Below the configured activation rate for that STAR family, the additional spacing is inactive. Future changes may configure or disable individual families without silently changing the others.

## Sequence protection and queueing

Unstable aircraft are ordered by the earliest physically achievable, policy-valid slot. They cannot displace Stable or frozen aircraft.

Stable aircraft retain their established relative order. When an earlier legal slot becomes vacant, the first eligible queued aircraft is promoted automatically according to queue order, physical feasibility, and current protection rules; no controller acceptance command is required. A slot more than 30 seconds ahead of current operational TETA is not feasible.

Superstable and manually frozen aircraft retain their captured slot except under a specifically authorized manual workflow or a confirmed go-around. A late raw TETA is informational and must never automatically release Superstable or change its captured operational TETA and slot. Queue offers are removed when they can no longer be used or when the aircraft becomes fully frozen.

TMA freeze must not use entry into a configured terminal path as its boundary. The approved horizontal boundary is the EKCH Copenhagen Approach `MultiPolygon` from the SimAware TRACON project, vendored from [`Boundaries/EKCH/EKCH.json` at commit `d860ed77135b057168148184880a41cc183bf881`](https://github.com/vatsimnetwork/simaware-tracon-project/blob/d860ed77135b057168148184880a41cc183bf881/Boundaries/EKCH/EKCH.json). The operational volume extends from the surface to strictly below FL195; an observation at FL195 or above is outside it. This operator-approved boundary remains valid until explicitly superseded and does not cycle automatically with AIRAC data. Production configuration must use a validated local/versioned copy with source provenance rather than fetching the mutable upstream file during runtime.

### Holding-stack ordering

When two eligible aircraft are confirmed in the same holding stack, AMAN may use physical stack order to correct their sequence. The existing behavior prefers the lowest observed aircraft first after explicit manual order and protected-slot rules.

This behavior must be configurable per STAR entry family. The minimum policy values are:

- `disabled`: holding altitude does not affect sequence order;
- `lowest-first`: the lowest confirmed aircraft in the same holding stack is preferred first.

The initial default for every STAR entry family is `disabled`: sequence order follows the current landing-time/slot calculation without reordering aircraft from their physical position in a holding stack. Different STAR families or holding IDs must not influence one another. When a family is explicitly configured as `lowest-first`, missing/stale altitude or uncertain holding detection leaves the normal order intact. Manual order, Stable protection, and Superstable/manual freezes retain precedence. A validated TMA freeze has the same precedence. Configuration uses an enum so future strategies can be added without changing the meaning of a boolean.

## Landing-runway GAPs and approach stops

An authorized FMP must be able to insert a GAP on a selected landing runway group to represent an approach stop or other temporary loss of landing capacity.

A GAP is first-class persisted operational state with:

- stable identity and command/audit identity;
- runway group;
- required explicit absolute UTC start and end, or an unambiguous start plus slot count; neither form has a default;
- operational reason;
- creator and creation time;
- active/removed/expired state.

Slot-count input is converted to an absolute UTC interval when the command is accepted, using the then-current arrival rate. The persisted GAP is always time-based, so a later rate change never resizes it.

The frontend renders a GAP distinctly from an aircraft and from automatic separation. When a GAP is inserted, every aircraft already assigned inside the interval is moved to a later valid opportunity using all normal rate, wake, STAR, lifecycle, and queue policies, irrespective of Stable, Superstable, manual, or validated TMA freeze protection. The displacement and resulting revision remain explicitly audited.

Automatic sequencing never assigns an aircraft inside an active GAP. A later explicitly authorized manual placement inside the GAP is allowed, remains visibly exceptional, and is audited; the GAP itself remains active for all other traffic.

Removing or expiring a GAP reopens capacity and triggers deterministic normal resequencing. A GAP must survive restart and replay and must never be represented as a fake aircraft or callsign.

## Go-around behavior

When a go-around is confirmed, AMAN creates a new physical prediction using the configured ten-minute go-around delay. The aircraft is reinserted into the earliest feasible slot and downstream movable traffic is shifted as necessary until a valid sequence is restored.

Automatic detection creates a visible confirmation request and must not mutate lifecycle, slots, or protected traffic by itself. An authorized controller must confirm the detected episode before AMAN applies the go-around; an explicit authorized controller report may act as that confirmation. Rejection and duplicate observations must not create repeated mutations or prompts for the same episode. Detection evidence, confirmation, frozen-slot exceptions, and reset of direct/route facts remain explicit and auditable.

## Presentation

The AMAN frontend shows, at minimum:

- callsign and sequence order;
- runway group and assigned slot;
- STAR entry family, feeder fix, and relevant holding fix;
- lifecycle phase and freeze reason;
- raw and operational TETA where appropriate;
- gain/lose instruction;
- queue opportunity, warnings, degraded data, manual overrides, and GAPs.

Gain/lose is transported internally as signed seconds. Controller-facing presentation uses rounded whole minutes: `Gnn` for time to gain, `Lnn` for time to lose/delay, and `=00` when the absolute value is below 30 seconds. Absolute minutes are rounded with `floor((abs(seconds) + 30) / 60)`, zero-padded to two digits, and capped as `G99+` or `L99+`. Labels must consistently use **LOSE**, not **LOOSE**.

Selecting/scheduling the active landing runway group and setting/scheduling a runway group's arrival rate are independent FMP operations. A rate command must never change runway selection or reassign flights; only the dedicated runway-selection command may perform selection-driven reassignment.

## Implementation audit

This section records what the repository does today so that agents do not mistake an implemented behavior for an approved requirement. It was checked against the code on 2026-09-08. The intended behavior in the sections above remains authoritative after the open decisions are resolved.

| Area | Current implementation | Relationship to this specification |
| --- | --- | --- |
| Route terminology | EKCH configuration calls TESPI/TUDLO/MONAK/TIDVU/ERNOV `feeders`; downstream fixes such as TNO and KOR are undifferentiated path fixes. `SelectedFeeder` is also published as both feeder and STAR. | Model change required; the approved feeder-fix mapping is recorded above. |
| Controller directs | The trajectory domain can apply route facts and can infer track-aligned off-route recovery. The EuroScope/backend event contract does not carry an explicit controller-issued direct-to fact. | Ingestion and transport work required. Track inference is not a substitute for the clearance. |
| Stable/Superstable clock | The operational lifecycle currently uses hard-coded landing-TETA horizons. A second prediction reducer uses holding-fix ETA for Superstable. Neither consistently uses ETA to a distinct feeder fix. | Must be consolidated on configured feeder-fix ETA. |
| Freeze policy | Entering the configured terminal path can create a `tma` freeze. Separately, a Superstable flight whose raw TETA is over four minutes later than its slot is automatically unfrozen and resequenced. | Both conflict with the approved behavior. Track corrections in #560 and #561; the approved TMA volume is recorded above. |
| Same-STAR spacing | Enabled for all configured EKCH runway groups at 20 arrivals/hour and above, with one empty grid opportunity. Identity is the current outer `SelectedFeeder`. | Migrate the retained values to explicit per-STAR-family configuration and corrected family identity. |
| Holding order | Confirmed aircraft in the same holding are globally ordered lowest-first, after manual/protected rules. The strategy is not configurable by STAR. | Per-STAR configuration required. |
| Queueing | Backend queue offers describe occupied earlier slots and expire with a revision. There is no accept-offer command, and the calculation does not allocate a newly vacant slot. | Automatic promotion is approved; track the correction in #562. |
| Runway group and rate | Setting a rate for a runway group also schedules that group as selected; there is no independent runway-selection command. | Independent operations are approved; track the correction in #563. |
| WTC/L | The predictor applies a performance/wind model to Light aircraft. Light piston aircraft are excluded from automatic sequencing until manually included. | All WTC/L must follow the documented RETA policy and remain eligible; track in #564. |
| GAP | No first-class runway GAP state or command exists. | Implementation required using the approved normalization and displacement policy above. |
| Go-around | A manual command applies a ten-minute delay and cascade. A surveillance detector exists in lifecycle/replay code but is not wired into the live operational service. | Live detection must request controller confirmation; track in #565. The exact time model remains open. |
| Gain/lose display | Backend publishes signed seconds. The web UI displays signed `m:ss`; the EuroScope Gain/Lose display remains separately tracked. | Rounded controller `Gxx`/`Lxx` presentation is approved and tracked in #334 for EuroScope and #567 for the web frontend. |
| FMP controls | Backend role authorization and commands exist, but the EKCH page currently passes `hasFMPAuthority=false`, so controls are unavailable. | A server-backed capability must reach the frontend; track in #566. |
| Freeze contract | Backend can publish freeze reason `tma`; the frontend validator accepts only `none`, `superstable`, and `manual`. | Current TMA-frozen state can invalidate the complete frontend AMAN payload; correct as part of #561. |
| MAESTRO workspace | The current route renders `AMANBoardView` beside a generic `AMANControls` sidebar. It does not implement the FMP/RWY/ACC workspace, three timelines, local target fields, or the two-row MAESTRO settings bar. | Frontend implementation work required against the design section and Figma references above. |
| TMT tools | No AMAN traffic-prediction, holding-information, or warning tool is present in the live AMAN route. | Traffic and holding tools require backend read models and frontend implementation. The initial warning policy is recorded above. |
| Aircraft operations | Current controls cover a subset of move/freeze/rate/ETA/go-around actions. Alternate/change runway, maximum delay, coordination, DSEQ, closure, reserved capacity, and confirmed removal do not exist as the complete designed workflows. | Add only through typed backend commands; reuse #557 for GAP and do not implement fake aircraft. |

Implementation references include `backend/config/aman/ekch-terminal-2609.json`, `backend/internal/aman/operational/service.go`, `backend/internal/aman/operational/mutations.go`, `backend/internal/aman/prediction/reducer.go`, `backend/internal/aman/sequence/queue.go`, `backend/internal/aman/lifecycle/go_around.go`, `backend/pkg/events/frontend/aman.go`, `frontend/src/api/aman.ts`, and `frontend/src/routes/ekch/AMAN.tsx`.

## Configuration and replay requirements

- Operational thresholds and per-STAR policies are versioned configuration, not scattered hard-coded constants.
- Configuration validation fails early with field/path-specific errors.
- Changes that alter route or sequencing meaning change the relevant configuration/dataset digest.
- Persisted state contains enough version and provenance information to replay the same result.
- Delayed, duplicated, or out-of-order inputs cannot regress accepted operational state.
- Degraded operation is visible; the system does not silently substitute a different semantic fix or time basis.

## Open operational decisions

Update this section when decisions are made:

1. Define runway-closure termination/removal and protected-slot interaction. Starting after an aircraft and starting at an explicit absolute UTC time are both approved.
2. Define Maximum Delay semantics, authorization, and interaction with Stable/Superstable traffic beyond the invariant that separation and protected traffic cannot be bypassed.

## GitHub issue relationship

Implementation work is tracked in GitHub issues under the AMAN epic. Issues should link to this file and update it when they resolve an open operational decision. This file must not become a checklist of code tasks.

The 2026-09-08 code-audit corrections are tracked by #560 (Superstable immutability), #561 (altitude-bounded TMA volume), #562 (automatic vacancy promotion), #563 (independent runway selection), #564 (WTC/L RETA policy), #565 (confirmed live go-around detection), #566 (server-backed FMP controls), #334 (EuroScope Gain/Lose formatting), and #567 (web Gain/Lose formatting).

The frontend design is tracked by parent feature #584 and its focused child issues: #585 (workspace shell/settings), #586 (FMP/RWY/ACC timelines), #587 (targets/dialogs), #588 (DSEQ/closure/reserved capacity), #589 (TMT traffic prediction), #590 (TMT holding information), and #591 (TMT warning decision and implementation).
