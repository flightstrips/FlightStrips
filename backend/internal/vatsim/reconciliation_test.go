package vatsim

import (
	"FlightStrips/internal/models"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApiDepartureBayPreservesCompletedDepartureAfterEuroscopeDisconnect(t *testing.T) {
	strip := &models.Strip{Bay: hiddenCompletedDepartureBay}
	flight := Flight{State: FlightStateOnline, FlightPlan: FlightPlan{Origin: "EKCH"}}

	assert.Empty(t, apiDepartureBay(strip, flight, "EKCH"))
}

func TestApiDepartureBayDoesNotClassifyLocalArrivalAsDeparture(t *testing.T) {
	strip := &models.Strip{Bay: hiddenArrivalBay}
	flight := Flight{State: FlightStateOnline, FlightPlan: FlightPlan{Origin: "EKCH", Destination: "EKCH"}}

	assert.Empty(t, apiDepartureBay(strip, flight, "EKCH"))
}

func TestApiDepartureBayCorrectsCompletedDepartureReclassifiedAsArrival(t *testing.T) {
	strip := &models.Strip{Bay: hiddenCompletedDepartureBay}
	flight := Flight{State: FlightStateOnline, FlightPlan: FlightPlan{Origin: "ESSA", Destination: " ekch "}}

	assert.Equal(t, hiddenArrivalBay, apiDepartureBay(strip, flight, "EKCH"))
}

type reconciliationTestStrips struct {
	bySession map[int32][]*models.Strip
	created   []*models.Strip
	updated   []*models.Strip
	deleted   []string
}

type reconciliationSessionFailureStrips struct {
	*reconciliationTestStrips
	failSession int32
	listed      []int32
}

func (s *reconciliationSessionFailureStrips) List(ctx context.Context, session int32) ([]*models.Strip, error) {
	s.listed = append(s.listed, session)
	if session == s.failSession {
		return nil, errors.New("forced session failure")
	}
	return s.reconciliationTestStrips.List(ctx, session)
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

type reconciliationTestAssignments struct {
	active      map[string]bool
	assignments map[string]*models.StandAssignment
}

type reconciliationBulkAssignments struct {
	items     []*models.StandAssignment
	listCalls int
	getCalls  int
}

func (s *reconciliationBulkAssignments) ListAssignments(context.Context, int32) ([]*models.StandAssignment, error) {
	s.listCalls++
	return s.items, nil
}

func (s *reconciliationBulkAssignments) GetAssignment(context.Context, int32, string) (*models.StandAssignment, error) {
	s.getCalls++
	return nil, pgx.ErrNoRows
}

func (s reconciliationTestAssignments) GetAssignment(_ context.Context, session int32, callsign string) (*models.StandAssignment, error) {
	if assignment := s.assignments[assignmentKey(session, callsign)]; assignment != nil {
		return assignment, nil
	}
	if s.active[assignmentKey(session, callsign)] {
		return &models.StandAssignment{SessionID: session, Callsign: callsign}, nil
	}
	return nil, pgx.ErrNoRows
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
	return nil, pgx.ErrNoRows
}

type reconciliationTestNotifier struct{ callsigns []string }

func (n *reconciliationTestNotifier) SendStripUpdate(_ int32, callsign string) {
	n.callsigns = append(n.callsigns, callsign)
}

type reconciliationTestDepartureLifecycle struct {
	cancelled []string
	processed []string
	released  []string
}

func (l *reconciliationTestDepartureLifecycle) ProcessDeparture(_ context.Context, _ int32, strip *models.Strip, _ DepartureFlightInfo) error {
	l.processed = append(l.processed, strip.Callsign)
	return nil
}

func (l *reconciliationTestDepartureLifecycle) CancelDeparture(_ context.Context, _ int32, callsign string) error {
	l.cancelled = append(l.cancelled, callsign)
	return nil
}

func (l *reconciliationTestDepartureLifecycle) ReleaseDepartureStand(_ context.Context, _ int32, callsign string) error {
	l.released = append(l.released, callsign)
	return nil
}

type reconciliationTestArrivalLifecycle struct {
	cancelled []string
	processed []string
}

type reconciliationOrderLifecycle struct {
	events     []string
	priorities map[string]int
}

type reconciliationConvergenceLifecycle struct {
	assignments     *reconciliationTestAssignments
	processed       []string
	capacityChanged bool
}

type reconciliationNonConvergingLifecycle struct {
	assignments *reconciliationTestAssignments
	processed   int
}

func (l *reconciliationNonConvergingLifecycle) ProcessArrival(_ context.Context, session int32, strip *models.Strip, _ ArrivalFlightInfo) error {
	l.processed++
	key := assignmentKey(session, strip.Callsign)
	updated := *l.assignments.assignments[key]
	updated.Version++
	l.assignments.assignments[key] = &updated
	return nil
}

func (*reconciliationNonConvergingLifecycle) CancelArrival(context.Context, int32, string) error {
	return nil
}

func (*reconciliationNonConvergingLifecycle) ArrivalProcessingPriority(*models.Strip, ArrivalFlightInfo, *models.StandAssignment) int {
	return 2
}

func (l *reconciliationConvergenceLifecycle) ProcessArrival(_ context.Context, session int32, strip *models.Strip, _ ArrivalFlightInfo) error {
	l.processed = append(l.processed, strip.Callsign)
	key := assignmentKey(session, strip.Callsign)
	if strip.Callsign == "AAA101" {
		if l.capacityChanged {
			l.assignments.assignments[key] = &models.StandAssignment{SessionID: session, Callsign: strip.Callsign, Stand: "E83", Stage: "ASSIGNED", Source: "AUTOMATIC"}
		}
		return nil
	}
	if !l.capacityChanged {
		l.capacityChanged = true
		updated := *l.assignments.assignments[key]
		updated.Version++
		l.assignments.assignments[key] = &updated
	}
	return nil
}

func (*reconciliationConvergenceLifecycle) CancelArrival(context.Context, int32, string) error {
	return nil
}

func (*reconciliationConvergenceLifecycle) ArrivalProcessingPriority(*models.Strip, ArrivalFlightInfo, *models.StandAssignment) int {
	return 2
}

func (l *reconciliationOrderLifecycle) ProcessDeparture(_ context.Context, _ int32, strip *models.Strip, _ DepartureFlightInfo) error {
	l.events = append(l.events, "departure:"+strip.Callsign)
	return nil
}

func (l *reconciliationOrderLifecycle) CancelDeparture(context.Context, int32, string) error {
	return nil
}

func (l *reconciliationOrderLifecycle) ProcessArrival(_ context.Context, _ int32, strip *models.Strip, _ ArrivalFlightInfo) error {
	l.events = append(l.events, "arrival:"+strip.Callsign)
	return nil
}

func (l *reconciliationOrderLifecycle) CancelArrival(_ context.Context, _ int32, callsign string) error {
	l.events = append(l.events, "cancel-arrival:"+callsign)
	return nil
}

func (l *reconciliationOrderLifecycle) ArrivalProcessingPriority(strip *models.Strip, _ ArrivalFlightInfo, assignment *models.StandAssignment) int {
	priority := l.priorities[strip.Callsign]
	if assignment != nil && assignment.Stage == "CONFIRMED" {
		return 3
	}
	return priority
}

func (l *reconciliationTestArrivalLifecycle) ProcessArrival(_ context.Context, _ int32, strip *models.Strip, _ ArrivalFlightInfo) error {
	l.processed = append(l.processed, strip.Callsign)
	return nil
}

func (l *reconciliationTestArrivalLifecycle) CancelArrival(_ context.Context, _ int32, callsign string) error {
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

func TestReconcileProcessesAllDeparturesBeforeArrivals(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	existingStand := "E82"
	cache := newReconciliationTestCache(now,
		Flight{CID: "1", Callsign: "AAA101", State: FlightStatePrefile, LastUpdated: now, FlightPlan: FlightPlan{Origin: "EGLL", Destination: "EKCH", Revision: 1}},
		Flight{CID: "2", Callsign: "BBB303", State: FlightStatePrefile, LastUpdated: now, FlightPlan: FlightPlan{Origin: "EDDF", Destination: "EKCH", Revision: 1}},
		Flight{CID: "4", Callsign: "CCC404", State: FlightStatePrefile, LastUpdated: now, FlightPlan: FlightPlan{Origin: "ESSA", Destination: "EKCH", Revision: 1}},
		Flight{CID: "3", Callsign: "ZZZ202", State: FlightStatePrefile, LastUpdated: now, FlightPlan: FlightPlan{Origin: "EKCH", Destination: "EDDF", Revision: 1}},
	)
	strips := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{7: {{Callsign: "CCC404", Session: 7, Destination: "EKCH", Stand: &existingStand}}}}
	lifecycle := &reconciliationOrderLifecycle{priorities: map[string]int{"AAA101": 1, "BBB303": 3, "CCC404": 3}}
	assignments := reconciliationTestAssignments{assignments: map[string]*models.StandAssignment{
		assignmentKey(7, "CCC404"): {SessionID: 7, Callsign: "CCC404", Stand: existingStand, Stage: "CONFIRMED"},
	}}
	reconciler := newTestReconciler(cache, reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, strips, assignments, nil, time.Second)
	reconciler.lifecycle = lifecycle
	reconciler.arrivalLifecycle = lifecycle

	require.NoError(t, reconciler.Reconcile(context.Background()))
	assert.Equal(t, []string{"departure:ZZZ202", "arrival:CCC404", "arrival:BBB303", "arrival:AAA101"}, lifecycle.events,
		"departure occupancy, existing commitments, and higher-stage arrivals must settle before new lower-stage arrivals choose stands")
}

func TestArrivalReconciliationConvergesAfterLaterFlightChangesCapacity(t *testing.T) {
	assignments := &reconciliationTestAssignments{assignments: map[string]*models.StandAssignment{
		assignmentKey(7, "AAA101"): {SessionID: 7, Callsign: "AAA101", Stand: "E71", Stage: "ESTIMATED", Source: "AUTOMATIC"},
		assignmentKey(7, "BBB202"): {SessionID: 7, Callsign: "BBB202", Stand: "E74", Stage: "ASSIGNED", Source: "AUTOMATIC"},
	}}
	stripStore := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{7: {
		{Callsign: "AAA101"}, {Callsign: "BBB202"},
	}}}
	lifecycle := &reconciliationConvergenceLifecycle{assignments: assignments}
	reconciler := &Reconciler{strips: stripStore, assignments: assignments, arrivalLifecycle: lifecycle}
	strips := map[string]*models.Strip{
		"AAA101": stripStore.bySession[7][0],
		"BBB202": stripStore.bySession[7][1],
	}
	flights := []Flight{{Callsign: "AAA101"}, {Callsign: "BBB202"}}

	require.NoError(t, reconciler.processArrivalFlights(context.Background(), 7, strips, flights, true))

	assert.Equal(t, "E83", assignments.assignments[assignmentKey(7, "AAA101")].Stand)
	assert.Equal(t, "ASSIGNED", assignments.assignments[assignmentKey(7, "AAA101")].Stage)
	assert.Equal(t, []string{"AAA101", "BBB202", "AAA101", "BBB202", "AAA101", "BBB202"}, lifecycle.processed,
		"the final pass must prove that no assignment changes on the same feed generation")
}

func TestArrivalReconciliationUsesFixedPassCap(t *testing.T) {
	assignments := &reconciliationTestAssignments{assignments: map[string]*models.StandAssignment{
		assignmentKey(7, "AAA101"): {SessionID: 7, Callsign: "AAA101", Stand: "E71", Stage: "ASSIGNED", Source: "AUTOMATIC"},
	}}
	strip := &models.Strip{Callsign: "AAA101"}
	lifecycle := &reconciliationNonConvergingLifecycle{assignments: assignments}
	reconciler := &Reconciler{
		strips: stripStoreWithSingleSession(7, strip), assignments: assignments, arrivalLifecycle: lifecycle,
	}

	err := reconciler.processArrivalFlights(context.Background(), 7, map[string]*models.Strip{"AAA101": strip}, []Flight{{Callsign: "AAA101"}}, true)

	require.ErrorContains(t, err, "fixed cap of 5 passes")
	assert.Equal(t, maxArrivalReconciliationPasses, lifecycle.processed)
}

func TestArrivalAssignmentStateUsesBulkSessionRead(t *testing.T) {
	assignments := &reconciliationBulkAssignments{items: []*models.StandAssignment{
		{SessionID: 7, Callsign: "AAA101", Stand: "E71", Stage: "ASSIGNED"},
		{SessionID: 7, Callsign: "BBB202", Stand: "E74", Stage: "CONFIRMED"},
	}}
	reconciler := &Reconciler{assignments: assignments}

	states, err := reconciler.arrivalAssignmentStates(context.Background(), 7, []Flight{{Callsign: "AAA101"}, {Callsign: "BBB202"}})

	require.NoError(t, err)
	assert.Equal(t, "E71", states["AAA101"].stand)
	assert.Equal(t, "E74", states["BBB202"].stand)
	assert.Equal(t, 1, assignments.listCalls)
	assert.Zero(t, assignments.getCalls)
}

func TestReconcileContinuesAfterSessionFailure(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	strips := &reconciliationSessionFailureStrips{
		reconciliationTestStrips: &reconciliationTestStrips{bySession: map[int32][]*models.Strip{1: {}, 2: {}}},
		failSession:              1,
	}
	reconciler := newTestReconciler(
		newReconciliationTestCache(now),
		reconciliationTestSessions{items: []*models.Session{{ID: 1, Name: "broken", Airport: "EKCH"}, {ID: 2, Name: "healthy", Airport: "EKCH"}}},
		strips,
		reconciliationTestAssignments{},
		nil,
		time.Second,
	)

	err := reconciler.Reconcile(context.Background())

	require.ErrorContains(t, err, "reconcile session 1 (broken)")
	assert.Equal(t, []int32{1, 2}, strips.listed)
}

func stripStoreWithSingleSession(session int32, strips ...*models.Strip) *reconciliationTestStrips {
	return &reconciliationTestStrips{bySession: map[int32][]*models.Strip{session: strips}}
}

func TestReconcileCancelsAbsentArrivalsBeforeAllocatingPresentFlights(t *testing.T) {
	now := time.Date(2026, 8, 3, 19, 50, 54, 0, time.UTC)
	cid := "1"
	cache := newReconciliationTestCache(now,
		Flight{CID: "2", Callsign: "DLH115", State: FlightStateOnline, LastUpdated: now, FlightPlan: FlightPlan{Origin: "EDDF", Destination: "EKCH", Revision: 1}},
	)
	strips := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{7: {
		{Callsign: "OLD100", Session: 7, Origin: "EGLL", Destination: "EKCH", VatsimCID: &cid},
	}}}
	lifecycle := &reconciliationOrderLifecycle{priorities: map[string]int{"DLH115": 2}}
	reconciler := newTestReconciler(cache, reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, strips, reconciliationTestAssignments{}, nil, time.Second)
	reconciler.lifecycle = lifecycle
	reconciler.arrivalLifecycle = lifecycle

	require.NoError(t, reconciler.Reconcile(context.Background()))

	assert.Equal(t, []string{"cancel-arrival:OLD100", "arrival:DLH115"}, lifecycle.events,
		"capacity released by the current feed generation must be visible to its arrival allocation")
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
	reconciler := newTestReconciler(cache, reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, strips, reconciliationTestAssignments{}, notifier, time.Second)

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
	assert.Empty(t, notifier.callsigns, "VATSIM-only strips must remain outside the operational frontend")
}

