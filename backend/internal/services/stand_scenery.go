package services

import (
	"FlightStrips/internal/scenery"
	"context"
	"fmt"
	"log/slog"
)

// PilotSceneryLookup returns the airport scenery a pilot declared in the EFB,
// or "" when none is known.
type PilotSceneryLookup interface {
	PilotScenery(ctx context.Context, cid int64, airport string) (string, error)
}

// SetPilotSceneries makes allocation avoid stands that are missing from the
// scenery each pilot declared.
func (s *StandAllocationService) SetPilotSceneries(lookup PilotSceneryLookup) {
	s.sceneries = lookup
}

// withPilotScenery resolves the pilot's scenery exclusions once per request. A
// lookup failure is logged and allocation continues without exclusions: a
// missing preference must never stop a stand being assigned.
func (s *StandAllocationService) withPilotScenery(ctx context.Context, request StandAllocationRequest) StandAllocationRequest {
	if s == nil || s.sceneries == nil || request.VatsimCID == nil || request.SceneryExcludedStands != nil {
		return request
	}
	id, err := s.sceneries.PilotScenery(ctx, *request.VatsimCID, request.Airport)
	if err != nil {
		slog.WarnContext(ctx, "Pilot scenery lookup failed; allocating without scenery exclusions",
			slog.String("callsign", request.Callsign), slog.Any("error", err))
		return request
	}
	excluded := scenery.ExcludedStands(request.Airport, id)
	if len(excluded) == 0 {
		return request
	}
	declared, _ := scenery.Lookup(request.Airport, id)
	request.SceneryExcludedStands, request.SceneryName = excluded, declared.Name
	return request
}

func (r StandAllocationRequest) sceneryExcludes(stand string) bool {
	_, excluded := r.SceneryExcludedStands[standName(stand)]
	return excluded
}

func (r StandAllocationRequest) sceneryExclusionReason(stand string) string {
	return fmt.Sprintf("%s is not available in %s", standName(stand), r.SceneryName)
}

// removeSceneryExcluded drops stands the pilot's scenery lacks from a
// candidate set, so neither automatic nor compatible manual allocation can
// choose them.
func removeSceneryExcluded[T any](request StandAllocationRequest, matches map[string]T) {
	for name := range matches {
		if request.sceneryExcludes(name) {
			delete(matches, name)
		}
	}
}
