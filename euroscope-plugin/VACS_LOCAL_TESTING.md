# Native VACS controls: local testing

## Try the local build

1. In EuroScope's plugin dialog, unload the existing FlightStrips plugin first. Do not load the release loader and this development core simultaneously.
2. Load `build/bin/FlightStripsPluginCore.dll` directly from this checkout. The core exports the EuroScope entry points; loading it directly avoids the release loader replacing the development build through its update check.
3. Keep `flightstrips_config.ini` and the runtime DLLs beside the core in `build/bin`. The build copies the config and test runtime dependencies there. Existing installed plugin files and preferences are not modified by this build.
4. Start VACS 3.x, enable remote control, sign in, and connect your controller position there.
5. In the FlightStrips panel, switch **VACS** to **On**. Use **Call...**, **Answer/Reject**, **Cancel**, or **End call**. No selected aircraft or FlightStrips backend login is required.
6. Check **Show in mini**, then minimize to try the compact controls. Uncheck it to restore the original header-only minimized mode. The checkbox can be set while VACS is Off.

Preferences are saved beside the loaded core in `flightstrips_user.ini`. For VACS on another trusted computer, edit its `[vacs] host` setting to a hostname or IP only; the plugin uses port 9600 and path `/ws`. Empty host means localhost. Voice/audio stay in VACS. Disabling integration or hiding/closing the panel does not send End.

Use `.fs open` to reopen the panel or `.fs reset` if it is off-screen. Normal header dragging, close/minimize controls, session selection, statistics, and PDC controls remain available.

## Next-sector calling tag item

The optional **VACS next sector** tag item shows a small phone icon using EuroScope's native telephone glyph (ANSI byte `0xB6`, supported by the standard and DK COOPANS EuroScope fonts) for aircraft you track locally. Left-click calls that destination; right-click opens the controller picker. TopSky's **SI** field and its click bindings stay unchanged. Calling uses the existing VACS connection and works independently of FlightStrips backend login.

### Destination selection

The EuroScope SDK exposes callbacks for a plugin to fill its own tag items, but no API to read another plugin's rendered SI text. This field resolves a destination independently, in this order:

1. **Active handoff:** the native handoff controller callsign and position ID.
2. **Native coordination intent:** a nonempty `GetCoordinatedNextController()` in a known state other than `REFUSED`. Accepted, manually accepted, requested and `NONE` states are allowed. Whether particular EuroScope or TopSky manual assignments populate this field before acceptance, especially in `NONE`, needs live verification. Refused or unknown states fall back to prediction; that prediction may still name the controller who refused.
3. **Route prediction:** the first valid controller ID different from your own, starting at `GetSectorExitMinutes()`. This avoids selecting an upstream sector when you assume an aircraft before it enters your airspace. Short sectors are valid destinations. Unknown exit time (`-1`) or an exit outside the available prediction data (up to 60 minutes) gives no prediction. The position ID is resolved through EuroScope to a connected controller's full callsign.

Empty and punctuation-only position IDs such as `--` are ignored in predictions. Explicit handoff/coordination callsigns with no usable short ID use a derived display label; that label is never used as a VACS matching key.

Agreement with TopSky SI has not been measured or verified live. How TopSky manual SI overrides affect native SDK data is also unverified. Prediction results are forecasts, not confirmation of coordination.

### Display and calling

- **Blank:** the aircraft is not tracked locally.
- **Phone icon, normal tag colour:** VACS is ready and exactly one next-controller client matches.
- **Phone icon, muted grey:** no destination resolved, VACS is not ready, or there is no unique VACS client match.
- **Left-click:** checks VACS readiness, resolves the selected aircraft again and fetches a fresh client list. The sector ID, full controller callsign and VACS client ID must match the target FlightStrips last generated for that aircraft, within the previous 10 seconds. Missing, expired, changed or previously unavailable targets cannot dial. Callback `ItemString` text is diagnostic only, so a custom tag family's callback text does not cause a false destination-change refusal. Long sector IDs remain fully available for identity validation because only a one-character phone is displayed. Zero or multiple distinct VACS matches cannot dial. Failed attempts show a FlightStrips notification.
- **Right-click:** opens the two-column **VACS CALL** picker with controller labels and frequencies. If VACS is unavailable, the empty picker shows its state label.

