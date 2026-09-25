package vatsim

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/models"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type observationTestSink struct {
	observations   []aman.FlightObservation
	sourceHealth   []aman.DataStatus
	err            error
	errsByCallsign map[string]error
}

func (s *observationTestSink) Observe(_ context.Context, observation aman.FlightObservation) error {
	if s.err != nil {
		return s.err
	}
	if err := s.errsByCallsign[observation.Callsign]; err != nil {
		return err
	}
	s.observations = append(s.observations, observation)
	return nil
}

func (s *observationTestSink) ObserveSourceHealth(_ context.Context, status aman.DataStatus, _ time.Time) error {
	s.sourceHealth = append(s.sourceHealth, status)
	return nil
}

func newObservationTestWorker(t *testing.T, cache *Cache, now *time.Time, sink *observationTestSink) (*ObservationWorker, *reconciliationTestSessions) {
	t.Helper()
	sessions := &reconciliationTestSessions{items: []*models.Session{{ID: 7, Name: "LIVE", Airport: "EKCH"}}}
	worker, err := NewObservationWorker(ObservationWorkerDependencies{
		Cache: cache, Sessions: sessions, Sink: sink, EnabledAirports: []string{"EKCH"}, StaleAfter: time.Minute,
		Now: func() time.Time { return *now },
	})
	require.NoError(t, err)
	return worker, sessions
}

func TestObservationWorkerMapsPrefileAndOnlineFlightsForLiveSession(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	cache := newReconciliationTestCache(now,
		Flight{CID: "101", Callsign: "SAS101", State: FlightStatePrefile, LastUpdated: now.Add(-time.Minute), FlightPlan: FlightPlan{Origin: "ENGM", Destination: "EKCH", Aircraft: "A20N/M-SDE2", AircraftShort: "A20N", RequestedLevel: "F350", Route: "NEXIL M725", EOBT: "1130", EnrouteDuration: "0145", Revision: 4}},
		Flight{CID: "202", Callsign: "SAS202", State: FlightStateOnline, Latitude: 55.1, Longitude: 12.1, Altitude: 18000, Groundspeed: 420, LastUpdated: now, FlightPlan: FlightPlan{Origin: "EGLL", Destination: "EKCH", Aircraft: "B738/H-SDE2", AircraftShort: "B738", RequestedLevel: "34000", Route: "L620", EOBT: "0930", EnrouteDuration: "0200", Revision: 7}},
	)
	sink := &observationTestSink{}
	worker, _ := newObservationTestWorker(t, cache, &now, sink)

	require.NoError(t, worker.Publish(context.Background()))
	require.Len(t, sink.observations, 2)
	sort.Slice(sink.observations, func(i, j int) bool { return sink.observations[i].Callsign < sink.observations[j].Callsign })
	prefile, online := sink.observations[0], sink.observations[1]
	require.Equal(t, aman.DataFresh, prefile.SourceStatus)
	require.Equal(t, "SAS101", prefile.Callsign)
	require.Equal(t, "ENGM", prefile.Origin)
	require.Equal(t, "EKCH", prefile.Destination)
	require.Equal(t, "A20N", *prefile.AircraftType)
	require.Equal(t, "M", *prefile.WakeCategory)
	require.Equal(t, 35000, *prefile.RequestedLevel)
	require.Equal(t, "NEXIL M725", *prefile.FiledRoute)
	require.Equal(t, uint64(4), *prefile.FlightPlan.Revision)
	require.Equal(t, now.Add(-time.Minute), *prefile.FlightPlan.ObservedAt)
	require.Equal(t, time.Date(2026, time.July, 18, 11, 30, 0, 0, time.UTC), *prefile.PlannedTiming.EstimatedOffBlockTime)
	require.Equal(t, time.Hour+45*time.Minute, *prefile.PlannedTiming.EstimatedEnrouteTime)
	require.Nil(t, prefile.Surveillance)

	require.NotNil(t, online.Surveillance)
	require.Equal(t, 55.1, online.Surveillance.LatitudeDegrees)
	require.Equal(t, 12.1, online.Surveillance.LongitudeDegrees)
	require.Equal(t, 18000, *online.Surveillance.AltitudeFeet)
	require.Equal(t, float64(420), *online.Surveillance.GroundspeedKnots)
	require.Equal(t, uint64(now.UnixMilli()), *online.Surveillance.Sequence)
	require.Equal(t, now, *online.Surveillance.ObservedAt)
	require.Equal(t, now, *online.TakeoffDetected)
	require.Equal(t, "H", *online.WakeCategory)
	require.Equal(t, 34000, *online.RequestedLevel)
}

