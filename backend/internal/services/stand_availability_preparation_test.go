package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/sat"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStandAvailabilityPreparedAssignmentsKeepBlockingRules(t *testing.T) {
	stands, _ := lifecyclePolicyFixture(t)
	now := time.Date(2026, 10, 2, 20, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name      string
		stage     string
		direction sat.AssignmentDirection
		yield     bool
		displace  string
		expired   bool
		want      map[string][]string
	}{
		{"direct and both adjacency directions", StageAssigned, sat.AssignmentDirectionArrival, false, "", false, map[string][]string{"A1": {"reserved by SAS2"}, "A2": {"blocked by allocated neighbor  a1 "}, "B1": {"blocked by allocated neighbor  a1 "}}},
		{"estimated stays blocked", StageEstimated, sat.AssignmentDirectionArrival, false, "", false, map[string][]string{"A1": {"soft-reserved by SAS2"}, "A2": {"blocked by allocated neighbor  a1 "}, "B1": {"blocked by allocated neighbor  a1 "}}},
		{"later stage yields estimated", StageEstimated, sat.AssignmentDirectionArrival, true, "", false, map[string][]string{}},
		{"explicit arrival displacement", StageAssigned, sat.AssignmentDirectionArrival, false, StageAssigned, false, map[string][]string{}},
		{"departure cannot be displaced", StageAssigned, sat.AssignmentDirectionDeparture, false, StageAssigned, false, map[string][]string{"A1": {"reserved by SAS2"}, "A2": {"blocked by allocated neighbor  a1 "}, "B1": {"blocked by allocated neighbor  a1 "}}},
		{"expired assignment", StageAssigned, sat.AssignmentDirectionArrival, false, "", true, map[string][]string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clockCalls := 0
			s := &StandAllocationService{stands: stands, now: func() time.Time { clockCalls++; return now }, planningBlocks: map[string][]string{"SAS2": {" a2 "}}}
			request := StandAllocationRequest{Airport: "EKCH", Callsign: "SAS1", Direction: sat.AssignmentDirectionArrival, Stage: StageConfirmed, DisplaceStage: tc.displace}
			assignment := &models.StandAssignment{Callsign: "SAS2", Stand: " a1 ", Stage: tc.stage, Direction: string(tc.direction)}
			if tc.expired {
				assignment.ExpiresAt = &now
			}
			matches := map[string]sat.StandCompatibilityMatch{"A1": {}, "A2": {}, "B1": {Blocks: []string{" a1 "}}}
			result := s.availabilityWithEstimated(request, []*models.StandAssignment{nil, {Callsign: "sas1", Stand: "A1"}, assignment}, nil, matches, tc.yield)
			require.Equal(t, tc.want, result)
			require.Equal(t, 1, clockCalls)
			require.Equal(t, []string{" a2 "}, s.planningBlocks["SAS2"])
			require.Equal(t, " a1 ", assignment.Stand)
		})
	}
}

func TestStandAvailabilityPreparationDoesNotSurviveLaterInvocation(t *testing.T) {
	stands, _ := lifecyclePolicyFixture(t)
	now := time.Now()
	s := &StandAllocationService{stands: stands, now: func() time.Time { return now }, planningBlocks: map[string][]string{"SAS2": {"A2"}}}
	request := StandAllocationRequest{Airport: "EKCH", Callsign: "SAS1", Direction: sat.AssignmentDirectionArrival}
	assignment := &models.StandAssignment{Callsign: "SAS2", Stand: "A1", Stage: StageAssigned, Direction: string(sat.AssignmentDirectionArrival)}
	matches := map[string]sat.StandCompatibilityMatch{"A1": {}, "A2": {}, "B1": {}}
	first := s.availability(request, []*models.StandAssignment{assignment}, nil, matches)
	require.Contains(t, first, "A2")
	require.NotContains(t, first, "B1")
	s.planningBlocks["SAS2"] = []string{"B1"}
	second := s.availability(request, []*models.StandAssignment{assignment}, nil, matches)
	require.NotContains(t, second, "A2")
	require.Contains(t, second, "B1")
	assignment.ExpiresAt = &now
	require.Empty(t, s.availability(request, []*models.StandAssignment{assignment}, nil, matches))
}