FlightStrips captures the aircraft, generated icon, sector ID, full controller callsign, matched client ID and timestamp during its own tag callback. It re-resolves the destination on click and requires the identities to agree; a different controller with the same icon is rejected too. The snapshot is used for comparison, not as a stale call destination. Callback text and both generated/fresh identities are logged for each target check.

### Manual EuroScope tag configuration

No tag layout or `.prf` is shipped for this item. Add it to your own layout:

1. Open EuroScope's **OTHER SET** menu and choose **TAG** to open Tag Items Setup.
2. Select the tag family and label type you use.
3. Add the **FlightStrips** item **VACS next sector** in a new field. The field needs only enough room for one phone glyph.
4. Set that field's left-button function to **FlightStrips > VACS call next sector** and its right-button function to **FlightStrips > VACS call...**.
5. Save the layout, keeping SI and its existing bindings intact.

Dialog wording may vary with the EuroScope version; these steps have not been checked in the live host.

Validation on 19 November 2026: Win32 Release build passed and all **674 CTest tests passed**. The resolver, matching, label and click-guard tests cover the automated paths; they do not verify host integration or audio. Logs: `build/vacs-phone-local-build.log` and `build/vacs-phone-local-tests.log`.

### Live checks still required

- Confirm EuroScope selects the clicked aircraft when the VACS item is clicked through TopSky. Inspect `VACS next-sector target check` logs for the callback text and generated/fresh destinations. Exact callback text is not required for validation.
- Compare the field with SI for normal routing, early assumption, skip-sector coordination, requested/refused coordination and manual SI overrides.
- Check an aircraft still tracked after leaving your airspace: if exit time becomes `-1`, prediction is unavailable unless handoff or native coordination supplies a destination.
- Check TopSky foreign-item filtering and all minimized/extended tag modes.
- Verify the native picker and an end-to-end VACS call with two real controller clients, including ringing/audio.

## Rebuild and run tests

From a Visual Studio developer PowerShell with CMake on PATH, at the repository root:

```powershell
cmake -S euroscope-plugin -B euroscope-plugin/build -G "Visual Studio 17 2022" -A Win32
cmake --build euroscope-plugin/build --config Release --parallel 6
ctest --test-dir euroscope-plugin/build -C Release --output-on-failure -j8
```

Focused checks:

```powershell
ctest --test-dir euroscope-plugin/build -C Release --output-on-failure -R "Vacs|UserConfig|InfoPanel|WebSocketTest"
```

The native render test needs the installed EuroScope font. It writes real GDI+ renders from the production panel drawing code into `build/bin/native-preview`:

- `expanded.png` and `expanded-connected.png`: 160 x 293 px.
- `mini-header.png`: 160 x 15 px.
- `mini-ready.png`: 160 x 33 px.
- `mini-incoming.png`, `mini-outgoing.png`, `mini-connecting.png`, and `mini-connected.png`: 160 x 51 px, with caller identity above the actions.
- `expanded-incoming.png`: 160 x 293 px.

These are offscreen renders using the actual 9 pt EuroScope font, not mockups or screenshots of a live EuroScope radar screen. Text measurement tests ensure **EKDK_UC_CTR**, **EKDK_UC_CTR (2)**, and **EKCH_DEP_APP** fit expanded and mini identity rectangles in every call state without truncation, alongside **Answer** and **Reject** button checks. Longer arbitrary names still ellipsize safely; hover over the identity or actions for the complete caller and call state. They do not verify EuroScope popup placement or operating-system DPI scaling.

## Acceptance coverage

Local result on 21 October 2026: **Win32 Release build passed; all 520 CTest tests passed (0 failures)**, including full-callsign preservation through incoming, answering, connected, and outgoing states. The suite includes real binary/text loopback transport checks and real-font GDI+ rendering. The DLL's PE headers previously confirmed x86, and its direct EuroScope initialization/exit exports were present. Live host/audio checks below remain pending.