func TestObservationWorkerUsesSourceObservedTimeForPlannedTimingAcrossRetry(t *testing.T) {
	sourceTime := time.Date(2026, time.July, 18, 23, 58, 0, 0, time.UTC)
	flight := Flight{CID: "101", Callsign: "SAS101", State: FlightStatePrefile, LastUpdated: sourceTime, FlightPlan: FlightPlan{Origin: "ENGM", Destination: "EKCH", EOBT: "0005", EnrouteDuration: "0100", Revision: 1}}
	cache := newReconciliationTestCache(sourceTime, flight)
	firstNow := sourceTime.Add(time.Minute)
	firstSink := &observationTestSink{}
	firstWorker, _ := newObservationTestWorker(t, cache, &firstNow, firstSink)
	require.NoError(t, firstWorker.Publish(context.Background()))

	// A restarted worker can retry the same source fact much later without
	// changing the service day used for its planned departure time.
	retryNow := sourceTime.Add(48 * time.Hour)
	retrySink := &observationTestSink{}
	retryWorker, _ := newObservationTestWorker(t, cache, &retryNow, retrySink)
	require.NoError(t, retryWorker.Publish(context.Background()))
	require.Equal(t, *firstSink.observations[0].PlannedTiming.EstimatedOffBlockTime, *retrySink.observations[0].PlannedTiming.EstimatedOffBlockTime)
	require.Equal(t, time.Date(2026, time.July, 19, 0, 5, 0, 0, time.UTC), *retrySink.observations[0].PlannedTiming.EstimatedOffBlockTime)
}

func TestPreserveNewerObservationFactsKeepsFirstTakeoffDetection(t *testing.T) {
	first := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	later := first.Add(time.Minute)
	previous := aman.FlightObservation{TakeoffDetected: &first}
	next := aman.FlightObservation{TakeoffDetected: &later}

	preserved := preserveNewerObservationFacts(previous, next)

	require.NotNil(t, preserved.TakeoffDetected)
	require.Equal(t, first, *preserved.TakeoffDetected)
}

func TestPlannedTimingIgnoresZeroEnrouteDuration(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)

	timing := plannedTiming(now, FlightPlan{EOBT: "1130", EnrouteDuration: "0000"})

	require.NotNil(t, timing)
	require.NotNil(t, timing.EstimatedOffBlockTime)
	require.Nil(t, timing.EstimatedEnrouteTime)
}

func TestWakeCategoryAndRequestedLevelMappingRejectInvalidSourceValues(t *testing.T) {
	for _, test := range []struct {
		aircraft, level string
		wake            *string
		feet            *int
	}{
		{"A320/L-S", "FL250", stringPointer("L"), intPointer(25000)},
		{"A388/J-S", "41000", stringPointer("J"), intPointer(41000)},
		{"A320/X-S", "F999", nil, nil},
		{"A320", "bad", nil, nil},
	} {
		require.Equal(t, test.wake, wakeCategory(test.aircraft))
		require.Equal(t, test.feet, requestedLevelFeet(test.level))
	}
}

