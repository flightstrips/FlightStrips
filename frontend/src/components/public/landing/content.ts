import type { SIState } from "./StripSpecimen";

/**
 * Landing copy.
 *
 * House rule for this file: every capability claim names something that is
 * already merged, and carries a `src:` comment pointing at the implementation.
 * Anything not yet built belongs in `roadmap`, which the page renders under an
 * explicit "planned" label. If you cannot cite it, do not claim it.
 */

export const SITE = {
  docs: "https://docs.flightstrips.dk",
  discord: "https://discord.gg/vpBk2mpg74",
  github: "https://github.com/flightstrips",
  email: "info@flightstrips.dk",
} as const;

export const HERO = {
  badge: "Live at EKCH Kastrup",
  headline: "The strip board every position is working from.",
  // src: docs/getting-started/intro.md — plugin supplies identity + live data,
  // server holds shared session state, web app is where strips are operated.
  standfirst:
    "FlightStrips is a shared electronic strip board for VATSIM controllers. The EuroScope plugin supplies live flight data, the server holds one authoritative session, and every connected position reads and writes the same strips.",
  note: "Built for EKCH Kastrup. Free for controllers. Open source.",
} as const;

/** External systems the server actually talks to. */
export const CONNECTIONS = [
  // src: frontend/src/providers/auth-provider.tsx + backend/internal/services/authentication.go
  { label: "VATSIM SSO", detail: "Plugin and browser matched by CID" },
  // src: backend/internal/euroscope/ — hub, sync_service, handlers
  { label: "EuroScope plugin", detail: "Two-way flight-plan and ground-state sync" },
  // src: backend/internal/pdc/hoppie.go — ARINC and classic PDC request parsing
  { label: "Hoppie ACARS", detail: "Pre-departure clearance delivery" },
  // src: backend/internal/ecfmp/client.go — https://ecfmp.vatsim.net/api/v1
  { label: "ECFMP", detail: "Live flow measures" },
  // src: backend/internal/metar/poller.go — AFV ATIS feed
  { label: "VATSIM ATIS", detail: "METAR and ATIS codes" },
] as const;

export type PositionKey = "clr" | "apn" | "gnd" | "twr" | "seq";

/** Which real strip the panel shows, and the SI state to paint on it. */
export type StripDemo = {
  variant: "cleared" | "arrival";
  si: SIState;
  nextLabel?: string;
  callsign: string;
  caption: string;
};

export type PositionPanel = {
  key: PositionKey;
  tab: string;
  title: string;
  summary: string;
  /** Shipped capabilities only. */
  capabilities: string[];
  layoutLabel: string;
  layout: string;
  strips: StripDemo[];
};

