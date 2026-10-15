# Text pilot support

Implements [issue #834](https://github.com/flightstrips/FlightStrips/issues/834).
The [original manual](text-pilot-support/original-manual.md) is preserved in this repository.
The [controller guide](../../docs/src/content/docs/procedures/text-pilots.mdx) explains operation.

## Scope and decisions

Communication type `T` enables the text workflow. `R` displays a receive-only indicator but keeps voice behavior. `V` and unknown display an empty indicator. The square is one third of its parent cell's height, bottom-right, with a 1px left divider on departure strips and no divider on arrival strips; `T` uses `#FF6D4D`. Departure placement is the callsign cell for delivery/cleared/tower strips and the aircraft-type cell for apron push/taxi strips. Arrival placement is the rightmost cell.

Seven composer variants cover push/start, apron taxi-out/in, tower taxi-out/in, SI, and takeoff/landing. The editable message field supports phrase toggles, ERASE, SEND and ESC. Pilot replies are read in EuroScope. PDC remains a separate workflow.

| Trigger | Accepted operational action | Cancellation |
| --- | --- | --- |
| Push/taxi/holding-point selection | Staged until SEND; existing backend handlers validate each action | ESC discards staged actions and retains the map |
| SI with owned strip and selected station | Transfer at SEND; primary or cross-coupled coverage resolves the target | ESC leaves ownership untouched |
| SI with outgoing transfer and REMAIN | Cancel transfer at SEND | ESC preserves transfer |
| Incoming split SI | Existing assume behavior | No text menu |
| Stand SEND REQ | Accept assignment first, then open taxi-in with accepted stand | ESC preserves assignment |
| Runway button | Accept runway action first, open text menu with resulting red/green state | ESC preserves runway state |
| Manual ATC clearance | Accept clearance first, generate text from accepted strip | Failed submission exposes a recovery composer |

SEQPLN's initial READBACK CORRECT is recorded only after successful submission, keyed by controller callsign and aircraft within the current session. CLX cleared strips select readback and, when covered, apron 121.905. Tower-in special K3/K2/B/A/F presets apply to TWTE/GEGW; TWRGND retains generic taxi-map defaults.

Clearance text chooses SID, flight-plan route, or radar vectors, then destination, initial climb in feet, and assigned squawk. Without a SID it adds assigned heading. Existing `SA`/`SA 3000'` SID selections yield straight-ahead instructions; no new EuroScope heading sentinel is introduced. PDC issue/acknowledgement does not generate a second frequency clearance.

## Transport and failure handling

Frontend `send_frequency_message` carries request ID, callsign, message, optional accepted-action batch, `clearance`, and `prepare`. Preparation is restricted to one stand/runway action or a validation-only PUSH move. Pushback preparation checks ownership and backend TSAT validation before opening the text composer, without moving the strip or setting its release point. A blocking validation returns `validation_blocked`; the controller must resolve it and retry. SEND rechecks timing, closing the composer and opening validation if conditions have changed. The backend rejects commands, multiline/empty/oversize text, mismatched callsigns, unsupported actions, non-text aircraft, and observer operations. Operational actions use existing service validation; partial success is reported as `applied_actions`. They are not a database transaction.

EuroScope protobuf events 54/55 carry the request and result. The plugin advertises `frequency_messages` on its token. Routing requires the same session, CID and primary position, a non-observer connection, and that capability. The chosen connection is pinned; the session master is not used and requests are not replayed after reconnect.

The plugin uses the documented [FREQ operation](https://www.euroscope.hu/wp/editing-and-function-keys/): select the flight through the SDK, populate the empty command input and dispatch the numeric-keypad Multiply key (`VK_MULTIPLY`, virtual key 106 / `0x6A`, scan code `0x37`). It checks the current primary frequency and refuses to overwrite controller input. `submitted` means the command input was consumed, not that the pilot received/read the message. A failed FREQ dispatch can leave the text in the command input for inspection.

The frontend waits 12 seconds for the matching request/callsign result, blocks duplicate pending sends, shows failures, and retains edited text. A retry skips actions already reported as applied. On an uncertain timeout, it never replays operational actions automatically; the controller must inspect EuroScope and strip state before resending. A recovery clearance with no pending actions sends the edited message as ordinary text.

Bind EuroScope FREQ to numpad Multiply. No plugin key configuration is required; the former INI override is ignored.

## Design references

[Figma source](https://www.figma.com/design/csKJuv9WCgO36UjsE1lC2h/Project-FS---Flightstrps-by-VATSCA): indicator 3584:371; SI 3586:471; push 3551:6409; apron-out 3551:6410; apron-in 3551:6819; tower-out 3551:6411; tower-in 3551:6540; runway 3576:8658. Geometry for tower-out, push/start, SI, apron-out, apron-in, tower-in, and runway is stored in `frontend/src/components/text-pilot/storyboard-layouts.json`, recovered from the original Figma design-context responses. Those composers scale the storyboard as one frame, preserving taxiway placement, station ordering, button dimensions, colors, and typography. They reuse the repository's dialog component. Rubik is bundled with its OFL license under a feature-specific font family; the application-wide font configuration is unchanged. Required functional adaptations: push includes SEND/ESC in its unused lower space; SI includes ERASE between ESC and SEND; the editable message fields use white backgrounds; the SI frequency spelling is corrected. Selection colors reflect the current draft rather than the example aircraft in Figma.

The user supplied tower-out and indicator SVG exports and screenshots on 2026-10-11, preserved in [figma/](text-pilot-support/figma/). SVGs are layout references only; no SVG fonts, text paths, or full-screen SVG rendering are used in the application. Tower-out now uses the complete 1390 x 985 layout, including separate taxiway/holding-point controls for repeated labels, both crossing buttons, and native SEND/ESC/ERASE controls. Typography continues to use the previously recovered Rubik styles. Indicator placement and divider geometry were checked against the supplied reference without changing unrelated strip layout. The repeated CROSS RWY 04R/22L labels are retained as supplied and share one phrase selection.

The user confirmed that the SI/apron-in/tower-in dividers are plain lines with Figma weight 3 and that the tower-in arrow is literal `------>` text. These are now implemented as native 3px layout lines (scaled with each frame) and centered literal text. No external divider or arrow SVG assets are required. The annotated `*-missing-details.png` screenshots document the prior gaps, not the current implementation. No full pixel-comparison claim is made for the recovered code-only references.

## Verification and EuroScope acceptance checklist

Automated checks cover phrase selection, wind, preserved edits, request/result correlation, no automatic timeout replay, observer/disconnected rejection, clearance generation, command/action validation, sender routing, capability negotiation and C++ protobuf encode/decode. Browser fixtures exercise all seven variants at 1920x1080 and 1280x720 without a real connection. Additional checks cover opening the composer above a pushback map and returning to the map with ESC, departure and arrival indicator placement for text/receive/voice pilots and compact-cell geometry. The composer is mounted in the controller `/app` route. SI CONTACT highlights with a selected frequency; taxi-in stand controls resolve the accepted draft stand or the strip stand.

Before release, verify in a controlled EuroScope test session:

1. Confirm the numpad Multiply FREQ key sends command-line text on the primary frequency and addresses ASEL. Do not use a live operational frequency for the test.
2. Select aircraft A in EuroScope, SEND a message for B from FlightStrips, and verify B is addressed and the text is broadcast on the sending controller's frequency.
3. Use two controllers and verify a non-master controller sends through their own plugin. Verify primary-frequency changes are rejected.
4. Leave text in the command input and verify SEND fails without overwriting it. Change the key binding and verify failure is visible.
5. Disconnect the plugin during a request, inspect uncertain state, and verify reconnect does not resend.
6. Exercise SI incoming/outgoing/other ownership, TSAT rejection, cross-coupled stations, staged map cancellation, accepted stand cancellation and accepted runway cancellation.
7. Issue manual and PDC clearances separately and verify one intended message per clearance flow. Verify readback defaults reset on a session change.

Live EuroScope command submission and network receipt have not been validated by the automated tests.

## Local verification results (2026-10-10)

- Backend EuroScope/frontend/event package tests pass.
- Win32 Debug plugin build and all 463 CTest cases pass.
- Frontend production build passes; focused composer, store, phrase, SI and stand tests pass.
- All seven Chromium composer browser checks pass. Firefox could not launch in this environment (`spawn UNKNOWN`).
- The full frontend suite has one pre-existing failure: `TacticalStrips.test.tsx` expects a 167px dialog but renders 107px. No tactical-strip implementation was changed.
- Documentation site builds successfully, including this feature's controller guide.

Operational action validation is delegated to the existing action handlers, including corrective holding-point assignments and TSAT PUSH retries. If an action is accepted but message submission fails, the composer locks phrase/text/ERASE edits while retaining SEND and ESC; retry sends the same instructions without replaying accepted actions. Close and reopen to change instructions. Failures with no accepted actions leave editing available.

EuroScope communication type is normalized case-insensitively: live SDK/session data may contain lowercase `t`, `r`, or `v`. Snapshot, reconnect, and incremental plugin updates resolve the controller-assigned override first and fall back to the filed preference, emitting canonical uppercase values (or empty for unknown). The backend canonicalizes both initial strip snapshots and incremental communication events, and uses the same normalization for message eligibility. The frontend displays that canonical backend state without interpreting EuroScope casing or flight-plan remarks. Existing lowercase session values need no database migration.

FREQ dispatch is pinned to numpad Multiply. The former `text_pilot.frequency_key` override is ignored. Native `SendInput` injects numpad scan-code `0x37` key-down/key-up after focusing EuroScope and selecting ASEL. Direct `WM_KEYDOWN`/`WM_CHAR` dispatch is not used for FREQ because it bypasses EuroScope shortcut processing. Submission stays pending until a later EuroScope timer callback observes input consumption; a two-second timeout reports failure without resending or clearing input. Only one FREQ submission may be pending. Held modifiers, focus failure, or native input rejection are reported as errors.

## Latest verification (2026-10-15)

- Win32 Debug plugin build and all 472 CTest cases pass, including native numpad scan-code input and deferred submission acknowledgement checks.
- Frontend production build and the full frontend suite (834 tests across 93 files) pass with the rebased main dependencies. The Chromium pushback map/composer check also passes.
- Backend communication normalization, frequency messages, pushback/move validation and handover tests pass.
- The user confirmed that native numpad Multiply submission works in their local EuroScope session. Automated tests do not independently verify pilot/network receipt; the remaining multi-controller and failure-path manual checks above still apply.
- Docker-based backend integration tests could not run in this Windows setup (`rootless Docker is not supported on Windows`).
