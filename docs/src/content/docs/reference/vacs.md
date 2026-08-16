---
title: VACS voice control
description: Use VACS to call other controllers from Flight Strips, then learn how the connection is configured.
sidebar:
  order: 2
---

Flight Strips can use [VACS](https://github.com/vacs-project/vacs) for controller-to-controller voice calls. VACS handles the audio; Flight Strips gives you a phone button for starting and controlling calls.

## Quick user guide

### Set it up

1. Start VACS.
2. In VACS, sign in to VATSIM, connect to your controller position, and enable remote control.
3. In Flight Strips, click the **settings gear** beside the clock.
4. Select **Enable VACS voice integration**.
5. Close settings. A **phone button** appears beside the gear.

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

Flight Strips connects directly from your browser to the VACS remote-control WebSocket. The Flight Strips backend does not carry voice traffic. VACS and WebRTC handle the audio; Flight Strips sends only call-control actions such as dial, accept, reject, and end.

To build the dial list, Flight Strips compares each controller on the strip board with the clients reported by VACS. A controller is available when one of these values matches:

- the Flight Strips position and the VACS `positionId`;
- the Flight Strips callsign and the VACS `positionId`;
- the Flight Strips callsign and the VACS `displayName`; or
- the Flight Strips position and the VACS `displayName`.

Flight Strips does not use controller VATSIM CIDs for this matching. If the position or callsign names differ between the two systems, the controller appears under **Not on VACS**.

## Configuration

### Choosing the VACS computer

Flight Strips always connects to port **9600** at `/ws`. It chooses the hostname in this order:

1. The **VACS machine address** entered in Flight Strips settings.
2. The local IP reported for the associated EuroScope computer.
3. `localhost` when neither of the above is available.

Normally, leave **VACS machine address** empty. Flight Strips then uses the EuroScope computer reported for your position, or the same computer as the browser when no address was reported.

Enter an address only when you need to override that choice. Use an IP address or hostname, without `ws://`, a port, or a path.

Examples:

| Setting | Connection |
| --- | --- |
| Empty, with EuroScope IP `192.168.1.10` | `ws://192.168.1.10:9600/ws` |
| Empty, without a reported EuroScope IP | `ws://localhost:9600/ws` |
| `vacs-pc.local` | `ws://vacs-pc.local:9600/ws` |

VACS remote control must be enabled on the selected computer, and the browser must be able to reach TCP port 9600. Saving another address reconnects the integration immediately when it is enabled.

### Saved browser settings

The settings are stored in the current browser profile. They do not automatically follow you to another browser or device.

| Setting | Browser storage key | Default |
| --- | --- | --- |
| Integration enabled | `flightstrips.vacs.enabled` | Off |
| VACS machine address | `flightstrips.vacs.host` | Empty |

## Troubleshooting

### The phone button is grey

Hover over it and follow the displayed instruction:

| Tooltip | What to check |
| --- | --- |
| VACS not running, or remote control not enabled | Start VACS and enable remote control in VACS settings |
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

The browser integration is implemented in `frontend/src/vacs/`, with the phone and dial controls in `frontend/src/components/commandbar/`. URL selection and hostname normalisation are in `frontend/src/lib/vacs-settings.ts`.

The client uses VACS invoke commands including `signaling_start_call`, `signaling_accept_call`, and `signaling_end_call`. Rejecting a waiting call also uses `signaling_end_call`. Call-target enum tags are lowercase, and the outgoing call source contains `clientId`, `positionId`, and `stationId`.
