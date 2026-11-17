package routefact

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/models"
	"github.com/stretchr/testify/require"
)

type scopeTestKey struct{}

type scopeTestGeometry struct {
	geometryReader
	onScope func()
	calls   int
}

func (g *scopeTestGeometry) WithGeometryReadScope(ctx context.Context, airport navdata.AirportID, keys []navdata.RouteKey) (context.Context, error) {
	g.calls++
	if g.onScope != nil {
		g.onScope()
	}
	return context.WithValue(ctx, scopeTestKey{}, g.calls), nil
}

type scopeTestPublisher struct{ t *testing.T }

func (p scopeTestPublisher) PublishAMANState(ctx context.Context, _ aman.AirportState) error {
	require.Equal(p.t, 2, ctx.Value(scopeTestKey{}), "the successful attempt owns the publication view")
	return nil
}

type scopeTestReconciler struct {
	t     *testing.T
	calls int
}

func (r *scopeTestReconciler) Reconcile(context.Context) {
	r.t.Fatal("route facts must not reconcile every session")
}
func (r *scopeTestReconciler) ReconcileSession(ctx context.Context, airport string) {
	r.calls++
	require.Equal(r.t, int32(42), aman.SessionID(ctx))
	require.Equal(r.t, "EKCH", airport)
	require.Equal(r.t, 2, ctx.Value(scopeTestKey{}))
}

type scopeConflictRepository struct {
	*memoryRepository
	attempts int
}

func (r *scopeConflictRepository) Commit(ctx context.Context, commit aman.StateCommit) (aman.CommitResult, error) {
	r.attempts++
	if r.attempts == 1 {
		r.state.Revision++
		return aman.CommitResult{}, &aman.DomainError{Class: aman.ErrorRevisionConflict}
	}
	return r.memoryRepository.Commit(ctx, commit)
}

func TestDirectToRefreshesNavigationScopeAfterCommitConflict(t *testing.T) {
	now := time.Date(2026, 11, 16, 20, 16, 7, 0, time.UTC)
	repo := &scopeConflictRepository{memoryRepository: &memoryRepository{state: aman.AirportState{Airport: "EKCH", Revision: 7, Flights: []aman.AMANFlight{routedFlight()}}}}
	geo := &scopeTestGeometry{geometryReader: geometry(now)}
	reconciler := &scopeTestReconciler{t: t}
	service, err := New(Dependencies{Repository: repo, Strips: &stripReader{strip: &models.Strip{Session: 42, Callsign: "SAS123", TrackingController: "EKCH_A_APP"}},
		Geometry: geo, Publisher: scopeTestPublisher{t}, Reconciler: reconciler, Now: func() time.Time { return now }})
	require.NoError(t, err)
	target := "KEMAX"
	require.NoError(t, service.ReportDirectTo(t.Context(), 42, "EKCH", "SAS123", "EKCH_A_APP", &target, now))
	require.Equal(t, 2, geo.calls)
	require.Equal(t, 1, repo.commits)
	require.Equal(t, 1, reconciler.calls)
}

func TestDirectToRechecksTrackingAuthorityAfterScopedNavigationLoad(t *testing.T) {
	now := time.Date(2026, 11, 16, 20, 16, 7, 0, time.UTC)
	repo := &memoryRepository{state: aman.AirportState{Airport: "EKCH", Revision: 7, Flights: []aman.AMANFlight{routedFlight()}}}
	strip := &models.Strip{Session: 42, Callsign: "SAS123", TrackingController: "EKCH_A_APP"}
	geo := &scopeTestGeometry{geometryReader: geometry(now), onScope: func() { strip.TrackingController = "EKCH_B_APP" }}
	service, err := New(Dependencies{Repository: repo, Strips: &stripReader{strip: strip}, Geometry: geo, Publisher: &publisher{}, Reconciler: &reconciler{}, Now: func() time.Time { return now }})
	require.NoError(t, err)
	target := "KEMAX"
	err = service.ReportDirectTo(t.Context(), 42, "EKCH", "SAS123", "EKCH_A_APP", &target, now)
	requireDomainClass(t, err, aman.ErrorUnauthorized)
	require.Zero(t, repo.commits)
}
