// Package webapi exposes on-demand AMAN inspection data. The realtime AMAN
// event remains a small operational board projection; this package is the
// explicit read path for route geometry and prediction evidence.
package webapi

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/shared"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"
)

type WebAPI struct {
	auth   shared.AuthenticationService
	states aman.AirportStateReader
}

func New(auth shared.AuthenticationService, states aman.AirportStateReader) *WebAPI {
	return &WebAPI{auth: auth, states: states}
}

func (a *WebAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /aman/airports/{airport}/flights/{flightID}/detail", a.handleFlightDetail)
}

func (a *WebAPI) handleFlightDetail(w http.ResponseWriter, r *http.Request) {
	if !a.authenticate(w, r) {
		return
	}
	if a.states == nil {
		writeError(w, http.StatusServiceUnavailable, "AMAN detail is unavailable")
		return
	}
	airport := strings.ToUpper(strings.TrimSpace(r.PathValue("airport")))
	flightID := strings.TrimSpace(r.PathValue("flightID"))
	if airport == "" || flightID == "" {
		writeError(w, http.StatusBadRequest, "airport and flight ID are required")
		return
	}
	state, err := a.states.LoadAirportState(r.Context(), airport)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "AMAN state is unavailable")
		return
	}
	flight := findFlight(state.Flights, aman.FlightID(flightID))
	if flight == nil {
		writeError(w, http.StatusNotFound, "AMAN flight was not found")
		return
	}
	detail, err := mapDetail(state, *flight)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "AMAN flight detail is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (a *WebAPI) authenticate(w http.ResponseWriter, r *http.Request) bool {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer"))
	if header == "" || token == header || token == "" || a.auth == nil {
		writeError(w, http.StatusUnauthorized, "invalid authorization header")
		return false
	}
	if _, err := a.auth.Validate(token); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return false
	}
	return true
}

type flightDetail struct {
	Airport     string        `json:"airport"`
	Revision    uint64        `json:"revision"`
	GeneratedAt string        `json:"generated_at"`
	Flight      flightSummary `json:"flight"`
	Position    *position     `json:"position"`
	Calculation *calculation  `json:"calculation"`
	TETABasis   *tetaBasis    `json:"teta_basis"`
	SlotBasis   *slotBasis    `json:"slot_basis"`
}

