package frontend

import "FlightStrips/internal/coordinationrequest"

const (
	AMANMoveFlightType            EventType = "aman.move_flight"
	AMANLockFlightType            EventType = "aman.lock_flight"
	AMANUnlockFlightType          EventType = "aman.unlock_flight"
	AMANDesequenceFlightType      EventType = "aman.desequence_flight"
	AMANResumeFlightType          EventType = "aman.resume_flight"
	AMANRemoveFlightType          EventType = "aman.remove_flight"
	AMANSetRateType               EventType = "aman.set_rate"
	AMANSelectRunwayGroupType     EventType = "aman.select_runway_group"
	AMANSetActiveRunwayGroupsType EventType = "aman.set_active_runway_groups"
	AMANAcceptTETAType            EventType = "aman.accept_teta"
	AMANKeepFPLETAType            EventType = "aman.keep_fpl_eta"
	AMANSetManualETAType          EventType = "aman.set_manual_eta"
	AMANResetTETAOverrideType     EventType = "aman.reset_teta_override"
	AMANSetManualFeederETAType    EventType = "aman.set_manual_feeder_eta"
	AMANResetManualFeederETAType  EventType = "aman.reset_manual_feeder_eta"
	AMANRecomputeFlightType       EventType = "aman.recompute_flight"
	AMANChangeRunwayType          EventType = "aman.change_runway"
	AMANReportGoAroundType        EventType = "aman.report_go_around"
	AMANConfirmGoAroundType       EventType = "aman.confirm_go_around"
	AMANRejectGoAroundType        EventType = "aman.reject_go_around"
	AMANCreateGapType             EventType = "aman.create_gap"
	AMANRemoveGapType             EventType = "aman.remove_gap"
	AMANPlaceFlightAtTimeType     EventType = "aman.place_flight_at_time"
	AMANSubmitCoordinationType    EventType = "aman.submit_coordination_request"
	AMANCoordinationStateType     EventType = "aman.coordination_state"
)

