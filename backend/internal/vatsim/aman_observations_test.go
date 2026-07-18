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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type observationTestBinder struct {
	mu       sync.Mutex
	next     int
	byCID    map[string]aman.FlightID
	bindings []aman.VATSIMFlightIdentity
}

func (b *observationTestBinder) BindVATSIMFlight(_ context.Context, identity aman.VATSIMFlightIdentity) (aman.FlightID, error) {
	if err := identity.Validate(); err != nil {
		return "", err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.byCID == nil {
		b.byCID = make(map[string]aman.FlightID)
	}
	if id, ok := b.byCID[identity.VATSIMCID]; ok {
		b.bindings = append(b.bindings, identity)
		return id, nil
	}
	b.next++
	id := aman.FlightID("flight-" + string(rune('0'+b.next)))
	b.byCID[identity.VATSIMCID] = id
	b.bindings = append(b.bindings, identity)
	return id, nil
}

type observationTestSink struct {
	observations []aman.FlightObservation
	err          error
	errsByCID    map[string]error
}

func (s *observationTestSink) Observe(_ context.Context, observation aman.FlightObservation) error {
	if s.err != nil {
		return s.err
	}
	if err := s.errsByCID[observation.VATSIMCID]; err != nil {
		return err
	}
	s.observations = append(s.observations, observation)
	return nil
}

func newObservationTestWorker(t *testing.T, cache *Cache, now *time.Time, sink *observationTestSink) (*ObservationWorker, *observationTestBinder) {
	t.Helper()
	binder := &observationTestBinder{}
	worker, err := NewObservationWorker(ObservationWorkerDependencies{
		Cache: cache, Identities: binder, Sink: sink, EnabledAirports: []string{"EKCH"}, StaleAfter: time.Minute,
		Now: func() time.Time { return *now },
	})
	require.NoError(t, err)
	return worker, binder
}

func TestObservationWorkerMapsPrefileAndOnlineFlightsWithoutSession(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	cache := newReconciliationTestCache(now,
		Flight{CID: "101", Callsign: "SAS101", State: FlightStatePrefile, LastUpdated: now.Add(-time.Minute), FlightPlan: FlightPlan{Origin: "ENGM", Destination: "EKCH", Aircraft: "A20N/M-SDE2", AircraftShort: "A20N", RequestedLevel: "F350", Route: "NEXIL M725", EOBT: "1130", EnrouteDuration: "0145", Revision: 4}},
		Flight{CID: "202", Callsign: "SAS202", State: FlightStateOnline, Latitude: 55.1, Longitude: 12.1, Altitude: 18000, Groundspeed: 420, LastUpdated: now, FlightPlan: FlightPlan{Origin: "EGLL", Destination: "EKCH", Aircraft: "B738/H-SDE2", AircraftShort: "B738", RequestedLevel: "34000", Route: "L620", EOBT: "0930", EnrouteDuration: "0200", Revision: 7}},
	)
	sink := &observationTestSink{}
	worker, _ := newObservationTestWorker(t, cache, &now, sink)

	require.NoError(t, worker.Publish(context.Background()))
	require.Len(t, sink.observations, 2, "no session or EuroScope client is needed")
	sort.Slice(sink.observations, func(i, j int) bool { return sink.observations[i].VATSIMCID < sink.observations[j].VATSIMCID })
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

func TestObservationWorkerPreservesFlightIDAndRejectsOlderFacts(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	newer := Flight{CID: "101", Callsign: "SAS101", State: FlightStateOnline, Latitude: 55.1, Longitude: 12.1, Altitude: 18000, Groundspeed: 420, LastUpdated: now, FlightPlan: FlightPlan{Origin: "EGLL", Destination: "EKCH", Route: "NEW ROUTE", Revision: 8}}
	cache := newReconciliationTestCache(now, newer)
	sink := &observationTestSink{}
	worker, _ := newObservationTestWorker(t, cache, &now, sink)
	require.NoError(t, worker.Publish(context.Background()))
	first := sink.observations[0]

	older := newer
	older.Callsign = "SAS102"
	older.LastUpdated = now.Add(-time.Minute)
	older.Latitude = 54.0
	older.FlightPlan.Route = "OLD ROUTE"
	older.FlightPlan.Revision = 7
	setObservationCacheSnapshot(cache, now.Add(time.Second), nil, older)
	now = now.Add(time.Second)
	require.NoError(t, worker.Publish(context.Background()))
	require.Len(t, sink.observations, 2)
	second := sink.observations[1]
	require.Equal(t, first.FlightID, second.FlightID, "callsign corrections must not rekey AMAN state")
	require.Equal(t, "SAS102", second.Callsign)
	require.Equal(t, "NEW ROUTE", *second.FiledRoute)
	require.Equal(t, uint64(8), *second.FlightPlan.Revision)
	require.Equal(t, 55.1, second.Surveillance.LatitudeDegrees)
	require.Equal(t, *first.Surveillance.Sequence, *second.Surveillance.Sequence)
}

func TestObservationWorkerReusesKnownIDButRebindsCallsignCorrection(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	flight := Flight{CID: "101", Callsign: "SAS101", State: FlightStatePrefile, LastUpdated: now, FlightPlan: FlightPlan{Origin: "ENGM", Destination: "EKCH", Revision: 1}}
	cache := newReconciliationTestCache(now, flight)
	sink := &observationTestSink{}
	worker, binder := newObservationTestWorker(t, cache, &now, sink)
	require.NoError(t, worker.Publish(context.Background()))
	require.NoError(t, worker.Publish(context.Background()))
	require.Len(t, binder.bindings, 1, "unchanged source fact reuses the delivered FlightID")
	flight.Callsign = "SAS102"
	setObservationCacheSnapshot(cache, now.Add(time.Second), nil, flight)
	now = now.Add(time.Second)
	require.NoError(t, worker.Publish(context.Background()))
	require.Len(t, binder.bindings, 2, "callsign correction must verify the active binding")
}

func TestObservationWorkerContinuesAfterPerFlightMappingAndDeliveryFailures(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	malformed := Flight{CID: "101", Callsign: "BAD101", State: FlightStatePrefile, LastUpdated: now, FlightPlan: FlightPlan{Destination: "EKCH", Revision: 1}}
	good := Flight{CID: "202", Callsign: "SAS202", State: FlightStatePrefile, LastUpdated: now, FlightPlan: FlightPlan{Origin: "ENGM", Destination: "EKCH", Revision: 1}}
	deliveryFailure := Flight{CID: "303", Callsign: "SAS303", State: FlightStatePrefile, LastUpdated: now, FlightPlan: FlightPlan{Origin: "EGLL", Destination: "EKCH", Revision: 1}}
	cache := newReconciliationTestCache(now, malformed, good, deliveryFailure)
	sink := &observationTestSink{errsByCID: map[string]error{"303": errors.New("temporary sink failure")}}
	worker, _ := newObservationTestWorker(t, cache, &now, sink)
	err := worker.Publish(context.Background())
	require.ErrorContains(t, err, "map VATSIM observation for CID 101")
	require.ErrorContains(t, err, "publish VATSIM observation for CID 303")
	require.Len(t, sink.observations, 1)
	require.Equal(t, "202", sink.observations[0].VATSIMCID)
	require.Contains(t, worker.known, "202")
	require.NotContains(t, worker.known, "303")

	delete(sink.errsByCID, "303")
	setObservationCacheSnapshot(cache, now, nil, good, deliveryFailure)
	require.NoError(t, worker.Publish(context.Background()))
	require.Len(t, sink.observations, 2, "the valid retry publishes while already-delivered facts are not flooded")
	require.Equal(t, "303", sink.observations[1].VATSIMCID)
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
	reconciler := newTestReconciler(cache, reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, strips, reconciliationTestAssignments{}, nil, time.Second)
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
