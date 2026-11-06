package frontend

import "FlightStrips/pkg/models"

const StripAnnotate EventType = "strip.annotate"

const (
	StripCreateLocal       EventType = "strip.create-local"
	FindConnectedFlights   EventType = "flight.find-connected"
	StripAdmitConnected    EventType = "strip.admit-connected"
	ConnectedFlightResults EventType = "flight.connected-results"
	StripRemove            EventType = "strip.remove"
	StripRestore           EventType = "strip.restore"
)

type StripAnnotationEvent struct {
	Callsign string `json:"callsign"`
	models.StripRevision
	Field   string `json:"field"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
}

type StripVisibilityEvent struct {
	Callsign string `json:"callsign"`
	models.StripRevision
}

type ConnectedFlightSearchRequest struct {
	Definition string `json:"definition_revision"`
	RequestID  string `json:"request_id"`
	Query      string `json:"query"`
}
type ConnectedFlightAdmissionRequest struct {
	Definition string                          `json:"definition_revision"`
	Flight     models.ConnectedFlightCandidate `json:"flight"`
}
type ConnectedFlightSearchResult struct {
	RequestID string                            `json:"request_id"`
	Flights   []models.ConnectedFlightCandidate `json:"flights"`
}

func (r ConnectedFlightSearchResult) GetType() EventType       { return ConnectedFlightResults }
func (r ConnectedFlightSearchResult) Marshal() ([]byte, error) { return marshall(r) }