func TestObservationWorkerPreservesCallsignAndRejectsOlderFacts(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	newer := Flight{CID: "101", Callsign: "SAS101", State: FlightStateOnline, Latitude: 55.1, Longitude: 12.1, Altitude: 18000, Groundspeed: 420, LastUpdated: now, FlightPlan: FlightPlan{Origin: "EGLL", Destination: "EKCH", Route: "NEW ROUTE", Revision: 8}}
	cache := newReconciliationTestCache(now, newer)
	sink := &observationTestSink{}
	worker, _ := newObservationTestWorker(t, cache, &now, sink)
	require.NoError(t, worker.Publish(context.Background()))
	first := sink.observations[0]

	older := newer
	older.LastUpdated = now.Add(-time.Minute)
	older.Latitude = 54.0
	older.FlightPlan.Route = "OLD ROUTE"
	older.FlightPlan.Revision = 7
	setObservationCacheSnapshot(cache, now.Add(time.Second), nil, older)
	now = now.Add(time.Second)
	require.NoError(t, worker.Publish(context.Background()))
	require.Len(t, sink.observations, 1, "older facts do not republish unchanged guidance")
	second := worker.known["SAS101"]
	require.Equal(t, first.Callsign, second.Callsign)
	require.Equal(t, "SAS101", second.Callsign)
	require.Equal(t, "NEW ROUTE", *second.FiledRoute)
	require.Equal(t, uint64(8), *second.FlightPlan.Revision)
	require.Equal(t, 55.1, second.Surveillance.LatitudeDegrees)
	require.Equal(t, *first.Surveillance.Sequence, *second.Surveillance.Sequence)
}

func TestObservationWorkerTreatsCallsignChangeAsNewFlight(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	flight := Flight{CID: "101", Callsign: "SAS101", State: FlightStatePrefile, LastUpdated: now, FlightPlan: FlightPlan{Origin: "ENGM", Destination: "EKCH", Revision: 1}}
	cache := newReconciliationTestCache(now, flight)
	sink := &observationTestSink{}
	worker, _ := newObservationTestWorker(t, cache, &now, sink)
	require.NoError(t, worker.Publish(context.Background()))
	require.NoError(t, worker.Publish(context.Background()))
	require.Len(t, sink.observations, 1, "unchanged source fact is not republished")
	flight.Callsign = "SAS102"
	setObservationCacheSnapshot(cache, now.Add(time.Second), nil, flight)
	now = now.Add(time.Second)
	require.NoError(t, worker.Publish(context.Background()))
	require.Contains(t, worker.known, "SAS102")
	require.NotContains(t, worker.known, "SAS101")
}

func TestObservationWorkerDoesNotRepublishForCIDOnlyChange(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	flight := Flight{CID: "101", Callsign: "SAS101", State: FlightStatePrefile, LastUpdated: now, FlightPlan: FlightPlan{Origin: "ENGM", Destination: "EKCH", Revision: 1}}
	cache := newReconciliationTestCache(now, flight)
	sink := &observationTestSink{}
	worker, _ := newObservationTestWorker(t, cache, &now, sink)
	require.NoError(t, worker.Publish(context.Background()))
	first := sink.observations[0]
	flight.CID, flight.Callsign = "202", "sas101"
	now = now.Add(time.Second)
	setObservationCacheSnapshot(cache, now, nil, flight)
	require.NoError(t, worker.Publish(context.Background()))
	require.Len(t, sink.observations, 1, "CID is not part of an AMAN observation")
	require.Equal(t, "SAS101", first.Callsign)
}

func TestObservationWorkerSkipsNonLiveSessions(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	flight := Flight{CID: "101", Callsign: "SAS101", State: FlightStateOnline, LastUpdated: now,
		FlightPlan: FlightPlan{Origin: "ENGM", Destination: "EKCH", Revision: 1}}
	cache := newReconciliationTestCache(now, flight)
	sink := &observationTestSink{}
	worker, sessions := newObservationTestWorker(t, cache, &now, sink)
	sessions.items[0].Name = "PLAYBACK"

	require.NoError(t, worker.Publish(context.Background()))
	require.Empty(t, sink.observations)
	require.Equal(t, []aman.DataStatus{aman.DataDisconnected}, sink.sourceHealth)
}

