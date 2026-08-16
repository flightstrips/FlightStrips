---
title: How the system fits together
description: How the web app, server and EuroScope plugin form one shared strip system.
---

Flight Strips has three connected parts. A new user needs all three for an operational, writable strip board.

| Part | What it does | What you do |
| --- | --- | --- |
| **EuroScope plugin** | Reports your airport, callsign, primary frequency, connection mode and live EuroScope state. It also applies supported web-app changes to your EuroScope client. | Load the plugin, sign in and connect EuroScope on the correct position. |
| **Flight Strips server** | Places controllers and strips in the same airport session, stores the shared state and broadcasts updates. | Nothing directly. Wait for a submitted action to appear on the shared board before repeating it. |
| **Web app** | Displays the controller layout and turns strip actions into server events. | Sign in with the same VATSIM account as the plugin, verify the layout, then operate the strips. |

## Why both sign-ins matter

The plugin and browser authenticate separately. The server matches them by the VATSIM CID in their access tokens.

If the web app is signed in but the matching EuroScope plugin is not online and synchronized, the app shows **Waiting for ES connection**. It does not ask you to choose a session manually. When the matching plugin completes its first synchronization, the server associates the browser with that controller, airport and session.

A EuroScope observer is associated in the same way, but the web app is read-only and labels the position **(OBS)**. Observer mode suppresses operational changes.

## What is shared

The server snapshot sent to the web app includes the strips, controllers, runway setup, layout, ownership routes and feature state for the session. Later events update that state for connected clients.

This means that ownership, strip order and pending coordination are not private annotations in your browser. They are shared operational state. If another controller transfers or updates a strip, your board receives the result from the server.

## What comes from EuroScope

The plugin implements events for flight-plan and surveillance-related values including aircraft state, position, ground state, runway, squawk, cleared altitude, heading, controller tracking and coordination. Which fields matter on screen depends on the strip type and controller layout.

The plugin only connects to the Flight Strips server when it has:

- an authenticated user;
- a Direct, Sweatbox or Playback EuroScope connection;
- a recognized airport;
- a usable primary frequency.

For normal use, this is why you should fix the EuroScope connection or primary frequency instead of trying to repair the browser session.

## What Flight Strips adds

The shared system adds the workflow that is not represented by a single EuroScope tag: strip bays and ordering, ownership and handoffs, controller layouts, validation prompts, PDC state and configured automatic transitions.

These rules are airport- and state-dependent. Learn the common behaviour in [Concepts](/concepts/), then use the procedure page for your position. Do not assume that a movement which is automatic in one bay is automatic everywhere.

## Optional systems

Optional integrations extend the board but are not part of its connection chain. VACS, for example, adds controller-to-controller call controls to the command bar; the strip board continues to work without it. See [VACS voice control](/reference/vacs/) only if your operation uses VACS.
