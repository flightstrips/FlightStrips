---
title: Pre-departure clearance (PDC)
description: How EKCH controllers see PDC requests on strips, handle faults, issue clearances, and Web PDC delivery.
sidebar:
  order: 2
---

This page describes how **pre-departure clearance** appears in FlightStrips for controllers.

For issuing clearances from the strip and flight plan editor, see also [Clearance delivery](/ekch/clerance-delivery/).

## Strip colours and PDC state

Strips expose a **PDC state** alongside the usual bay layout. Backgrounds help you see at a glance whether a pilot is waiting, needs attention, or is already cleared via PDC:

## Datalink Clerance

When the pilot’s client sends a PDC/DCL-style request into FlightStrips, the strip updates as appropriate. Open the flight plan from the strip (**Destination/Stand** column) to review fields, fix issues, and issue clearance using your normal [Clearance delivery](/ekch/clerance-delivery/) workflow.

### Faults (yellow)

If the strip is **REQUESTED_WITH_FAULTS**, treat it as **manual coordination**: the flight plan may be inconsistent with the sector rules, missing data, or otherwise not safe to auto-clear.

- Open the flight plan, correct what you can, or coordinate with the pilot by **voice**.
- The flight plan UI can expose **Revert to voice** when datalink delivery is not appropriate so you can complete the clearance on frequency.
- Do not rely on pilots seeing internal validation strings; external/Web surfaces show **generic** text to pilots.

### Cleared (navy)

After you issue clearance through the system, the strip moves to **CLEARED** for PDC. Continue to use normal handoff and frequency discipline.

## Web PDC (no datalink)

Some pilots **cannot** send PDC from the simulator. They use the **`/pdc`** Web PDC page (authenticated, same VATSIM login as the app):

- They POST a **JSON request** to the backend (`/api/pdc/request`) with callsign, ATIS letter, stand, and optional remarks.
- The backend records a **web PDC request** and associates it with the strip/session so your operational picture stays consistent.
- When you **issue clearance** in the PDC service, delivery can target **Web** when CPDLC/Hoppie is not used, so the pilot sees the text on the web page.
- Pilots are asked to **acknowledge receipt** via **`/api/pdc/acknowledge`** so you have a positive confirmation where datalink ack does not exist.

Operationally: if you see a web-side request for an aircraft you are clearing, ensure the clearance you issue matches what you would send on datalink; the pilot will read it on the Web PDC UI (Simple + Raw views).

## Coordination checklist

1. **Purple** — PDC requested; review plan and issue if valid.
2. **Yellow** — Faults; open plan, fix or coordinate, then issue or revert to voice.
3. **Navy** — Cleared via PDC; monitor as usual.
