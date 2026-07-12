package vatsim

import (
	"FlightStrips/internal/models"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type reconciliationTestStrips struct {
	bySession map[int32][]*models.Strip
	created   []*models.Strip
	updated   []*models.Strip
	deleted   []string
}

func (s *reconciliationTestStrips) List(_ context.Context, session int32) ([]*models.Strip, error) {
	return s.bySession[session], nil
}

func (s *reconciliationTestStrips) Create(_ context.Context, strip *models.Strip) error {
	s.created = append(s.created, strip)
	s.bySession[strip.Session] = append(s.bySession[strip.Session], strip)
	return nil
}

func (s *reconciliationTestStrips) Update(_ context.Context, strip *models.Strip) (int64, error) {
	s.updated = append(s.updated, strip)
	return 1, nil
}

func (s *reconciliationTestStrips) UpdateVatsimSource(_ context.Context, session int32, callsign string, source models.VatsimStripSource) (int64, error) {
	for _, strip := range s.bySession[session] {
		if strip.Callsign != callsign {
			continue
		}
		strip.VatsimCID = ptr(source.CID)
		strip.VatsimRevision = int64ptr(source.Revision)
		strip.VatsimSeenAt = timeptr(source.SeenAt)
		if strip.EuroscopeSeenAt == nil || !strip.EuroscopeSeenAt.After(source.SeenAt) {
			if !controllerModified(strip, "route") {
				strip.Route = ptr(source.Route)
			}
		}
		s.updated = append(s.updated, strip)
		return 1, nil
	}
	return 0, nil
}

func (s *reconciliationTestStrips) UpdateArrivalETA(_ context.Context, session int32, callsign string, eta models.ArrivalETA) (int64, error) {
	for _, strip := range s.bySession[session] {
		if strip.Callsign != callsign {
			continue
		}
		strip.ArrivalETA = &eta
		s.updated = append(s.updated, strip)
		return 1, nil
	}
	return 0, nil
}

func (s *reconciliationTestStrips) UpdateBayAndSequence(_ context.Context, session int32, callsign, bay string, sequence int32) (int64, error) {
	for _, strip := range s.bySession[session] {
		if strip.Callsign == callsign {
			strip.Bay = bay
			strip.Sequence = &sequence
			s.updated = append(s.updated, strip)
			return 1, nil
		}
	}
	return 0, nil
}

func (s *reconciliationTestStrips) Delete(_ context.Context, session int32, callsign string) error {
	s.deleted = append(s.deleted, callsign)
	return nil
}

type reconciliationTestSessions struct{ items []*models.Session }

func (s reconciliationTestSessions) List(context.Context) ([]*models.Session, error) {
	return s.items, nil
}

type reconciliationTestAssignments struct{ active map[string]bool }

func (s reconciliationTestAssignments) GetAssignment(_ context.Context, session int32, callsign string) (*models.StandAssignment, error) {
	if s.active[assignmentKey(session, callsign)] {
		return &models.StandAssignment{SessionID: session, Callsign: callsign}, nil
	}
	return nil, errors.New("not found")
}

func assignmentKey(session int32, callsign string) string {
	return string(rune(session)) + ":" + callsign
}

// expiryTestAssignments reports an assignment with a controllable expiry so the
// reconciler's retainer can distinguish active reservations from expired ones.
type expiryTestAssignments struct {
	expiry map[string]*time.Time
}

func (s expiryTestAssignments) GetAssignment(_ context.Context, session int32, callsign string) (*models.StandAssignment, error) {
	if expiry, ok := s.expiry[assignmentKey(session, callsign)]; ok {
		return &models.StandAssignment{SessionID: session, Callsign: callsign, ExpiresAt: expiry}, nil
	}
	return nil, errors.New("not found")
}

type reconciliationTestNotifier struct{ callsigns []string }

func (n *reconciliationTestNotifier) SendStripUpdate(_ int32, callsign string) {
	n.callsigns = append(n.callsigns, callsign)
}

type reconciliationTestDepartureLifecycle struct {
	cancelled []string
}

func (*reconciliationTestDepartureLifecycle) ProcessDeparture(context.Context, int32, *models.Strip, DepartureFlightInfo) error {
	return nil
}

func (l *reconciliationTestDepartureLifecycle) CancelDeparture(_ context.Context, _ int32, callsign string) error {
	l.cancelled = append(l.cancelled, callsign)
	return nil
}

func newReconciliationTestCache(now time.Time, flights ...Flight) *Cache {
	cache := NewCache("", time.Second, nil)
	snapshot := newCacheSnapshot(now, now)
	for _, flight := range flights {
		snapshot.add(flight)
	}
	cache.snapshot = snapshot
	return cache
}

func TestReconcileCreatesPrefileDepartureAndHiddenArrivals(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	cache := newReconciliationTestCache(now,
		Flight{CID: "1", Callsign: "SAS101", State: FlightStatePrefile, LastUpdated: now, FlightPlan: FlightPlan{Origin: "EKCH", Destination: "EGLL", Revision: 4}},
		Flight{CID: "2", Callsign: "SAS202", State: FlightStatePrefile, LastUpdated: now, FlightPlan: FlightPlan{Origin: "EGLL", Destination: "EKCH", Revision: 5}},
		Flight{CID: "3", Callsign: "SAS303", State: FlightStateOnline, Latitude: 51.5, Longitude: -0.4, Altitude: 32000, LastUpdated: now, FlightPlan: FlightPlan{Origin: "EGLL", Destination: "EKCH", Revision: 6}},
	)
	strips := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{}}
	notifier := &reconciliationTestNotifier{}
	reconciler := NewReconciler(cache, reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, strips, reconciliationTestAssignments{}, notifier, time.Second)

	require.NoError(t, reconciler.Reconcile(context.Background()))
	require.Len(t, strips.created, 3)
	byCallsign := map[string]*models.Strip{}
	for _, strip := range strips.created {
		byCallsign[strip.Callsign] = strip
	}
	assert.Equal(t, hiddenDepartureBay, byCallsign["SAS101"].Bay)
	assert.Equal(t, hiddenArrivalBay, byCallsign["SAS202"].Bay)
	assert.Equal(t, hiddenArrivalBay, byCallsign["SAS303"].Bay)
	assert.Nil(t, byCallsign["SAS202"].PositionLatitude)
	assert.Equal(t, 51.5, *byCallsign["SAS303"].PositionLatitude)
	assert.Equal(t, "3", *byCallsign["SAS303"].VatsimCID)
	assert.Equal(t, int64(6), *byCallsign["SAS303"].VatsimRevision)
	assert.Len(t, notifier.callsigns, 3)
}