func TestReconcileKeepsOnlineAPIDepartureHidden(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	sequence := int32(1000)
	existing := &models.Strip{Callsign: "SAS101", Session: 7, Origin: "EKCH", Destination: "EGLL", Bay: hiddenDepartureBay, Sequence: &sequence}
	cache := newReconciliationTestCache(now, Flight{CID: "1", Callsign: "SAS101", State: FlightStateOnline, LastUpdated: now, FlightPlan: FlightPlan{Origin: "EKCH", Destination: "EGLL", Revision: 4}})
	strips := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{7: {existing}}}
	reconciler := newTestReconciler(cache, reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, strips, reconciliationTestAssignments{}, nil, time.Second)

	require.NoError(t, reconciler.Reconcile(context.Background()))
	assert.Equal(t, hiddenDepartureBay, existing.Bay)
}

func TestReconcileRequiresEuroscopeStripBeforeOnlineDepartureBlock(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	cache := newReconciliationTestCache(now,
		Flight{CID: "1", Callsign: "SAS101", State: FlightStateOnline, LastUpdated: now, FlightPlan: FlightPlan{Origin: "EKCH", Destination: "EGLL", Revision: 4}},
		Flight{CID: "2", Callsign: "SAS202", State: FlightStatePrefile, LastUpdated: now, FlightPlan: FlightPlan{Origin: "EKCH", Destination: "EDDF", Revision: 5}},
	)
	strips := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{}}
	lifecycle := &reconciliationTestDepartureLifecycle{}
	reconciler := newTestReconciler(cache, reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, strips, reconciliationTestAssignments{}, nil, time.Second)
	reconciler.lifecycle = lifecycle

	require.NoError(t, reconciler.Reconcile(context.Background()))
	assert.Equal(t, []string{"SAS202"}, lifecycle.processed,
		"the offline prefile may reserve a stand, but an API-only online flight must not create a departure block")

	var onlineStrip, prefileStrip *models.Strip
	for _, strip := range strips.created {
		switch strip.Callsign {
		case "SAS101":
			onlineStrip = strip
		case "SAS202":
			prefileStrip = strip
		}
	}
	require.NotNil(t, onlineStrip)
	require.NotNil(t, prefileStrip)
	onlineStrip.EuroscopeSeenAt = &now
	prefileStrip.EuroscopeSeenAt = &now
	lifecycle.processed = nil

	require.NoError(t, reconciler.Reconcile(context.Background()))
	assert.Equal(t, []string{"SAS101"}, lifecycle.processed,
		"the online departure may enter the block path after EuroScope has supplied the strip, while an ES-owned prefile must not reset that state")
}

