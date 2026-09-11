package operational

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"FlightStrips/internal/aman/terminal"
	"github.com/stretchr/testify/require"
)

func TestTMAFreezeCapturesFirstFreshOutsideToInsideEdgeOnce(t *testing.T) {
	base := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	service := testTMAService(t)
	flight := testTMAFlight(base)

	require.Empty(t, service.observeTMAEntry(&flight, tmaObservation(base, 1, 3, 10_000), base))
	require.Equal(t, aman.TMAOutside, flight.TMAEntry.LastContainment)
	require.Empty(t, service.observeTMAEntry(&flight, tmaObservation(base.Add(time.Second), 1, 1, 10_000), base.Add(time.Second)))
	require.Equal(t, aman.FreezeTMA, flight.FreezeReason)
	require.Equal(t, base.Add(20*time.Minute), *flight.FrozenOperationalTETA)
	require.Equal(t, base.Add(21*time.Minute), flight.FrozenSlot.Time)

	flight.Prediction.OperationalTETA = base.Add(30 * time.Minute)
	flight.Slot.Time = base.Add(31 * time.Minute)
	require.Empty(t, service.observeTMAEntry(&flight, tmaObservation(base.Add(2*time.Second), 1, 3, 10_000), base.Add(2*time.Second)))
	require.Empty(t, service.observeTMAEntry(&flight, tmaObservation(base.Add(3*time.Second), 1, 1, 10_000), base.Add(3*time.Second)))
	require.Equal(t, base.Add(20*time.Minute), *flight.FrozenOperationalTETA)
	require.Equal(t, base.Add(21*time.Minute), flight.FrozenSlot.Time)
}

func TestTMAFreezeUsesApprovedHorizontalAndStrictAltitudeEdges(t *testing.T) {
	base := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name                                         string
		outsideLat, outsideLon, insideLat, insideLon float64
		outsideAltitude, insideAltitude              int
	}{
		{name: "strictly below FL195", outsideLat: 1, outsideLon: 1, insideLat: 1, insideLon: 1, outsideAltitude: 19_500, insideAltitude: 19_499},
		{name: "horizontal boundary included", outsideLat: 1, outsideLon: 3, insideLat: 1, insideLon: 0, outsideAltitude: 10_000, insideAltitude: 10_000},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, flight := testTMAService(t), testTMAFlight(base)
			require.Empty(t, service.observeTMAEntry(&flight, tmaObservation(base, test.outsideLat, test.outsideLon, test.outsideAltitude), base))
			require.Empty(t, service.observeTMAEntry(&flight, tmaObservation(base.Add(time.Second), test.insideLat, test.insideLon, test.insideAltitude), base.Add(time.Second)))
			require.Equal(t, aman.FreezeTMA, flight.FreezeReason)
		})
	}
}

func TestTMAFreezeRejectsStaleAndInvalidSurveillanceWithoutAdvancingCursor(t *testing.T) {
	base := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	service, flight := testTMAService(t), testTMAFlight(base)
	require.Empty(t, service.observeTMAEntry(&flight, tmaObservation(base, 1, 3, 10_000), base))

	stale := tmaObservation(base.Add(time.Second), 1, 1, 10_000)
	require.Equal(t, "tma_entry_surveillance_stale", service.observeTMAEntry(&flight, stale, base.Add(tmaSurveillanceFresh+2*time.Second)))
	require.Equal(t, base, flight.TMAEntry.LastObservedAt)
	invalid := tmaObservation(base.Add(2*time.Second), math.NaN(), 1, 10_000)
	require.Equal(t, "tma_entry_surveillance_invalid", service.observeTMAEntry(&flight, invalid, base.Add(2*time.Second)))
	require.Equal(t, base, flight.TMAEntry.LastObservedAt)
	require.Equal(t, aman.FreezeNone, flight.FreezeReason)
}

func TestTMAMissingOrInvalidGeometryDegradesHealthAndNeverTriggers(t *testing.T) {
	base := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	for _, path := range []string{filepath.Join(t.TempDir(), "missing.json"), filepath.Join(t.TempDir(), "invalid.json")} {
		if filepath.Base(path) == "invalid.json" {
			require.NoError(t, os.WriteFile(path, []byte(`{"type":"Feature","geometry":{"type":"Point","coordinates":[]}}`), 0o600))
		}
		service, err := New(Dependencies{
			Repository: &memoryRepository{}, Materializer: readyNavigation{}, Geometry: terminalIdentityGeometry{}, Wind: unavailableWind{}, Publisher: &recordingPublisher{},
			Terminal:      terminal.Configuration{Airport: "EKCH", RunwayGroups: []terminal.RunwayGroup{{ID: "ARRIVAL-22"}}},
			TMAVolumePath: path, Airports: []string{"EKCH"}, Mode: aman.ModeAuthoritative, Now: func() time.Time { return base },
		})
		require.NoError(t, err)
		service.observeNavigationCache(t.Context(), "EKCH")
		health := service.TechnicalHealth(t.Context())
		require.Equal(t, aman.HealthUnavailable, health.Navigation.Status)
		require.Equal(t, "terminal_geometry_invalid", health.Navigation.Reason)
		flight := testTMAFlight(base)
		require.Empty(t, service.observeTMAEntry(&flight, tmaObservation(base, 1, 3, 10_000), base))
		require.Empty(t, service.observeTMAEntry(&flight, tmaObservation(base.Add(time.Second), 1, 1, 10_000), base.Add(time.Second)))
		require.Nil(t, flight.TMAEntry)
		require.Equal(t, aman.FreezeNone, flight.FreezeReason)
	}
}