type flightSummary struct {
	ID             string  `json:"id"`
	Callsign       string  `json:"callsign"`
	LifecycleState string  `json:"lifecycle_state"`
	DataStatus     string  `json:"data_status"`
	RunwayGroupID  *string `json:"runway_group_id"`
	Feeder         *string `json:"feeder"`
	Star           *string `json:"star"`
	HoldingFix     *string `json:"holding_fix"`
	AircraftType   *string `json:"aircraft_type"`
	WakeCategory   *string `json:"wake_category"`
}
type position struct {
	Latitude         float64  `json:"latitude"`
	Longitude        float64  `json:"longitude"`
	AltitudeFeet     *int     `json:"altitude_feet"`
	GroundspeedKnots *float64 `json:"groundspeed_knots"`
	TrackTrueDegrees *float64 `json:"track_true_degrees"`
	ObservedAt       string   `json:"observed_at"`
}
type calculation struct {
	NoWindDurationSeconds int64            `json:"no_wind_duration_seconds"`
	DurationSeconds       int64            `json:"duration_seconds"`
	DistanceToGoNM        *float64         `json:"distance_to_go_nm"`
	Legs                  []calculationLeg `json:"legs"`
}
type calculationLeg struct {
	ID                    string  `json:"id"`
	From                  string  `json:"from"`
	To                    string  `json:"to"`
	StartLatitude         float64 `json:"start_latitude"`
	StartLongitude        float64 `json:"start_longitude"`
	EndLatitude           float64 `json:"end_latitude"`
	EndLongitude          float64 `json:"end_longitude"`
	DistanceNM            float64 `json:"distance_nm"`
	CourseTrueDegrees     float64 `json:"course_true_degrees"`
	NoWindDurationSeconds *int64  `json:"no_wind_duration_seconds"`
	DurationSeconds       int64   `json:"duration_seconds"`
}
type tetaBasis struct {
	RawTETA              string      `json:"raw_teta"`
	RawRETA              string      `json:"raw_reta"`
	OperationalTETA      string      `json:"operational_teta"`
	GeneratedAt          string      `json:"generated_at"`
	InputObservedAt      string      `json:"input_observed_at"`
	OperationalReason    string      `json:"operational_reason"`
	FreezeReason         string      `json:"freeze_reason"`
	FrozenAt             *string     `json:"frozen_at"`
	Confidence           string      `json:"confidence"`
	ModelVersion         string      `json:"model_version"`
	ConfigVersion        string      `json:"config_version"`
	PerformanceProfileID *string     `json:"performance_profile_id"`
	WeatherSource        *string     `json:"weather_source"`
	Sources              []string    `json:"sources"`
	DegradationReason    *string     `json:"degradation_reason"`
	RawSamples           []rawSample `json:"raw_samples"`
	Baseline             *baseline   `json:"baseline"`
	ETAReview            *etaReview  `json:"eta_review"`
}
type rawSample struct {
	TETA        string `json:"teta"`
	GeneratedAt string `json:"generated_at"`
}
type baseline struct {
	ArrivalAt        string `json:"arrival_at"`
	AirborneSensedAt string `json:"airborne_sensed_at"`
	Source           string `json:"source"`
}
type etaReview struct {
	Status                    string  `json:"status"`
	InitialBaselineTETA       string  `json:"initial_baseline_teta"`
	CalculatedOperationalTETA string  `json:"calculated_operational_teta"`
	SelectedTETA              string  `json:"selected_teta"`
	ManualTETA                *string `json:"manual_teta"`
}
type slotBasis struct {
	Time            string         `json:"time"`
	RunwayGroupID   string         `json:"runway_group_id"`
	Reason          string         `json:"reason"`
	Sequence        int            `json:"sequence"`
	Revision        uint64         `json:"revision"`
	RatePerHour     uint32         `json:"rate_per_hour"`
	RateEffectiveAt *string        `json:"rate_effective_at"`
	PreviousFlight  *slotNeighbour `json:"previous_flight"`
	Frozen          bool           `json:"frozen"`
}
type slotNeighbour struct {
	Callsign string `json:"callsign"`
	SlotTime string `json:"slot_time"`
}

func mapDetail(state aman.AirportState, flight aman.AMANFlight) (flightDetail, error) {
	generatedAt, err := format(state.GeneratedAt)
	if err != nil {
		return flightDetail{}, err
	}
	result := flightDetail{Airport: state.Airport, Revision: uint64(state.Revision), GeneratedAt: generatedAt, Flight: flightSummary{
		ID: string(flight.ID), Callsign: flight.CurrentCallsign, LifecycleState: string(flight.State), DataStatus: string(flight.DataStatus),
		RunwayGroupID: stringPointer(flight.SelectedRunwayGroup), Feeder: cloneString(flight.SelectedFeeder), Star: cloneString(flight.SelectedFeeder), HoldingFix: cloneString(flight.SelectedHolding),
	}}
	if observation := flight.LatestObservation; observation != nil && observation.Surveillance != nil {
		result.Flight.AircraftType, result.Flight.WakeCategory = cloneString(observation.AircraftType), cloneString(observation.WakeCategory)
		observedAt, formatErr := format(*observation.Surveillance.ObservedAt)
		if formatErr != nil {
			return flightDetail{}, formatErr
		}
		result.Position = &position{Latitude: observation.Surveillance.LatitudeDegrees, Longitude: observation.Surveillance.LongitudeDegrees, AltitudeFeet: cloneInt(observation.Surveillance.AltitudeFeet), GroundspeedKnots: cloneFloat(observation.Surveillance.GroundspeedKnots), TrackTrueDegrees: cloneFloat(observation.Surveillance.TrackTrueDegrees), ObservedAt: observedAt}
	}
	if prediction := flight.Prediction; prediction != nil && prediction.Publishable {
		result.Calculation = mapCalculation(prediction)
		basis, mapErr := mapTETABasis(flight, *prediction)
		if mapErr != nil {
			return flightDetail{}, mapErr
		}
		result.TETABasis = &basis
	}
	if flight.Slot != nil {
		basis, mapErr := mapSlotBasis(state, flight)
		if mapErr != nil {
			return flightDetail{}, mapErr
		}
		result.SlotBasis = &basis
	}
	return result, nil
}