func TestReconcileReleasesPushDepartureAfterLifecycleProcessing(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	sequence := int32(1000)
	state := "PUSH"
	existing := &models.Strip{
		Callsign: "SAS303", Session: 7, Origin: "EKCH", Destination: "EDDF",
		Bay: "PUSH", State: &state, Sequence: &sequence, EuroscopeSeenAt: &now,
	}
	cache := newReconciliationTestCache(now,
		Flight{CID: "3", Callsign: "SAS303", State: FlightStateOnline, LastUpdated: now,
			FlightPlan: FlightPlan{Origin: "EKCH", Destination: "EDDF", Revision: 3}},
	)
	strips := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{7: {existing}}}
	lifecycle := &reconciliationTestDepartureLifecycle{}
	reconciler := newTestReconciler(cache, reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, strips, reconciliationTestAssignments{}, nil, time.Second)
	reconciler.lifecycle = lifecycle

	require.NoError(t, reconciler.Reconcile(context.Background()))
	assert.Equal(t, []string{"SAS303"}, lifecycle.processed)
	assert.Equal(t, []string{"SAS303"}, lifecycle.released,
		"PUSH must win over stale stand coordinates restored by lifecycle processing")
}

func TestReconcileMovesExistingAPIPrefileOutOfCLX(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	sequence := int32(1000)
	existing := &models.Strip{Callsign: "SAS101", Session: 7, Origin: "EKCH", Destination: "EGLL", Bay: "NOT_CLEARED", Sequence: &sequence}
	cache := newReconciliationTestCache(now, Flight{CID: "1", Callsign: "SAS101", State: FlightStatePrefile, LastUpdated: now, FlightPlan: FlightPlan{Origin: "EKCH", Destination: "EGLL", Revision: 4}})
	strips := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{7: {existing}}}
	reconciler := newTestReconciler(cache, reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, strips, reconciliationTestAssignments{}, nil, time.Second)

	require.NoError(t, reconciler.Reconcile(context.Background()))
	assert.Equal(t, hiddenDepartureBay, existing.Bay)
}