func TestStandAvailabilityPreparationUsesExactCommittedVariant(t *testing.T) {
	stands, err := sat.LoadStandCapabilities(strings.NewReader("STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30\nBLOCKS:A2\nSTAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30\nBLOCKS:B1\nSTAND:EKCH:A2:N055.38.42.710:E012.38.33.450:30\nSTAND:EKCH:B1:N055.39.42.710:E012.38.33.450:30\n"))
	require.NoError(t, err)
	stand, ok := stands.Lookup("EKCH", "A1")
	require.True(t, ok)
	require.Len(t, stand.Variants, 2)
	variant := allocationVariantKey("EKCH", "A1", stand.Variants[1].Line)
	s := &StandAllocationService{stands: stands, now: time.Now}
	assignment := &models.StandAssignment{Callsign: "SAS2", Stand: "A1", Stage: StageAssigned, Direction: string(sat.AssignmentDirectionArrival), MatchedVariant: &variant}
	request := StandAllocationRequest{Airport: "EKCH", Callsign: "SAS1", Direction: sat.AssignmentDirectionArrival}
	result := s.availability(request, []*models.StandAssignment{assignment}, nil, map[string]sat.StandCompatibilityMatch{"A2": {}, "B1": {}})
	require.NotContains(t, result, "A2")
	require.Equal(t, []string{"blocked by allocated neighbor A1"}, result["B1"])
}

func TestCanonicalBlockPreparationMatchesUnpreparedAdjacency(t *testing.T) {
	names := []string{"A1", " a1 ", "a2", " B1", "", "UNKNOWN", "æ1"}
	lists := [][]string{nil, {}, {" a1 ", "a2"}, {"B1", "UNKNOWN", ""}, {"Æ1"}}
	for _, left := range lists {
		for _, right := range lists {
			for _, candidate := range names {
				for _, assigned := range names {
					require.Equal(t, blocksEachOther(left, right, candidate, assigned), canonicalBlocksEachOther(canonicalStandBlocks(left), canonicalStandBlocks(right), standName(candidate), standName(assigned)))
				}
			}
		}
	}
}

func TestAvailabilityPreparedDeadlineAllowsNonOverlappingUse(t *testing.T) {
	stands, _ := lifecyclePolicyFixture(t)
	now := time.Now()
	release, arrival := now.Add(30*time.Minute), now.Add(40*time.Minute)
	s := &StandAllocationService{stands: stands, now: func() time.Time { return now }, planningBlocks: map[string][]string{"SAS2": {" a2 "}}}
	assignment := &models.StandAssignment{Callsign: "SAS2", Stand: "A1", Stage: StageDepartureBlock, Direction: string(sat.AssignmentDirectionDeparture), ExpiresAt: &release}
	request := StandAllocationRequest{Airport: "EKCH", Callsign: "SAS1", Direction: sat.AssignmentDirectionArrival, Stage: StageAssigned, ETA: &arrival}
	matches := map[string]sat.StandCompatibilityMatch{"A1": {}, "A2": {}}
	require.Empty(t, s.availability(request, []*models.StandAssignment{assignment}, nil, matches))
	arrival = now.Add(20 * time.Minute)
	require.Equal(t, map[string][]string{"A1": {"reserved by SAS2"}, "A2": {"blocked by allocated neighbor A1"}}, s.availability(request, []*models.StandAssignment{assignment}, nil, matches))
}

func TestAvailabilityPreparedManualBlocksUseConfiguredUnionAndFreshAdjacency(t *testing.T) {
	stands, err := sat.LoadStandCapabilities(strings.NewReader("STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30\nBLOCKS:A2\nSTAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30\nBLOCKS:B1\nSTAND:EKCH:A2:N055.38.42.710:E012.38.33.450:30\nSTAND:EKCH:B1:N055.39.42.710:E012.38.33.450:30\n"))
	require.NoError(t, err)
	reason := " closed "
	s := &StandAllocationService{stands: stands, now: time.Now, planningBlockAdjacency: map[string][]string{"B1": {" a2 "}}, planningOccupancy: map[string]string{"OTHER": "B1", "sas1": "A1"}}
	request := StandAllocationRequest{Airport: "EKCH", Callsign: "SAS1"}
	matches := map[string]sat.StandCompatibilityMatch{"A1": {Blocks: []string{"A2"}}, "A2": {}, "B1": {}}
	blocks := []*models.StandBlock{nil, {Stand: " b1 ", Reason: &reason}}
	result := s.availability(request, nil, blocks, matches)
	// A1's selected variant omits B1: physical occupancy does not block it,
	// but the manual block uses the configured union across both variants.
	require.Equal(t, []string{"blocked by manual block B1: closed"}, result["A1"])
	require.Equal(t, []string{"blocked by manual block B1: closed"}, result["A2"])
	require.Equal(t, []string{"physically occupied by OTHER", "manually blocked: closed"}, result["B1"])
	require.Equal(t, []string{" a2 "}, s.planningBlockAdjacency["B1"])
	require.Equal(t, []string{"A2"}, matches["A1"].Blocks)
	s.planningBlockAdjacency["B1"] = nil
	delete(s.planningOccupancy, "OTHER")
	result = s.availability(request, nil, blocks, matches)
	require.NotContains(t, result, "A2")
	require.Equal(t, []string{"manually blocked: closed"}, result["B1"])
}
