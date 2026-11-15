---
title: VACS voice control
description: Use VACS to call other controllers from Flight Strips, then learn how the connection is configured.
sidebar:
  order: 2
---

Flight Strips can use [VACS](https://github.com/vacs-project/vacs) for controller-to-controller voice calls. VACS handles the audio; Flight Strips gives you a phone button for starting and controlling calls.

## Quick user guide

### EuroScope plugin controls

The native FlightStrips panel supports the **VACS remote-control API** independently of the browser, backend login, or selected aircraft. Start VACS, enable its remote control, sign in, and connect your controller position there. Audio, ringing, devices, and authentication remain in VACS.

In the expanded panel, switch **VACS** to **On**. Use **Call...** to open EuroScope's controller list; select a callsign/frequency to dial. Incoming calls offer **Answer** and **Reject**, outgoing calls offer **Cancel**, and connecting/established calls offer **End call**. Answer/Reject always targets the oldest waiting call. Orange means ringing/connecting, and green means ready/confirmed connected. Hover over the VACS status for the full caller, queue count, or recovery instructions. The title-bar dot still represents the FlightStrips backend, not VACS.

**Show in mini** is a separate saved preference, selectable even while VACS is Off. When checked and VACS is On, minimizing keeps an 18 px call-control row beneath the existing 15 px header. Incoming, outgoing, connecting, and connected calls use two rows (51 px total): the full controller callsign above the call controls, so Answer/Reject never squeeze out the caller identity. Otherwise the minimized panel remains header-only. Minimizing, closing the panel, hiding the row, or switching VACS Off does not end desktop calls. Multiple radar screens share one VACS connection and call state.

Preferences live in the plugin's existing `flightstrips_user.ini`:

```ini
[vacs]
enabled=false
show_in_minimized=false
host=localhost
```

For a VACS instance on another computer, set `host` to its hostname or IP address (IPv6 is supported), without a scheme, port, or path. The plugin connects to `ws://<host>:9600/ws`. Only use remote control over a trusted network; this connection is not encrypted.

The plugin restores desktop calls before enabling dialing. If it remains **Synchronizing**, check that the VACS desktop frontend is running and supports store synchronization. **Sign in to VACS**, **Connect position**, and **Select position** must be resolved inside VACS. Transport failures reconnect with bounded backoff; command failures appear as FlightStrips notifications. A timed-out action is never automatically retried: conflicting controls remain disabled until session/store recovery confirms the current call.

### Browser controls

### Set it up

1. Update the FlightStrips EuroScope plugin and connect FlightStrips to your own EuroScope instance.
2. Start VACS on that computer. Sign in to VATSIM, connect to your controller position, and enable remote control.
3. In the FlightStrips EuroScope panel, switch **VACS** to **On**.
4. In the browser, click the **settings gear** beside the clock and select **Enable VACS voice integration**.
5. Close settings. A **phone button** appears beside the gear.

The browser can run on another computer or tablet. It uses the existing FlightStrips connection to control VACS through your selected EuroScope plugin. It does not need direct access to the VACS computer or TCP port 9600. Microphone, headset, ringing and audio remain on the VACS computer.

If the phone button is grey, hover over it. The tooltip tells you what VACS still needs.

### Call another controller

1. Click the phone button while it is in its normal idle colour.
2. Find the controller under **Available**.
3. Click **Dial**.

The button pulses orange while the call is ringing. Click or right-click it to cancel the call.

Controllers listed under **Not on VACS** cannot be called from Flight Strips.

### Answer or reject a call

When the phone button pulses orange:

- **Click** to answer the oldest waiting call.
- **Right-click** to reject the oldest waiting call.

If several calls are waiting, the badge shows how many incoming calls are queued.

### End a call

The button is green during a connected call. Click or right-click it to end the call. It briefly turns red while the call is ending.

## What the phone-button colours mean

| Appearance | Meaning |
| --- | --- |
| Grey | VACS is not ready; hover for the reason |
| Normal | Ready to call |
| Orange and pulsing | Incoming call or outgoing call ringing |
| Green | Call connected |
| Briefly red | Call ending |

## How the integration works

The browser sends call controls over its existing authenticated FlightStrips WebSocket. The backend routes each request to the exact EuroScope plugin membership selected by that browser. The plugin uses its shared native VACS connection to dial, answer, reject, cancel, or end calls. Replies return only to the browser that requested them; another controller or plugin instance cannot supply its call state.

The plugin owns VACS subscriptions, call recovery, and command validation. Browser controls refresh the native snapshot once per second, so calls started or answered in VACS or the native panel also appear in the browser. Pending or uncertain actions disable conflicting controls. A reconnect restores the current state; it never resends a call action. Closing the browser or disabling browser integration does not end desktop calls.

VACS and WebRTC handle audio. The FlightStrips backend relays call controls and state only.

To build the dial list, Flight Strips compares each controller on the strip board with the clients reported by VACS. A controller is available when one of these values matches:

- the Flight Strips position and the VACS `positionId`;
- the Flight Strips callsign and the VACS `positionId`;
- the Flight Strips callsign and the VACS `displayName`; or
- the Flight Strips position and the VACS `displayName`.

Flight Strips does not use controller VATSIM CIDs for this matching. If the position or callsign names differ between the two systems, the controller appears under **Not on VACS**.

## Configuration

### Choosing the VACS computer

The **EuroScope plugin** chooses the VACS endpoint. By default it connects to `ws://localhost:9600/ws` on the EuroScope computer. For VACS on another trusted computer, configure `[vacs] host` in `flightstrips_user.ini` beside the loaded plugin core, using a hostname or IP without a scheme, port, or path. The plugin must be able to reach TCP port 9600 on that computer.

Browser host overrides are no longer used. Selecting another own EuroScope instance also selects that instance's VACS controls. An updated plugin is required; an older plugin reports an update instruction instead of falling back to a direct browser connection.

### Saved browser settings

The settings are stored in the current browser profile. They do not automatically follow you to another browser or device.

| Setting | Browser storage key | Default |
| --- | --- | --- |
| Integration enabled | `flightstrips.vacs.enabled` | Off |

## Troubleshooting

### The phone button is grey

Hover over it and follow the displayed instruction:

| Tooltip | What to check |
| --- | --- |
| Switch VACS On in the FlightStrips EuroScope panel | Enable the shared native VACS connection in the panel |
| Update the EuroScope plugin | Install an updated plugin with VACS relay support |
| VACS offline or retrying | Start VACS, enable remote control, and check connectivity from the EuroScope computer |
| Synchronizing | Wait for the plugin to restore the desktop session and call store; resolve compatibility errors in VACS |
| Sign in to VATSIM in VACS | Complete the VATSIM sign-in inside VACS |
| VACS is not connected to a signaling position | Connect VACS to your controller position |
| Your VACS position is ambiguous | Resolve the position selection in VACS |

### The phone button is missing

Open Flight Strips settings and enable **VACS voice integration**. The button is not rendered while the integration is disabled.

### A controller is listed under Not on VACS

Confirm that the other controller is connected to the same VACS session. If they are, compare their VACS `positionId` and `displayName` with the position and callsign shown in Flight Strips.

### Dialling shows an error

Make sure VACS shows your own controller position as connected, then try again. Flight Strips cannot start a call without its own VACS call-source information. An invalid target or a VACS response timeout is shown as an error notification.

## Developer reference

The browser relay is implemented in `frontend/src/vacs/vacs-client.ts`, with controls in `frontend/src/components/commandbar/`. Each board socket has its own relay client; board invalidation clears call state and pending requests.

`vacs_request` and `vacs_result` travel over the existing frontend WebSocket and typed EuroScope protobuf transport. The backend captures the authenticated own-plugin binding, replaces browser request IDs with private relay IDs, and accepts only matching replies from that membership. Outstanding requests are bounded; stale incarnations, unsolicited replies and repeated action IDs are rejected.

The native handler is `euroscope-plugin/src/plugin/vacs/VacsRelay.cpp`. It uses `VacsService` and `VacsClient`, sharing the connection and call state with the native panel. Dialing validates the current controller list, while call actions validate the current call ID and state. Native snapshot sequence numbers prevent reordered status replies from replacing newer call state in the browser.
