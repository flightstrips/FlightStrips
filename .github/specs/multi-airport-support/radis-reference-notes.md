# RADIS Figma reference notes

Additional implementation evidence was reviewed on 2026-10-30 in
[PR #958](https://github.com/flightstrips/FlightStrips/pull/958). See the
[prototype review](radis-prototype-review.md) for concrete interactions and
integration implications. Its feed-only implementation is a demo shortcut;
the agreed target still updates ES through each initiating controller's plugin.

## Provenance and limits

Inspected on 2026-10-29 through the connected Figma bridge, using the user's
current selection:

- File name: `Project FS / Flightstrps by VATSCA`.
- Page: `Page 1` (`0:1`).
- Selected section: `FlightStrips for RADIS/EKBI` (`2676:1558`).
- Bridge file identifier: `unsaved-mvu1kf4g-ogmcjk1b`.

The bridge exposed a local/unsaved identifier, not a durable Figma file URL.
The standard Figma session endpoint rejected that identifier; inspection used
the connected bridge's read-only selection, metadata, screenshot, and prototype
tools. A shareable file URL should be recorded when available.

These observations describe the selected design, not a complete specification.
Visible sample values, colors, and copied flight-plan content do not establish
production airport configuration or enabled integrations. No Figma changes were
made.

## Observed structure

| Element | Node | Observed content |
| --- | --- | --- |
| Main board | `3313:371` (`Frame 43`) | ACTIVE on the left, PASSIVE on the right, RUNWAY below ACTIVE; a command bar along the bottom. |
| Command bar | `2676:350` | Airport indicator, departure/arrival runway controls, QNH/weather/time information, FIND, NEW, and X. The sample airport is EKYT despite the section's EKBI name. |
| Arrival strip example | `2676:401` | Callsign, origin/destination, route/point text, aircraft type, wake category, squawk, runway, timing fields, stand, date, and an airport/direction marker. |
| Departure strip with annotations | `2676:3041` | Additional RFL field, departure marker, and annotations for opening altitude and numeric-input menus. Its screenshot was inspected. |
| Local VFR examples | `2676:1183`, `2676:1246` | Same origin and destination, V flight-rules marker, and altitude/other local-flight fields. These establish a design example, not complete circuit handling. |
| Strip/menu annotations | `2677:3161`, `2677:3164`, `2677:3169` | OPEN ROUTE MENU, OPEN RWY MENU, and OPEN FPL MENU labels associated with example strip cells. |
| Runway menu | `2677:3172` | Runway selections; actual values need airport configuration. |
| Route/heading menu | `2677:3254` | Heading-like numeric choices, POINT, OK, and ESC. |
| Altitude menu | `2677:3449` | Altitude/flight-level choices, OK, and ESC. |
| Numeric menu | `2677:3456` | Numeric input controls. |
| Additional strip list | `3368:371` | Compact strip and SORT/DISMISS controls, with a counter. The inner layer is named STARTUP; that alone does not establish the list's operational purpose. Its screenshot was inspected. |
| Flight-plan form | `2676:2832` | Departure/arrival field groups including route, runway, altitude, heading, stand, remarks, and timing fields. Which are editable and which synchronize remains open. |
| Clearance form | `3373:662` | ATC CLEARANCE WINDOW with CLD and ESC actions. The section contains sample content also seen in other airport designs; exact RADIS behavior requires confirmation. |
| Further FPL examples | `3373:371` | Flight-plan view and supporting menus. Text node `3373:660` proposes REA updating EOBT and REA to the current time; applicability to RADIS is unconfirmed. |
| Airport diagrams | `3330:387`, `3330:427` | RWY Dialogue and Point Dialogue examples. |

## Prototype inspection

The prototype scan covered 2,120 nodes without truncation. There are no flow
starting points. Inspected interactions are empty click reactions or CLOSE
actions; no navigation/transition graph establishes the operational sequence.
The prototype therefore does not answer board-state transitions, permission
rules, or EuroScope side effects.

## Behavior confirmed by the user after inspection

- One shared board per airport, with no ownership. Anyone working that airport
  can edit and move strips; placement and order are shared.
- Bay movements update EuroScope. Relevant field edits, including squawk and
  altitude, also update EuroScope for now; this may change later.
- Relevant flights appear automatically.
- NEW creates a shared airport-local strip only for now. Actual ES flight-plan
  creation is deferred.
- FIND searches only connected pilots whose flight origin or destination matches
  the selected airport. Prefiles are excluded.
- Include all current prototype features/menus, notes, de-icing marks, REA/ATA
  timestamp controls, and restoration of manually removed strips.
- Use the existing EKCH move/error/reconciliation behavior.
- PDC and AMAN are disabled at EKBI/EKYT, with no scheduled activation.

These are user-confirmed requirements, rather than behavior inferred from the
Figma prototype. See the main discovery document for coverage/mastership rules.

## Remaining questions

- What EuroScope state/clearance changes correspond to each bay movement,
  including reverse movements and differences between arrivals and departures?
- Which flights qualify for automatic inclusion, and in which initial bay?
- Which automatic movement, ordering, removal, and recovery rules exist?
- What are the detailed restoration limits and shared visibility rules? NEW,
  FIND population, inclusion of current prototype features, and restoration are
  settled. The source review records which controls are implemented versus
  placeholders; the source review records DISMISS as closing its dialog.
- What is the complete editable-field list and each field's EuroScope mapping?
- How are arrival/departure/local-flight strip types selected or changed?
- What are the actual field/ES mappings for the included current prototype
  controls? NEW does not create an ES flight plan; PDC/AMAN stay disabled.
- What behavior differs between EKBI and EKYT beyond configured values?

Return to the [multi-airport discovery document](../multi-airport-support.md).