func TestTMAFreezeRestartReplayIsIdempotent(t *testing.T) {
	base := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	service, flight := testTMAService(t), testTMAFlight(base)
	require.Empty(t, service.observeTMAEntry(&flight, tmaObservation(base, 1, 3, 10_000), base))
	encoded, err := json.Marshal(flight)
	require.NoError(t, err)
	var restarted aman.AMANFlight
	require.NoError(t, json.Unmarshal(encoded, &restarted))

	inside := tmaObservation(base.Add(time.Second), 1, 1, 10_000)
	require.Empty(t, service.observeTMAEntry(&restarted, inside, base.Add(time.Second)))
	want := restarted
	require.Empty(t, service.observeTMAEntry(&restarted, inside, base.Add(time.Second)))
	require.Equal(t, want, restarted)
}

func TestTMAEntryPreservesExistingManualAndSuperstableProtection(t *testing.T) {
	base := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	for _, reason := range []aman.FreezeReason{aman.FreezeManual, aman.FreezeSuperstable} {
		service, flight := testTMAService(t), testTMAFlight(base)
		flight.FreezeReason, flight.FrozenAt = reason, &base
		frozenTETA, frozenSlot := flight.Prediction.OperationalTETA, *flight.Slot
		flight.FrozenOperationalTETA, flight.FrozenSlot = &frozenTETA, &frozenSlot
		require.Empty(t, service.observeTMAEntry(&flight, tmaObservation(base, 1, 3, 10_000), base))
		require.Empty(t, service.observeTMAEntry(&flight, tmaObservation(base.Add(time.Second), 1, 1, 10_000), base.Add(time.Second)))
		require.Equal(t, reason, flight.FreezeReason)
		require.True(t, flight.TMAEntry.FreezeTriggered)
	}
}

func TestConfirmedGoAroundAtomicallyRecapturesFreshInsideTMAFlight(t *testing.T) {
	base := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("ARRIVAL-22")
	service := &Service{deps: Dependencies{Terminal: terminal.Configuration{RunwayGroups: []terminal.RunwayGroup{{ID: group}}}}}
	effective := base
	state := aman.AirportState{
		Revision: 4,
		RunwayGroups: []aman.RunwayGroupPolicy{{
			ID: group, Selected: true, ActiveRatePerHour: 20, RateEffectiveAt: &effective,
			RateSchedule: []aman.RunwayGroupRatePoint{{EffectiveAt: effective, ArrivalsPerHour: 20}},
		}},
	}
	flight := operationalFlight("GO-AROUND-TMA", group, "MONAK", "M", base.Add(3*time.Minute))
	flight.State = aman.StateStable
	oldSlot := aman.Slot{Time: base.Add(3 * time.Minute), RunwayGroupID: group, Sequence: 1, Revision: state.Revision, Reason: string(sequence.ReasonRateWTC)}
	oldTETA := flight.Prediction.OperationalTETA
	flight.Slot, flight.FrozenSlot = &oldSlot, &oldSlot
	flight.FreezeReason, flight.FrozenAt, flight.FrozenOperationalTETA = aman.FreezeTMA, &base, &oldTETA
	flight.TMAEntry = &aman.TMAEntryState{LastContainment: aman.TMAInside, LastObservedAt: base, FreezeTriggered: true}
	state.Flights = []aman.AMANFlight{flight}

	command := aman.ReportGoAroundCommand{
		Metadata: aman.CommandMetadata{CommandID: "go-around-tma", ExpectedRevision: state.Revision},
		FlightID: flight.ID, DetectedAt: base,
	}
	mutation, err := service.ReportGoAround(aman.CommandContext{ReceivedAt: base.Add(time.Second)}, command)
	require.NoError(t, err)
	change, err := mutation(state)
	require.NoError(t, err)
	updated := change.State.Flights[0]

	require.Equal(t, aman.FreezeTMA, updated.FreezeReason)
	require.Equal(t, base.Add(DefaultGoAroundDelay), updated.Prediction.OperationalTETA)
	require.Equal(t, aman.OperationalReasonTMAFreeze, updated.Prediction.OperationalReason)
	require.NotNil(t, updated.Slot)
	require.NotEqual(t, oldSlot.Time, updated.Slot.Time)
	require.Equal(t, *updated.Slot, *updated.FrozenSlot)
	require.Equal(t, updated.Prediction.OperationalTETA, *updated.FrozenOperationalTETA)
	require.True(t, updated.TMAEntry.FreezeTriggered)
	require.Len(t, change.Audit, 1, "release, delay, allocation, and recapture share one command result")
}

func testTMAService(t *testing.T) *Service {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tma.json")
	document := `{"type":"Feature","geometry":{"type":"MultiPolygon","coordinates":[[[[0,0],[2,0],[2,2],[0,2],[0,0]]]]}}`
	require.NoError(t, os.WriteFile(path, []byte(document), 0o600))
	volume, err := terminal.LoadTMAVolume(path)
	require.NoError(t, err)
	return &Service{tmaVolume: &volume}
}

func testTMAFlight(base time.Time) aman.AMANFlight {
	return aman.AMANFlight{
		FreezeReason: aman.FreezeNone,
		Prediction:   &aman.Prediction{OperationalTETA: base.Add(20 * time.Minute), OperationalReason: aman.OperationalReasonPredicted, Publishable: true},
		Slot:         &aman.Slot{Time: base.Add(21 * time.Minute), RunwayGroupID: "ARRIVAL-22", Sequence: 1, Revision: 3, Reason: string(sequence.ReasonRateWTC)},
	}
}

func tmaObservation(at time.Time, latitude, longitude float64, altitude int) aman.FlightObservation {
	return aman.FlightObservation{SourceStatus: aman.DataFresh, Surveillance: &aman.SurveillanceFact{
		LatitudeDegrees: latitude, LongitudeDegrees: longitude, AltitudeFeet: &altitude, ObservedAt: &at,
	}}
}
