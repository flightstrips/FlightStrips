import {getApiUrl} from "@/lib/api-url";

export interface AMANFlightDetail {
  airport: string;
  revision: number;
  generated_at: string;
  flight: {
    id: string;
    callsign: string;
    lifecycle_state: string;
    data_status: string;
    runway_group_id: string | null;
    feeder: string | null;
    star: string | null;
    holding_fix: string | null;
    aircraft_type: string | null;
    wake_category: string | null;
    filed_route: string | null;
  };
  position: AMANPosition | null;
  calculation: AMANCalculation | null;
  filed_route_geometry: AMANFiledRouteGeometry | null;
  teta_basis: AMANTETABasis | null;
  slot_basis: AMANSlotBasis | null;
}

export interface AMANPosition {
  latitude: number;
  longitude: number;
  altitude_feet: number | null;
  groundspeed_knots: number | null;
  track_true_degrees: number | null;
  observed_at: string;
}

export interface AMANCalculation {
  no_wind_duration_seconds: number;
  duration_seconds: number;
  distance_to_go_nm: number | null;
  legs: AMANCalculationLeg[];
  segments: AMANCalculationSegment[];
}

export interface AMANCalculationLeg {
  id: string;
  from: string;
  to: string;
  start_latitude: number;
  start_longitude: number;
  end_latitude: number;
  end_longitude: number;
  distance_nm: number;
  course_true_degrees: number;
  no_wind_duration_seconds: number | null;
  duration_seconds: number;
}

export interface AMANCalculationSegment {
  route_leg_index: number;
  pre_tod: boolean;
  phase_id: string;
  phase_name: string;
  phase_formula: string;
  distance_nm: number;
  course_true_degrees: number;
  start_altitude_feet: number;
  end_altitude_feet: number;
  altitude_feet: number;
  indicated_airspeed_knots: number | null;
  no_wind_groundspeed_knots: number;
  groundspeed_knots: number;
  tailwind_knots: number | null;
  no_wind_duration_seconds: number;
  duration_seconds: number;
}

export interface AMANFiledRouteGeometry {
  legs: AMANRouteGeometryLeg[];
  reasons: string[];
}

export interface AMANRouteGeometryLeg {
  id: string;
  from: string;
  to: string;
  start_latitude: number;
  start_longitude: number;
  end_latitude: number;
  end_longitude: number;
}

export interface AMANTETABasis {
  raw_teta: string;
  raw_reta: string;
  operational_teta: string;
  generated_at: string;
  input_observed_at: string;
  operational_reason: string;
  freeze_reason: string;
  frozen_at: string | null;
  confidence: string;
  model_version: string;
  config_version: string;
  performance_profile_id: string | null;
  weather_source: string | null;
  sources: string[];
  degradation_reason: string | null;
  raw_samples: Array<{teta: string; generated_at: string}>;
  baseline: {arrival_at: string; airborne_sensed_at: string; source: string} | null;
  eta_review: {
    status: string;
    initial_baseline_teta: string;
    calculated_operational_teta: string;
    selected_teta: string;
    manual_teta: string | null;
  } | null;
}

export interface AMANSlotBasis {
  time: string;
  runway_group_id: string;
  reason: string;
  sequence: number;
  revision: number;
  rate_per_hour: number;
  rate_effective_at: string | null;
  previous_flight: {callsign: string; slot_time: string} | null;
  frozen: boolean;
}

export async function fetchAMANFlightDetail(token: string, airport: string, flightID: string, signal?: AbortSignal): Promise<AMANFlightDetail> {
  const response = await fetch(getApiUrl(`/api/aman/airports/${encodeURIComponent(airport)}/flights/${encodeURIComponent(flightID)}/detail`), {
    headers: {Authorization: `Bearer ${token}`}, signal,
  });
  if (!response.ok) {
    const payload = await response.json().catch(() => null) as {error?: string} | null;
    throw new Error(payload?.error ?? `AMAN detail request failed (${response.status})`);
  }
  return await response.json() as AMANFlightDetail;
}
