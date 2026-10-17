package frontend

// InboundDeparture is an informational finding, independent of departure CDM
// validation and never a reason to block an arrival strip action.
type InboundDeparture struct {
	Callsign             string `json:"callsign"`
	Departure            string `json:"departure"`
	Arrival              string `json:"arrival"`
	Ctot                 string `json:"ctot"`
	Atot                 string `json:"atot"`
	ETA                  string `json:"eta"`
	Status               string `json:"status"`
	DeviationSeconds     int    `json:"deviation_seconds"`
	OutsideWindowSeconds int    `json:"outside_window_seconds"`
}

type InboundDeparturesEvent struct {
	SessionID int32              `json:"session_id"`
	Airport   string             `json:"airport"`
	Enabled   bool               `json:"enabled"`
	UpdatedAt string             `json:"updated_at"`
	Stale     bool               `json:"stale"`
	Flights   []InboundDeparture `json:"flights"`
}

func (e InboundDeparturesEvent) Marshal() ([]byte, error) { return marshall(e) }
func (e InboundDeparturesEvent) GetType() EventType       { return "inbound_departures" }
