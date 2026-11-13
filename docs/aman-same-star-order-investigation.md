# Same-STAR automatic order investigation — 13 November 2026

## Evidence and limits

The manual test used `.manual-test/backend.log` and the local PostgreSQL
`aman_session_audit_records` from worktree `52e8`, together with
`8. EKCH_Holding_Stress (Rwy22L).txt`. No running session, route clearance,
scenario, or production configuration was changed by this investigation.

Both SAS1778 and SAS46R have the simulator route
`LUTUS TESPI ROSBI TNO CH653 CH645 CH638 CH631 CH626 ABEGI`. Their filed
routes differ upstream: SAS1778 includes ODEVA/GUNPA, SAS46R BIVKI/ODRID.
The scenario places them close together and starts SAS1778 three minutes
before SAS46R. That supports the reported stream order but does not reconstruct
their later live positions or all sequencing inputs.

The revision 37 audit at `2026-11-13T17:52:55.3616608Z` contains
`automatic_order_optimization` for SAS46R, AUA2864, DLH4PM, RYR79RU and VKG129.
These records identify participating flights, not the trial pair, optimizer
inputs, intermediate order or the cause of a particular crossing.

The guidance log makes a useful distinction (its wall clock is Copenhagen time):

| Revision | SAS1778 target | SAS46R target | Observation |
| --- | --- | --- | --- |
| 34 | 18:22:45 UTC | Not yet assigned | SAS1778 vacancy promotion |
| 37 | 18:22:45 UTC | 18:25:45 UTC | SAS46R gets its first target and optimizer marker; still behind SAS1778 |
| 38 | 18:23:45 UTC | 18:26:45 UTC | Controller command changes targets; relative order retained |
| 39 | 18:23:45 UTC | 18:20:45 UTC | Automatic update advances SAS46R six minutes, with no optimizer marker |

At revision 39, SAS1778's operational estimate was about 18:16:36 UTC and
SAS46R's about 18:19:25 UTC. SAS1778's logged degradation was basic terminal
fallback following unusable route geometry / missing or ambiguous ODEVA.
Thus revision 37's optimizer marker does **not** establish when the visible
inversion happened. Revision 39 is consistent with ordinary reallocation into
a vacancy ahead of a retained reservation. Complete historical pure inputs,
lifecycle/holding evidence and optimizer trials were not persisted, so this is
not an exact replay proving every internal branch taken.

## Code findings

- `sequenceSTARProgress` intentionally rejects holding-derived/manual feeder
  clocks, stale data, unpublished predictions and missing terminal identity.
  Passed-feeder comparisons require matching remaining route suffixes. The
  reported holding-derived feeder clocks cannot prove free-flight order.
- The movable optimizer previously accepted a lower-delay trial if its total
  known STAR inversion count did not increase. Unknown progress counts no
  inversion, so it allowed unsupported crossings. Aggregate scoring also let
  one corrected inversion compensate for another crossing.
- Stable same-STAR exchanges previously allowed lower-delay swaps without
  comparable physical evidence. Equal-delay swaps already needed evidence.
- Initial placement checked runway spacing and protected slot occupancy but
  could insert ordinary later inbound traffic into a gap before an earlier
  same-STAR reservation. Keeping the leading slot's time does not retain its
  relative order. This path runs before optimization.
- Gap filling already restricts overtakes to independent streams, and vacancy
  compaction retains Stable order. Actual holding queues, holding exchanges,
  controller order and frozen reservations have distinct policies.

## Implemented policy

Ordinary automatic traffic on the same explicitly selected STAR family now
retains committed relative order when route progress is incomparable. New
arrivals use initial operational timing order as a conservative insertion
boundary. That timing is a scheduling fallback, not physical evidence. Placement
cannot jump an earlier same-STAR reservation, including a Stable reservation,
merely because a separated runway gap exists ahead of it.

Movable allocation respects committed same-STAR predecessors. Optimizer trials
must justify **every** reversed same-STAR pair with comparable physical progress,
including flights moved indirectly by reallocation. Stable exchanges also
require affirmative physical precedence, while retaining reachability, wake,
neighbor spacing and slot protections. Existing physical tolerances remain
15 seconds before the feeder and 0.5 NM after it.

Committed reservations determine the order in which movable flights are
allocated. Physical corrections are evaluated during placement and optimization,
where the other arrivals and their crossing constraints are available. Physical
and reservation-fallback comparisons are not mixed as hard allocation
dependencies: with committed A–B–C, missing progress for B and physical C-before-A
would otherwise create a cycle and stop allocation for every runway. The
incomparable pairs retain A–B–C; a fully comparable crossing can still be corrected.

Controller manual order/position, manual freezes, explicit rebuild ranks,
go-arounds and actual holding priorities retain their dedicated rules.
Cross-family delay optimization and gap filling remain available. An already
inverted same-STAR reservation is not automatically repaired from uncertain
geometry: it requires comparable physical evidence or controller intervention.
Contradictory precedence constraints fail allocation and let operational
reconciliation retain its previous state.

No route or direct clearance is synthesized. Missing route geometry remains
visible as degraded prediction evidence; the fix does not promote fallback
landing estimates or holding release clocks into physical STAR progress.

`aman.star_order_evaluated` audit records now capture same-STAR pairs at initial
insertion or changed committed relative order. They contain prior slots,
operational estimates, lifecycle/protection/controller/holding context, nullable
physical progress, comparability, and prior/final pair order. Unchanged established
pairs are omitted. Payload order is deterministic. These records describe the
input/final pair relationship, not every optimizer trial or a complete replay.

## Validation

Regression coverage includes missing/one-sided/incompatible/near-equal progress,
delay-saving wake inversions, proven physical corrections, unknown crossings
hidden by aggregate inversion scoring, Stable insertion boundaries, committed
Unstable order despite ETA changes, deterministic input permutations, and the
operational holding-derived feeder / basic fallback case. Existing explicit
swaps, holding priority, stable compaction, cross-family gap filling, separation,
freeze and manual-position tests remain part of the AMAN suite.

Validation passed with `go test ./internal/aman/...` from `backend`.
