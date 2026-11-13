import type {FrontendStrip} from "./models";
import type {BoardDefinition} from "./sessions";

export interface StripAnnotations {
  review_required?: boolean;
  notes: string; deicing: boolean; rea: string | null; ata: string | null;
  atd: string; endurance_pob: string; qnh: string; rnav: string; vfr: boolean;
}
export interface StripBoardState {
 created_at?:string;

  manual_admission?: boolean;
  atd_manual?: boolean;
  annotations: StripAnnotations; removed_at: string | null; removal_reason: string; local: boolean;
}
export interface StripRevision { strip_id: number; version: number; definition_revision: string }
export interface StripAnnotationEvent extends StripRevision {
  type: "strip.annotate"; callsign: string; field: keyof StripAnnotations; value: string; enabled: boolean;
}
export interface StripFieldEdit {
  route?: string; heading?: number; altitude?: number; sid?: string; runway?: string;
  assigned_squawk?: string; stand?: string; eobt?: string; remarks?: string; aircraft_type?: string;
}
export function stripRevision(strip: FrontendStrip | undefined, definition: BoardDefinition | null | undefined): StripRevision | null {
  return strip?.id && definition?.board_state?.revisioned && definition.definition_revision
    ? {strip_id: strip.id, version: strip.version, definition_revision: definition.definition_revision} : null;
}
export function paperClock(stamp: string | null | undefined): string {
  return stamp ? stamp.slice(11, 16).replace(":", "") : "";
}

export interface LocalStripCreation {
 callsign:string; origin:string; destination:string; aircraft_type:string; route:string;
 registration:string; requested_altitude:number; runway:string; notes:string;
}
export interface ConnectedFlightCandidate {
 callsign:string;cid:string;logon_time:string;source_revision:string;origin:string;destination:string;aircraft_type:string;
}
export interface StripCreateLocalEvent extends LocalStripCreation {type:"strip.create-local";definition_revision:string}
export interface FindConnectedFlightsEvent {type:"flight.find-connected";definition_revision:string;request_id:string;query:string}
export interface StripAdmitConnectedEvent {type:"strip.admit-connected";definition_revision:string;flight:ConnectedFlightCandidate}
export interface ConnectedFlightSearchResult {type:import("./models").EventType.FrontendConnectedFlightResults;request_id:string;flights:ConnectedFlightCandidate[]}

export interface StripVisibilityEvent extends StripRevision {type:"strip.remove"|"strip.restore";callsign:string}
