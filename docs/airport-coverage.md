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
The default source age limit is 45 seconds, independent of airport grace and
plugin startup delay; application assembly can inject a different age and clock.
A failed refresh makes retained source data stale immediately. Expired or future
timestamps cannot confirm takeover or absence. Unknown/stale evidence preserves
established membership. Simulation providers must supply their own timestamped,
environment-scoped inventory through `CoverageSimulated`; without it, inventory
stays unknown. Public LIVE evidence is never substituted for training/playback.

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

This implementation starts on the reviewed MA-05 shared-source slice, not a claim
that the full MA-05 epic has completed. The stack must be coordinated/rebased onto
the appropriate completed MA-05 state before the cross-epic prerequisite is
declared complete. Real airport responsibilities, frequencies, regions, master
values and grace remain rollout inputs. Only synthetic coverage fixtures are
added; regional PDC/AMAN remain disabled and no new airport is activated.
