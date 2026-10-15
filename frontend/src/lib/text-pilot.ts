import { ActionType, Bay, CommunicationType, type FrontendController, type FrontendSendEvent, type FrontendStrip as Strip } from "@/api/models";
import { decodeMetar } from "@/lib/metarDecode";

export type TextPilotVariant = "push" | "apron-out" | "apron-in" | "tower-out" | "tower-in" | "si" | "runway" | "clearance";
export interface TextPilotDraft {
  callsign: string;
  variant: TextPilotVariant;
  point?: string;
  holdingPoint?: boolean;
  splitTowerDefaults?: boolean;
  stand?: string;
  confirmed?: boolean;
  arrival?: boolean;
  actions?: FrontendSendEvent[];
  onSubmitted?: () => void;
  initialSelected?: string[];
  initialText?: string;
  readbackKey?: string;
  transferAllowed?: () => boolean;
  startRequestTransfer?: boolean;
}
export interface FrequencyMessageResult {
  type: "frequency_message_result";
  request_id: string;
  callsign: string;
  submitted: boolean;
  error?: string;
  applied_actions: number;
  validation_blocked?: boolean;
  prepared?: boolean;
  stand?: string;
  runway_confirmed?: boolean;
}
export interface Phrase { id: string; label: string; text: string; tone: "light" | "phrase" | "point"; disabled?: boolean; }
export const TEXT_STATIONS = [
  { name: "Kastrup Delivery", frequency: "119.905", sectors: ["DEL"] },
  { name: "Kastrup Apron", frequency: "121.905", sectors: ["SEQ"] },
  { name: "Kastrup Apron", frequency: "121.630", sectors: ["AA"] },
  { name: "Kastrup Apron", frequency: "121.730", sectors: ["AD"] },
  { name: "Kastrup Tower", frequency: "118.580", sectors: ["GW", "GWA", "GWD"] },
  { name: "Kastrup Tower", frequency: "121.830", sectors: ["GE"] },
  { name: "Kastrup Tower", frequency: "118.105", sectors: ["TW"] },
  { name: "Kastrup Tower", frequency: "119.355", sectors: ["TE"] },
  { name: "Kastrup Departure", frequency: "124.980", sectors: [] },
  { name: "Kastrup Departure", frequency: "120.255", sectors: [] },
  { name: "Copenhagen APP", frequency: "119.805", sectors: [] },
  { name: "Copenhagen APP", frequency: "118.455", sectors: [] },
];
export function stationController(frequency: string, controllers: FrontendController[]) {
  const sectors = TEXT_STATIONS.find(s => s.frequency === frequency)?.sectors ?? [];
  const active = controllers.filter(c => !c.observer && !c.callsign.toUpperCase().endsWith("_OBS"));
  return active.find(c => c.position === frequency) ?? active.find(c => c.owned_sectors?.some(s => sectors.includes(s)));
}

const phrase = (id: string, label: string, text = label.charAt(0) + label.slice(1).toLowerCase(), tone: Phrase["tone"] = "phrase"): Phrase => ({ id, label, text, tone });
const taxiways = ["A", "B", "C", "D", "F", "G", "G1", "G2", "I", "J", "K", "K1", "K2", "K3", "L", "M", "N1", "N2", "P", "Q", "R", "S", "T", "V", "V1", "V2", "W", "X", "Y", "Z", "C32"];
const runways = ["04L", "04R", "22R", "22L", "12", "30"];
const points = ["A1", "A2", "A3", "A4", "A5", "B1", "B2", "B3", "B4", "E1", "E2", "E3", "F1", "V1", "V2", "G1", "G2", "K1", "K2", "K3"];

export function surfaceWind(metar: string): string {
  const wind = decodeMetar(metar);
  if (wind.windSpeedKts == null) return "";
  if (wind.windSpeedKts === 0) return "Wind calm.";
  if (wind.windDirection === "VRB") return `Wind variable, ${wind.windSpeedKts} knots.`;
  return wind.windDegrees == null ? "" : `Wind ${String(wind.windDegrees).padStart(3, "0")} degrees, ${wind.windSpeedKts} knots.`;
}

