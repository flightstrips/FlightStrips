---
title: AMAN CPH feature specification
status: living-specification
audience: maintainers-and-coding-agents
source_revision: operator-decisions-2026-09-08
airport: EKCH
---

# AMAN CPH feature specification

## Purpose and maintenance

This is the internal, repository-owned description of AMAN CPH. It turns the original `AMAN-CPH V0.1` concept document and later operator corrections into a single implementation reference.

Maintainers and coding agents should update this file when an operational decision changes. GitHub issues describe individual pieces of work; this file describes how the complete feature is expected to behave. Where this specification conflicts with an old planning issue or the original draft, this specification is the intended product behavior unless a newer recorded decision says otherwise.

## Product objective

AMAN CPH predicts when inbound aircraft can reach Copenhagen, assigns landing-runway slots, presents a stable arrival sequence, and tells controllers how much time each aircraft should gain or lose. The backend owns the prediction, lifecycle, sequence, persistence, and command validation. FlightStrips clients supply operational observations and present the resulting state.

The system must remain deterministic across restarts and replay. Navigation geometry, configuration, prediction inputs, manual actions, and degraded states must retain enough provenance to explain a result.

## Terminology

These concepts are deliberately distinct:

- **STAR entry family**: the outer arrival-route family, currently TESPI, TUDLO, MONAK, TIDVU, or ERNOV. It is used to select terminal geometry and may be used by STAR-family sequencing policy.
- **Feeder fix**: the downstream operational fix from which Stable and Superstable timing is measured. Examples include TNO and KOR. A feeder fix is not automatically the STAR entry family or holding fix.
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

## Terminal route model

Every configured STAR-family/runway path must identify, separately:

1. STAR entry family;
2. feeder fix;
3. selected holding;
4. merge fix;
5. final approach and runway threshold.

The active configuration already contains route fixes such as TNO and KOR inside its paths, but currently names the outer TESPI/TUDLO-style family as the feeder. The model must be corrected without losing the outer family identity. Each configured feeder fix must resolve uniquely and occur on its associated path.

TNO and KOR are confirmed examples. Remaining feeder-fix mappings must be verified from the operational design rather than guessed, especially where a family uses different downstream fixes for different runway directions.

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

### Same-STAR spacing: current behavior and open decision

The current EKCH configuration enables same-STAR spacing for every runway group at 20 arrivals per hour and above with one empty grid opportunity between aircraft from the same STAR entry family.

This is sometimes described as requiring an “alternating arrival,” but the implementation does not require another aircraft to occupy the intervening opportunity; it can remain empty. Below the activation rate, the additional same-family spacing is inactive.

Whether to retain, reconfigure, or disable this policy is an open operational decision. Until that decision is recorded, implementations must not silently change the live default. Future wording should call it **same-STAR-family spacing** rather than alternating arrivals.

## Sequence protection and queueing

Unstable aircraft are ordered by the earliest physically achievable, policy-valid slot. They cannot displace Stable or frozen aircraft.

Stable aircraft retain their established relative order. When an earlier legal slot becomes vacant, the first eligible queued aircraft is promoted automatically according to queue order, physical feasibility, and current protection rules; no controller acceptance command is required. A slot more than 30 seconds ahead of current operational TETA is not feasible.

Superstable and manually frozen aircraft retain their captured slot except under a specifically authorized manual workflow or a confirmed go-around. A late raw TETA is informational and must never automatically release Superstable or change its captured operational TETA and slot. Queue offers are removed when they can no longer be used or when the aircraft becomes fully frozen.

TMA freeze must not use entry into a configured terminal path as its boundary. It may operate only against a dedicated, versioned three-dimensional TMA volume supplied and approved by the operator, including horizontal geometry and applicable altitude limits. The required TMA box is an external prerequisite and must not be guessed. Until it is available and validated, TMA-based freeze behavior must not be enabled.

### Holding-stack ordering

When two eligible aircraft are confirmed in the same holding stack, AMAN may use physical stack order to correct their sequence. The existing behavior prefers the lowest observed aircraft first after explicit manual order and protected-slot rules.

This behavior must be configurable per STAR entry family. The minimum policy values are:

- `disabled`: holding altitude does not affect sequence order;
- `lowest-first`: the lowest confirmed aircraft in the same holding stack is preferred first.

Different STAR families or holding IDs must not influence one another. Missing/stale altitude or uncertain holding detection leaves the normal order intact. Manual order, Stable protection, and Superstable/manual freezes retain precedence. A validated TMA freeze has the same precedence. Configuration should use an enum so future strategies can be added without changing the meaning of a boolean.

## Landing-runway GAPs and approach stops

An authorized FMP must be able to insert a GAP on a selected landing runway group to represent an approach stop or other temporary loss of landing capacity.

A GAP is first-class persisted operational state with:

- stable identity and command/audit identity;
- runway group;
- absolute UTC start and end, or an unambiguous start plus slot count;
- operational reason;
- creator and creation time;
- active/removed/expired state.