func TestReconcileDoesNotMoveEuroscopeOwnedPrefileToHidden(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	existing := &models.Strip{Callsign: "SAS101", Session: 7, Origin: "EKCH", Destination: "EGLL", Bay: "NOT_CLEARED", EuroscopeSeenAt: &now}
	cache := newReconciliationTestCache(now, Flight{CID: "1", Callsign: "SAS101", State: FlightStatePrefile, LastUpdated: now, FlightPlan: FlightPlan{Origin: "EKCH", Destination: "EGLL", Revision: 4}})
	strips := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{7: {existing}}}
	reconciler := newTestReconciler(cache, reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, strips, reconciliationTestAssignments{}, nil, time.Second)

	require.NoError(t, reconciler.Reconcile(context.Background()))
	assert.Equal(t, "NOT_CLEARED", existing.Bay)
}

func TestReconcileKeepsEuroscopeFieldsAndProtectsControllerEdits(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	euroscopeSeen := now.Add(time.Minute)
	controllerRoute := "CONTROLLER ROUTE"
	existing := &models.Strip{
		Callsign: "SAS404", Session: 7, Origin: "EKCH", Destination: "EDDF", Route: &controllerRoute,
		Stand: ptr("A12"), Bay: "NOT_CLEARED", EuroscopeSeenAt: &euroscopeSeen,
		ControllerModifiedFields: []string{"route", "stand"},
	}
	cache := newReconciliationTestCache(now, Flight{CID: "4", Callsign: "SAS404", State: FlightStateOnline, LastUpdated: now, FlightPlan: FlightPlan{Origin: "EKCH", Destination: "EDDF", Route: "VATSIM ROUTE", Revision: 7}})
	strips := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{7: {existing}}}
	reconciler := newTestReconciler(cache, reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, strips, reconciliationTestAssignments{}, nil, time.Second)

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
	existing := &models.Strip{Callsign: "SAS707", Session: 7, Origin: "EKCH", Destination: "EDDF", Route: &oldRoute, Bay: "NOT_CLEARED", EuroscopeSeenAt: &euroscopeSeen}
	cache := newReconciliationTestCache(now, Flight{CID: "7", Callsign: "SAS707", State: FlightStateOnline, LastUpdated: now, FlightPlan: FlightPlan{Origin: "EKCH", Destination: "EDDF", Route: "NEW ROUTE", Revision: 8}})
	strips := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{7: {existing}}}
	reconciler := newTestReconciler(cache, reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, strips, reconciliationTestAssignments{}, nil, time.Second)

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
	reconciler := newTestReconciler(cache, reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, strips, assignments, nil, time.Second)

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
	reconciler := newTestReconciler(newReconciliationTestCache(now), reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, strips, reconciliationTestAssignments{}, nil, time.Second)
	lifecycle := &reconciliationTestDepartureLifecycle{}
	reconciler.lifecycle = lifecycle

	require.NoError(t, reconciler.Reconcile(context.Background()))
	assert.Equal(t, []string{"SAS505"}, lifecycle.cancelled)
}