export function taxiLimit(draft: TextPilotDraft, runway: string): string {
  const point = draft.point ?? "";
  if (draft.stand) return `to stand ${draft.stand}.`;
  if (draft.holdingPoint) return `for Holding point ${point}, Runway ${runway}.`;
  if (point.includes("/")) {
    const [from, to] = point.split("/");
    return `TWY ${from}, Hold short of TWY ${to}.`;
  }
  if (draft.variant === "apron-out" && ["A", "F", "D", "B", "K3", "K2", "K1"].includes(point)) return `TWY ${point}, Hold short RWY 12/30.`;
  return point ? `TWY ${point}.` : "";
}

export function textPilotPhrases(draft: TextPilotDraft, strip: Pick<Strip, "runway"> & { stand?: string }, metar: string): Phrase[] {
  if (draft.variant === "si") return [
    phrase("readback", "READBACK CORRECT.", "Readback correct."),
    phrase("startup", "START UP APPROVED.", "Start up approved."),
    phrase("hold", "HOLD POSITION!", "Hold position!"),
    phrase("remain", "REMAIN ON THIS FREQUENCY.", "Remain on this frequency."),
    ...TEXT_STATIONS.map(s => phrase(`freq:${s.frequency}`, `${s.name.toUpperCase()}\n${s.frequency}`, `Contact ${s.name} on frequency ${s.frequency}.`, "point")),
  ];
  if (draft.variant === "runway") return [
    { ...phrase("wind", "[WIND]", surfaceWind(metar), "point"), disabled: !surfaceWind(metar) },
    phrase("continue-approach", "CONTINUE APPROACH"), phrase("land", "CLEARED TO LAND"),
    phrase("takeoff", "CLEARED FOR TAKEOFF"), phrase("lineup", "LINE UP AND WAIT"),
    ...runways.map(r => phrase(`rwy:${r}`, `RWY${r}`, `RWY ${r},`, "light")),
    phrase("long-rollout", "LONG ROLLOUT APPROVED", undefined, "point"), phrase("expedite-vacating", "EXPEDITE VACATING", undefined, "point"),
    phrase("wake", "CAUTION WAKE TURBULENCE", undefined, "point"), phrase("rolling", "START ROLLING", undefined, "point"),
  ];
  if (draft.variant === "clearance") return [];
  if (draft.variant === "push") {
    const direction = ({ N: "North", E: "East", W: "West", S: "South" } as Record<string,string>)[draft.point ?? ""];
    const end = direction ? `Face ${direction}.` : `Release point ${draft.point ?? ""}.`;
    return [phrase("push", "PUSH AND START APPROVED", "Push and start approved,"), phrase("limit", direction ? `FACE ${direction.toUpperCase()}` : `RELEASE POINT ${draft.point ?? ""}`, end, "point"),
      ...["J", "P", "Q", "R", "S", "T", "V", "W", "Y", "Z"].map(p => phrase(`release:${p}`, p, `Release point ${p}`, "light")),
      ...Array.from({ length: 9 }, (_, i) => phrase(`number:${i+1}`, String(i+1), String(i+1), "light")),
      ...["North", "East", "West", "South"].map(d => phrase(`face:${d}`, d.toUpperCase(), `Face ${d}.`, "point")),
      phrase("standby", "STANDBY"), phrase("start", "START UP APPROVED", "Start up approved."), phrase("approved", "APPROVED", "Approved."),
    ];
  }
  const tower = draft.variant.startsWith("tower");
  return [phrase("taxi", "TAXI VIA", "Taxi via"), phrase("standby", "STANDBY"), phrase("hold", "HOLD POSITION", "Hold position."),
    phrase("continue", "CONTINUE"), phrase("continue-via", "CONTINUE VIA", "Continue via"), phrase("rwy-12", "RWY 12/30", "RWY 12/30,"), phrase("hold-short-of", "HOLD SHORT OF", "Hold short of"), phrase("hold-22l", "HOLD SHORT RWY 22L", "Hold short RWY 22L."), phrase("via", "VIA"), phrase("hold-short", "HOLD SHORT", "Hold short"), ...(tower || draft.variant === "apron-in" ? [phrase("expedite", "EXPEDITE")] : []),
    ...taxiways.map(t => phrase(`twy:${t}`, t, `TWY ${t},`, "light")),
    ...(tower ? runways.map(r => phrase(`rwy:${r}`, `RWY${r}`, `RWY ${r},`, "light")) : []),
    ...(draft.variant === "tower-out" ? points.map(p => phrase(`hp:${p}`, p, `for Holding point ${p}, Runway ${strip.runway}.`, "point")) : []),
    phrase("cross-12", "CROSS RWY 12/30", "Cross RWY 12/30,"), phrase("cross-22", "CROSS RWY 22L/04R", "Cross RWY 22L/04R,"),
    phrase("hold-12", "HOLD SHORT RWY 12/30", "Hold short RWY 12/30."), phrase("hold-22", "HOLD SHORT RWY 22L/04R", "Hold short RWY 22L/04R."),
    phrase("hold-k", "HOLD SHORT TWY K.", "Hold short TWY K."), phrase("hold-z", "HOLD SHORT TWY Z.", "Hold short TWY Z."),
    phrase("hold-k3", "HOLD SHORT TWY Z ON K3.", "Hold short TWY Z on K3."), phrase("via-f", "VIA F", "Via F,"),
    ...(draft.point && !draft.stand ? [phrase("limit", draft.point, taxiLimit(draft, strip.runway), "point")] : []),
    ...(["apron-in", "tower-in"].includes(draft.variant) && (draft.stand || strip.stand) ? [phrase("stand", draft.stand || strip.stand || "", `to stand ${draft.stand || strip.stand}.`, "point")] : []),
  ];
}

