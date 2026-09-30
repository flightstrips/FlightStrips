package services

import (
	"FlightStrips/internal/cdm"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"fmt"
)

// NewCdmCandidateWebAPI supplies the existing sequence view entirely from
// accepted typed state. No SQL repository or legacy service is constructed.
func NewCdmCandidateWebAPI(auth shared.AuthenticationService, candidate *CdmCandidate) (*cdm.CandidateWebAPI, error) {
	if candidate == nil {
		return nil, fmt.Errorf("CDM candidate unavailable")
	}
	return cdm.NewCandidateWebAPI(auth, func(ctx context.Context) ([]cdm.CandidateSequenceInput, error) {
		global, err := candidate.Writer.Read(ctx, globalRef())
		if err != nil {
			return nil, err
		}
		inputs := []cdm.CandidateSequenceInput{}
		for _, registry := range global.EntitiesByKind(pb.EntityKind_SESSION_REGISTRY) {
			r := registry.GetValue().GetSessionRegistry()
			if r.State != pb.SessionRegistry_ACTIVE {
				continue
			}
			state, err := candidate.Writer.Read(ctx, sessionRef(r.Id))
			if err != nil {
				return nil, err
			}
			seed, err := cdmSeed(state, r.Id)
			if err != nil {
				return nil, err
			}
			page, _, err := candidate.Config.Read(ctx, seed.Airport)
			if err != nil {
				return nil, err
			}
			config, err := clusterCdmConfig(page, seed)
			if err != nil {
				return nil, err
			}
			input := cdm.CandidateSequenceInput{Session: &models.Session{ID: r.Id, Name: seed.Name, Airport: seed.Airport}, Config: config}
			input.Session.ActiveRunways.DepartureRunways = config.ActiveDepartureRunways
			input.Session.ActiveRunways.ArrivalRunways = config.ActiveArrivalRunways
			for _, e := range state.EntitiesByKind(pb.EntityKind_STRIP) {
				m := cdmModel(state, e, r.Id)
				if m.Origin == seed.Airport {
					input.Strips = append(input.Strips, m)
				}
			}
			inputs = append(inputs, input)
		}
		return inputs, nil
	})
}
