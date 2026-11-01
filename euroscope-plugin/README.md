# FlightStrips plugin for EuroScope

This is the FlightStrips for the EuroScope which handles all the communication with the FlightStrips API.

The on-screen FlightStrips panel and departure-clearance popup are automatically kept inside the current radar area. If the saved panel position still needs to be recovered, enter `.fs reset` in EuroScope's command line to move it back near the top-left corner and reopen it.

The expanded panel always shows `Master: <callsign>` below the connection status, even when Stats is collapsed. This identifies the controller whose EuroScope runway configuration governs the session. It shows `Master: None` when connected without a known master, and `Master: Unknown` while disconnected or reconnecting rather than displaying a stale callsign.

## Force release a tower tag

If a tag is stuck with a FlightStrips tower controller, an APP or CTR controller can send **ROF** from TopSky and then assume the released tag normally. The tower's plugin handles the `/ROF/<aircraft>/<requesting controller>` scratchpad pulse and ends EuroScope tracking only when the aircraft matches the pulse, the tag is tracked locally, and both controllers are online controller positions: `_TWR` (facility 4) to `_APP` (facility 5) or `_CTR` (facility 6). Split positions are supported.

Observers, DEL/GND requests, and APP/CTR-owned tags do not trigger a release. Normal airborne-to-airborne ROF coordination is unchanged. This does not automatically assume the tag or change the FlightStrips strip owner; EuroScope tracking updates continue through the existing synchronization path. The request uses TopSky's existing broadcast protocol, which identifies the requester in the payload rather than exposing an authenticated sender to the receiving plugin.

Release is handled on the tracking EuroScope instance without requiring a FlightStrips backend connection or radar correlation. Only the native tag is released; a failed release is logged and does not trigger a backend ownership change.

### Implementation path

- Normal transfers originate in `backend/internal/services/strip_coordination.go` and are applied through `MessageService::HandleCoordinationHandoverEvent` (`StartTracking` / `InitiateHandoff`) and `HandleEsAssumeOnlyEvent` (`AcceptHandoff`).
- ROF recovery runs in `GroundTagReleaseHandler::ControllerFlightPlanDataEvent`, registered with the unfiltered flight-plan event handlers, and calls `EndTracking` after validating the pulse and controller roles. The main plugin only dispatches events; recovery does not require radar relevance, a pending transfer, or a matching next-controller route, since those may be the broken state being recovered.
- `FlightPlanService::OnTimer` detects the resulting tracking-controller change. `StripService.HandleTrackingControllerChanged` preserves strip ownership on release and handles the subsequent assumption through existing coordination and bay-movement rules.

### Native verification checklist

Use a test session with two EuroScope instances; unit tests cannot exercise live VATSIM tracking or TopSky's transient scratchpad delivery.

- Track a departure on TWR, send ROF from APP and from CTR, then assume it on the requester. Repeat with a stuck outgoing handoff and with split controller callsigns.
- Confirm the tag is released only on its tracking instance, including when that instance is not the FlightStrips master or has no backend connection.
- Confirm APP-to-CTR, CTR-to-APP, DEL/GND and observer requests do not automatically drop a tag.
- Confirm malformed pulses or a pulse naming another aircraft do not drop the selected tag, and normal scratchpad/holding updates continue to work.
- After assumption, check that FlightStrips receives the tracking-controller update and completes the existing coordination/bay transition.

## Credit

Many parts of the plugin is inspired by the [UK Controller Plugin](https://github.com/VATSIM-UK/uk-controller-plugin)

