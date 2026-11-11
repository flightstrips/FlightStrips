package services

import (
	"FlightStrips/internal/sat"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestArrivalRestrictionsRejectAssignmentsButKeepPhysicalObservations(t *testing.T) {
	const reason = "Not available for arrival stand assignment"
	registry, err := sat.LoadStandCapabilities(strings.NewReader(
		"STAND:EKCH:C10:N055.37.39.290:E012.38.58.230:35\nARRIVAL_CLOSED:" + reason + "\n",
	))
	require.NoError(t, err)
	stand, ok := registry.Lookup("EKCH", "C10")
	require.True(t, ok)
	evaluation := sat.StandCompatibilityEvaluation{
		Matches: []sat.StandCompatibilityMatch{{Stand: stand}},
	}
	service := &StandAllocationService{stands: registry, now: time.Now}
	request := StandAllocationRequest{
		Airport: "EKCH", Callsign: "SAS123", Stand: "C10",
		Direction: sat.AssignmentDirectionArrival,
	}

	for _, command := range []StandAllocationCommand{CompatibleManualStand, IncompatibleManualOverride} {
		_, _, _, _, _, err := service.selectStand(command, request, evaluation, nil, nil, nil)
		require.ErrorIs(t, err, ErrArrivalStandRestricted)
		require.ErrorContains(t, err, reason)
	}

	_, _, _, _, _, err = service.selectStand(AutomaticStandAllocation, request, evaluation, nil, nil, nil)
	require.ErrorIs(t, err, ErrNoAvailableStand)

	_, _, selected, _, _, err := service.selectStand(observedStandAllocation, request, evaluation, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.Equal(t, "C10", selected.Stand.Name)

	request.Direction = sat.AssignmentDirectionDeparture
	_, _, selected, _, _, err = service.selectStand(CompatibleManualStand, request, evaluation, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.Equal(t, "C10", selected.Stand.Name)
}