export const POSITIONS: PositionPanel[] = [
  {
    key: "clr",
    tab: "Clearance delivery",
    title: "Clearance delivery",
    // src: backend/internal/clx/validation.go, backend/internal/pdc/
    summary:
      "Issue clearances by voice or as a PDC over Hoppie ACARS. The board tracks each clearance state and blocks the ones that do not check out.",
    capabilities: [
      // src: backend/internal/pdc/hoppie.go — parses both ARINC RCD and classic
      // "REQUEST PREDEP CLEARANCE" formats, handles WILCO / UNABLE replies
      "PDC request, uplink and WILCO tracking over Hoppie",
      // src: backend/internal/clx/validation.go
      "Clearance validation before the strip can move on",
      // src: backend/internal/services/strip_wrong_squawk_validation.go,
      //      strip_duplicate_squawk_validation.go
      "Wrong-squawk and duplicate-squawk prompts",
      // src: backend/internal/services/strip_ctot_validation.go
      "CTOT conflict prompts on the strip",
      // src: backend/internal/services/strip_pdc_invalid_validation.go
      "Invalid-PDC and custom PDC validation rules",
    ],
    layoutLabel: "Layout",
    layout: "CLX — the clearance layout, with its own bays and command bar.",
    strips: [
      { variant: "cleared", si: "assumed", nextLabel: "A", callsign: "SAS1462", caption: "Cleared, held by you, apron next" },
      { variant: "cleared", si: "sending", nextLabel: "A", callsign: "KLM18X", caption: "Transfer sent to apron, awaiting acceptance" },
    ],
  },
  {
    key: "apn",
    tab: "Apron",
    title: "Apron",
    // src: backend/internal/services/departure_push_release.go, stand_actions.go
    summary:
      "Pushback and startup release, plus the stand picture for both flows. Stand changes propagate to the strip and back to EuroScope.",
    capabilities: [
      // src: backend/internal/services/departure_push_release.go
      "Pushback release tracked per strip",
      // src: backend/internal/services/stand_allocation.go + internal/sat/
      "Automatic stand allocation with aircraft and geometry compatibility",
      // src: backend/internal/sat/stand_compatibility.go, stand_capability.go
      "Stand capability, wingspan and neighbour-blocking rules",
      // src: backend/internal/standstatus/webapi.go, frontend/src/pages/stand-status.tsx
      "Live stand-status board at /stand",
      // src: backend/internal/services/arrival_lifecycle.go
      "Arrival stand coordination on the same board",
    ],
    layoutLabel: "Layouts",
    layout: "Apron departures and arrivals work as separate bays so the flows do not cross-talk.",
    strips: [
      { variant: "cleared", si: "receiving", nextLabel: "A", callsign: "NAX41K", caption: "Inbound transfer waiting for you to assume" },
      { variant: "arrival", si: "assumed", nextLabel: "GE", callsign: "DLH820", caption: "Arrival on stand, ground next" },
    ],
  },
  {
    key: "gnd",
    tab: "Ground",
    title: "Ground",
    // src: backend/config/ekch.yaml — layouts GEGW, sectors, routes
    summary:
      "Taxi and crossing coordination across a split field. Ownership routes are configured per airport, so a strip arrives where the procedure says it should.",
    capabilities: [
      // src: backend/internal/services/strip_coordination.go
      "Ownership transfer with next and previous controller tracking",
      // src: backend/internal/services/strip_auto_assume.go
      "Configured automatic assume on handover",
      // src: backend/internal/services/strip_taxiway_type_validation.go
      "Taxiway-type validation against the aircraft",
      // src: frontend/src/components/strip/ half-strips for crossings
      "Crossing and memory-aid half-strips",
      // src: backend/config/ekch.yaml — routes:
      "Airport-configured ownership routes between positions",
    ],
    layoutLabel: "Layout",
    layout: "GEGW — ground east and ground west share one board.",
    strips: [
      { variant: "cleared", si: "assumed", nextLabel: "TE", callsign: "SAS907", caption: "Held by ground, tower next" },
      { variant: "arrival", si: "released", callsign: "BAW810", caption: "Released — passed on, kept for context" },
    ],
  },
  {
    key: "twr",
    tab: "Tower",
    title: "Tower",
    // src: backend/config/ekch.yaml — layouts TWTE, TWRGND; runways:
    summary:
      "Runway work with the validations that catch the common slips, and a combined layout for when you are the only one online.",
    capabilities: [
      // src: backend/internal/services/strip_landing_clearance_validation.go
      "Landing-clearance validation",
      // src: backend/internal/services/strip_runway_type_validation.go
      "Runway-type validation against the aircraft",
      // src: backend/internal/services/strip_missed_approach.go
      "Missed-approach handling with configured handover",
      // src: backend/internal/euroscope/hub_runways.go
      "Runway configuration shared across every connected position",
      // src: backend/config/ekch.yaml — layout TWRGND; docs/procedures/twr-bandbox.mdx
      "TWRGND bandbox layout for single-controller operations",
    ],
    layoutLabel: "Layouts",
    layout: "TWTE for a split tower, TWRGND when tower and ground are one position.",
    strips: [
      { variant: "cleared", si: "assumed", nextLabel: "GE", callsign: "SAS1783", caption: "Departure held by tower" },
      { variant: "arrival", si: "concerned", callsign: "AFR1462", caption: "Concerned — yours next, not yet assumed" },
    ],
  },
  {
    key: "seq",
    tab: "Sequence and flow",
    title: "Sequence and flow",
    // src: backend/internal/cdm/, backend/internal/aman/
    summary:
      "A-CDM on the departure side and an arrival manager on the other. Both feed the same strips the tower is already working.",
    capabilities: [
      // src: backend/internal/cdm/calculate.go, time_math.go, sequence.go
      "TOBT, TSAT, TTOT and CTOT calculated and re-sequenced continuously",
      // src: backend/internal/cdm/deice_minutes.go, adverse_conditions.go
      "De-icing time and adverse-condition allowances in the calculation",
      // src: backend/internal/cdm/wake_spacing.go, backend/config/ekch/sidInterval.txt
      "Wake-turbulence and SID-interval spacing on the departure sequence",
      // src: backend/internal/ecfmp/service.go, matcher.go
      "ECFMP flow measures matched to flights and shown on the strip",
      // src: backend/internal/aman/ — predictor, trajectory, sequence
      "Arrival manager with trajectory prediction and sequencing",
      // src: frontend/src/pages/cdm.tsx
      "Read-only CDM sequence view at /cdm",
    ],
    layoutLabel: "Layouts",
    layout: "AA and AD for the arrival and departure overview, EST for the estimate view.",
    strips: [
      { variant: "cleared", si: "assumed", nextLabel: "A", callsign: "FIN842", caption: "TSAT issued; CTOT shown when a slot applies" },
      { variant: "arrival", si: "unconcerned", callsign: "KLM1148", caption: "Unconcerned — on the board for awareness only" },
    ],
  },
];

