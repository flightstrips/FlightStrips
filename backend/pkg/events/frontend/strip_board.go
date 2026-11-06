package frontend

import "FlightStrips/pkg/models"

const StripAnnotate EventType = "strip.annotate"

type StripAnnotationEvent struct {
	Callsign string `json:"callsign"`
	models.StripRevision
	Field   string `json:"field"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
}
