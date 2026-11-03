package operational

import (
	"strings"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/sequence"
	"FlightStrips/internal/aman/terminal"
)

type starResolution struct {
	feeder   navdata.FeederID
	inferred bool
	reason   string
}

// resolveObservationSTAR identifies only published, configured procedures.
// A procedure for another runway can identify the family for a forecast, but
// never changes the reported clearance or the flight's selected runway.
func (s *Service) resolveObservationSTAR(observation aman.FlightObservation, group aman.RunwayGroupID) starResolution {
	assigned := strings.ToUpper(strings.TrimSpace(stringValue(observation.AssignedSTAR)))
	if assigned == "" {
		if feeder, ok := s.feeder(stringValue(observation.FiledRoute), group); ok {
			return starResolution{feeder: feeder}
		}
		return starResolution{reason: string(sequence.WarningUnknownSTARFamily)}
	}
	if len(s.deps.Terminal.Paths) == 0 {
		for _, configured := range s.deps.Terminal.Feeders {
			if terminal.MatchesSTAR(configured, terminal.Path{}, assigned) {
				return starResolution{feeder: configured.ID}
			}
		}
		return starResolution{reason: string(sequence.WarningUnknownSTARFamily)}
	}

	families := make(map[navdata.FeederID]struct{})
	matchedGroup := false
	for _, configured := range s.deps.Terminal.Feeders {
		for _, path := range s.deps.Terminal.Paths {
			if path.Feeder == configured.ID && terminal.MatchesSTAR(configured, path, assigned) {
				families[configured.ID] = struct{}{}
				matchedGroup = matchedGroup || path.RunwayGroup == group
			}
		}
	}
	if len(families) == 0 {
		return starResolution{reason: string(sequence.WarningUnknownSTARFamily)}
	}
	if len(families) != 1 {
		return starResolution{reason: aman.STARFamilyAmbiguous}
	}
	for family := range families {
		for _, path := range s.deps.Terminal.Paths {
			if path.Feeder == family && path.RunwayGroup == group {
				return starResolution{feeder: family, inferred: !matchedGroup}
			}
		}
		return starResolution{feeder: family, reason: aman.STARRunwayUnavailable}
	}
	return starResolution{reason: string(sequence.WarningUnknownSTARFamily)}
}

func (s *Service) observationFeeder(observation aman.FlightObservation, group aman.RunwayGroupID) (navdata.FeederID, bool) {
	resolved := s.resolveObservationSTAR(observation, group)
	return resolved.feeder, resolved.reason == ""
}
