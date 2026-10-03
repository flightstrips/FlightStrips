package services

import (
	"testing"

	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
)

func TestCdmCandidateStalePositionUsesDefaultWithoutBlockingSequence(t *testing.T) {
	h := newCdmHarness(t, false)
	state := h.state()
	seed, err := cdmSeed(state, h.id)
	if err != nil {
		t.Fatal(err)
	}
	positions := []cluster.KVPosition{{Stale: true, Value: &pb.PositionValue{AircraftKey: "SAS1", Observation: &pb.PositionValue_Position{Position: &pb.AircraftPosition{Latitude: 0, Longitude: 0}}}}}
	c := h.c[h.owner(sessionRef(h.id))]
	change, err := c.plan(h.ctx, h.id, state, seed, h.config, nil, positions, "cdm-recalculate", "stale-position-test", h.now)
	if err != nil {
		t.Fatal(err)
	}
	for _, entity := range change.Changes {
		if entity.Key == "SAS1" && entity.GetUpsert().GetCdmState().GetTtot() != nil {
			return
		}
	}
	t.Fatal("stale aircraft blocked departure sequence calculation")
}
