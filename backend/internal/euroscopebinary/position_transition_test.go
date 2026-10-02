package euroscopebinary

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/shared"
	pb "FlightStrips/pkg/events/cluster"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestPositionTransitionsRetainMovementAndLandingRules(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir("../.."))
	defer os.Chdir(cwd)
	require.NoError(t, config.InitConfig())
	now := time.Now().UTC()
	arrival := &pb.Strip{Callsign: "SAS123", Departure: "ESSA", Destination: "EKCH", Bay: shared.BAY_ARR_HIDDEN, Runway: "22L", Marked: true, Stand: "A17", OwnerCid: "111111"}
	position := &pb.AircraftPosition{Latitude: 55.624, Longitude: 12.6654, AltitudeFeet: 20}
	landed := derivePositionStrip(arrival, position, "EKCH", now)
	require.NotNil(t, landed)
	require.Equal(t, now, landed.Aldt.AsTime())
	require.True(t, landed.Marked)
	require.Equal(t, arrival.Stand, landed.Stand)
	require.Equal(t, arrival.OwnerCid, landed.OwnerCid)
	require.Nil(t, arrival.Aldt, "planning must detach input")
	landed.Bay = shared.BAY_FINAL
	vacated := derivePositionStrip(landed, &pb.AircraftPosition{Latitude: 55.63, Longitude: 12.65, AltitudeFeet: 20}, "EKCH", now.Add(time.Second))
	require.Equal(t, shared.BAY_TWY_ARR, vacated.Bay)
	require.Equal(t, now, vacated.Aldt.AsTime(), "do not rewrite landing clock")
	wrong := *arrival
	wrong.Runway = "04L"
	wrongResult := derivePositionStrip(&wrong, position, "EKCH", now)
	if wrongResult != nil {
		require.Nil(t, wrongResult.Aldt, "assigned runway must match")
	}
	airborne := &pb.Strip{Callsign: "SAS124", Departure: "EKCH", Destination: "ESSA", Bay: shared.BAY_DEPART}
	departed := derivePositionStrip(airborne, &pb.AircraftPosition{Latitude: 55.65, Longitude: 12.68, AltitudeFeet: 3000}, "EKCH", now)
	require.Equal(t, shared.BAY_AIRBORNE, departed.Bay)
	departed.Aldt = timestamppb.New(now)
	require.Nil(t, derivePositionStrip(departed, &pb.AircraftPosition{Latitude: 55.63, Longitude: 12.65, AltitudeFeet: 20}, "EKCH", now), "late ground reports cannot reverse departure")
}
