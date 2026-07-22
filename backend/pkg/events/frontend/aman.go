package frontend

import (
	"fmt"
	"time"

	"FlightStrips/internal/aman"
)

const AMANWireVersion = 1

const (
	AMANStateType           EventType = "aman.state"
	AMANCommandRejectedType EventType = "aman.command_rejected"
)

// AMANStateEvent is the only frontend AMAN state event. Data is a complete
// replacement; the wire contract has no patch, gap-repair, or subscription
// messages.
type AMANStateEvent struct {
	Version int       `json:"version"`
	Data    AMANState `json:"data"`
}

func (e AMANStateEvent) Marshal() ([]byte, error) { return marshall(e) }
func (AMANStateEvent) GetType() EventType         { return AMANStateType }

type AMANState struct {
	Airport         string              `json:"airport"`
	Revision        uint64              `json:"revision"`
	GeneratedAt     string              `json:"generated_at"`
	PolicyVersion   string              `json:"policy_version"`
	EffectiveMode   string              `json:"effective_mode"`
	Authoritative   bool                `json:"authoritative"`
	Flights         []AMANFlight        `json:"flights"`
	RunwayGroups    []AMANRunwayGroup   `json:"runway_groups"`
	TechnicalHealth AMANTechnicalHealth `json:"technical_health"`
}

type AMANFlight struct {
	FlightID        string           `json:"flight_id"`
	Callsign        string           `json:"callsign"`
	LifecycleState  string           `json:"lifecycle_state"`
	DataStatus      string           `json:"data_status"`
	RunwayGroupID   *string          `json:"runway_group_id"`
	Feeder          *string          `json:"feeder"`
	HoldingFix      *string          `json:"holding_fix"`
	HoldingFixETA   *string          `json:"holding_fix_eta"`
	RouteFact       *AMANRouteFact   `json:"route_fact"`
	RawTETA         *string          `json:"raw_teta"`
	OperationalTETA *string          `json:"operational_teta"`
	GainLossSeconds *int64           `json:"gain_loss_seconds"`
	FreezeReason    string           `json:"freeze_reason"`
	FrozenAt        *string          `json:"frozen_at"`
	Confidence      *string          `json:"confidence"`
	Provenance      *AMANProvenance  `json:"provenance"`
	InputAgeSeconds *int64           `json:"input_age_seconds"`
	GeometryVersion *string          `json:"geometry_version"`
	GeometryDigest  *string          `json:"geometry_digest"`
	DistanceToGoNM  *float64         `json:"distance_to_go_nm"`
	Slot            *AMANSlot        `json:"slot"`
	Order           *int             `json:"order"`
	ETAReview       *AMANETAReview   `json:"eta_review"`
	QueueOffers     []AMANQueueOffer `json:"queue_offers"`
}

type AMANRouteFact struct {
	ID         string `json:"id"`
	Fix        string `json:"fix"`
	ObservedAt string `json:"observed_at"`
	State      string `json:"state"`
}

type AMANProvenance struct {
	ModelVersion         string   `json:"model_version"`
	ConfigVersion        string   `json:"config_version"`
	PerformanceProfileID *string  `json:"performance_profile_id"`
	WeatherSource        *string  `json:"weather_source"`
	Sources              []string `json:"sources"`
}

type AMANSlot struct {
	Time          string `json:"time"`
	RunwayGroupID string `json:"runway_group_id"`
	Sequence      int    `json:"sequence"`
	Revision      uint64 `json:"revision"`
	Reason        string `json:"reason"`
}

type AMANETAReview struct {
	Status                    string  `json:"status"`
	CreatedAt                 string  `json:"created_at"`
	DeadlineAt                string  `json:"deadline_at"`
	ResolvedAt                *string `json:"resolved_at"`
	Actor                     *string `json:"actor"`
	Note                      *string `json:"note"`
	InitialBaselineTETA       string  `json:"initial_baseline_teta"`
	CalculatedOperationalTETA string  `json:"calculated_operational_teta"`
	SelectedTETA              string  `json:"selected_teta"`
	ManualTETA                *string `json:"manual_teta"`
}

