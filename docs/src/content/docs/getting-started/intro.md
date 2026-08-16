---
title: Start here
description: What Flight Strips is, what it does for you, and what you remain responsible for.
---

Flight Strips is a shared electronic strip board for controllers. It combines live EuroScope data with the workflow recorded on the strip board, so connected controllers work from the same strips, ownership and pending coordination.

You do not use Flight Strips instead of EuroScope. You use the two together:

- **EuroScope** supplies your controller identity, position and live flight data through the Flight Strips plugin.
- **The web app** is where you read and operate the strip board.
- **The Flight Strips server** connects both sides, maintains the shared session and sends each change to the other connected clients.

## What you must do

Flight Strips supports the workflow; it does not make operational decisions for you. You must:

- connect EuroScope on the correct callsign and primary frequency;
- sign into the plugin and web app with the same VATSIM account;
- check that the web app opened the correct controller layout and runway setup;
- record clearances, transfers and other controller decisions when they happen;
- respond when a strip asks for judgement or acknowledgement.

## What happens automatically

Once EuroScope and the web app are connected, Flight Strips:

- associates the browser with your EuroScope session using your VATSIM CID;
- synchronizes the shared strip and controller state;
- keeps the board layout and expected controller route aligned with the connected positions;
- sends supported strip edits back to the EuroScope client associated with you;
- performs only the automatic transitions and validations documented for the relevant workflow.

Automatic does not mean invisible. Colours, split ownership boxes, blinking fields and validation messages show when the system has changed state or needs a controller to act.

## Before working traffic

Follow this short route through the documentation:

1. Read [How the system fits together](/getting-started/features/) so you know which component is responsible for what.
2. Use [Connect EuroScope](/getting-started/es-plugin/) to load, sign in and verify the plugin.
3. Complete the [First-session checklist](/getting-started/first-session/) before operating a strip.
4. Learn [Strip anatomy and types](/concepts/strip-anatomy/), then [Ownership and handoffs](/concepts/ownership/).

After that, use the procedure for your controller position or airport. Optional integrations such as [VACS voice control](/reference/vacs/) are not required to operate the strip board.
