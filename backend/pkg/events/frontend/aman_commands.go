package frontend

const (
	AMANMoveFlightType        EventType = "aman.move_flight"
	AMANLockFlightType        EventType = "aman.lock_flight"
	AMANUnlockFlightType      EventType = "aman.unlock_flight"
	AMANSetRateType           EventType = "aman.set_rate"
	AMANAcceptTETAType        EventType = "aman.accept_teta"
	AMANKeepFPLETAType        EventType = "aman.keep_fpl_eta"
	AMANSetManualETAType      EventType = "aman.set_manual_eta"
	AMANResetTETAOverrideType EventType = "aman.reset_teta_override"
	AMANReportGoAroundType    EventType = "aman.report_go_around"
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

type AMANSetRateRequest struct {
	AMANCommandMeta
	RunwayGroupID   string `json:"runway_group_id"`
	ArrivalsPerHour uint32 `json:"arrivals_per_hour"`
	EffectiveAt     string `json:"effective_at"`
}

type AMANSetManualETARequest struct {
	AMANCommandMeta
	FlightID  string `json:"flight_id"`
	ManualETA string `json:"manual_eta"`
}

type AMANReportGoAroundRequest struct {
	AMANCommandMeta
	FlightID   string `json:"flight_id"`
	DetectedAt string `json:"detected_at"`
}

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

type AMANSetRateMessage struct {
	Type    EventType          `json:"type"`
	Version int                `json:"version"`
	Data    AMANSetRateRequest `json:"data"`
}

type AMANSetManualETAMessage struct {
	Type    EventType               `json:"type"`
	Version int                     `json:"version"`
	Data    AMANSetManualETARequest `json:"data"`
}

type AMANReportGoAroundMessage struct {
	Type    EventType                 `json:"type"`
	Version int                       `json:"version"`
	Data    AMANReportGoAroundRequest `json:"data"`
}