func mapCalculation(prediction *aman.Prediction) *calculation {
	if prediction.Calculation == nil {
		return nil
	}
	result := &calculation{NoWindDurationSeconds: seconds(prediction.Calculation.NoWindDuration), DurationSeconds: seconds(prediction.Calculation.Duration), DistanceToGoNM: cloneFloat(prediction.DistanceToGoNM), Legs: make([]calculationLeg, len(prediction.Calculation.Legs))}
	for i, leg := range prediction.Calculation.Legs {
		result.Legs[i] = calculationLeg{ID: leg.ID, From: leg.From, To: leg.To, StartLatitude: leg.StartLatitude, StartLongitude: leg.StartLongitude, EndLatitude: leg.EndLatitude, EndLongitude: leg.EndLongitude, DistanceNM: leg.DistanceNM, CourseTrueDegrees: leg.CourseTrueDegrees, NoWindDurationSeconds: optionalSeconds(leg.NoWindDuration), DurationSeconds: seconds(leg.Duration)}
	}
	return result
}

func mapTETABasis(flight aman.AMANFlight, prediction aman.Prediction) (tetaBasis, error) {
	raw, err := format(prediction.RawTETA)
	if err != nil {
		return tetaBasis{}, err
	}
	operational, err := format(prediction.OperationalTETA)
	if err != nil {
		return tetaBasis{}, err
	}
	generated, err := format(prediction.GeneratedAt)
	if err != nil {
		return tetaBasis{}, err
	}
	observed, err := format(prediction.InputObservedAt)
	if err != nil {
		return tetaBasis{}, err
	}
	result := tetaBasis{RawTETA: raw, OperationalTETA: operational, GeneratedAt: generated, InputObservedAt: observed, OperationalReason: string(prediction.OperationalReason), FreezeReason: string(flight.FreezeReason), RawSamples: make([]rawSample, len(flight.RawTETASamples))}
	result.Confidence, result.ModelVersion, result.ConfigVersion = string(prediction.Confidence), prediction.ModelVersion, prediction.ConfigVersion
	result.PerformanceProfileID, result.WeatherSource, result.DegradationReason = cloneString(prediction.PerformanceProfileID), cloneString(prediction.WeatherSource), cloneString(prediction.DegradationReason)
	result.Sources = slices.Clone(prediction.Sources)
	if prediction.RawRETA != nil {
		value, formatErr := format(*prediction.RawRETA)
		if formatErr != nil {
			return tetaBasis{}, formatErr
		}
		result.RawRETA = value
	}
	if flight.FrozenAt != nil {
		value, formatErr := format(*flight.FrozenAt)
		if formatErr != nil {
			return tetaBasis{}, formatErr
		}
		result.FrozenAt = &value
	}
	for i, sample := range flight.RawTETASamples {
		teta, tetaErr := format(sample.TETA)
		generatedAt, generatedErr := format(sample.GeneratedAt)
		if tetaErr != nil || generatedErr != nil {
			return tetaBasis{}, errors.Join(tetaErr, generatedErr)
		}
		result.RawSamples[i] = rawSample{TETA: teta, GeneratedAt: generatedAt}
	}
	if flight.ArrivalBaseline != nil {
		arrival, arrivalErr := format(flight.ArrivalBaseline.ArrivalAt)
		sensed, sensedErr := format(flight.ArrivalBaseline.AirborneSensedAt)
		if arrivalErr != nil || sensedErr != nil {
			return tetaBasis{}, errors.Join(arrivalErr, sensedErr)
		}
		result.Baseline = &baseline{ArrivalAt: arrival, AirborneSensedAt: sensed, Source: string(flight.ArrivalBaseline.Source)}
	}
	if flight.ETAReview != nil {
		initial, initialErr := format(flight.ETAReview.InitialBaselineTETA)
		calculated, calculatedErr := format(flight.ETAReview.CalculatedOperationalTETA)
		selected, selectedErr := format(flight.ETAReview.SelectedTETA)
		if initialErr != nil || calculatedErr != nil || selectedErr != nil {
			return tetaBasis{}, errors.Join(initialErr, calculatedErr, selectedErr)
		}
		review := &etaReview{Status: string(flight.ETAReview.Status), InitialBaselineTETA: initial, CalculatedOperationalTETA: calculated, SelectedTETA: selected}
		if flight.ETAReview.ManualTETA != nil {
			manual, manualErr := format(*flight.ETAReview.ManualTETA)
			if manualErr != nil {
				return tetaBasis{}, manualErr
			}
			review.ManualTETA = &manual
		}
		result.ETAReview = review
	}
	return result, nil
}