func TestReconcilePromotesAPIDepartureWhenPilotConnects(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	sequence := int32(1000)
	existing := &models.Strip{Callsign: "SAS101", Session: 7, Origin: "EKCH", Destination: "EGLL", Bay: hiddenDepartureBay, Sequence: &sequence}
	cache := newReconciliationTestCache(now, Flight{CID: "1", Callsign: "SAS101", State: FlightStateOnline, LastUpdated: now, FlightPlan: FlightPlan{Origin: "EKCH", Destination: "EGLL", Revision: 4}})
	strips := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{7: {existing}}}
	reconciler := NewReconciler(cache, reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, strips, reconciliationTestAssignments{}, nil, time.Second)

	require.NoError(t, reconciler.Reconcile(context.Background()))
	assert.Equal(t, plannedDepartureBay, existing.Bay)
}

func TestReconcileMovesExistingAPIPrefileOutOfCLX(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	sequence := int32(1000)
	existing := &models.Strip{Callsign: "SAS101", Session: 7, Origin: "EKCH", Destination: "EGLL", Bay: plannedDepartureBay, Sequence: &sequence}
	cache := newReconciliationTestCache(now, Flight{CID: "1", Callsign: "SAS101", State: FlightStatePrefile, LastUpdated: now, FlightPlan: FlightPlan{Origin: "EKCH", Destination: "EGLL", Revision: 4}})
	strips := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{7: {existing}}}
	reconciler := NewReconciler(cache, reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, strips, reconciliationTestAssignments{}, nil, time.Second)

	require.NoError(t, reconciler.Reconcile(context.Background()))
	assert.Equal(t, hiddenDepartureBay, existing.Bay)
}

func TestReconcileDoesNotMoveEuroscopeOwnedPrefileToHidden(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	existing := &models.Strip{Callsign: "SAS101", Session: 7, Origin: "EKCH", Destination: "EGLL", Bay: plannedDepartureBay, EuroscopeSeenAt: &now}
	cache := newReconciliationTestCache(now, Flight{CID: "1", Callsign: "SAS101", State: FlightStatePrefile, LastUpdated: now, FlightPlan: FlightPlan{Origin: "EKCH", Destination: "EGLL", Revision: 4}})
	strips := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{7: {existing}}}
	reconciler := NewReconciler(cache, reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, strips, reconciliationTestAssignments{}, nil, time.Second)

	require.NoError(t, reconciler.Reconcile(context.Background()))
	assert.Equal(t, plannedDepartureBay, existing.Bay)
}