func TestReconcileDoesNotCancelEuroscopeOwnedDepartureLifecycleWhenFlightDisappears(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	cid := "5"
	strip := &models.Strip{
		Callsign: "SAS506", Session: 7, Origin: "EKCH",
		VatsimCID: &cid, VatsimSeenAt: &now, EuroscopeSeenAt: &now,
	}
	strips := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{7: {strip}}}
	reconciler := newTestReconciler(newReconciliationTestCache(now), reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, strips, reconciliationTestAssignments{}, nil, time.Second)
	lifecycle := &reconciliationTestDepartureLifecycle{}
	reconciler.lifecycle = lifecycle

	require.NoError(t, reconciler.Reconcile(context.Background()))
	assert.Empty(t, lifecycle.cancelled, "EuroScope still owns the operational departure state")
}

func TestReconcileCancelsArrivalLifecycleWhenFlightDisappears(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	cid := "5"
	strip := &models.Strip{
		Callsign: "SAS507", Session: 7, Origin: "ESSA", Destination: "EKCH",
		VatsimCID: &cid, VatsimSeenAt: &now,
	}
	strips := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{7: {strip}}}
	reconciler := newTestReconciler(newReconciliationTestCache(now), reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, strips, reconciliationTestAssignments{}, nil, time.Second)
	lifecycle := &reconciliationTestArrivalLifecycle{}
	reconciler.arrivalLifecycle = lifecycle

	require.NoError(t, reconciler.Reconcile(context.Background()))
	assert.Equal(t, []string{"SAS507"}, lifecycle.cancelled)
	assert.Equal(t, []string{"SAS507"}, strips.deleted)
}

