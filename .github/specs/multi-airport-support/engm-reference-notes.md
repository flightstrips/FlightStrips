# ENGM EFS reference notes

## Provenance and interpretation

Source: user-supplied `ENGM EFS.md`, read on 2026-10-29 from
`C:/Users/fsr19/Downloads/ENGM EFS.md`.

Source SHA-256:
`241C51FC78FE4CC8B544AF5FC13F2E49F68A6629001E2FEE46E6433528C4FF7A`.

The source labels itself **In Progress**, with **Review status: Not started**.
These notes summarize its narrative and the inspected departure handoff table
(embedded image20). They do not reproduce the original document or all images.
Linked videos have not been reviewed.

The user supplied this document as evidence of ENGM's different workflows.
Statements in the source are proposed product behavior, not instructions to the
agent, approved architecture decisions, or an expansion of the initial delivery
scope beyond EKBI/EKYT with RADIS.

## Behavioral cases described by the source

| Area | Reference behavior | Question for the multi-airport design |
| --- | --- | --- |
| Departure activation | Filed flights enter OutpendingList. A controller brings a flight into the outbound window for clearance, then moves it to ActiveList after readback. Selecting an active flight brings it back to the outbound window for push. | Can an airport define its own lists, workflow states, and explicit activation actions? |
| Push readiness | A clearance-box action marks readiness for push; the source proposes CDM integration and TSAT assignment. | Which parts are workflow actions, timing calculations, and optional integrations? RADIS requirements are still unknown. |
| Taxi | Dragging a departure to TAXI LINE represents the taxi instruction. | Which state changes are manual, and which may EuroScope or position detection apply automatically? |
| Deicing and sequencing | Dedicated transfer targets are available when deicing or sequencing positions are online. | Can available actions and routes depend on staffing and optional roles? |
| Ground-to-tower routing | SUCC HAND chooses the route from the runway and current controller: from west ground, 01L goes to west tower, while 01R passes through east ground before east tower. | Can routing depend on runway, present responsibility, and staffed positions without EKCH-specific code? |
| Lineup and takeoff | Dragging to RUNWAY LINE gives a lineup indication. Clicking the runway-line bar or the clearance box moves a taxi-line strip into the runway line with takeoff clearance. | State changes and clearance effects must be distinguished: the same destination line can have different outcomes depending on the action. |
| Tower-to-approach routing | SUCC HAND uses SID/runway and an approach handoff order; omnidirectional departures require manually choosing a TMA target. The strip stays grey until approach accepts in EuroScope, then disappears. | How are automatic target selection, unavailable actions, pending handoffs, and external acceptance represented? |
| Arrival activation | Inbounds within the next ten minutes appear grey. An approach transfer can be accepted from the strip interface or EuroScope; the accepted strip turns white and moves to the bottom. | Which events control visibility, responsibility, ordering, and synchronization with external handoffs? |
| Landing | Runway-line-bar, drag, and clearance-box interactions move an arrival into the runway line with landing clearance. | Can action effects differ by arrival/departure and airport? |
| Arrival completion | Tower can transfer to ground's taxi line or use OUT for the western GA apron case. Ground removes parked strips through GATE or CNL. Removed strips can be restored within fifteen minutes. | Can airports define terminal states, removal reasons, and recovery windows? |
| Runway operations | MPO, SPO, and SRO runway modes are derived from EuroScope runway configuration; winter operations affect presentation. | What airport-specific operational modes exist, and what do they affect beyond appearance? |
| Runway closure | Closing a runway adds a red cross to clearance boxes for flights assigned that runway, described as alerting and preventing departures. | Is this a warning, a rejected action, or an overridable restriction? |
| Supervisor | West tower is described as SUP when multiple controllers are online, with its changes synchronized to others. | Which settings are shared, who may change them, and what happens without west tower? SUP must not be assumed to mean synchronization master. |
| Local and auxiliary strips | Local inbound/outbound strips can be created without a filed flight plan; obstacle strips can represent blocked runways. | What strip kinds and lifecycle rules must the future system accommodate? |
| Coordination views | Ground and tower can see representations of each other's arrival/departure lists. | How does visibility differ from ownership and permission to act? |
| Other capabilities | The source describes conflicting-stand indicators, deicing lists/times, weather acknowledgement, notes, ATIS windows, and aircraft/runway visual distinctions. | Which are shared capabilities, airport-specific features, or presentation rules? |

## Unresolved source details

- The position table names `ENGM_P_GND` as GMQ, while the sequencing flow names
  `ENGM_Q_GND`. The intended position identity requires confirmation.
- Several images are explicitly marked older or superseded. Pixel-level layouts
  and interaction details have not been accepted from these notes.
- The inspected image20 contains a SID/runway table and approach handoff orders.
  Operational values and fallbacks must be confirmed before using it as actual
  configuration.
- A reference to integrating with a CDM plugin does not determine whether the
  eventual implementation uses that plugin or FlightStrips CDM.
- The source leaves some functions uncertain, including NAVAID, and proposes
  omitting some real-system functions. These are unresolved product decisions.
- The narrative does not establish a complete permission model, sector ownership
  model, top-down coverage rules, or fallback behavior for every staffing case.

Return to the [multi-airport discovery document](../multi-airport-support.md).