func mapSlotBasis(state aman.AirportState, flight aman.AMANFlight) (slotBasis, error) {
	slot := *flight.Slot
	timeValue, err := format(slot.Time)
	if err != nil {
		return slotBasis{}, err
	}
	result := slotBasis{Time: timeValue, RunwayGroupID: string(slot.RunwayGroupID), Reason: slot.Reason, Sequence: slot.Sequence, Revision: uint64(slot.Revision), Frozen: flight.FreezeReason != aman.FreezeNone}
	for _, group := range state.RunwayGroups {
		if group.ID == slot.RunwayGroupID {
			result.RatePerHour = group.ActiveRatePerHour
			if group.RateEffectiveAt != nil {
				value, formatErr := format(*group.RateEffectiveAt)
				if formatErr != nil {
					return slotBasis{}, formatErr
				}
				result.RateEffectiveAt = &value
			}
			break
		}
	}
	prior := slices.Clone(state.Flights)
	slices.SortFunc(prior, func(left, right aman.AMANFlight) int {
		if left.Slot == nil {
			return 1
		}
		if right.Slot == nil {
			return -1
		}
		return left.Slot.Sequence - right.Slot.Sequence
	})
	for _, candidate := range prior {
		if candidate.Slot != nil && candidate.Slot.RunwayGroupID == slot.RunwayGroupID && candidate.Slot.Sequence == slot.Sequence-1 {
			candidateTime, formatErr := format(candidate.Slot.Time)
			if formatErr != nil {
				return slotBasis{}, formatErr
			}
			result.PreviousFlight = &slotNeighbour{Callsign: candidate.CurrentCallsign, SlotTime: candidateTime}
			break
		}
	}
	return result, nil
}

func findFlight(flights []aman.AMANFlight, id aman.FlightID) *aman.AMANFlight {
	for i := range flights {
		if flights[i].ID == id {
			return &flights[i]
		}
	}
	return nil
}
func format(value time.Time) (string, error) { return aman.FormatTime(value) }
func seconds(value time.Duration) int64      { return value.Round(time.Second).Milliseconds() / 1000 }
func optionalSeconds(value time.Duration) *int64 {
	if value <= 0 {
		return nil
	}
	result := seconds(value)
	return &result
}
func stringPointer(value *aman.RunwayGroupID) *string {
	if value == nil {
		return nil
	}
	result := string(*value)
	return &result
}
func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}
func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}
func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