/**
 * Exactly the backgrounds SIBox.tsx paints, in the order it decides them.
 * The docs also mention a grey unconcerned box, but no shipped code paints one
 * — unconcerned and released both resolve to --color-si-unconcerned — so this
 * page describes the three solid states and the two split ones instead.
 * src: src/components/strip/SIBox.tsx
 */
export const OWNERSHIP = [
  {
    swatch: "#F0F0F0",
    title: "Assumed",
    body: "You own the strip. Transfers, command-bar actions and field edits are enabled.",
  },
  {
    swatch: "#FA02FC",
    title: "Concerned",
    body: "Your position is in the strip’s route but has not held it yet. Click to assume it when nobody owns it.",
  },
  {
    swatch: "#DD6A12",
    title: "Not yours right now",
    body: "Either your position has already handled the strip, or it is not in the route at all. It stays on the board for context.",
  },
  {
    swatch: "linear-gradient(to right, #F0F0F0 50%, #DD6A12 50%)",
    title: "Transfer out, or a tag request",
    body: "You offered the strip onward, or another controller asked for it with REQ. Click to recall the transfer, or to accept the request.",
  },
  {
    swatch: "linear-gradient(to right, #FA02FC 50%, #F0F0F0 50%)",
    title: "Offered to you",
    body: "Someone is handing the strip to your position. Ownership only moves once you click to assume it.",
  },
] as const;

/** Deeper capability narrative. Each entry cites its implementation. */
export const CAPABILITIES = [
  {
    id: "shared-state",
    eyebrow: "Shared state",
    title: "One session, not four browsers guessing.",
    // src: docs/getting-started/features.md — "What is shared"
    body: "Ownership, strip order and pending coordination are server state, not private annotations. When another controller transfers or edits a strip, your board receives the result rather than a local guess.",
    detail: "The snapshot sent to each web app carries the strips, controllers, runway setup, layout, ownership routes and feature state for the session. Later events update that state for every connected client.",
    link: { label: "How it fits together", href: `${SITE.docs}/getting-started/features/` },
  },
  {
    id: "euroscope",
    eyebrow: "EuroScope",
    title: "The plugin is the data path, both ways.",
    // src: backend/internal/euroscope/sync_service.go, handlers.go; docs/getting-started/features.md
    body: "The plugin reports your airport, callsign, primary frequency, connection mode and live EuroScope state — and applies supported changes made in the web app back to your EuroScope client.",
    detail: "It covers aircraft and ground state, position, runway, squawk, cleared altitude, heading, controller tracking and coordination. It only connects with an authenticated user, a Direct, Sweatbox or Playback connection, a recognised airport and a usable primary frequency.",
    link: { label: "Connect EuroScope", href: `${SITE.docs}/getting-started/es-plugin/` },
  },
  {
    id: "validation",
    eyebrow: "Validation",
    title: "The board says when something does not add up.",
    // src: backend/internal/services/strip_validation.go + the per-rule files
    body: "Validation runs on the strip itself: wrong or duplicate squawk, CTOT conflicts, landing clearance, runway type against the aircraft, taxiway type, and PDC rules configured per airport.",
    detail: "Rules are prioritised so the strip shows the one that matters, and they are declared in the airport configuration rather than hard-coded into the client.",
    link: { label: "Validation status", href: `${SITE.docs}/procedures/validation-status/` },
  },
  {
    id: "pilots",
    eyebrow: "Pilots",
    title: "Pilots have a way in, without a phone call.",
    // src: backend/internal/pilot/webapi.go, backend/internal/efb/webapi.go,
    //      frontend/src/pages/pilot-flight.tsx, frontend/src/pages/efb.tsx
    body: "A pilot signs in with VATSIM, finds their flight by callsign, and sees the clearance and CDM picture the controller is working from — EOBT, TOBT, CTOT, pushback point and PDC state.",
    detail: "From the same pages a pilot can request a PDC, confirm a TOBT and request a stand. The controller board receives those as ordinary events, and the ATIS and METAR come from the live VATSIM feed.",
    link: { label: "Open the pilot view", href: "/pilot" },
  },
] as const;

