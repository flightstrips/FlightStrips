# Multi-airport implementation issue index

Published on 2026-11-02 in flightstrips/FlightStrips. The eight epics and
41 child issues use native GitHub sub-issue relationships and a non-redundant
blocked-by prerequisite graph. The source plan remains [epic-plan.md](epic-plan.md).

Specification PR: [#972](https://github.com/flightstrips/FlightStrips/pull/972). Merging this specification does not
complete the implementation issues. No assignees, milestones, or release dates
were invented. Issue progress is authoritative in GitHub, not this static index.

## Epics

| Plan ID | GitHub epic | Title | Child issues |
| --- | --- | --- | --- |
| MA-01 | [#973](https://github.com/flightstrips/FlightStrips/issues/973) | Load combined airport definitions in production | 5 |
| MA-02 | [#974](https://github.com/flightstrips/FlightStrips/issues/974) | Route workflows through coded handlers and preserve EKCH | 5 |
| MA-03 | [#975](https://github.com/flightstrips/FlightStrips/issues/975) | Support independent airport memberships over one plugin connection | 5 |
| MA-04 | [#976](https://github.com/flightstrips/FlightStrips/issues/976) | Coordinate airport coverage, takeover, and automatic rejoin | 5 |
| MA-05 | [#977](https://github.com/flightstrips/FlightStrips/issues/977) | Isolate airport service state while sharing sources and clients | 6 |
| MA-06 | [#978](https://github.com/flightstrips/FlightStrips/issues/978) | Bind each frontend tab to an authorized airport session | 4 |
| MA-07 | [#979](https://github.com/flightstrips/FlightStrips/issues/979) | Deliver the integrated RADIS board for EKBI/EKYT | 6 |
| MA-08 | [#980](https://github.com/flightstrips/FlightStrips/issues/980) | Validate and activate the release in an offline window | 5 |

## Prerequisite links

These are completion prerequisites. Work may start earlier against agreed
contracts, as explained in the epic plan. Native links omit redundant edges;
the complete prerequisite lists are retained in each epic body.

| Prerequisite | Dependent epic |
| --- | --- |
| MA-01 [#973](https://github.com/flightstrips/FlightStrips/issues/973) | MA-02 [#974](https://github.com/flightstrips/FlightStrips/issues/974) |
| MA-02 [#974](https://github.com/flightstrips/FlightStrips/issues/974) | MA-03 [#975](https://github.com/flightstrips/FlightStrips/issues/975) |
| MA-03 [#975](https://github.com/flightstrips/FlightStrips/issues/975) | MA-04 [#976](https://github.com/flightstrips/FlightStrips/issues/976) |
| MA-05 [#977](https://github.com/flightstrips/FlightStrips/issues/977) | MA-04 [#976](https://github.com/flightstrips/FlightStrips/issues/976) |
| MA-02 [#974](https://github.com/flightstrips/FlightStrips/issues/974) | MA-05 [#977](https://github.com/flightstrips/FlightStrips/issues/977) |
| MA-03 [#975](https://github.com/flightstrips/FlightStrips/issues/975) | MA-06 [#978](https://github.com/flightstrips/FlightStrips/issues/978) |
| MA-04 [#976](https://github.com/flightstrips/FlightStrips/issues/976) | MA-07 [#979](https://github.com/flightstrips/FlightStrips/issues/979) |
| MA-06 [#978](https://github.com/flightstrips/FlightStrips/issues/978) | MA-07 [#979](https://github.com/flightstrips/FlightStrips/issues/979) |
| MA-07 [#979](https://github.com/flightstrips/FlightStrips/issues/979) | MA-08 [#980](https://github.com/flightstrips/FlightStrips/issues/980) |

## Child issues

| Plan ID | Parent epic | GitHub issue | Title |
| --- | --- | --- | --- |
| MA-01.1 | MA-01 [#973](https://github.com/flightstrips/FlightStrips/issues/973) | [#981](https://github.com/flightstrips/FlightStrips/issues/981) | Implement strict combined airport configuration loading |
| MA-01.2 | MA-01 [#973](https://github.com/flightstrips/FlightStrips/issues/973) | [#982](https://github.com/flightstrips/FlightStrips/issues/982) | Register immutable airport definitions and typed handler settings |
| MA-01.3 | MA-01 [#973](https://github.com/flightstrips/FlightStrips/issues/973) | [#983](https://github.com/flightstrips/FlightStrips/issues/983) | Resolve airport configuration resources consistently |
| MA-01.4 | MA-01 [#973](https://github.com/flightstrips/FlightStrips/issues/973) | [#984](https://github.com/flightstrips/FlightStrips/issues/984) | Load EKCH from its combined airport definition |
| MA-01.5 | MA-01 [#973](https://github.com/flightstrips/FlightStrips/issues/973) | [#985](https://github.com/flightstrips/FlightStrips/issues/985) | Verify production loader and schema parity |
| MA-02.1 | MA-02 [#974](https://github.com/flightstrips/FlightStrips/issues/974) | [#986](https://github.com/flightstrips/FlightStrips/issues/986) | Define production workflow hooks and typed action contracts |
| MA-02.2 | MA-02 [#974](https://github.com/flightstrips/FlightStrips/issues/974) | [#987](https://github.com/flightstrips/FlightStrips/issues/987) | Move EKCH workflow decisions behind its coded handler |
| MA-02.3 | MA-02 [#974](https://github.com/flightstrips/FlightStrips/issues/974) | [#988](https://github.com/flightstrips/FlightStrips/issues/988) | Execute scoped workflow decisions through existing move and field paths |
| MA-02.4 | MA-02 [#974](https://github.com/flightstrips/FlightStrips/issues/974) | [#989](https://github.com/flightstrips/FlightStrips/issues/989) | Separate human own-plugin authority from automatic executor resolution |
| MA-02.5 | MA-02 [#974](https://github.com/flightstrips/FlightStrips/issues/974) | [#990](https://github.com/flightstrips/FlightStrips/issues/990) | Verify production EKCH workflows and custom-handler extensibility |
| MA-03.1 | MA-03 [#975](https://github.com/flightstrips/FlightStrips/issues/975) | [#991](https://github.com/flightstrips/FlightStrips/issues/991) | Add membership and session context to the existing plugin protocol |
| MA-03.2 | MA-03 [#975](https://github.com/flightstrips/FlightStrips/issues/975) | [#992](https://github.com/flightstrips/FlightStrips/issues/992) | Separate backend plugin transport identity from airport memberships |
| MA-03.3 | MA-03 [#975](https://github.com/flightstrips/FlightStrips/issues/975) | [#993](https://github.com/flightstrips/FlightStrips/issues/993) | Maintain independent airport membership state in the EuroScope plugin |
| MA-03.4 | MA-03 [#975](https://github.com/flightstrips/FlightStrips/issues/975) | [#994](https://github.com/flightstrips/FlightStrips/issues/994) | Elect and synchronize masters independently per airport session |
| MA-03.5 | MA-03 [#975](https://github.com/flightstrips/FlightStrips/issues/975) | [#995](https://github.com/flightstrips/FlightStrips/issues/995) | Keep control updates and discovery available without airport memberships |
| MA-04.1 | MA-04 [#976](https://github.com/flightstrips/FlightStrips/issues/976) | [#996](https://github.com/flightstrips/FlightStrips/issues/996) | Evaluate configured airport responsibilities and radio eligibility |
| MA-04.2 | MA-04 [#976](https://github.com/flightstrips/FlightStrips/issues/976) | [#997](https://github.com/flightstrips/FlightStrips/issues/997) | Build shared normalized staffing and radio evidence for coverage |
| MA-04.3 | MA-04 [#976](https://github.com/flightstrips/FlightStrips/issues/976) | [#998](https://github.com/flightstrips/FlightStrips/issues/998) | Apply membership deltas and bounded airport takeover |
| MA-04.4 | MA-04 [#976](https://github.com/flightstrips/FlightStrips/issues/976) | [#999](https://github.com/flightstrips/FlightStrips/issues/999) | Define deterministic coverage evidence freshness behavior |
| MA-04.5 | MA-04 [#976](https://github.com/flightstrips/FlightStrips/issues/976) | [#1000](https://github.com/flightstrips/FlightStrips/issues/1000) | Rejoin airports automatically through the retained control connection |
| MA-05.1 | MA-05 [#977](https://github.com/flightstrips/FlightStrips/issues/977) | [#1001](https://github.com/flightstrips/FlightStrips/issues/1001) | Preserve shared VATSIM and transceiver acquisition across airports |
| MA-05.2 | MA-05 [#977](https://github.com/flightstrips/FlightStrips/issues/977) | [#1002](https://github.com/flightstrips/FlightStrips/issues/1002) | Load CDM data per airport and isolate operational calculations |
| MA-05.3 | MA-05 [#977](https://github.com/flightstrips/FlightStrips/issues/977) | [#1003](https://github.com/flightstrips/FlightStrips/issues/1003) | Scope stand definitions and assignment state to airport sessions |
| MA-05.4 | MA-05 [#977](https://github.com/flightstrips/FlightStrips/issues/977) | [#1004](https://github.com/flightstrips/FlightStrips/issues/1004) | Use scoped text-pilot inputs and shared backend weather observations |
| MA-05.5 | MA-05 [#977](https://github.com/flightstrips/FlightStrips/issues/977) | [#1005](https://github.com/flightstrips/FlightStrips/issues/1005) | Preserve shared clients and enforce regional PDC and AMAN disablement |
| MA-05.6 | MA-05 [#977](https://github.com/flightstrips/FlightStrips/issues/977) | [#1006](https://github.com/flightstrips/FlightStrips/issues/1006) | Separate shared resource lifetime from scoped service work |
| MA-06.1 | MA-06 [#978](https://github.com/flightstrips/FlightStrips/issues/978) | [#1007](https://github.com/flightstrips/FlightStrips/issues/1007) | Discover authorized frontend sessions and explicit own instances |
| MA-06.2 | MA-06 [#978](https://github.com/flightstrips/FlightStrips/issues/978) | [#1008](https://github.com/flightstrips/FlightStrips/issues/1008) | Add per-tab airport selection and switching |
| MA-06.3 | MA-06 [#978](https://github.com/flightstrips/FlightStrips/issues/978) | [#1009](https://github.com/flightstrips/FlightStrips/issues/1009) | Bind board subscriptions and snapshots to the selected session |
| MA-06.4 | MA-06 [#978](https://github.com/flightstrips/FlightStrips/issues/978) | [#1010](https://github.com/flightstrips/FlightStrips/issues/1010) | Reject stale frontend traffic and preserve scoped read-only reconnect |
| MA-07.1 | MA-07 [#979](https://github.com/flightstrips/FlightStrips/issues/979) | [#1011](https://github.com/flightstrips/FlightStrips/issues/1011) | Map the current RADIS prototype features to production actions |
| MA-07.2 | MA-07 [#979](https://github.com/flightstrips/FlightStrips/issues/979) | [#1012](https://github.com/flightstrips/FlightStrips/issues/1012) | Implement the RADIS handler and authoritative shared board storage |
| MA-07.3 | MA-07 [#979](https://github.com/flightstrips/FlightStrips/issues/979) | [#1013](https://github.com/flightstrips/FlightStrips/issues/1013) | Integrate RADIS strip menus and annotation controls into the host frontend |
| MA-07.4 | MA-07 [#979](https://github.com/flightstrips/FlightStrips/issues/979) | [#1014](https://github.com/flightstrips/FlightStrips/issues/1014) | Implement shared RADIS local-strip creation and connected-flight FIND |
| MA-07.5 | MA-07 [#979](https://github.com/flightstrips/FlightStrips/issues/979) | [#1015](https://github.com/flightstrips/FlightStrips/issues/1015) | Implement RADIS appearance, manual removal and strip restoration |
| MA-07.6 | MA-07 [#979](https://github.com/flightstrips/FlightStrips/issues/979) | [#1016](https://github.com/flightstrips/FlightStrips/issues/1016) | Complete RADIS service integration and airport acceptance fixtures |
| MA-08.1 | MA-08 [#980](https://github.com/flightstrips/FlightStrips/issues/980) | [#1017](https://github.com/flightstrips/FlightStrips/issues/1017) | Capture EKCH baseline behavior and define release acceptance |
| MA-08.2 | MA-08 [#980](https://github.com/flightstrips/FlightStrips/issues/980) | [#1018](https://github.com/flightstrips/FlightStrips/issues/1018) | Verify cross-application multi-airport isolation and shared resource ownership |
| MA-08.3 | MA-08 [#980](https://github.com/flightstrips/FlightStrips/issues/980) | [#1019](https://github.com/flightstrips/FlightStrips/issues/1019) | Approve concrete EKBI and EKYT deployment definitions and service data |
| MA-08.4 | MA-08 [#980](https://github.com/flightstrips/FlightStrips/issues/980) | [#1020](https://github.com/flightstrips/FlightStrips/issues/1020) | Measure production EKCH regression and concurrent-airport performance |
| MA-08.5 | MA-08 [#980](https://github.com/flightstrips/FlightStrips/issues/980) | [#1021](https://github.com/flightstrips/FlightStrips/issues/1021) | Rehearse and execute the coordinated offline multi-airport release |