type AMANCommandMeta struct {
	CommandID        string `json:"command_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
}

type AMANMoveFlightRequest struct {
	AMANCommandMeta
	FlightID       string  `json:"flight_id"`
	RunwayGroupID  string  `json:"runway_group_id"`
	BeforeFlightID *string `json:"before_flight_id,omitempty"`
	AfterFlightID  *string `json:"after_flight_id,omitempty"`
}

type AMANFlightRequest struct {
	AMANCommandMeta
	FlightID string `json:"flight_id"`
}

type AMANChangeRunwayRequest struct {
	AMANCommandMeta
	FlightID      string `json:"flight_id"`
	RunwayGroupID string `json:"runway_group_id"`
}

type AMANSetRateRequest struct {
	AMANCommandMeta
	RunwayGroupID   string `json:"runway_group_id"`
	ArrivalsPerHour uint32 `json:"arrivals_per_hour"`
	EffectiveAt     string `json:"effective_at"`
}

type AMANSelectRunwayGroupRequest struct {
	AMANCommandMeta
	RunwayGroupID string `json:"runway_group_id"`
	EffectiveAt   string `json:"effective_at"`
}

type AMANSetActiveRunwayGroupsRequest struct {
	AMANCommandMeta
	RunwayGroupIDs []string `json:"runway_group_ids"`
}

type AMANSetManualETARequest struct {
	AMANCommandMeta
	FlightID  string `json:"flight_id"`
	ManualETA string `json:"manual_eta"`
}

type AMANSetManualFeederETARequest struct {
	AMANCommandMeta
	FlightID  string `json:"flight_id"`
	FeederETA string `json:"feeder_eta"`
}

type AMANReportGoAroundRequest struct {
	AMANCommandMeta
	FlightID   string `json:"flight_id"`
	DetectedAt string `json:"detected_at"`
}

type AMANGoAroundDecisionRequest struct {
	AMANCommandMeta
	FlightID  string `json:"flight_id"`
	EpisodeID string `json:"episode_id"`
}

type AMANCreateGapRequest struct {
	AMANCommandMeta
	RunwayGroupID string  `json:"runway_group_id"`
	Start         string  `json:"start"`
	End           *string `json:"end,omitempty"`
	SlotCount     *uint32 `json:"slot_count,omitempty"`
	Label         string  `json:"label"`
}

type AMANRemoveGapRequest struct {
	AMANCommandMeta
	RunwayGroupID string `json:"runway_group_id"`
	GapID         string `json:"gap_id"`
}

type AMANPlaceFlightAtTimeRequest struct {
	AMANCommandMeta
	FlightID      string `json:"flight_id"`
	RunwayGroupID string `json:"runway_group_id"`
	SlotTime      string `json:"slot_time"`
	AllowGap      *bool  `json:"allow_gap"`
}

type AMANSubmitCoordinationRequest struct {
	AMANCommandMeta
	FlightID  string `json:"flight_id"`
	Kind      string `json:"kind"`
	Route     string `json:"route,omitempty"`
	DirectTo  string `json:"direct_to,omitempty"`
	Requested string `json:"requested,omitempty"`
}

type AMANSubmitCoordinationMessage struct {
	Type    EventType                     `json:"type"`
	Version int                           `json:"version"`
	Data    AMANSubmitCoordinationRequest `json:"data"`
}

type AMANCoordinationStateEvent struct {
	Type     EventType                     `json:"type"`
	Version  int                           `json:"version"`
	Revision uint64                        `json:"revision"`
	Requests []coordinationrequest.Request `json:"requests"`
}

func (e AMANCoordinationStateEvent) Marshal() ([]byte, error) { return marshall(e) }
func (AMANCoordinationStateEvent) GetType() EventType         { return AMANCoordinationStateType }

type AMANMoveFlightMessage struct {
	Type    EventType             `json:"type"`
	Version int                   `json:"version"`
	Data    AMANMoveFlightRequest `json:"data"`
}

type AMANFlightMessage struct {
	Type    EventType         `json:"type"`
	Version int               `json:"version"`
	Data    AMANFlightRequest `json:"data"`
}

type AMANChangeRunwayMessage struct {
	Type    EventType               `json:"type"`
	Version int                     `json:"version"`
	Data    AMANChangeRunwayRequest `json:"data"`
}

type AMANSetRateMessage struct {
	Type    EventType          `json:"type"`
	Version int                `json:"version"`
	Data    AMANSetRateRequest `json:"data"`
}

type AMANSelectRunwayGroupMessage struct {
	Type    EventType                    `json:"type"`
	Version int                          `json:"version"`
	Data    AMANSelectRunwayGroupRequest `json:"data"`
}

type AMANSetActiveRunwayGroupsMessage struct {
	Type    EventType                        `json:"type"`
	Version int                              `json:"version"`
	Data    AMANSetActiveRunwayGroupsRequest `json:"data"`
}

type AMANSetManualETAMessage struct {
	Type    EventType               `json:"type"`
	Version int                     `json:"version"`
	Data    AMANSetManualETARequest `json:"data"`
}

type AMANSetManualFeederETAMessage struct {
	Type    EventType                     `json:"type"`
	Version int                           `json:"version"`
	Data    AMANSetManualFeederETARequest `json:"data"`
}

type AMANReportGoAroundMessage struct {
	Type    EventType                 `json:"type"`
	Version int                       `json:"version"`
	Data    AMANReportGoAroundRequest `json:"data"`
}

type AMANGoAroundDecisionMessage struct {
	Type    EventType                   `json:"type"`
	Version int                         `json:"version"`
	Data    AMANGoAroundDecisionRequest `json:"data"`
}

type AMANCreateGapMessage struct {
	Type    EventType            `json:"type"`
	Version int                  `json:"version"`
	Data    AMANCreateGapRequest `json:"data"`
}

type AMANRemoveGapMessage struct {
	Type    EventType            `json:"type"`
	Version int                  `json:"version"`
	Data    AMANRemoveGapRequest `json:"data"`
}

type AMANPlaceFlightAtTimeMessage struct {
	Type    EventType                    `json:"type"`
	Version int                          `json:"version"`
	Data    AMANPlaceFlightAtTimeRequest `json:"data"`
}
