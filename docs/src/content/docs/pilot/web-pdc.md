---
title: Web PDC
description: Submit a pre-departure clearance from the browser when your simulator has no datalink PDC.
sidebar:
  order: 2
---

Use **Web PDC** when your aircraft or client **does not** support sending a PDC/DCL request to FlightStrips over datalink. You complete a short form; when ATC issues your clearance, it appears on the same page.

## Open the page

1. Log in to FlightStrips with your **VATSIM** account.
2. Go to **`/pdc`**.

## Submit a request

Fill in the fields accurately:

| Field | What to enter |
| ----- | ------------- |
| **Callsign** | Your active VATSIM callsign (as filed). |
| **ATIS information letter** | The current ATIS letter you have copied. |
| **Stand number** | Your parking position as shown on the chart. |
| **Remarks** | Optional short notes for ATC. |


## While you wait

After a successful submit, the **Your request** panel shows that the request was received and may show a reference id. The page **polls automatically** for updates. Keep the tab open; you do not need to resubmit unless you intentionally start a new request.

Typical states:

- **Awaiting clearance** — ATC has not finished processing yet. Stay on frequency.
- **Faults / coordination** — The strip may need controller attention to your flight plan. The web UI shows **generic** guidance (stay on frequency; ATC will coordinate). It does **not** expose internal validation messages. If you are unsure, call ATC.
- **Cleared** — Your clearance text is shown. Read it carefully.
- **Error** — Something went wrong server-side. You may submit again or use voice.

Avoid hammering **Request clearance**: the API may rate-limit repeated posts (short cooldown between submissions).

## Reading the clearance

When status is **Cleared**:

- Use **Simple** for a quick field-by-field view with icons (when parsing is available).
- Use **Raw** for the exact datalink-style text — this is the authoritative string to copy or cross-check.
- **Copy full message** copies the complete clearance text to the clipboard.

Always cross-check against your route, SID, runway, squawk, and ATIS. “Simple” is a convenience; **Raw** matches what was generated for delivery.

## Confirm receipt

When you have read and understood the clearance, tick the confirmation and submit **Confirm receipt** so ATC knows you have acknowledged the PDC. This is required for the Web PDC workflow where datalink ack is not available.

## New request or lost session

- **New request** clears the stored request and lets you file again (for example after a callsign or stand change).
- If the server returns **not found** or **gone**, your saved request id is cleared; submit a fresh request.

## Privacy and scope

Only submit data you would send to ATC for a departure clearance. If your division publishes additional rules (event-only PDC, specific airports), follow those as well.

Related for controllers: [Pre-departure clearance (controllers)](/ekch/pre-departure-clearance/).
