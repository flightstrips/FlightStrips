package services

import (
	"FlightStrips/internal/models"
)

func valueString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func existingETASource(a *models.StandAssignment) *string {
	if a == nil {
		return nil
	}
	return a.ETASource
}