func TestObservationWorkerOfflineReplayPublishesWithoutLiveSession(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	flight := Flight{CID: "101", Callsign: "SAS101", State: FlightStateOnline, LastUpdated: now,
		FlightPlan: FlightPlan{Origin: "ENGM", Destination: "EKCH", Revision: 1}}
	cache := newReconciliationTestCache(now, flight)
	sink := &observationTestSink{}
	sessions := &reconciliationTestSessions{items: []*models.Session{{ID: 7, Name: "PLAYBACK", Airport: "EKCH"}}}
	worker, err := NewObservationWorker(ObservationWorkerDependencies{
		Cache: cache, Sessions: sessions, Sink: sink, EnabledAirports: []string{"EKCH"}, OfflineReplay: true,
		StaleAfter: time.Minute, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	require.NoError(t, worker.Publish(context.Background()))
	require.Len(t, sink.observations, 1)
	require.Equal(t, aman.ObservationProviderVATSIM, sink.observations[0].Provider)
}

func TestObservationWorkerRetractsFlightsWhenLastLiveSessionEnds(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	flight := Flight{CID: "101", Callsign: "SAS101", State: FlightStateOnline, LastUpdated: now,
		FlightPlan: FlightPlan{Origin: "ENGM", Destination: "EKCH", Revision: 1}}
	cache := newReconciliationTestCache(now, flight)
	sink := &observationTestSink{}
	worker, sessions := newObservationTestWorker(t, cache, &now, sink)
	require.NoError(t, worker.Publish(context.Background()))

	sessions.items[0].Name = "PLAYBACK"
	now = now.Add(time.Second)
	require.NoError(t, worker.Publish(context.Background()))
	require.Len(t, sink.observations, 2)
	require.True(t, sink.observations[1].Missing)
	require.Equal(t, aman.DataDisconnected, sink.observations[1].SourceStatus)
	require.Equal(t, []aman.DataStatus{aman.DataFresh, aman.DataDisconnected}, sink.sourceHealth)
	require.Empty(t, worker.known)
}

func TestObservationWorkerRetractsPreviousAirportOnDestinationChange(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	flight := Flight{CID: "101", Callsign: "SAS101", State: FlightStateOnline, LastUpdated: now,
		FlightPlan: FlightPlan{Origin: "ENGM", Destination: "EKCH", Revision: 1}}
	cache := newReconciliationTestCache(now, flight)
	sink := &observationTestSink{}
	sessions := &reconciliationTestSessions{items: []*models.Session{
		{ID: 7, Name: "LIVE", Airport: "EKCH"},
		{ID: 8, Name: "LIVE", Airport: "EKBI"},
	}}
	worker, err := NewObservationWorker(ObservationWorkerDependencies{
		Cache: cache, Sessions: sessions, Sink: sink, EnabledAirports: []string{"EKCH", "EKBI"}, StaleAfter: time.Minute,
		Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	require.NoError(t, worker.Publish(context.Background()))

	now = now.Add(time.Second)
	flight.LastUpdated = now
	flight.FlightPlan.Destination = "EKBI"
	flight.FlightPlan.Revision = 2
	setObservationCacheSnapshot(cache, now, nil, flight)
	require.NoError(t, worker.Publish(context.Background()))

	require.Len(t, sink.observations, 3)
	require.Equal(t, "EKCH", sink.observations[1].Destination)
	require.True(t, sink.observations[1].Missing)
	require.Equal(t, "EKBI", sink.observations[2].Destination)
	require.False(t, sink.observations[2].Missing)
	require.Equal(t, "EKBI", worker.known["SAS101"].Destination)
}

func TestObservationWorkerIgnoresCIDChanges(t *testing.T) {
	now := time.Date(2026, time.September, 14, 18, 0, 0, 0, time.UTC)
	flight := Flight{CID: "101", Callsign: "SAS123", State: FlightStateOnline, LastUpdated: now,
		Latitude: 55, Longitude: 12, Altitude: 18000, Groundspeed: 400,
		FlightPlan: FlightPlan{Origin: "ENGM", Destination: "EKCH", Route: "OLD ROUTE", Revision: 8, EOBT: "1700", EnrouteDuration: "0100"}}
	cache := newReconciliationTestCache(now, flight)
	sink := &observationTestSink{}
	worker, _ := newObservationTestWorker(t, cache, &now, sink)
	require.NoError(t, worker.Publish(context.Background()))
	first := sink.observations[0]
	require.NotNil(t, first.TakeoffDetected)
	now = now.Add(15 * time.Second)
	flight.CID, flight.LastUpdated = "202", now
	flight.Latitude, flight.Longitude, flight.Altitude, flight.Groundspeed = 56, 13, 0, 0
	flight.FlightPlan = FlightPlan{Origin: "ESSA", Destination: "EKCH", Route: "NEW ROUTE", Revision: 1, EOBT: "1800", EnrouteDuration: "0200"}
	setObservationCacheSnapshot(cache, now, nil, flight)
	require.NoError(t, worker.Publish(context.Background()))
	require.Len(t, sink.observations, 2)
	current := sink.observations[1]
	require.Equal(t, first.Callsign, current.Callsign)
	require.Equal(t, "OLD ROUTE", *current.FiledRoute)
	require.Equal(t, "ENGM", current.Origin)
	require.EqualValues(t, 8, *current.FlightPlan.Revision)
	require.NotNil(t, current.TakeoffDetected)
}

func TestObservationWorkerPublishesExplicitDisappearance(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	flight := Flight{CID: "101", Callsign: "SAS101", State: FlightStateOnline, Latitude: 55, Longitude: 12, Altitude: 10000, Groundspeed: 300, LastUpdated: now, FlightPlan: FlightPlan{Origin: "ENGM", Destination: "EKCH", Revision: 1}}
	cache := newReconciliationTestCache(now, flight)
	sink := &observationTestSink{}
	worker, _ := newObservationTestWorker(t, cache, &now, sink)
	require.NoError(t, worker.Publish(context.Background()))

	now = now.Add(15 * time.Second)
	setObservationCacheSnapshot(cache, now, nil)
	require.NoError(t, worker.Publish(context.Background()))
	require.Len(t, sink.observations, 2)
	require.True(t, sink.observations[1].Missing)
	require.Equal(t, aman.DataFresh, sink.observations[1].SourceStatus)
	require.NotContains(t, worker.known, "SAS101")
}

func TestObservationWorkerPublishesHealthForFreshEmptySnapshot(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	cache := newReconciliationTestCache(now)
	sink := &observationTestSink{}
	worker, _ := newObservationTestWorker(t, cache, &now, sink)
	require.NoError(t, worker.Publish(context.Background()))
	require.Empty(t, sink.observations)
	require.Equal(t, []aman.DataStatus{aman.DataFresh}, sink.sourceHealth)
}

func TestObservationWorkerContinuesAfterPerFlightMappingAndDeliveryFailures(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	malformed := Flight{CID: "101", Callsign: "BAD101", State: FlightStatePrefile, LastUpdated: now, FlightPlan: FlightPlan{Destination: "EKCH", Revision: 1}}
	good := Flight{CID: "202", Callsign: "SAS202", State: FlightStatePrefile, LastUpdated: now, FlightPlan: FlightPlan{Origin: "ENGM", Destination: "EKCH", Revision: 1}}
	deliveryFailure := Flight{CID: "303", Callsign: "SAS303", State: FlightStatePrefile, LastUpdated: now, FlightPlan: FlightPlan{Origin: "EGLL", Destination: "EKCH", Revision: 1}}
	cache := newReconciliationTestCache(now, malformed, good, deliveryFailure)
	sink := &observationTestSink{errsByCallsign: map[string]error{"SAS303": errors.New("temporary sink failure")}}
	worker, _ := newObservationTestWorker(t, cache, &now, sink)
	err := worker.Publish(context.Background())
	require.ErrorContains(t, err, "map VATSIM observation for callsign BAD101")
	require.ErrorContains(t, err, "publish VATSIM observation for callsign SAS303")
	require.Len(t, sink.observations, 1)
	require.Equal(t, "SAS202", sink.observations[0].Callsign)
	require.Contains(t, worker.known, "SAS202")
	require.NotContains(t, worker.known, "SAS303")

	delete(sink.errsByCallsign, "SAS303")
	setObservationCacheSnapshot(cache, now, nil, good, deliveryFailure)
	require.NoError(t, worker.Publish(context.Background()))
	require.Len(t, sink.observations, 2, "the valid retry publishes while already-delivered facts are not flooded")
	require.Equal(t, "SAS303", sink.observations[1].Callsign)
}

func TestObservationWorkerPublishesSourceStatusTransitionsWithoutFlooding(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	flight := Flight{CID: "101", Callsign: "SAS101", State: FlightStatePrefile, LastUpdated: now, FlightPlan: FlightPlan{Origin: "ENGM", Destination: "EKCH", Revision: 1}}
	cache := newReconciliationTestCache(now, flight)
	sink := &observationTestSink{}
	worker, _ := newObservationTestWorker(t, cache, &now, sink)

	require.NoError(t, worker.Publish(context.Background()))
	now = now.Add(2 * time.Minute)
	require.NoError(t, worker.Publish(context.Background()))
	require.NoError(t, worker.Publish(context.Background()), "unchanged stale state is not re-emitted")
	setObservationCacheSnapshot(cache, cache.snapshot.timestamp, errors.New("network unavailable"))
	require.NoError(t, worker.Publish(context.Background()))
	setObservationCacheSnapshot(cache, now, nil, flight)
	require.NoError(t, worker.Publish(context.Background()))

	require.Len(t, sink.observations, 4)
	require.Equal(t, []aman.DataStatus{aman.DataFresh, aman.DataStale, aman.DataDisconnected, aman.DataFresh}, []aman.DataStatus{
		sink.observations[0].SourceStatus, sink.observations[1].SourceStatus, sink.observations[2].SourceStatus, sink.observations[3].SourceStatus,
	})
}

func TestObservationWorkerIsolatedFromSATReconciliationFailure(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	flight := Flight{CID: "101", Callsign: "SAS101", State: FlightStatePrefile, LastUpdated: now, FlightPlan: FlightPlan{Origin: "ENGM", Destination: "EKCH", Revision: 1}}
	cache := newReconciliationTestCache(now, flight)
	strips := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{}}
	reconciler := newTestReconciler(cache, reconciliationTestSessions{items: []*models.Session{{ID: 7, Name: "LIVE", Airport: "EKCH"}}}, strips, reconciliationTestAssignments{}, nil, time.Second)
	reconciler.arrivalLifecycle = failingArrivalLifecycle{}
	require.Error(t, reconciler.Reconcile(context.Background()))

	sink := &observationTestSink{}
	worker, _ := newObservationTestWorker(t, cache, &now, sink)
	require.NoError(t, worker.Publish(context.Background()))
	require.Len(t, sink.observations, 1)
}

type failingArrivalLifecycle struct{}

func (failingArrivalLifecycle) ProcessArrival(context.Context, int32, *models.Strip, ArrivalFlightInfo) error {
	return errors.New("SAT unavailable")
}

func (failingArrivalLifecycle) CancelArrival(context.Context, int32, string) error { return nil }

func setObservationCacheSnapshot(cache *Cache, timestamp time.Time, refreshError error, flights ...Flight) {
	snapshot := newCacheSnapshot(timestamp, timestamp)
	for _, flight := range flights {
		snapshot.add(flight)
	}
	cache.snapshot = snapshot
	cache.lastRefreshError = refreshError
}

func TestObservationPackageForbidsSATEuroScopeAndETALeakage(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	source, err := os.ReadFile(filepath.Join(filepath.Dir(file), "aman_observations.go"))
	require.NoError(t, err)
	for _, forbidden := range []string{"internal/sat", "internal/euroscope", "ArrivalETA", "TETA", "FlightPlanDTO"} {
		require.NotContains(t, string(source), forbidden)
	}
}

func stringPointer(value string) *string { return &value }
func intPointer(value int) *int          { return &value }