type AMANQueueOffer struct {
	FlightID        string   `json:"flight_id"`
	RunwayGroupID   string   `json:"runway_group_id"`
	CandidateSlot   AMANSlot `json:"candidate_slot"`
	QueuePosition   int      `json:"queue_position"`
	ExpiresAt       string   `json:"expires_at"`
	AirportRevision uint64   `json:"airport_revision"`
	Reason          string   `json:"reason"`
}

// AMANRunwayGroup is intentionally limited to state persisted on
// aman.AirportState. Rate schedules are not currently part of that persisted
// aggregate and must not be borrowed from sequence-engine policy types.
type AMANRunwayGroup struct {
	ID string `json:"id"`
}

type AMANTechnicalHealth struct {
	Status           string              `json:"status"`
	Ready            bool                `json:"ready"`
	BlockedReasons   []string            `json:"blocked_reasons"`
	VATSIM           AMANComponentHealth `json:"vatsim"`
	Navigation       AMANComponentHealth `json:"navigation"`
	Weather          AMANComponentHealth `json:"weather"`
	Repository       AMANComponentHealth `json:"repository"`
	Predictor        AMANComponentHealth `json:"predictor"`
	ReplayValidation AMANComponentHealth `json:"replay_validation"`
}

type AMANComponentHealth struct {
	Status     string   `json:"status"`
	Reason     *string  `json:"reason"`
	UpdatedAt  *string  `json:"updated_at"`
	AgeSeconds *float64 `json:"age_seconds"`
}

type AMANCommandRejectedEvent struct {
	Version int                  `json:"version"`
	Data    AMANCommandRejection `json:"data"`
}

func (e AMANCommandRejectedEvent) Marshal() ([]byte, error) { return marshall(e) }
func (AMANCommandRejectedEvent) GetType() EventType         { return AMANCommandRejectedType }

type AMANCommandRejection struct {
	CommandID       string `json:"command_id"`
	Code            string `json:"code"`
	Message         string `json:"message"`
	CurrentRevision uint64 `json:"current_revision"`
	Retryable       bool   `json:"retryable"`
}

// NewAMANStateEvent projects one coherent domain state and its matching
// technical-health reading into the transport-only V1 DTO. Callers must obtain
// both values atomically; this mapper rejects mismatched desired/effective mode.
func NewAMANStateEvent(state aman.AirportState, effectiveMode aman.EffectiveRolloutMode, health aman.TechnicalHealth) (AMANStateEvent, error) {
	if err := state.Validate(); err != nil {
		return AMANStateEvent{}, fmt.Errorf("map AMAN state event: %w", err)
	}
	if health.EffectiveMode != effectiveMode {
		return AMANStateEvent{}, fmt.Errorf("map AMAN state event: effective mode and health must belong to the same state")
	}
	if health.DesiredMode != state.Mode {
		return AMANStateEvent{}, fmt.Errorf("map AMAN state event: desired mode does not match persisted state")
	}

	generatedAt, err := aman.FormatTime(state.GeneratedAt)
	if err != nil {
		return AMANStateEvent{}, err
	}
	technicalHealth, err := mapAMANTechnicalHealth(health)
	if err != nil {
		return AMANStateEvent{}, fmt.Errorf("map AMAN technical health: %w", err)
	}
	data := AMANState{
		Airport: state.Airport, Revision: uint64(state.Revision), GeneratedAt: generatedAt,
		PolicyVersion: state.PolicyVersion, EffectiveMode: string(effectiveMode), Authoritative: state.Authoritative,
		Flights: make([]AMANFlight, len(state.Flights)), RunwayGroups: make([]AMANRunwayGroup, len(state.RunwayGroups)),
		TechnicalHealth: technicalHealth,
	}
	for i := range state.Flights {
		data.Flights[i], err = mapAMANFlight(state.GeneratedAt, state.Flights[i])
		if err != nil {
			return AMANStateEvent{}, fmt.Errorf("map AMAN flight %q: %w", state.Flights[i].ID, err)
		}
	}
	for i, group := range state.RunwayGroups {
		data.RunwayGroups[i] = AMANRunwayGroup{ID: string(group.ID)}
	}
	return AMANStateEvent{Version: AMANWireVersion, Data: data}, nil
}

