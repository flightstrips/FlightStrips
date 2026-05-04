---
title: Clearance Delivery
description: How to issue departure clearance from the strip view.
sidebar:
  order: 2
---

![Clearance Delivery view](../../../assets/ekch-clr-del-view.jpg)

## Issue a clearance

Click the **Destination / Stand** column on the strip to open the flight plan editor.

![Open flight plan](../../../assets/ekch-clr-del-open-fpl.jpg)

The flight plan has three field types:

- **Disabled** — read-only; pilots must refile to change.
- **Dropdown** — valid options for this strip.
- **Free-text** — manually editable, e.g. `ROUTE`.

![Flight plan editor](../../../assets/ekch-clr-del-fpl.jpg)
![SID selector](../../../assets/ekch-clr-del-sid-selector.jpg)
![Runway selector](../../../assets/ekch-clr-del-rwy-selector.jpg)
![Altitude selector](../../../assets/ekch-clr-del-alt-selector.jpg)

## Datalink / Pre-departure clearance (PDC)

When a pilot submits a PDC/DCL request, the strip changes colour to indicate its state:

- **Purple** — PDC received and valid; no controller action required.
- **Yellow** — PDC contains an issue or manual remarks; controller action required.

![PDC strip example](../../../assets/ekch-clr-del-dlc.jpg)

For a yellow strip, click **Destination / Stand** to open the flight plan with an additional **REVERT TO VOICE** option — use it if further coordination with the pilot is needed.

![PDC flight plan view](../../../assets/ekch-clr-del-fpl-dlc.jpg)

For full PDC strip states and validation details, see [Pre-departure clearance](/concepts/pre-departure-clearance/) and [Validation status](/procedures/validation-status/).