func TestReconcileKeepsEuroscopeFieldsAndProtectsControllerEdits(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	euroscopeSeen := now.Add(time.Minute)
	controllerRoute := "CONTROLLER ROUTE"
	existing := &models.Strip{
		Callsign: "SAS404", Session: 7, Origin: "EKCH", Destination: "EDDF", Route: &controllerRoute,
		Stand: ptr("A12"), Bay: plannedDepartureBay, EuroscopeSeenAt: &euroscopeSeen,
		ControllerModifiedFields: []string{"route", "stand"},
	}
	cache := newReconciliationTestCache(now, Flight{CID: "4", Callsign: "SAS404", State: FlightStateOnline, LastUpdated: now, FlightPlan: FlightPlan{Origin: "EKCH", Destination: "EDDF", Route: "VATSIM ROUTE", Revision: 7}})
	strips := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{7: {existing}}}
	reconciler := NewReconciler(cache, reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, strips, reconciliationTestAssignments{}, nil, time.Second)

	require.NoError(t, reconciler.Reconcile(context.Background()))
	assert.Empty(t, strips.created, "the matching callsign must not create a duplicate strip")
	assert.Equal(t, controllerRoute, *existing.Route)
	assert.Equal(t, "A12", *existing.Stand)
	assert.Equal(t, "4", *existing.VatsimCID, "provenance is retained even when EuroScope wins fields")
}

func TestReconcileUsesNewerVatsimFieldsWhenEuroScopeDataIsOlder(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	euroscopeSeen := now.Add(-time.Minute)
	oldRoute := "OLD ROUTE"
	existing := &models.Strip{Callsign: "SAS707", Session: 7, Origin: "EKCH", Destination: "EDDF", Route: &oldRoute, Bay: plannedDepartureBay, EuroscopeSeenAt: &euroscopeSeen}
	cache := newReconciliationTestCache(now, Flight{CID: "7", Callsign: "SAS707", State: FlightStateOnline, LastUpdated: now, FlightPlan: FlightPlan{Origin: "EKCH", Destination: "EDDF", Route: "NEW ROUTE", Revision: 8}})
	strips := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{7: {existing}}}
	reconciler := NewReconciler(cache, reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, strips, reconciliationTestAssignments{}, nil, time.Second)

	require.NoError(t, reconciler.Reconcile(context.Background()))
	require.Len(t, strips.updated, 1)
	assert.Equal(t, "NEW ROUTE", *existing.Route)
}

func TestReconcileCleanupAndDisconnectRetentionRespectOtherOwners(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	vatsimCID := "5"
	stale := &models.Strip{Callsign: "SAS505", Session: 7, VatsimCID: &vatsimCID}
	assignedCID := "6"
	assigned := &models.Strip{Callsign: "SAS606", Session: 7, VatsimCID: &assignedCID}
	cache := newReconciliationTestCache(now)
	strips := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{7: {stale, assigned}}}
	assignments := reconciliationTestAssignments{active: map[string]bool{assignmentKey(7, "SAS606"): true}}
	reconciler := NewReconciler(cache, reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, strips, assignments, nil, time.Second)

	require.NoError(t, reconciler.Reconcile(context.Background()))
	assert.Equal(t, []string{"SAS505"}, strips.deleted)
	assert.True(t, reconciler.RetainsStrip(context.Background(), 7, "SAS606"))
	assert.False(t, reconciler.RetainsStrip(context.Background(), 7, "SAS505"))
}

func TestReconcileCancelsDepartureLifecycleWhenFlightDisappears(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	cid := "5"
	strip := &models.Strip{Callsign: "SAS505", Session: 7, Origin: "EKCH", VatsimCID: &cid}
	strips := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{7: {strip}}}
	reconciler := NewReconciler(newReconciliationTestCache(now), reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, strips, reconciliationTestAssignments{}, nil, time.Second)
	lifecycle := &reconciliationTestDepartureLifecycle{}
	reconciler.SetDepartureLifecycle(lifecycle)

	require.NoError(t, reconciler.Reconcile(context.Background()))
	assert.Equal(t, []string{"SAS505"}, lifecycle.cancelled)
}

func TestRetainsStripHonorsReservationExpiry(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	active := now.Add(15 * time.Minute)
	expired := now.Add(-time.Minute)
	assignments := expiryTestAssignments{expiry: map[string]*time.Time{
		assignmentKey(7, "SAS1"): &active,
		assignmentKey(7, "SAS2"): &expired,
	}}
	cache := newReconciliationTestCache(now)
	reconciler := NewReconciler(cache, reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, &reconciliationTestStrips{bySession: map[int32][]*models.Strip{}}, assignments, nil, time.Second, WithClock(func() time.Time { return now }))

	assert.True(t, reconciler.RetainsStrip(context.Background(), 7, "SAS1"), "an active reservation keeps the strip alive")
	assert.False(t, reconciler.RetainsStrip(context.Background(), 7, "SAS2"), "an expired reservation no longer retains the strip")
}
