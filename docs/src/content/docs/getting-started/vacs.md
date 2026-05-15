---
title: VACS voice integration
description: Connect FlightStrips to VACS for controller-to-controller voice calls from the command bar.
sidebar:
  order: 4
---

FlightStrips can integrate with **[VACS](https://github.com/vacs-project/vacs)** (VATSIM ATC Communication System) so you can place and receive voice calls between controllers without leaving the strip board. The integration is **frontend-only**: FlightStrips talks to VACS over a WebSocket on port **9600**; no voice traffic passes through the FlightStrips backend.

## Prerequisites

Before enabling the integration, make sure VACS is available where FlightStrips can reach it:

1. **VACS is running** with remote control enabled, reachable at `ws://localhost:9600/ws` on this machine, or at `ws://<host>:9600/ws` on another PC on your network (see [remote VACS host](#remote-vacs-host) below).
2. **Remote control is enabled** in VACS settings. Without it, FlightStrips cannot connect and the phone button stays disabled.
3. **You are signed in to VATSIM in VACS** so signaling and WebRTC can authenticate.
4. **VACS is connected to a signaling position** (your controller position is known to VACS). If VACS reports an ambiguous position, resolve it in VACS before dialing.

FlightStrips does not install or bundle VACS; you run VACS separately, same as EuroScope or your browser.

## Enable the integration

The feature is **off by default**. To turn it on:

1. Open the **settings** control in the command bar (gear icon next to the clock).
2. Check **Enable VACS voice integration**.
3. Optionally enter a **VACS machine address** (IP or hostname) if VACS runs on another computer; leave the field empty to use this machine (`localhost`).

Preferences are stored in your browser:

| Setting | `localStorage` key |
| --- | --- |
| Integration on/off | `flightstrips.vacs.enabled` |
| Remote host (optional) | `flightstrips.vacs.host` |

Reloading the page keeps your choices; other browsers or profiles start with the feature disabled and no custom host.

When enabled, a **phone** button appears in the command bar between the settings gear and the clock.

### Remote VACS host

Use the address field when FlightStrips runs in a browser on one PC but VACS runs on another (for example EuroScope and VACS on a gaming PC, FlightStrips on a tablet). Enter only the **IP or hostname** — not a full `ws://` URL. The port is always **9600**.

- Empty field → `ws://localhost:9600/ws`
- `192.168.1.10` → `ws://192.168.1.10:9600/ws`

Remote control must be enabled on that VACS instance, and your network must allow TCP to port 9600. Changing the address reconnects immediately when the integration is enabled.

## Phone button states

Hover the button for a short explanation of the current state.

| Appearance | Meaning |
| --- | --- |
| Grayed out | VACS unavailable, not authenticated, disconnected from signaling, or position ambiguous |
| Normal (bay color) | Idle — ready to dial |
| Orange, pulsing | Incoming call **or** outgoing call ringing |
| Green | Connected — on an active call |
| Brief red flash | Call is ending |

If more than one call is waiting, a small badge on the button shows the count (incoming only).

## Placing a call

1. While idle, **click** the phone button.
2. The **dial** dialog lists other controllers on the FlightStrips board:
   - **Available** — matched to a VACS client; use **Dial**.
   - **Not on VACS** — on the board but no matching VACS client (shown dimmed).
3. With more than ten other controllers, a search box filters by callsign or position.

While the callee’s phone is ringing, your button uses the same **orange pulsing** style as an incoming call. **Click** or **right-click** the button to cancel the outgoing attempt.

Matching uses **position and callsign** from FlightStrips against VACS `positionId` and `displayName`. FlightStrips does not send VATSIM CIDs for controller matching; both systems must agree on position/callsign naming for a peer to appear as dialable.

## Receiving a call

When a call arrives:

- The button turns **orange** and pulses.
- **Click** to accept (oldest waiting call if several are queued).
- **Right-click** to reject.

## During and after a call

- **Connected:** the button is **green**. **Click** to end the call (brief red flash while hang-up is processed).
- **Right-click** while connected also ends the call.

Audio is handled entirely by VACS and WebRTC; FlightStrips only drives signaling (start, accept, reject, end) via the VACS remote-control API.

## Troubleshooting

### Phone button is grayed out

| Tooltip message | What to do |
| --- | --- |
| VACS not running, or remote control not enabled | Start VACS; enable remote control in VACS settings |
| Sign in to VATSIM in VACS | Complete VATSIM login inside VACS |
| VACS is not connected to a signaling position | Connect VACS to your position; check network and VATSIM session |
| Your VACS position is ambiguous | Resolve position selection in VACS |

### Controller appears under “Not on VACS”

They are on the FlightStrips board but FlightStrips could not match them to a VACS `ClientInfo` entry. Typical causes:

- They are not running VACS or are not on the same VACS session.
- Their VACS `positionId` / `displayName` does not match their FlightStrips callsign or position string.

### Dial fails with an error toast

Check the browser console for VACS invoke errors. Common issues include invalid call targets or missing `CallSource` data when your own VACS client is not yet in the session snapshot. Retry after VACS shows you as connected.

### Integration enabled but no phone button

Confirm the checkbox in settings is on and refresh the page. The button is not rendered when the integration is disabled.

## For developers

Implementation lives under `frontend/src/vacs/` (`VacsClient`, subscriptions, matching) and `frontend/src/components/commandbar/` (`VACSBTN`, `VacsDialModal`). Tests: `frontend/src/vacs/vacs-client.test.ts`, `VACSBTN.test.tsx`.

- WebSocket URL: `buildVacsWsUrl()` in `frontend/src/lib/vacs-settings.ts` (localhost when host is empty; port 9600 fixed).
- `VacsClient.updateUrl()` reconnects when the host setting changes.
- Signaling uses VACS invoke commands such as `signaling_start_call`, `signaling_accept_call`, and `signaling_end_call`.
- Call targets use lowercase enum tags (`client`, `position`, `station`); `source` is a `CallSource` object (`clientId`, `positionId`, `stationId`), not a plain position string.

To work on the UI locally, run VACS with remote control on port 9600 and enable the integration in FlightStrips settings.
