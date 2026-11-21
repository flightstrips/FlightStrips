package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/vatsim"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLifecycleRequestsApplyCargoRemarksAndResetFailureFingerprint(t *testing.T) {
	dir := filepath.Join("config", "ekbi")
	stands, err := sat.LoadStandCapabilityFile(filepath.Join(dir, "GRpluginStands.txt"))
	require.NoError(t, err)
	policy, err := sat.LoadAirlineAssignmentFile(filepath.Join(dir, "airline_assignment.json"), stands.ForAirport("EKBI"))
	require.NoError(t, err)
	for _, direction := range []string{"arrival", "departure"} {
		t.Run(direction, func(t *testing.T) {
			strip := &models.Strip{Callsign: "ZZZ001", Origin: "EKBI", Destination: "EGLL", AircraftType: strp("A320")}
			allocations := &StandAllocationService{}
			var request func() StandAllocationRequest
			if direction == "arrival" {
				strip.Origin, strip.Destination = "EGLL", "EKBI"
				lifecycle := &ArrivalLifecycleService{allocations: allocations, borders: sat.NewAirportCountryRegistry()}
				request = func() StandAllocationRequest {
					return lifecycle.buildRequest(1, strip, vatsim.ArrivalFlightInfo{}, StageConfirmed, nil, nil)
				}
			} else {
				lifecycle := &DepartureLifecycleService{allocations: allocations, borders: sat.NewAirportCountryRegistry(), now: time.Now}
				request = func() StandAllocationRequest {
					return lifecycle.buildRequest(1, strip, vatsim.DepartureFlightInfo{}, StageReserved, nil)
				}
			}
			passenger := request()
			selection, err := policy.SelectStand(passenger.AssignmentFacts, []string{"4", "26"}, func() float64 { return 0 })
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, "26", selection.Stand)
			strip.Remarks = strp("RMK/CARGO")
			cargo := request()
			selection, err = policy.SelectStand(cargo.AssignmentFacts, []string{"4", "26"}, func() float64 { return 0 })
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, "4", selection.Stand, "automatic lifecycle requests must use the cargo stand pool")
			passengerKey, passengerFingerprint := automaticFailureKeyAndFingerprint(passenger)
			cargoKey, cargoFingerprint := automaticFailureKeyAndFingerprint(cargo)
			require.Equal(t, passengerKey, cargoKey)
			require.NotEqual(t, passengerFingerprint, cargoFingerprint, "a cargo remark must permit retry of a suppressed allocation")
		})
	}
}