func NewAMANCommandRejectedEvent(commandID string, currentRevision aman.SequenceRevision, domainError *aman.DomainError, retryable bool) (AMANCommandRejectedEvent, error) {
	if commandID == "" || domainError == nil || !domainError.Class.Valid() {
		return AMANCommandRejectedEvent{}, fmt.Errorf("map AMAN command rejection: command ID and stable domain error are required")
	}
	return AMANCommandRejectedEvent{Version: AMANWireVersion, Data: AMANCommandRejection{
		CommandID: commandID, Code: string(domainError.Class), Message: domainError.Message,
		CurrentRevision: uint64(currentRevision), Retryable: retryable,
	}}, nil
}

func mapAMANFlight(generatedAt time.Time, flight aman.AMANFlight) (AMANFlight, error) {
	result := AMANFlight{
		FlightID: string(flight.ID), Callsign: flight.CurrentCallsign, LifecycleState: string(flight.State),
		DataStatus: string(flight.DataStatus), RunwayGroupID: stringPointer(flight.SelectedRunwayGroup),
		Feeder: cloneString(flight.SelectedFeeder), HoldingFix: cloneString(flight.SelectedHolding),
		FreezeReason: string(flight.FreezeReason), Order: cloneInt(flight.Order), QueueOffers: make([]AMANQueueOffer, len(flight.QueueOffers)),
	}
	var err error
	if result.FrozenAt, err = formatOptionalTime(flight.FrozenAt); err != nil {
		return AMANFlight{}, err
	}
	if flight.ActiveRouteFact != nil {
		observedAt, formatErr := aman.FormatTime(flight.ActiveRouteFact.ObservedAt)
		if formatErr != nil {
			return AMANFlight{}, formatErr
		}
		state := flight.ActiveRouteFact.State
		if state == "" {
			state = aman.RouteFactActive
		}
		result.RouteFact = &AMANRouteFact{ID: flight.ActiveRouteFact.ID, Fix: flight.ActiveRouteFact.Fix, ObservedAt: observedAt, State: string(state)}
	}
	if flight.Prediction != nil {
		prediction := flight.Prediction
		if result.RawTETA, err = formatOptionalValue(prediction.RawTETA); err != nil {
			return AMANFlight{}, err
		}
		if result.OperationalTETA, err = formatOptionalValue(prediction.OperationalTETA); err != nil {
			return AMANFlight{}, err
		}
		if result.HoldingFixETA, err = formatOptionalTime(prediction.HoldingFixETA); err != nil {
			return AMANFlight{}, err
		}
		confidence := string(prediction.Confidence)
		result.Confidence = &confidence
		result.Provenance = &AMANProvenance{ModelVersion: prediction.ModelVersion, ConfigVersion: prediction.ConfigVersion, PerformanceProfileID: cloneString(prediction.PerformanceProfileID), WeatherSource: cloneString(prediction.WeatherSource), Sources: append([]string(nil), prediction.Sources...)}
		age := generatedAt.Sub(prediction.InputObservedAt)
		if age < 0 {
			return AMANFlight{}, fmt.Errorf("prediction input time follows state generation")
		}
		ageSeconds, secondsErr := aman.WholeSeconds(age)
		if secondsErr != nil {
			return AMANFlight{}, secondsErr
		}
		result.InputAgeSeconds = &ageSeconds
		result.GeometryVersion = stringValue(prediction.DatasetVersion)
		result.GeometryDigest = stringValue(prediction.GeometryDigest)
		result.DistanceToGoNM = cloneFloat(prediction.DistanceToGoNM)
	}
	if flight.Slot != nil {
		mapped, mapErr := mapAMANSlot(*flight.Slot)
		if mapErr != nil {
			return AMANFlight{}, mapErr
		}
		result.Slot = &mapped
		if flight.Prediction != nil {
			seconds, secondsErr := aman.WholeSeconds(flight.Prediction.OperationalTETA.Sub(flight.Slot.Time))
			if secondsErr != nil {
				return AMANFlight{}, secondsErr
			}
			result.GainLossSeconds = &seconds
		}
	}
	if flight.ETAReview != nil {
		result.ETAReview, err = mapAMANETAReview(*flight.ETAReview)
		if err != nil {
			return AMANFlight{}, err
		}
	}
	for i, offer := range flight.QueueOffers {
		candidate, mapErr := mapAMANSlot(offer.CandidateSlot)
		if mapErr != nil {
			return AMANFlight{}, mapErr
		}
		expiresAt, formatErr := aman.FormatTime(offer.ExpiresAt)
		if formatErr != nil {
			return AMANFlight{}, formatErr
		}
		result.QueueOffers[i] = AMANQueueOffer{FlightID: string(offer.FlightID), RunwayGroupID: string(offer.RunwayGroupID), CandidateSlot: candidate, QueuePosition: offer.QueuePosition, ExpiresAt: expiresAt, AirportRevision: uint64(offer.AirportRevision), Reason: string(offer.Reason)}
	}
	return result, nil
}

