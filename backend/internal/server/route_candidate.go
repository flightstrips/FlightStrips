package server

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/vatsim"
)

// ComputeCandidateRoute uses the same pure route policy as RecalculateSession.
// Its caller supplies accepted session/sector/radio inputs; no repository or
// transaction is involved. Delivery and persistence belong to the caller.
func ComputeCandidateRoute(strip *models.Strip, session *models.Session, owners []*models.SectorOwner, coverage []config.ControllerCoverage) ([]string, *models.NextDisplay, bool, error) {
	radio := routeRadioState{coverage: map[string]map[string]struct{}{}, roleByPrimary: map[string]string{}}
	for _, controller := range coverage {
		primary := vatsim.NormalizeFrequency(controller.Frequency)
		if primary == "" {
			continue
		}
		radio.coverage[primary] = map[string]struct{}{}
		for _, f := range controller.CoveredFrequencies {
			radio.coverage[primary][vatsim.NormalizeFrequency(f)] = struct{}{}
		}
		radio.roleByPrimary[primary] = controller.Name
	}
	state, update, err := computeRouteStateForStrip(strip, session, owners, radio)
	return state.NextOwners, state.NextDisplay, update, err
}
