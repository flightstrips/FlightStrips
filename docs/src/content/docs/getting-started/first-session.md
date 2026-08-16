---
title: First-session checklist
description: Verify the connection, layout and shared state before operating a strip.
---

Use this checklist after the plugin panel shows **Connected** and the web app has opened.

## 1. Confirm that the browser found EuroScope

The web app may briefly show **Connecting to FlightStrips**, followed by **Waiting for ES connection** while the server associates and synchronizes the matching plugin.

The strip board should then replace the waiting screen. If it does not:

- confirm that the plugin panel still says **Connected**;
- confirm that the plugin and web app use the same VATSIM account;
- check your EuroScope primary frequency and callsign;
- allow the plugin to finish its first session synchronization.

Flight Strips does not provide a manual session selector in this flow. Fix the EuroScope side when the wrong or no session is associated.

## 2. Check your position and mode

Look at the station box on the left of the command bar. It shows the current scope and your EuroScope primary frequency.

- A normal writable position uses the green station box.
- An observer uses a yellow station box, includes **(OBS)** after the frequency and cannot send operational strip actions.
- An observer frequency that has no available layout shows **INVALID** and an explanatory screen.

If the position is wrong, correct the primary frequency in EuroScope. Do not compensate by choosing another layout and then work traffic as the wrong position.

## 3. Verify the controller layout

Flight Strips receives a recommended layout for your position. The available EKCH layouts include clearance delivery, sequence planning, apron departure, apron arrival, combined apron, ground/tower and tower east/west views.

If the expected layout is not displayed, select the station box, choose the correct scope and select **OK**. Treat this as a display choice only; it does not change your EuroScope position.

## 4. Verify runway information

The command bar displays the first configured departure and arrival runway. A runway value uses an alert background when Flight Strips has detected a mismatch with the runway configuration reported through EuroScope.

Resolve a mismatch before relying on runway-dependent layouts, routes or validations. The runway-pair buttons also expose the shared **OPEN**, **LOW VIS** and **CLOSED** status used by the board; change them only under the applicable procedure.

## 5. Read the board before clicking

Check which bays are visible and whether the strips match the traffic you expect. Then learn the two interactions that affect almost every workflow:

1. [Strip anatomy and types](/concepts/strip-anatomy/) explains what each box means in each implemented strip presentation.
2. [Ownership and handoffs](/concepts/ownership/) explains when a strip is yours, offered to you or waiting for coordination.

The board is shared. Give a submitted action time to return as updated server state before repeating it.

## 6. Know when Flight Strips will stop

Configured automatic behaviour continues only while its conditions are valid. When the system needs controller judgement, it uses a validation status, changed colour, split ownership box or blinking field instead of silently deciding for you.

Read [Validation status](/concepts/validation-status/) before acknowledging alerts, and [Pre-departure clearance](/concepts/pre-departure-clearance/) before handling PDC requests.

You are now ready to open the procedure for your position or airport.