export function defaultTextSelections(draft: TextPilotDraft): string[] {
  if (draft.initialSelected) return draft.initialSelected;
  if (draft.variant === "clearance" || draft.variant === "si") return [];
  if (draft.variant === "push") return ["push", "limit"];
  if (draft.variant === "runway") return [...(draft.confirmed ? ["wind"] : []), draft.arrival ? (draft.confirmed ? "land" : "continue-approach") : (draft.confirmed ? "takeoff" : "lineup")];
  if (draft.variant === "tower-in" && draft.splitTowerDefaults && !draft.stand) {
    const defaults: Record<string,string[]> = {
      K3: ["taxi", "rwy:30", "twy:K3", "hold-k3"], K2: ["taxi", "rwy:30", "twy:K2", "hold-k"],
      B: ["taxi", "twy:B", "cross-12", "hold-z"], A: ["taxi", "twy:A", "cross-12", "hold-z"],
      F: ["taxi", "twy:A", "cross-12", "via-f", "hold-z"],
    };
    if (draft.point && defaults[draft.point]) return defaults[draft.point];
  }
  if (draft.variant === "apron-out" && draft.point && ["A", "F", "D", "B", "K3", "K2", "K1"].includes(draft.point)) return ["taxi", `twy:${draft.point}`, "hold-12"];
  if (draft.holdingPoint) return ["taxi", `hp:${draft.point}`];
  return ["taxi", ...(draft.stand ? ["stand"] : draft.point ? ["limit"] : [])];
}

// Add/remove only the generated phrase's occurrence, retaining edits elsewhere.
export function togglePhraseText(text: string, token: string, selected: boolean): string {
  if (!token) return text;
  if (selected) {
    const index = text.lastIndexOf(token);
    return index < 0 ? text : (text.slice(0,index) + text.slice(index+token.length)).replace(/ {2,}/g," ").trim();
  }
  return `${text.trim()} ${token}`.trim();
}

export function textTaxiActions(callsign: string, point: string, bay?: Bay, acknowledge = false): FrontendSendEvent[] {
  return [
    { type: ActionType.FrontendReleasePoint, callsign, release_point: point },
    ...(bay ? [{ type: ActionType.FrontendMove, callsign, bay } as FrontendSendEvent] : []),
    ...(acknowledge ? [{ type: ActionType.FrontendAcknowledgeUnexpectedChange, callsign, field_name: "release_point" } as FrontendSendEvent] : []),
  ];
}
export const isTextPilot = (strip?: Pick<Strip, "communication_type">) => strip?.communication_type === CommunicationType.Text;

export function clearancePreview(strip: Strip): string {
  const straightAhead = /^SA(?:\s|$)/i.test(strip.sid ?? "");
  const via = strip.sid && !straightAhead ? `${strip.sid} Departure` : strip.route?.trim() ? "Flight plan route" : "Radar vectors";
  let text = `Cleared to ${strip.destination}, via ${via}, Initial climb ${strip.cleared_altitude ?? ""} feet, Squawk ${strip.assigned_squawk ?? ""}.`;
  if (straightAhead) text += " After departure, Climb straight ahead.";
  else if (!strip.sid && strip.heading && strip.heading > 0) text += ` After departure, Fly heading ${String(strip.heading).padStart(3,"0")}.`;
  return text;
}