/** Airport scope: what is live, what is planned. Kept deliberately separate. */
export const SCOPE = {
  eyebrow: "Scope",
  title: "Built for Kastrup first.",
  // src: backend/config/ekch.yaml (1084 lines), backend/config/ekch/, frontend/src/store/airports/ekch.ts
  live: {
    heading: "EKCH, today",
    body: "Copenhagen is the field FlightStrips was built around, and the only one running today. Its positions, sectors, ownership routes, runways, layouts, stands, taxi zones, SID intervals and CDM parameters are all defined in the airport configuration.",
    points: [
      "Clearance, apron, ground, tower, arrival and departure layouts",
      "Stand allocation against the real stand geometry and capabilities",
      "A-CDM parameters, taxi zones and de-icing tuned for the field",
    ],
  },
  planned: {
    heading: "The Danish regionals, planned",
    body: "The Danish regional fields are the next target, together with RADIS. Both are roadmap rather than product: no regional configuration and no RADIS integration exist yet.",
    points: [
      "Regional airport configurations",
      "RADIS integration for the regional positions",
    ],
  },
  onboarding: {
    heading: "Somewhere else?",
    body: "An airport is a configuration, not a fork: positions, bays, ownership routes, runways, layouts, stands and CDM parameters are data. If you want your field on FlightStrips, get in touch and we will help you get started.",
  },
} as const;

export const FOOTER_COLUMNS = [
  {
    title: "Product",
    links: [
      { label: "Start here", href: `${SITE.docs}/getting-started/intro/` },
      { label: "How it fits together", href: `${SITE.docs}/getting-started/features/` },
      { label: "EuroScope plugin", href: `${SITE.docs}/getting-started/es-plugin/` },
      { label: "First-session checklist", href: `${SITE.docs}/getting-started/first-session/` },
    ],
  },
  {
    title: "Concepts",
    links: [
      { label: "Strip anatomy", href: `${SITE.docs}/concepts/strip-anatomy/` },
      { label: "Ownership and handoffs", href: `${SITE.docs}/concepts/ownership/` },
      { label: "Pre-departure clearance", href: `${SITE.docs}/concepts/pre-departure-clearance/` },
      { label: "Validation status", href: `${SITE.docs}/procedures/validation-status/` },
      { label: "Alone as tower", href: `${SITE.docs}/procedures/twr-bandbox/` },
    ],
  },
  {
    title: "Kastrup",
    links: [
      { label: "Clearance delivery", href: `${SITE.docs}/ekch/clr-del/` },
      { label: "Apron departures", href: `${SITE.docs}/ekch/apn-dep/` },
      { label: "Ground east and west", href: `${SITE.docs}/ekch/ge-gw/` },
      { label: "Tower east and west", href: `${SITE.docs}/ekch/te-tw/` },
      { label: "Sequence planner", href: `${SITE.docs}/ekch/seq-pln/` },
    ],
  },
] as const;

/**
 * Photography. Annotations follow the art-direction reference supplied with the
 * images; alt text describes what is in frame. These are stylised production
 * images of controllers working the board, not screenshots of the product.
 */
export const PHOTOS = {
  ops: {
    src: "/landing/ops-room.webp",
    srcSmall: "/landing/ops-room-800.webp",
    width: 1448,
    height: 1086,
    index: "01",
    topLeft: ["Operations", "People", "Technology"],
    bottomRight: ["Plan", "Coordinate", "Execute"],
    alt: "A controller speaking into a handheld microphone at a position, with the airport ground map on the monitors beside them and colleagues working further down the room.",
  },
  field: {
    src: "/landing/controller-map.webp",
    srcSmall: "/landing/controller-map-800.webp",
    width: 1448,
    height: 1086,
    index: "02",
    topLeft: ["Real", "Data", "Real", "Impact"],
    bottomRight: ["From", "Runways", "To possibilities"],
    alt: "A controller at a laptop showing the airport ground map, with the tower view of runways and taxiways on the screen behind.",
  },
  together: {
    src: "/landing/strip-board.webp",
    srcSmall: "/landing/strip-board-800.webp",
    width: 1448,
    height: 1086,
    index: "03",
    topLeft: ["Better", "Systems", "Brighter", "Skies"],
    bottomRight: ["Human", "Judgment", "Amplified"],
    alt: "Two controllers side by side at one screen, one pointing at a strip on the board, with the tower view and ground maps around them.",
  },
} as const;
