package server

import (
	"FlightStrips/internal/models"
	"encoding/json"
	"slices"
)

type routeInputFingerprint struct {
	DepartureRunways []string          `json:"departure_runways"`
	ArrivalRunways   []string          `json:"arrival_runways"`
	Owners           []routeInputOwner `json:"owners"`
	Radio            []routeInputRadio `json:"radio"`
}

type routeInputOwner struct {
	Position   string   `json:"position"`
	Identifier string   `json:"identifier"`
	Sectors    []string `json:"sectors"`
}

type routeInputRadio struct {
	PrimaryFrequency   string   `json:"primary_frequency"`
	Role               string   `json:"role"`
	CoveredFrequencies []string `json:"covered_frequencies"`
}

func buildRouteInputFingerprint(session *models.Session, owners []*models.SectorOwner, radio routeRadioState) (string, error) {
	fingerprint := routeInputFingerprint{
		DepartureRunways: slices.Clone(session.ActiveRunways.DepartureRunways),
		ArrivalRunways:   slices.Clone(session.ActiveRunways.ArrivalRunways),
		Owners:           make([]routeInputOwner, 0, len(owners)),
		Radio:            make([]routeInputRadio, 0, len(radio.coverage)),
	}

	for _, owner := range owners {
		if owner == nil {
			continue
		}
		sectors := slices.Clone(owner.Sector)
		slices.Sort(sectors)
		fingerprint.Owners = append(fingerprint.Owners, routeInputOwner{
			Position:   owner.Position,
			Identifier: owner.Identifier,
			Sectors:    sectors,
		})
	}
	slices.SortFunc(fingerprint.Owners, func(left, right routeInputOwner) int {
		if result := compareStrings(left.Position, right.Position); result != 0 {
			return result
		}
		if result := compareStrings(left.Identifier, right.Identifier); result != 0 {
			return result
		}
		return slices.Compare(left.Sectors, right.Sectors)
	})

	for primaryFrequency, coveredSet := range radio.coverage {
		covered := make([]string, 0, len(coveredSet))
		for frequency := range coveredSet {
			covered = append(covered, frequency)
		}
		slices.Sort(covered)
		fingerprint.Radio = append(fingerprint.Radio, routeInputRadio{
			PrimaryFrequency:   primaryFrequency,
			Role:               radio.roleByPrimary[primaryFrequency],
			CoveredFrequencies: covered,
		})
	}
	slices.SortFunc(fingerprint.Radio, func(left, right routeInputRadio) int {
		return compareStrings(left.PrimaryFrequency, right.PrimaryFrequency)
	})

	encoded, err := json.Marshal(fingerprint)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func compareStrings(left, right string) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func (s *Server) routeInputsMatch(sessionID int32, fingerprint string) bool {
	previous, ok := s.routeInputs.Load(sessionID)
	return ok && previous == fingerprint
}

func (s *Server) storeRouteInputs(sessionID int32, fingerprint string) {
	s.routeInputs.Store(sessionID, fingerprint)
}
