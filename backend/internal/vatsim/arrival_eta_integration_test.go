package vatsim_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"FlightStrips/internal/aman/compatibility"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/vatsim"

	"github.com/stretchr/testify/require"
)

func TestArrivalLifecycleUsesPersistedETAAfterAMANFallback(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	stale := models.ArrivalETA{Time: now.Add(-time.Minute), Source: compatibility.ArrivalETASource, CalculatedAt: now.Add(-compatibility.FreshFor - time.Minute)}
	fresh := models.ArrivalETA{Time: now.Add(30 * time.Minute), Source: compatibility.ArrivalETASource, CalculatedAt: now}
	filed := models.ArrivalETA{Time: now, Source: vatsim.ETAFiled, CalculatedAt: now, EOBT: "0800", EnrouteDuration: "0200"}
	for _, tc := range []struct {
		name             string
		filedDuration    string
		initial          *models.ArrivalETA
		refreshAMAN      bool
		afterFallback    bool
		afterAssignments bool
		afterProcessing  bool
		want             *models.ArrivalETA
	}{
		{name: "concurrent AMAN refresh wins over fallback write", filedDuration: "0200", refreshAMAN: true, want: &fresh},
		{name: "concurrent AMAN refresh wins over stale ETA clear", refreshAMAN: true, want: &fresh},
		{name: "expired AMAN ETA clears when no fallback is available"},
		{name: "filed fallback replaces expired AMAN ETA", filedDuration: "0200", want: &filed},
		{name: "AMAN refresh wins when the filed fallback is unchanged", initial: &filed, filedDuration: "0200", refreshAMAN: true, want: &fresh},
		{name: "AMAN refresh wins when VATSIM cannot recalculate a retained fallback", initial: &filed, refreshAMAN: true, want: &fresh},
		{name: "AMAN refresh after a successful fallback write wins at lifecycle", filedDuration: "0200", refreshAMAN: true, afterFallback: true, want: &fresh},
		{name: "AMAN refresh during the assignment read is included before ordering", initial: &filed, refreshAMAN: true, afterAssignments: true, want: &fresh},
		{name: "AMAN refresh in a pass with no assignment writes triggers another pass", initial: &filed, refreshAMAN: true, afterProcessing: true, want: &fresh},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			pool, queries := testdata.SetupTestDB(t)
			session := testdata.SeedTestSessionNamedWithSectors(t, queries, "LIVE", nil)
			repo := postgres.NewStripRepository(pool)
			const callsign = "SAS101"
			testdata.SeedTestStrip(t, queries, session, callsign)
			initial := stale
			if tc.initial != nil {
				initial = *tc.initial
			}
			_, err := repo.UpdateArrivalETA(ctx, session, callsign, initial)
			require.NoError(t, err)

			strips := &arrivalETARefreshStore{StripRepository: repo}
			assignments := &arrivalAssignmentRefreshStore{StandAssignmentRepository: postgres.NewStandAssignmentRepository(pool)}
			lifecycle := &arrivalETALifecycleRecorder{}
			if tc.refreshAMAN {
				refresh := func() {
					_, err := repo.UpdateArrivalETA(ctx, session, callsign, fresh)
					require.NoError(t, err)
				}
				switch {
				case tc.afterFallback:
					strips.afterFallback = refresh
				case tc.afterAssignments:
					assignments.afterList = refresh
				case tc.afterProcessing:
					lifecycle.afterProcess = refresh
				default:
					strips.afterList = refresh
				}
			}
			source := vatsim.NewSnapshotReplaySource()
			feed := fmt.Sprintf(`{
				"general": {"update_timestamp": %q},
				"pilots": [{
					"cid": 1, "callsign": %q, "last_updated": %q,
					"latitude": 55, "longitude": 10, "altitude": 2000, "groundspeed": 30,
					"flight_plan": {"departure": "EGLL", "arrival": "EKCH", "deptime": "0800", "enroute_time": %q, "revision_id": 1}
				}]
			}`, now.Format(time.RFC3339), callsign, now.Format(time.RFC3339), tc.filedDuration)
			require.NoError(t, source.Load(strings.NewReader(feed), now))
			reconciler, err := vatsim.NewReconciler(vatsim.ReconcilerDependencies{
				Cache: source, Sessions: postgres.NewSessionRepository(pool), Strips: strips,
				Assignments: assignments, DepartureLifecycle: lifecycle,
				ArrivalLifecycle: lifecycle, Notifier: lifecycle,
			}, time.Second, vatsim.WithClock(func() time.Time { return now }), vatsim.WithLegacyArrivalETAWriter(false),
				vatsim.WithAirportCoordinates(55.6181, 12.6560))
			require.NoError(t, err)

			require.NoError(t, reconciler.Reconcile(ctx))
			persisted, err := repo.GetByCallsign(ctx, session, callsign)
			require.NoError(t, err)
			require.Equal(t, tc.want, persisted.ArrivalETA)
			wantTiming := []*models.ArrivalETA{tc.want}
			if tc.afterProcessing {
				wantTiming = append([]*models.ArrivalETA{tc.initial}, wantTiming...)
			}
			require.Equal(t, wantTiming, lifecycle.prioritized)
			require.Equal(t, wantTiming, lifecycle.processed)
		})
	}
}

