package amancandidate

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/terminal"
	pb "FlightStrips/pkg/events/cluster"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"os"
	"testing"
	"time"
)

func TestCommittedTerminalPolicyPreservesOperationalSettings(t *testing.T) {
	data, err := os.ReadFile("../../config/aman/ekch-terminal-2609.json")
	require.NoError(t, err)
	var c terminal.Configuration
	require.NoError(t, json.Unmarshal(data, &c))
	wire := TerminalPolicy(c)
	decoded := decodeTerminalPolicy(wire)
	// Protobuf repeated fields canonicalize empty and absent slices alike.
	for i := range c.Feeders {
		if len(c.Feeders[i].Aliases) == 0 {
			c.Feeders[i].Aliases = nil
		}
	}
	require.Equal(t, c.RunwayGroups, decoded.RunwayGroups)
	require.Equal(t, c.Feeders, decoded.Feeders)
	require.Equal(t, c.Paths, decoded.Paths)
	require.Equal(t, c.STARFamilyPolicies, decoded.STARFamilyPolicies)
	require.NoError(t, decoded.ValidateOperationalSettings())
	require.True(t, proto.Equal(wire, TerminalPolicy(decoded)))
}

func TestFlightConversionPreservesHoldingLifecycleAndFrozenFacts(t *testing.T) {
	at := time.Now().UTC().Truncate(time.Second)
	level := int32(7000)
	alt := 9000
	family := "TESPI"
	feeder := "TESPI"
	holding := "TESPI-HOLD"
	f := &pb.AmanFlight{Callsign: "SAS123", State: "sequenced", SelectedFeeder: func() *string { v := string(feeder); return &v }(), SelectedStarFamily: func() *string { v := string(family); return &v }(), SelectedHolding: func() *string { v := string(holding); return &v }(),
		HoldingClearance:  &pb.AmanHoldingClearance{Hold: "TESPI", HoldType: "enroute", HoldEat: "1422", ClearedAltitude: &level, ObservedAt: timestamp(at)},
		HoldingStack:      &pb.AmanHoldingStack{HoldingId: "TESPI-HOLD", FirstObservedAt: timestamp(at.Add(-time.Minute)), Confirmed: true},
		LatestObservation: encodeAmanFlightObservation(&aman.FlightObservation{Callsign: "SAS123", Provider: aman.ObservationProviderEuroScope, ReconciledAt: at, Surveillance: &aman.SurveillanceFact{AltitudeFeet: &alt, ObservedAt: &at}, HoldingClearance: &aman.HoldingClearance{Hold: "TESPI", HoldType: aman.HoldingClearanceEnroute, ObservedAt: at}}),
		FrozenAt:          timestamp(at), FrozenOperationalTeta: timestamp(at.Add(time.Minute)), FreezeReason: "manual", FrozenSlot: &pb.AmanSlot{RunwayGroupId: "ARRIVAL-22", Sequence: 3, Revision: 8, Time: timestamp(at.Add(time.Minute))},
		Lifecycle: &pb.AmanLifecycle{Absence: &pb.AmanAbsence{MissingSince: timestamp(at), RemovalDueAt: timestamp(at.Add(time.Minute)), Remaining: durationpb.New(0)}},
	}
	encoded := encodeAmanFlight(decodeAmanFlight(f))
	require.True(t, proto.Equal(f, encoded), "before: %s\nafter: %s", f, encoded)
}

func TestTMAUsesValidatedAcceptedCoordinates(t *testing.T) {
	volume, err := terminal.LoadTMAVolume("../../config/aman/boundaries/EKCH.json")
	require.NoError(t, err)
	stored := encodeTMAVolume(volume)
	replayed, err := decodeTMAVolume(stored)
	require.NoError(t, err)
	require.Equal(t, volume.Coordinates(), replayed.Coordinates())
	for _, point := range [][3]float64{{55.7, 12.4, 6000}, {55.7, 12.4, 19500}, {56.7, 15, 6000}} {
		require.Equal(t, volume.Contains(point[0], point[1], point[2]), replayed.Contains(point[0], point[1], point[2]))
	}
	stored.Polygons[0].Rings[0].Coordinates = stored.Polygons[0].Rings[0].Coordinates[:3]
	_, err = decodeTMAVolume(stored)
	require.Error(t, err)
}