| Group | Automated coverage | Still requires a live session |
| --- | --- | --- |
| A: preferences/layout | Defaults, independent saved preferences, persistence across window/token saves, exact layout/height, existing clamp tests, shared-screen refresh callbacks, native-font rendering | Actual header dragging, off-screen recovery, reopen/minimize/close interactions, two ASR windows and supported display scaling |
| B: calling/menus | Stable target IDs, self/position/duplicate filtering, single pending gate, oldest-call queue, deduplication, answer/reject/cancel/end, active-call priority, state/call-ID validation | Native popup and mouse interaction, no-aircraft/backend-disconnected operation in the host, PDC interaction, two real controller clients |
| C: protocol/recovery | Injected transport/clock fixtures, bootstrap, existing call recovery, null-versus-omitted store display, malformed input, late replies, generation/revision guards, errors/timeouts, no automatic call retry, heartbeat, bounded backoff, shutdown | Real VACS desktop/session/store exchange, restart with an already-established call, real WebRTC state/audio and unsupported-server behavior |
| C02: framing | Real loopback WebSocket round trips verify text opt-in and binary-by-default, including binary payload bytes; existing backend tests run unchanged | Optional smoke check against the deployed FlightStrips backend |
| D: live Windows verification | Win32 Release build, plugin CTest suite and native GDI+ renders | End-to-end EuroScope/VACS call, audible ringing, device/audio ownership and live screenshots of the native picker |

Do not mark the live checks passed from the render fixtures. In particular, the picker screenshot and real call/audio verification remain pending until VACS is authenticated and two test controller clients are available.

## Environment and references

- Checkout inspected: `63e7ab07e9494821731817c2c0cfe0a09e0369e1`, reconciled with the handoff rather than assuming its older baseline.
- Build environment: Windows 11 Pro 10.0.26300, Visual Studio 2022 MSVC 19.44, Win32 Release.
- Installed EuroScope executable: 3.2.3.2; the render fixtures use its installed EuroScope font. No live EuroScope call session is claimed.
- Protocol reference: [`vacs-client-v3.0.1`](https://github.com/vacs-project/vacs/tree/vacs-client-v3.0.1), checked against the upstream remote server, protocol, call store, signaling/WebRTC listeners and store-sync source. VACS desktop version/audio behavior have not been live-tested.
- Logs from this run: `build/build.log`, `build/vacs-tests.log` and `build/test-results.log`.

## VACS 3.x protocol checks

The session snapshot contains authentication, connection and client information; calls are recovered by `remote_request_store_sync`. A full call-store response carries `incomingCalls` and an optional `callDisplay`. Live updates use `null` to leave the queue or display unchanged. Dialing uses `signaling_invite_to_call` with a `targets` array. The plugin consumes `signaling:call-invitation`, `signaling:outgoing-call`, `signaling:call-update` and per-peer WebRTC events. It does not write synthesized outgoing calls into the desktop store.

Regression fixtures cover the 3.x snapshot without legacy call fields, full/partial call-store recovery, outgoing events arriving before command replies, participant roster replacement, per-peer connection/degradation/reconnection, target rejection and scoped call errors. Live verification should include connecting the plugin during an existing call, dialing and answering in both the plugin and desktop, rejecting/cancelling calls, and a conference where one participant leaves or reconnects while another remains connected. Conference creation and participant management remain in VACS.

Validation on 15 November 2026: Win32 Release build passed, all 57 VACS-related CTest checks passed, and the full plugin suite passed all 592 tests. Build and suite output are in `build/vacs3-build-final.log` and `build/vacs3-tests.log`. Live EuroScope/VACS calling and audio remain unverified.

Review follow-up: regression coverage now includes repeated outgoing events for joined client/position/station targets, recovery of waiting calls alongside a finished-call display, and late answer replies after another call becomes active. The updated Win32 Release build passed all 62 VACS-related checks and all 597 plugin tests. Logs: `build/vacs3-review-build.log`, `build/vacs3-review-focused-tests.log`, and `build/vacs3-review-tests.log`.
No branch, commit, PR, release or deployment is created by this work.

## Browser relay verification

With the updated backend and frontend running, select this own EuroScope instance in the strip board and enable browser VACS integration. Keep the native panel's VACS switch On. Open the board on a second computer or tablet; the browser needs only its normal FlightStrips connection, not access to port 9600.

Verify dial, answer, reject, cancel and end from both native and browser controls. Changes should appear on the next browser refresh. Check multiple waiting calls, an existing desktop call on browser startup, native/browser actions competing for the same call, and reconnecting both the browser and plugin. Unload or disconnect the selected plugin and verify browser controls become unavailable without switching to another same-CID instance or the session master. Disable browser integration during a call and verify the desktop call continues. No call action should be replayed after reconnect or timeout.

These are live acceptance checks; automated relay, state-machine and transport tests do not verify real audio or a deployed multi-device session.
