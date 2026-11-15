# AMAN approach buffer and gain loss investigation

Checked against main at `84281a50` on 13 November 2026.

The historical comparisons below describe that revision before the guidance
fix. The implemented change is described in the next section.

## Implemented guidance fix

Pre-approach guidance now excludes time AMAN plans to absorb downstream while
preserving landing slots, raw prediction, EATs, release capacity and holding
priority. Accepted admissions supply their release/transit budget. A bounded
preview of the same release planner supplies provisional absorption outside
the commitment window without acquiring reservations or changing EATs. The
preview counts incoming traffic over its horizon, assumes no future departures
from approach, and is withdrawn for unusable workload or changed inputs.

In the rounded NSZ5YB reproduction, the five-minute landing difference and
299 seconds of planned approach absorption now produce one second of loss
instead of `L05`, including before a release reservation exists. Full/degraded
workload, ineligible or blocked streams and manual protection cannot obtain a
provisional discount. An adjustment cannot manufacture gain after some delay
has already been absorbed; inside approach the full landing drift applies.

Both feeds and diagnostic guidance use the effective target. EuroScope's
target timestamp follows that target while retaining the physical prediction;
the web keeps the landing slot and exposes optional `guidance_target_time`
and `approach_absorption_seconds`. Aircraft targets display the corrected
value without an additional tooltip. Existing EuroScope holding countdown
behavior is unchanged.

Tests cover partial/full absorption, genuine gain, changed slots/routes,
stale data, accepted admissions during outages, preview stream spacing and
input permutation, full/uncertain buffers, future incoming traffic, persistence,
disabled policy, and web/EuroScope agreement. The earlier characterization
test was replaced by
`TestApproachReleaseAdvanceExcludesAbsorptionFromBothGuidanceTransports`.
The holding-priority implementation is unchanged.

Airborne gain/loss still measures drift against the landing slot. Approach
buffer advances change EAT and allocate additional time in approach without
moving that slot. This distinction was present when the buffer shipped; the
history does not establish a regression that removed a buffer allowance from
gain/loss. It can nevertheless create a controller-facing ambiguity about
where the displayed delay should be absorbed.

There is also a confirmed change after Monday, 9 November: Tuesday's holding
priority fix can give a later aircraft a later landing slot and therefore
larger loss from unchanged predictions. That is a different mechanism from
subtracting an approach-buffer allowance.

## Comparison with Monday 9 November

### Confirmed change on Tuesday

