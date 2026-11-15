# Airport coverage coordination

The `remaining-responsibilities/v1` policy uses the immutable airport definition's
ordered responsibility providers and airborne fallback. It does not read strip
owners, sector owners, master priority, or runway setup to grant participation.
The existing `legacy-single-airport/v1` EKCH association remains unchanged.

The plugin declares deployment eligibility with a space-separated
`[coverage] eligibleAirports` setting. There is no default airport. The backend
intersects this declaration with enabled definitions, authenticated participation
authority, and configured identity/frequency matching. An optional application
authority predicate can restrict participation further. No declaration grants
session IDs, editable state, or command authority.

One application coordinator reads one generation of the existing shared VATSIM
and transceiver sources per pass. The reviewed prerequisite #1059 at `03032f5`
already acquires controller inventory and retains LIVE source demand for control
connections with zero memberships. No coverage worker performs upstream HTTP.
Controllers without FlightStrips participate in coverage evidence. ATIS and
observers do not take responsibility. Fresh own-instance plugin radio reports
override provider radios; an unknown tuned list retains provider evidence.
Duplicate plugin instances cannot inherit one another's tuning or eligibility.

Inventory, identity and radios have explicit fresh, stale and unknown states.
SWEATBOX environments use configured primary frequencies and callsigns only:
additional tuned radios are unavailable there, so their evidence is neither
required nor used for responsibility or takeover decisions. LIVE and playback
keep their configured tuned-frequency policy.
The default source age limit is 45 seconds, independent of airport grace and
plugin startup delay; application assembly can inject a different age and clock.
A failed refresh makes retained source data stale immediately. Expired or future
timestamps cannot confirm takeover or absence. Unknown/stale evidence preserves
established membership. Simulation providers must supply their own timestamped,
environment-scoped inventory through `CoverageSimulated`; without it, inventory
stays unknown. A connected simulated controller must also be present in that
source's inventory: connected plugins cannot fill a supposedly complete empty
roster. Public LIVE evidence is never substituted for training/playback.

## Simulation staffing acquisition

`cmd/server` accepts the optional `SIMULATED_STAFFING_FILE` path. The shared
coordinator reads this local snapshot once per pass (normally once per second),
regardless of the number of airports or memberships. An exercise-server bridge
or another authoritative simulation source must atomically replace the file
with its actual observations. No default roster is synthesized. The input is
trusted server configuration; plugins and browsers cannot configure this path
or declare roster completeness. This setting does not change LIVE admission.

The versioned JSON document contains `environments`, each with:

- `environment`: exact uppercase exercise/playback name from control evidence,
  such as `SWEATBOX`, `SWEATBOX_EXERCISE1`, or `PLAYBACK_<instance-id>`. `LIVE`
  and duplicate environments are rejected.
- `observed_at`: RFC3339 observation time. Reading the file never advances it.
  Future and expired observations cannot confirm membership or takeover.
- `controllers_complete`: true only when the source has acquired the whole
  exercise's controller inventory, including controllers without FlightStrips.
  A partial view or one connected plugin does not establish this fact.
- `controllers`: an explicit array of identities, each with `cid`, `callsign`,
  `observer`, `primary_frequency`, `tuned_frequencies_known`, and
  `tuned_frequencies`. CIDs must match the authenticated controller identities.
  Decimal MHz and Hz frequencies normalize through `vatsim/v1`.

For sources that report known radios, an explicit array is required, including
`[]` only when the source actually observed no additional tuned frequencies.
Unknown radios remain
unknown even if a partial list is supplied. Fresh own-instance plugin radio
evidence may supplement the roster's same identity; it never establishes a
complete inventory. SWEATBOX can omit radio facts and supplies coverage using
the observed primary frequency and callsign. For LIVE/playback definitions that
use tuned frequencies, all active controllers need fresh radio evidence before
absence/takeover can be decided.
Observers are ignored for responsibility selection.