// AMAN is fresh while the feed updates the strip but expires before the stand
// pass reloads it. The pass must use the VATSIM fallback, not a missing ETA
// that would bypass the lifecycle's estimate horizon.
func TestArrivalLifecycleSelectsFallbackWhenAMANExpiresBeforeStandPass(t *testing.T) {
	ctx := t.Context()
	start := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	clock := start
	expiring := models.ArrivalETA{Time: start.Add(20 * time.Minute), Source: compatibility.ArrivalETASource, CalculatedAt: start.Add(-compatibility.FreshFor + 30*time.Second)}
	pool, queries := testdata.SetupTestDB(t)
	session := testdata.SeedTestSessionNamedWithSectors(t, queries, "LIVE", nil)
	repo := postgres.NewStripRepository(pool)
	const callsign = "SAS101"
	testdata.SeedTestStrip(t, queries, session, callsign)
	_, err := repo.UpdateArrivalETA(ctx, session, callsign, expiring)
	require.NoError(t, err)

	// The assignment read happens after the feed update and before the reload.
	assignments := &arrivalAssignmentRefreshStore{
		StandAssignmentRepository: postgres.NewStandAssignmentRepository(pool),
		afterList:                 func() { clock = start.Add(time.Minute) },
	}
	lifecycle := &arrivalETALifecycleRecorder{}
	source := vatsim.NewSnapshotReplaySource()
	feed := fmt.Sprintf(`{
		"general": {"update_timestamp": %q},
		"pilots": [{
			"cid": 1, "callsign": %q, "last_updated": %q,
			"latitude": 55, "longitude": 10, "altitude": 2000, "groundspeed": 30,
			"flight_plan": {"departure": "EGLL", "arrival": "EKCH", "deptime": "0800", "enroute_time": "0300", "revision_id": 1}
		}]
	}`, start.Format(time.RFC3339), callsign, start.Format(time.RFC3339))
	require.NoError(t, source.Load(strings.NewReader(feed), start))
	reconciler, err := vatsim.NewReconciler(vatsim.ReconcilerDependencies{
		Cache: source, Sessions: postgres.NewSessionRepository(pool), Strips: repo,
		Assignments: assignments, DepartureLifecycle: lifecycle,
		ArrivalLifecycle: lifecycle, Notifier: lifecycle,
	}, time.Second, vatsim.WithClock(func() time.Time { return clock }), vatsim.WithLegacyArrivalETAWriter(false),
		vatsim.WithAirportCoordinates(55.6181, 12.6560))
	require.NoError(t, err)

	require.NoError(t, reconciler.Reconcile(ctx))

	persisted, err := repo.GetByCallsign(ctx, session, callsign)
	require.NoError(t, err)
	require.NotNil(t, persisted.ArrivalETA)
	require.Equal(t, vatsim.ETAFiled, persisted.ArrivalETA.Source)
	require.True(t, start.Add(time.Hour).Equal(persisted.ArrivalETA.Time), "EOBT 0800 + 0300 enroute")
	require.NotEmpty(t, lifecycle.processed)
	for _, eta := range lifecycle.processed {
		require.NotNil(t, eta, "the stand pass must never see a missing ETA while a filed fallback exists")
		require.Equal(t, vatsim.ETAFiled, eta.Source)
	}
}

type arrivalETARefreshStore struct {
	repository.StripRepository
	afterList     func()
	afterFallback func()
}

func (s *arrivalETARefreshStore) List(ctx context.Context, session int32) ([]*models.Strip, error) {
	strips, err := s.StripRepository.List(ctx, session)
	if err == nil && s.afterList != nil {
		s.afterList()
		s.afterList = nil
	}
	return strips, err
}

func (s *arrivalETARefreshStore) UpdateFallbackArrivalETA(ctx context.Context, session int32, callsign string, fallback *models.ArrivalETA, now time.Time) (*models.ArrivalETA, bool, error) {
	eta, changed, err := s.StripRepository.UpdateFallbackArrivalETA(ctx, session, callsign, fallback, now)
	if err == nil && changed && s.afterFallback != nil {
		s.afterFallback()
		s.afterFallback = nil
	}
	return eta, changed, err
}

type arrivalAssignmentRefreshStore struct {
	repository.StandAssignmentRepository
	afterList func()
}

func (s *arrivalAssignmentRefreshStore) ListAssignments(ctx context.Context, session int32) ([]*models.StandAssignment, error) {
	assignments, err := s.StandAssignmentRepository.ListAssignments(ctx, session)
	if err == nil && s.afterList != nil {
		s.afterList()
		s.afterList = nil
	}
	return assignments, err
}

type arrivalETALifecycleRecorder struct {
	prioritized  []*models.ArrivalETA
	processed    []*models.ArrivalETA
	afterProcess func()
}

func (r *arrivalETALifecycleRecorder) ArrivalProcessingPriority(strip *models.Strip, _ vatsim.ArrivalFlightInfo, _ *models.StandAssignment) int {
	r.prioritized = append(r.prioritized, strip.ArrivalETA)
	return 0
}

func (r *arrivalETALifecycleRecorder) ProcessArrival(_ context.Context, _ int32, strip *models.Strip, _ vatsim.ArrivalFlightInfo) error {
	r.processed = append(r.processed, strip.ArrivalETA)
	if r.afterProcess != nil {
		r.afterProcess()
		r.afterProcess = nil
	}
	return nil
}

func (*arrivalETALifecycleRecorder) CancelArrival(context.Context, int32, string) error { return nil }
func (*arrivalETALifecycleRecorder) ProcessDeparture(context.Context, int32, *models.Strip, vatsim.DepartureFlightInfo) error {
	return nil
}
func (*arrivalETALifecycleRecorder) CancelDeparture(context.Context, int32, string) error { return nil }
func (*arrivalETALifecycleRecorder) SendStripUpdate(int32, string)                        {}