func mapAMANSlot(slot aman.Slot) (AMANSlot, error) {
	value, err := aman.FormatTime(slot.Time)
	return AMANSlot{Time: value, RunwayGroupID: string(slot.RunwayGroupID), Sequence: slot.Sequence, Revision: uint64(slot.Revision), Reason: slot.Reason}, err
}

func mapAMANETAReview(review aman.ETAReview) (*AMANETAReview, error) {
	createdAt, err := aman.FormatTime(review.CreatedAt)
	if err != nil {
		return nil, err
	}
	deadlineAt, err := aman.FormatTime(review.DeadlineAt)
	if err != nil {
		return nil, err
	}
	initial, err := aman.FormatTime(review.InitialBaselineTETA)
	if err != nil {
		return nil, err
	}
	calculated, err := aman.FormatTime(review.CalculatedOperationalTETA)
	if err != nil {
		return nil, err
	}
	selected, err := aman.FormatTime(review.SelectedTETA)
	if err != nil {
		return nil, err
	}
	resolved, err := formatOptionalTime(review.ResolvedAt)
	if err != nil {
		return nil, err
	}
	manual, err := formatOptionalTime(review.ManualTETA)
	if err != nil {
		return nil, err
	}
	return &AMANETAReview{Status: string(review.Status), CreatedAt: createdAt, DeadlineAt: deadlineAt, ResolvedAt: resolved, Actor: cloneString(review.Actor), Note: cloneString(review.Note), InitialBaselineTETA: initial, CalculatedOperationalTETA: calculated, SelectedTETA: selected, ManualTETA: manual}, nil
}

func mapAMANTechnicalHealth(value aman.TechnicalHealth) (AMANTechnicalHealth, error) {
	blockedReasons := make([]string, len(value.BlockedReasons))
	copy(blockedReasons, value.BlockedReasons)
	components := []*aman.ComponentHealth{&value.VATSIM, &value.Navigation, &value.Weather, &value.Repository, &value.Predictor, &value.ReplayValidation}
	mapped := make([]AMANComponentHealth, len(components))
	for index, component := range components {
		var err error
		mapped[index], err = mapAMANComponentHealth(*component, value.Status)
		if err != nil {
			return AMANTechnicalHealth{}, err
		}
	}
	return AMANTechnicalHealth{
		Status: string(value.Status), Ready: value.Ready, BlockedReasons: blockedReasons,
		VATSIM: mapped[0], Navigation: mapped[1], Weather: mapped[2], Repository: mapped[3], Predictor: mapped[4], ReplayValidation: mapped[5],
	}, nil
}

func mapAMANComponentHealth(value aman.ComponentHealth, aggregateStatus aman.HealthStatus) (AMANComponentHealth, error) {
	reason := cloneNonEmptyString(value.Reason)
	updatedAt, err := formatOptionalTime(value.UpdatedAt)
	if err != nil {
		return AMANComponentHealth{}, err
	}
	status := value.Status
	if status == "" {
		status = aman.HealthUnavailable
		if aggregateStatus == aman.HealthDisabled {
			status = aman.HealthDisabled
		}
	}
	return AMANComponentHealth{Status: string(status), Reason: reason, UpdatedAt: updatedAt, AgeSeconds: cloneFloat(value.AgeSeconds)}, nil
}

func formatOptionalValue(value time.Time) (*string, error) {
	result, err := aman.FormatTime(value)
	return &result, err
}
func formatOptionalTime(value *time.Time) (*string, error) {
	if value == nil {
		return nil, nil
	}
	return formatOptionalValue(*value)
}
func stringPointer[T ~string](value *T) *string {
	if value == nil {
		return nil
	}
	result := string(*value)
	return &result
}
func stringValue(value string) *string { result := value; return &result }
func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}
func cloneNonEmptyString(value string) *string {
	if value == "" {
		return nil
	}
	result := value
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
