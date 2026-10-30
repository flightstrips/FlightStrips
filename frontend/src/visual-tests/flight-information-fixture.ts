import type {AMANFlightDetail} from "@/api/aman-detail";

export const flightInformationDetail: AMANFlightDetail = {
  airport: "EKCH", revision: 17, generated_at: "2026-10-20T21:00:00Z",
  flight: {
    callsign: "SAS1778", origin: "EKVG", destination: "EKCH", lifecycle_state: "stable", data_status: "fresh",
    runway_group_id: "ARRIVAL-22L", feeder: "ROSBI", star: "ROSBI1A", feeder_fix: "TNO",
    feeder_eta: "2026-10-20T21:29:03Z", derived_feeder_eta: "2026-10-20T21:29:03Z", feeder_sta: null,
    direct_to: "TNO", holding_fix: "ROSBI", aircraft_type: "A32N", wake_category: "M", filed_route: null,
  },
  position: null, calculation: {no_wind_duration_seconds: 1200, duration_seconds: 1200, distance_to_go_nm: 35, legs: [], segments: []}, filed_route_geometry: null,
  initial_timing: {feeder_eta: "2026-10-20T21:27:37Z", feeder_sta: null, runway_eta: "2026-10-20T21:27:37Z", runway_sta: null},
  star_route: ["CH626", "ABEGI", "EXTAR", "CH610", "CH2LF", "EKCH"].map((fix, index) => ({fix, eta: `2026-10-20T09:${[2, 4, 6, 7, 8, 9][index].toString().padStart(2, "0")}:00Z`})),
  teta_basis: {
    raw_teta: "2026-10-20T21:30:00Z", raw_reta: "2026-10-20T21:27:37Z", operational_teta: "2026-10-20T21:29:03Z",
    generated_at: "2026-10-20T21:00:00Z", input_observed_at: "2026-10-20T21:00:00Z", operational_reason: "predicted",
    freeze_reason: "none", frozen_at: null, confidence: "high", model_version: "preview", config_version: "preview",
    prediction_basis: "performance_wind", performance_profile_id: "A32N", weather_source: null, sources: [],
    degradation_reason: null, raw_samples: [], baseline: null, eta_review: null,
  },
  slot_basis: {time: "2026-10-20T21:40:00Z", runway_group_id: "ARRIVAL-22L", reason: "rate_wtc", sequence: 16, revision: 17, rate_per_hour: 30, rate_effective_at: null, previous_flight: null, frozen: false, infeasible: false},
  holding_plan: {holding_entry_time: "2026-10-20T21:20:00Z", approach_release_time: "2026-10-20T21:31:30Z", expected_holding_seconds: 690, post_holding_transit_seconds: 510},
};