func TestReconcileDoesNotCancelEuroscopeOwnedArrivalLifecycleWhenFlightDisappears(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	cid := "5"
	strip := &models.Strip{
		Callsign: "SAS508", Session: 7, Origin: "ESSA", Destination: "EKCH",
		VatsimCID: &cid, VatsimSeenAt: &now, EuroscopeSeenAt: &now,
	}
	strips := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{7: {strip}}}
	reconciler := newTestReconciler(newReconciliationTestCache(now), reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, strips, reconciliationTestAssignments{}, nil, time.Second)
	lifecycle := &reconciliationTestArrivalLifecycle{}
	reconciler.arrivalLifecycle = lifecycle

	require.NoError(t, reconciler.Reconcile(context.Background()))
	assert.Empty(t, lifecycle.cancelled, "EuroScope still owns the operational arrival state")
	assert.Empty(t, strips.deleted)
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
	reconciler := newTestReconciler(cache, reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, &reconciliationTestStrips{bySession: map[int32][]*models.Strip{}}, assignments, nil, time.Second, WithClock(func() time.Time { return now }))

	assert.True(t, reconciler.RetainsStrip(context.Background(), 7, "SAS1"), "an active reservation keeps the strip alive")
	assert.False(t, reconciler.RetainsStrip(context.Background(), 7, "SAS2"), "an expired reservation no longer retains the strip")
}