The frontend renders a GAP distinctly from an aircraft and from automatic separation. The sequence engine never assigns an aircraft inside an active GAP and moves affected movable aircraft to later valid opportunities using all normal rate, wake, STAR, lifecycle, and queue policies.

By default, inserting a GAP that intersects a protected Stable/Superstable/manual or validated TMA-frozen slot is rejected with a useful conflict. Any ability to override protected slots requires its own explicit authorization and product decision; it must not be implied by ordinary GAP creation.

Removing or expiring a GAP reopens capacity and triggers deterministic normal resequencing. A GAP must survive restart and replay and must never be represented as a fake aircraft or callsign.

## Go-around behavior

When a go-around is confirmed, AMAN creates a new physical prediction using the configured go-around model (the original concept assumed approximately 10 minutes). The aircraft is reinserted into the earliest feasible slot and downstream movable traffic is shifted as necessary until a valid sequence is restored.

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
| Route terminology | EKCH configuration calls TESPI/TUDLO/MONAK/TIDVU/ERNOV `feeders`; downstream fixes such as TNO and KOR are undifferentiated path fixes. `SelectedFeeder` is also published as both feeder and STAR. | Model change required; feeder-fix mappings need an operator decision. |
| Controller directs | The trajectory domain can apply route facts and can infer track-aligned off-route recovery. The EuroScope/backend event contract does not carry an explicit controller-issued direct-to fact. | Ingestion and transport work required. Track inference is not a substitute for the clearance. |
| Stable/Superstable clock | The operational lifecycle currently uses hard-coded landing-TETA horizons. A second prediction reducer uses holding-fix ETA for Superstable. Neither consistently uses ETA to a distinct feeder fix. | Must be consolidated on configured feeder-fix ETA. |
| Freeze policy | Entering the configured terminal path can create a `tma` freeze. Separately, a Superstable flight whose raw TETA is over four minutes later than its slot is automatically unfrozen and resequenced. | Both conflict with the approved behavior. Track corrections in #560 and #561; #561 requires the operator-provided altitude-bounded TMA volume. |
| Same-STAR spacing | Enabled for all configured EKCH runway groups at 20 arrivals/hour and above, with one empty grid opportunity. Identity is the current outer `SelectedFeeder`. | Live default is documented, but final policy and corrected identity remain open. |
| Holding order | Confirmed aircraft in the same holding are globally ordered lowest-first, after manual/protected rules. The strategy is not configurable by STAR. | Per-STAR configuration required. |
| Queueing | Backend queue offers describe occupied earlier slots and expire with a revision. There is no accept-offer command, and the calculation does not allocate a newly vacant slot. | Automatic promotion is approved; track the correction in #562. |
| Runway group and rate | Setting a rate for a runway group also schedules that group as selected; there is no independent runway-selection command. | Independent operations are approved; track the correction in #563. |
| WTC/L | The predictor applies a performance/wind model to Light aircraft. Light piston aircraft are excluded from automatic sequencing until manually included. | All WTC/L must follow the documented RETA policy and remain eligible; track in #564. |
| GAP | No first-class runway GAP state or command exists. | Implementation required. |
| Go-around | A manual command applies a ten-minute delay and cascade. A surveillance detector exists in lifecycle/replay code but is not wired into the live operational service. | Live detection must request controller confirmation; track in #565. The exact time model remains open. |
| Gain/lose display | Backend publishes signed seconds. The web UI displays signed `m:ss`; the EuroScope Gain/Lose display remains separately tracked. | Rounded controller `Gxx`/`Lxx` presentation is approved and tracked in #334 for EuroScope and #567 for the web frontend. |
| FMP controls | Backend role authorization and commands exist, but the EKCH page currently passes `hasFMPAuthority=false`, so controls are unavailable. | A server-backed capability must reach the frontend; track in #566. |
| Freeze contract | Backend can publish freeze reason `tma`; the frontend validator accepts only `none`, `superstable`, and `manual`. | Current TMA-frozen state can invalidate the complete frontend AMAN payload; correct as part of #561. |

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

1. Confirm the feeder fix for every STAR-family/runway path beyond the known TNO and KOR examples.
2. Decide whether current same-STAR-family spacing is retained, reconfigured, or disabled.
3. Select the initial holding-stack ordering value for each STAR entry family.
4. Confirm the default duration/slot-count interaction and protected-slot conflict workflow for an approach-stop GAP.
5. Confirm whether the current fixed ten-minute go-around delay remains the desired time model. Controller confirmation of automatic detection is already decided.

## GitHub issue relationship

Implementation work is tracked in GitHub issues under the AMAN epic. Issues should link to this file and update it when they resolve an open operational decision. This file must not become a checklist of code tasks.

The 2026-09-08 code-audit corrections are tracked by #560 (Superstable immutability), #561 (altitude-bounded TMA volume), #562 (automatic vacancy promotion), #563 (independent runway selection), #564 (WTC/L RETA policy), #565 (confirmed live go-around detection), #566 (server-backed FMP controls), #334 (EuroScope Gain/Lose formatting), and #567 (web Gain/Lose formatting).
