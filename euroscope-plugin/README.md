# FlightStrips plugin for EuroScope

This is the FlightStrips for the EuroScope which handles all the communication with the FlightStrips API.

The on-screen FlightStrips panel and departure-clearance popup are automatically kept inside the current radar area. If the saved panel position still needs to be recovered, enter `.fs reset` in EuroScope's command line to move it back near the top-left corner and reopen it.

The expanded panel always shows `Master: <callsign>` below the connection status, even when Stats is collapsed. This identifies the controller whose EuroScope runway configuration governs the session. It shows `Master: None` when connected without a known master, and `Master: Unknown` while disconnected or reconnecting rather than displaying a stale callsign.

## AMAN tag visibility

The **AMAN G/L** tag shows time-to-gain/time-to-lose guidance for EKDK controllers and ESMM APP/CTR positions when the aircraft is tracked by you or EuroScope predicts it will enter your active sectors. This includes unassumed aircraft and aircraft already inside your sectors. Unassumed aircraft that will never enter your sectors show neither guidance nor an unavailable placeholder. EKDK FMP retains its overview of untracked arrivals.

The existing destination-airport, holding-release, and inside-TMA rules still apply.

## Force release a tower tag

If a tag is stuck with a FlightStrips tower controller, a Departure, Approach, or Centre controller can send **ROF** from TopSky and then assume the released tag normally. The tower's plugin handles the `/ROF/<aircraft>/<requesting controller>` scratchpad pulse and ends EuroScope tracking when the pulse matches a valid live flight tracked locally by an online `_TWR` controller at facility 4. The named requester must be online as `_DEP` or `_APP` at facility 5, or `_CTR` at facility 6. Split callsigns are supported and role suffixes are case-insensitive.

Observers, DEL/GND requests, and DEP/APP/CTR-owned tags do not trigger a release. Normal airborne-to-airborne ROF coordination continues as usual. The requester assumes the tag in EuroScope, and existing tracking synchronization handles FlightStrips strip ownership. TopSky's broadcast pulse names a controller position in the payload and carries no authenticated sender identity.

Release is handled on the tracking EuroScope instance without requiring a FlightStrips backend connection or radar correlation. Only the native tag is released; a failed release is logged and does not trigger a backend ownership change.

### Implementation path

- Normal transfers originate in `backend/internal/services/strip_coordination.go` and are applied through `MessageService::HandleCoordinationHandoverEvent` (`StartTracking` / `InitiateHandoff`) and `HandleEsAssumeOnlyEvent` (`AcceptHandoff`).
- ROF recovery runs in `GroundTagReleaseHandler::ControllerFlightPlanDataEvent`, registered with the unfiltered flight-plan event handlers, and calls `EndTracking` after validating the pulse and controller roles. The main plugin only dispatches events; recovery does not require radar relevance, a pending transfer, or a matching next-controller route, since those may be the broken state being recovered.
- `FlightPlanService::OnTimer` detects the resulting tracking-controller change. `StripService.HandleTrackingControllerChanged` preserves strip ownership on release and handles the subsequent assumption through existing coordination and bay-movement rules.

### Native verification checklist

Use a test session with two EuroScope instances; unit tests cannot exercise live VATSIM tracking or TopSky's transient scratchpad delivery.

- Track a departure on TWR with a stuck outgoing handoff. Send ROF from a facility 5 DEP position and verify that TWR releases the tag and DEP can assume it normally. Repeat with split callsigns such as `EKCH_A_TWR` and `EKCH_N_DEP`.
- Repeat the release and assumption from APP at facility 5 and CTR at facility 6.
- Confirm the tag is released only on its tracking instance, including when that instance is not the FlightStrips master or has no backend connection.
- Confirm that airborne-to-airborne ROF follows normal coordination and keeps the current tag owner until the controller transfers it. Confirm the Tower tag stays tracked for DEP requests with a facility other than 5, DEL/GND requests, and observer requests.
- Confirm malformed pulses or a pulse naming another aircraft do not drop the selected tag, and normal scratchpad/holding updates continue to work.
- After assumption, check that FlightStrips receives the tracking-controller update and completes the existing coordination/bay transition.

## AMAN holding release reminder

The `AMAN G/L` tag item appears for aircraft with an active TopSky hold once AMAN's calculated EAT is four minutes away or less. It counts down as `L04`, `L03`, etc., then shows `G00` at EAT and gain minutes after EAT. Earlier holds and holds without a calculated EAT stay blank. This uses AMAN's EAT independently of EuroScope EAT writeback.

The usual arrival, controller, tracking and TMA visibility rules apply. Stale or disconnected data is grey, as with other AMAN guidance.

## Credit

Many parts of the plugin is inspired by the [UK Controller Plugin](https://github.com/VATSIM-UK/uk-controller-plugin)