func TestRetainsStripDoesNotKeepAdvisoryArrivalAfterFlightDisappears(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	assignment := &models.StandAssignment{
		SessionID: 7,
		Callsign:  "SAS926",
		Direction: "ARRIVAL",
		Stage:     "ESTIMATED",
		Source:    "AUTOMATIC",
	}
	assignments := reconciliationTestAssignments{assignments: map[string]*models.StandAssignment{
		assignmentKey(7, "SAS926"): assignment,
	}}
	reconciler := newTestReconciler(
		newReconciliationTestCache(now),
		reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}},
		&reconciliationTestStrips{bySession: map[int32][]*models.Strip{}},
		assignments,
		nil,
		time.Second,
		WithClock(func() time.Time { return now }),
	)

	assert.False(t, reconciler.RetainsStrip(context.Background(), 7, "SAS926"), "advisory assignment disappears with its VATSIM-only strip")

	eta := now.Add(30 * time.Minute)
	assignment.ETA = &eta
	assert.False(t, reconciler.RetainsStrip(context.Background(), 7, "SAS926"), "ESTIMATED remains advisory when timing becomes available")

	assignment.Stage = "ASSIGNED"
	assert.True(t, reconciler.RetainsStrip(context.Background(), 7, "SAS926"), "a close operational arrival retains its strip")
}
