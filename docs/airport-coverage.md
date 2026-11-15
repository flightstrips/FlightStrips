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

The application coordinator uses each physical instance's EuroScope controller
observation. Controllers without FlightStrips participate in that evidence;
ATIS and observers do not take responsibility. No coverage worker performs
upstream HTTP or requires a public VATSIM feed to be working.

Master participation and election use ES callsigns/primary frequencies plus the
configured airport policies. The shared transceiver cache remains connected to
server sector ownership, route selection and PDC, including relevant EKCH/EKBI
frequency carriage. It is not an input to master admission/election. Other
features retain their existing public-feed acquisition. Clearing tuned lists
for master participation does not change airport definitions or sector matching.

Inventory, identity and radios have explicit fresh, stale and unknown states.
Visible-roster participation uses primary frequencies and callsigns in LIVE,
SWEATBOX and playback. Additional radios are unavailable in exercises and are
not required for master admission. LIVE sector/route selection still uses the
existing transceiver observations where relevant.
The default source age limit is 45 seconds, independent of airport grace and
plugin startup delay; application assembly can inject a different age and clock.
A failed refresh makes retained source data stale immediately. Expired or future
timestamps cannot confirm takeover or absence. Unknown/stale evidence preserves
established membership. Public LIVE evidence is never substituted for
training/playback.

## Visible controller acquisition

The deployment policy assumes the controllers visible to EuroScope constitute
the complete relevant roster for master participation, in LIVE and exercises.
Multi-airport use must load the relevant airports/controllers into that view.

On the SDK thread, `GetVisibleControllers` reads `ControllerMyself` and exhausts
`ControllerSelectFirst/Next`, including controllers without FlightStrips. It
captures callsign, primary frequency and observer status without position-file
or airport filtering. ATIS is excluded and observers do not take duties. An
unavailable local identity yields unknown inventory rather than an empty roster.

`ControlEvent.visible_staffing` carries the complete flag, observation revision,
age and visible controllers over the authenticated physical transport. The initial
control report includes the list before any airport membership or operational
sync exists. The timer resamples the list every SDK tick, sends changes promptly,
and refreshes unchanged observations at the normal ten-second control interval.
Reconnects acquire a fresh list; no membership-scoped sync list is reused.

The normal `cmd/server` coordinator evaluates each instance's own observation in
that instance's environment. It does not merge lists from other instances or
borrow LIVE data. Duplicate/missing identities, a mismatched local frequency,
incomplete or expired observations cannot authorize new membership or
confirm a new takeover. An older observation cannot roll the same transport's
roster backward or renew a reused snapshot. Freshness uses server receipt time
and observation age rather than client UTC, so users need no clock matching or
server access. Public datafeed and transceiver acquisition are not prerequisites
for master membership in any environment. Missing source data preserves
established membership and does not extend a previously confirmed takeover
deadline.

The SDK does not provide CIDs for visible peers. Responsibility matching uses
their callsigns and primary frequencies; backend attachment confirmation matches
the same airport/environment/callsign. Human actions still require their exact
authenticated plugin instance and current membership context.

No manual JSON file, external roster producer, or `CoverageSimulated` dependency
is required in normal runtime. The temporary `SIMULATED_STAFFING_FILE` input has
been removed. Optional in-process staffing injection remains available for older
test/replay providers without a native control report. The browser harness sends
the same wire observations as the plugin rather than injecting a staffing feed.

## PR #1095 SWEATBOX reproduction

Read-only inspection of the isolated multi-airport test on 2026-11-15 confirmed
the loaded plugin declares `EKCH EKBI EKYT`, its controller row is `EKDK_CTR` on
`136.555`, and only the EKCH `SWEATBOX` session exists. EKBI's selected definition
matches `136.555` to `EKDK_UN_CTR`, carrying tower, approach, and delivery duties
when the complete exercise roster leaves them available. The generic
`EKDK_CTR` callsign and EKCH relevant-airport hint do not prevent that match.

The original server had no non-LIVE roster acquisition, and tuned radios were
required despite being unavailable in the exercise. Both gates are now handled
through the visible-roster control protocol and primary-frequency matching.
Production-app socket tests reproduce the `EKDK_CTR`/`136.555`, EKCH+EKBI case
with the public datafeed disabled and without an injected staffing provider,
then exercise non-plugin APP takeover in LIVE, SWEATBOX and playback,
departure, stale observations, reconnects and exercise isolation. The already
running test's profile, DLLs/config, scenario, processes and database are not
modified by these tests. An updated plugin and backend must both be loaded in
a subsequent authorized native run to use the new protocol.

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