[PR 1135](https://github.com/flightstrips/FlightStrips/pull/1135), commit
`3e752905`, merged **Tuesday, 10 November at 21:01:48 Copenhagen time**, changes
cross-stream holding priority. It is included in backend **3.8.3**, released
that evening. Before it, enabling approach release also allowed a later
aircraft already holding on another stream to bypass older holding traffic.
The fix removes that exception: older holding traffic gets priority unless
physical reachability or separation establishes capacity it cannot use.

A deterministic comparison uses the same runway rate, wake categories,
physical readiness and protected holder slots at every revision. Two older
aircraft hold on one stream with landing slots 40 and 41.5 minutes after the
reference time. They can physically reach the runway after ten minutes. A
later aircraft already holding on an independent stream has a physical and
operational prediction at 15 minutes:

| Code revision | Landing order | Later aircraft target | Its landing loss |
| --- | --- | --- | --- |
| Monday end of day, `7a40b763` | Later aircraft, older holder 1, older holder 2 | +15 minutes | 0 minutes |
| Tuesday immediately before PR 1135, `07a40e16` | Later aircraft, older holder 1, older holder 2 | +15 minutes | 0 minutes |
| PR 1135 merge, `3e752905` | Older holder 1, older holder 2, later aircraft | +43 minutes | 28 minutes |
| Current main, `84281a50` | Older holder 1, older holder 2, later aircraft | +43 minutes | 28 minutes |

The 28-minute result is a synthetic characterization of the changed rule,
not a measured delay from Monday's traffic. The commit boundary was executed
on both its parent and the merge commit, rather than inferred from the title.
Additional comparisons distinguish active holding from an aircraft merely
approaching its holding: in the matching example without active holding,
Monday already assigned the later target. When the older holders cannot
physically use the earlier opportunity, current code allows the other stream
through. The extended
`TestCrossFeederOvertakeRequiresEvidenceOfUnusedCapacity` covers both cases
for a later aircraft already holding.

This establishes a real way smaller loss could disappear after Monday. It
does not establish that PR 1135 caused NSZ5YB's five-minute loss: the supplied
20:11 snapshot had future holding entry, not an accepted early admission.
The captured later state also has no active holding clearance or stack for
NSZ5YB. The demonstrated larger-loss case specifically requires the later
aircraft already to be holding.

### Direct guidance and buffer comparison

`guidance.go` is byte-identical at the start of Monday (`832d20ef`), the end
of Monday (`7a40b763`) and current main. Its Git blob at all three revisions
is `428b6b84d16d298604aceb750c5e07a552c57693`. The web gain/loss formatter is
also unchanged since Monday. The EuroScope changes add holding EAT countdown
and visibility for concerned unassumed aircraft; they do not reduce or
increase ordinary airborne loss.

The future-readiness and early-release characterization test also passes
against Monday's actual planner and projections: the landing drift remains
-300 seconds before and after 299 seconds are allocated to approach. The
comparison omits only the EuroScope EAT wire assertion, because that field
was introduced on Tuesday. The web EAT/absorption assertions and EuroScope
landing target, physical prediction and gain/loss assertions run on both.

A read-only database capture from the manual test has session 3, revision
111, generated at `2026-11-13T20:28:22.0226792Z`. Running Monday's and current
EuroScope event projections against this same saved state yields identical
landing guidance, prediction and target for **all 45 flights**. This isolates
the projection; it is not a replay of the upstream prediction or scheduler.
At that later snapshot NSZ5YB is already predicted after its slot and both
versions return capped gain, so it must not be presented as the original
20:11 loss snapshot.

The audit data independently puts NSZ5YB's operational prediction near
20:26:39, its slot at 20:33:38 by revision 33 (20:08:52 UTC), and its frozen
slot at 20:33:18 by revision 45 (20:11:52 UTC). It does not preserve all raw
prediction inputs for an exact replay of that earlier snapshot. The later
captured flight uses the primary `performance_wind` prediction with Airacnet
provenance; the new terminal fallback cannot simply be assumed responsible.

### Scope of the result

The strongest identified change within the Monday-to-Friday window is the
tested Tuesday holding-priority change. Prediction sources and slot ordering
also changed Wednesday and Friday, but no exact historical NSZ5YB input replay
establishes them as its cause. Monday's absence of the holding countdown also
rules out that new display as something already working on Monday.

PR 1135 deliberately addresses holding admission priority. Its demonstrated
larger landing loss does not by itself justify reverting that priority rule.
The remaining product question is how to present early approach admission
and planned downstream absorption separately from landing drift while
respecting the applicable holding order. No runtime policy is changed here.
Production rollout timing could not be checked: the connected GitHub service
returned 404 for the separate infrastructure repository. Release dates above
describe repository releases, not a verified deployment time.

## Intended behavior and released history

- [Issue 887](https://github.com/flightstrips/FlightStrips/issues/887) requests
  an initial intake across independent streams when approach is quiet, with
  landing slots retained and additional approach time accounted for. It does
  not define proactive airborne gain/loss or a separate pre-approach target.
- The [feature specification](../.github/specs/aman-cph.md#approach-release-buffer-887)
  specifies separate intake planning, a 120-second admission window, and
  physical transit distinct from additional approach time. Its gain/loss
  section defines signed seconds and display rounding, without defining a
  buffer-adjusted guidance target.
- `f53cbf3d` initially transported operational TETA minus the landing slot to
  EuroScope. [PR 772](https://github.com/flightstrips/FlightStrips/pull/772),
  merged 21 September, deliberately changed both web and EuroScope guidance
  to physical RawTETA so frozen sequencing estimates would not preserve delay
  that the aircraft had already absorbed. `718ceb9e` capped gain at two
  minutes; `6935e16a` introduced the separate physical smoothing path.
- [PR 912](https://github.com/flightstrips/FlightStrips/pull/912), merged
  17 October as `603c8c7f`, added durable release reservations, EAT advances,
  and additional approach time. It did not change either gain/loss calculation
  or its target. The backend changelog includes this in version 3.4.0,
  released 18 October. This establishes released repository behavior, not
  which server/plugin versions were running in a particular past session.
- [PR 1137](https://github.com/flightstrips/FlightStrips/pull/1137), merged
  10 November, added a separate EuroScope holding countdown from EAT minus
  four minutes. For aircraft EuroScope reports as holding, the tag follows
  EAT instead of landing drift, subject to the existing visibility rules.
  This is one implemented behavior that can follow buffer advances.

The inspected mainline history provides no earlier airborne formula that
subtracts additional approach time or a quiet-approach allowance. A past tag
or EAT that appeared to do so could have reflected the separate release
display or different physical/slot inputs; the available evidence does not
identify the user's remembered session.

## When smaller loss values could have changed

The confirmed change of prediction reference is **21 September 2026 at
19:40 Copenhagen time**, commit `558d08a9`, PR 772. It first appears in
backend **2.3.0** and frontend **1.6.0**, both released that day. Both the web
projection and the backend's EuroScope feed changed together:

```text
Before: operational TETA - landing slot
After:  physical RawTETA - landing slot
```

For example, with a slot at 20:33, operational TETA at 20:31 and physical TETA
at 20:28, the old formula gives `L02` and the new formula gives `L05`.
Those are illustrative inputs, not recovered NSZ5YB values. The change can
also reduce loss: the regression fixture actually added in PR 772 has the
operational estimate nine minutes before the slot and the physical estimate
20 seconds after it, changing the result from `L09` to effectively on time.
Consequently the commit establishes when the reference changed, but does not
establish that it caused this particular recollection.

Later changes that can affect the displayed number or its inputs are:

| Date in 2026 | Change | Relevance to smaller loss |
| --- | --- | --- |
| 4 October | PR 838 caps positive gain at 120 seconds | Negative loss remains uncapped; it cannot explain removal of a loss allowance. |
| 4 October | PR 842 adds a separate median physical estimate, released in backend 3.1.2 | This replaces direct RawTETA for guidance and can change transient values, while continuing to ignore operational freezes. It also changes slot allocation, so actual loss can change through the target. |
| 17 October | PR 912 adds approach release reservations | The admission lookahead and separate EAT/absorption semantics exist in the initial implementation; this did not remove an earlier buffer discount. |
| 19 October | PR 931 bypasses physical smoothing on explicit route recovery | A newly accepted physical ETA can affect guidance immediately after recovery. |
| 20 October | PR 939 suppresses guidance for an unreliable prediction | This affects availability rather than introducing or removing a loss allowance. |
| 10 November | PR 1137 adds the holding countdown | It displays time until EAT for holding aircraft, independently of landing drift. |
| 13 November | PR 1151 adds fresh terminal fallback estimates after route/provider failures | Replacing a retained estimate with a different physical prediction can change loss while a landing slot stays fixed. This is a candidate only for affected flights; no before/after NSZ5YB evidence establishes it as the cause. |

The inspected EuroScope formatting history also does not show a recent
multi-minute loss discount being removed. PR 841 on 4 October changes the
near-zero label from `=00` to `G00` and controller visibility; it keeps the
same nearest-minute rounding. Web rounding changed on 10 September from
signed minutes/seconds to nearest-minute `Gnn`/`Lnn`, which cannot account
for a several-minute allowance.

The September reference change predates the user's Monday comparison point.
It is historical context, not an explanation for a change after 9 November.

## Current implementation

`GainLossPrediction` in `backend/internal/aman/guidance.go` returns a smoothed
physical estimate independently of operational freezes. Both event projections
call `GainLossGuidance(predicted, flight.Slot.Time)`. Negative seconds become
`Lnn`; neither the reservation nor additional approach time changes that
landing reference. The diagnostic log uses the same calculation.

`release.Plan` first considers readiness, stream order and spacing, current
workload and uncertainty, and the tapered advance allowance. It creates an
offer only when the resulting release is within the admission lookahead.
This is a window on the candidate release, not on its baseline EAT: a baseline
EAT more than two minutes away can already advance if readiness and the
candidate release fit the window.

`refreshApproachReleases` publishes an accepted reservation through the holding
plan. The plan keeps physical transit separate from
`slot - release - transit`, its additional approach time. The web detail dialog
shows EAT, accepted transit and that additional time. EuroScope receives EAT
separately and its holding tag counts down to it; ordinary airborne guidance
still follows the landing slot, and tags are suppressed inside the TMA.

## Scenario evidence and reproduction

The supplied live snapshot around 20:11 UTC has NSZ5YB physical TETA near
20:28 and a landing slot near 20:33. A five-minute loss follows directly from
those inputs. Its predicted holding entry and baseline EAT were still ahead,
with no release reservation and zero additional approach time. That snapshot
does not prove AMAN had committed to an early admission, or would do so later:
workload, uncertainty, spacing and readiness can change before admission.

The scenario file independently establishes NSZ5YB's filed ERNOV route, but
does not reconstruct its later surveillance or release decisions. The live
environment was left unchanged. The following synthetic test uses rounded
times resembling the supplied case, with an assumed 20:18 holding entry and
ten-minute transit, rather than claiming an exact replay:

| Calculation time UTC | Candidate release UTC | Reservation | Landing gain/loss | Additional approach time |
| --- | --- | --- | --- | --- |
| 20:11:00 | 20:18:01 | None, outside lookahead | -300 seconds | None |
| 20:16:00 | 20:18:01 | None, one second outside lookahead | -300 seconds | None |
| 20:16:01 | 20:18:01 | Offered, at lookahead boundary | -300 seconds | 299 seconds |

The baseline EAT is 20:23 throughout. The candidate remains strictly after
holding entry. `TestApproachReleaseAdvanceKeepsLandingGuidanceAndEATSeparate`
exercises the actual planner and both event projections, explicitly transferring
the reservation into the holding plan as the operational service does. It
characterizes current behavior rather than approving a new guidance policy.
Existing operational tests cover the service's reservation-to-plan mapping,
slot preservation and physical transit, and planner tests cover tapering,
blocked streams, uncertain workload and retained admissions.

## Controller implications and recommendation

If a controller interprets `L05` as five minutes to lose before approach, the
display does not reveal that AMAN may intend some of that time to be absorbed
downstream. Slowing an inbound aircraft can move its readiness later; that can
reduce the early intake the buffer was designed to permit. This is a plausible
feedback problem, not evidence of actual controller behavior or a demonstrated
operational outcome in the supplied session. Five minutes of landing drift is
also not five minutes of holding when an early release allocates that time to
approach.

Keep landing drift inspectable and distinguish it from a target for reaching
the release/approach reference. Show accepted EAT and downstream absorption
beside it so the controller can identify where planned delay belongs. A
release-specific gain/loss should compare a physical prediction and target at
the same release reference, with an explicit reference label. Subtracting
absorption from the existing value while continuing to publish a runway target
and runway prediction would make the transport and display meanings diverge.

To affect NSZ5YB at 20:11, a reservation-only adjustment is insufficient: there
was no accepted absorption to subtract. Proactive guidance before the admission
window needs a distinct provisional planning target, with defined workload
forecasting, stream constraints, uncertainty, expiry and commitment semantics.
It must remain distinguishable from an accepted EAT and capacity reservation.
Simply subtracting the maximum allowance, today's buffer allowance, or the
entire expected hold could hide real delay when no early admission is feasible.

No runtime calculation or operational policy is changed by this investigation.
The appropriate next product change is to define and present the release
reference alongside landing drift, then decide whether provisional guidance
before the reservation window is required. Validation should cover partial
downstream absorption, full/degraded buffers, independent and shared streams,
changing readiness/slots, retained or withdrawn offers, and agreement between
web and EuroScope references.

## Validation

Passed from `backend`:

```text
go test ./internal/aman ./internal/aman/release ./internal/aman/sequence ./internal/aman/operational ./pkg/events/frontend ./pkg/events/euroscope
```

The guidance fix additionally passes the full backend suite with an isolated
PostgreSQL test database (`go run ./internal/testing/testdb ./...`), 99 targeted
frontend tests and the production frontend build. The live scenario and
running manual test services were not modified. All 62 AMAN consumer contract
tests and lint on the changed frontend files also pass.