Missing files, invalid JSON, unknown fields, missing identity/inventory facts,
and duplicate identities fail the read. Source errors cannot grant new
membership or confirm a new takeover; established memberships and previously
confirmed takeover deadlines keep the normal behavior. A producer must refresh
observations within the 45-second source age limit. Do not touch/rewrite old
data with a new timestamp or infer empty staffing from a failed source read.

The browser harness supplies explicit rosters for its fully controlled fixture
exercises. These test peers and their radios are known by construction; their
source is not a production acquisition mechanism.

## PR #1095 SWEATBOX finding and remaining source requirement

Read-only inspection of the isolated multi-airport test on 2026-11-15 confirmed
the loaded plugin declares `EKCH EKBI EKYT`, its controller row is `EKDK_CTR` on
`136.555`, and only the EKCH `SWEATBOX` session exists. EKBI's selected definition
matches `136.555` to `EKDK_UN_CTR`, carrying tower, approach, and delivery duties
when the complete exercise roster leaves them available. The generic
`EKDK_CTR` callsign and EKCH relevant-airport hint do not prevent that match.

Two independent gates blocked regional admission in the unmodified test:
`cmd/server` previously supplied no `CoverageSimulated` source, and the native
plugin's `SendLoginEvent` sends no confirmed tuned-frequency list. The latter
is now intentionally ignored for SWEATBOX. EKCH joins through
`legacy-single-airport/v1`; EKBI uses `remaining-responsibilities/v1` and still
waits for fresh, complete controller inventory. Selecting an airport in the
backend or plugin eligibility does not replace that requirement.

The bundled EuroScope SDK exposes `CController::GetPrimaryFrequency` and
`ControllerSelectFirst/Next`. It exposes no complete tuned-radio list or
exercise-wide inventory completeness signal. The existing `ControlEvent`
contains only own-instance evidence, and operational sync controller lists are
membership-scoped, lack full radio/identity facts, and cannot bootstrap a
zero-membership connection. Public VATSIM/AFV LIVE feeds cannot describe local
SWEATBOX staffing.

An authoritative exercise roster producer is still required to make the
real test join EKBI safely. The file input supplies the server acquisition
boundary, but this repository does not contain a EuroScopeFsdServer adapter
capable of establishing roster completeness. The unavailable additional radios
are not a requirement in SWEATBOX. Do not generate a one-controller roster
from plugin presence or mark a primary radio as a complete tuned list. The
running test, its profile, plugin DLLs/config, scenario, processes and database
were left unchanged.

## Membership lifecycle

The coordinator applies only airport membership deltas. A partial replacement
retains remaining duties. After fresh evidence confirms full replacement,
relinquishment occurs at replacement backend attachment or the configured grace
deadline, whichever comes first. Attachment does not wait for role warmup or full
sync. Fresh returning responsibility cancels takeover; transient stale/unknown
evidence cannot extend an already confirmed deadline. One application timer
tracks source health and the earliest outstanding deadline, with control/join/
remove notifications prompting reevaluation.

Responsibility returning to an eligible still-connected instance invokes the
ordinary membership join path, including from zero memberships. Backend
restoration, the intentional plugin delay, current master election, full sync,
and offline/read-only transitions remain authoritative. Rejoin creates a new
membership incarnation; old contexts and human command bindings stay retired.
Human commands retain their exact own-plugin authority, and genuine automatic
effects retain the existing same-session executor.

The five MA-04 layers follow all six reviewed MA-05 layers in the existing
combined stack #1062. Their base is #1073 at
`0808538e1b8fc1cf07c8bab91374deea3e80d785`; the original shared-source starting
point was #1059 at `03032f5`. MA-05 branch heads remain unchanged. Publication does
not declare either epic merged or operationally activated. Real airport
responsibilities, frequencies, regions, master values and grace remain rollout
inputs. Only synthetic coverage fixtures are added; regional PDC/AMAN remain
disabled and no new airport is activated.
