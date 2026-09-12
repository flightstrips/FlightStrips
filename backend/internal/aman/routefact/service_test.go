package routefact

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/coordinationrequest"
	internalModels "FlightStrips/internal/models"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReportDirectToPersistsBackendOwnedFactAndPublishes(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	cid := "1234567"
	repository := &memoryRepository{state: aman.AirportState{
		Airport: "EKCH", Revision: 7, Flights: []aman.AMANFlight{routedFlight(cid)},
	}}
	strips := &stripReader{strip: &internalModels.Strip{Callsign: "SAS123", Session: 42, TrackingController: "EKCH_A_APP", VatsimCID: &cid}}
	publisher := &publisher{}
	reconciler := &reconciler{}
	correlator := &correlator{}
	service, err := New(Dependencies{
		Repository: repository,
		Strips:     strips,
		Geometry:   geometry(now),
		Publisher:  publisher,
		Reconciler: reconciler,
		Now:        func() time.Time { return now },
		NewID:      func() string { return "fact-1" },
		Correlator: correlator,
	})
	require.NoError(t, err)

	fix := " kemax "
	err = service.ReportDirectTo(context.Background(), 42, "ekch", "sas123", "ekch_a_app", &fix, now.Add(-time.Minute))
	require.NoError(t, err)
	require.Equal(t, aman.SequenceRevision(8), repository.state.Revision)
	require.Equal(t, 1, repository.commits)
	require.Equal(t, 1, publisher.calls)
	require.Equal(t, 1, reconciler.calls)
	fact := repository.state.Flights[0].ActiveRouteFact
	require.Equal(t, &aman.RouteFact{
		ID: "fact-1", FlightID: "flight-1", Fix: "KEMAX", Issuer: "EKCH_A_APP",
		ObservedAt: now.Add(-time.Minute), ReceivedAt: now,
		DatasetVersion: "2608|revision-a|2026-08-19T12:00:00Z|2026-08-21T12:00:00Z", State: aman.RouteFactActive,
	}, fact)
	require.Equal(t, coordinationrequest.ClearanceFact{Airport: "EKCH", FlightID: "flight-1", FactID: "fact-1", Kind: coordinationrequest.KindRouteDirect,
		Value: "KEMAX", Issuer: "EKCH_A_APP", ObservedAt: now.Add(-time.Minute)}, correlator.fact)

	// A repeated callback is idempotent across a fresh service instance because
	// the accepted fact is part of the persisted aggregate.
	restarted, err := New(Dependencies{
		Repository: repository, Strips: strips, Geometry: geometry(now), Publisher: publisher, Reconciler: reconciler,
		Now: func() time.Time { return now.Add(time.Second) }, NewID: func() string { return "fact-2" },
	})
	require.NoError(t, err)
	require.NoError(t, restarted.ReportDirectTo(context.Background(), 42, "EKCH", "SAS123", "EKCH_A_APP", &fix, now))
	require.Equal(t, 1, repository.commits)
	require.Equal(t, 1, publisher.calls)
	require.Equal(t, 1, reconciler.calls)

	// Clearing the direct-to is also a durable replacement, not a deletion of
	// the audit fact.
	require.NoError(t, restarted.ReportDirectTo(context.Background(), 42, "EKCH", "SAS123", "EKCH_A_APP", nil, now))
	require.Equal(t, aman.RouteFactCleared, repository.state.Flights[0].ActiveRouteFact.State)
	require.Empty(t, repository.state.Flights[0].ActiveRouteFact.Fix)
	require.Equal(t, 2, repository.commits)
	require.Equal(t, 2, publisher.calls)
	require.Equal(t, 2, reconciler.calls)
}

type correlator struct {
	fact coordinationrequest.ClearanceFact
}

func (c *correlator) ObserveClearance(_ context.Context, fact coordinationrequest.ClearanceFact) (coordinationrequest.CommitResult, error) {
	c.fact = fact
	return coordinationrequest.CommitResult{}, nil
}

func TestReportDirectToEnforcesCurrentTrackingControllerAndNavigationSnapshot(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	cid := "1234567"
	repository := &memoryRepository{state: aman.AirportState{Airport: "EKCH", Revision: 2, Flights: []aman.AMANFlight{routedFlight(cid)}}}
	strips := &stripReader{strip: &internalModels.Strip{Callsign: "SAS123", Session: 42, TrackingController: "EKCH_A_APP", VatsimCID: &cid}}
	service, err := New(Dependencies{
		Repository: repository, Strips: strips, Geometry: geometry(now), Publisher: &publisher{}, Reconciler: &reconciler{},
		Now: func() time.Time { return now }, NewID: func() string { return "fact" },
	})
	require.NoError(t, err)

	fix := "KEMAX"
	err = service.ReportDirectTo(context.Background(), 42, "EKCH", "SAS123", "EKCH_B_APP", &fix, now)
	requireDomainClass(t, err, aman.ErrorUnauthorized)
	mismatch := "7654321"
	strips.strip.VatsimCID = &mismatch
	err = service.ReportDirectTo(context.Background(), 42, "EKCH", "SAS123", "EKCH_A_APP", &fix, now)
	requireDomainClass(t, err, aman.ErrorNotFound)
	strips.strip.VatsimCID = &cid
	repository.state.Flights[0].State = aman.StateLanded
	err = service.ReportDirectTo(context.Background(), 42, "EKCH", "SAS123", "EKCH_A_APP", &fix, now)
	requireDomainClass(t, err, aman.ErrorNotFound)
	repository.state.Flights[0].State = aman.StateAirborne

	strips.strip.TrackingController = "EKCH_B_APP" // explicit handoff
	unknown := "NOPE"
	err = service.ReportDirectTo(context.Background(), 42, "EKCH", "SAS123", "EKCH_B_APP", &unknown, now)
	requireDomainClass(t, err, aman.ErrorDegradedOrIncompleteGeometry)
	offPath := "OFFPATH"
	err = service.ReportDirectTo(context.Background(), 42, "EKCH", "SAS123", "EKCH_B_APP", &offPath, now)
	requireDomainClass(t, err, aman.ErrorDegradedOrIncompleteGeometry)
	repository.state.Flights[0].RouteProgress = &aman.RouteProgress{
		GeometryDigest: "route-digest", FlightPlanRevision: 7, RunwayGroupID: "GROUP", RejoinLegIndex: 1,
	}
	behind := "PASSED"
	err = service.ReportDirectTo(context.Background(), 42, "EKCH", "SAS123", "EKCH_B_APP", &behind, now)
	requireDomainClass(t, err, aman.ErrorDegradedOrIncompleteGeometry)
	require.Zero(t, repository.commits)
	require.NoError(t, service.ReportDirectTo(context.Background(), 42, "EKCH", "SAS123", "EKCH_B_APP", &fix, now))
	require.Equal(t, "EKCH_B_APP", repository.state.Flights[0].ActiveRouteFact.Issuer)
	err = service.ReportDirectTo(context.Background(), 42, "EKCH", "SAS123", "EKCH_A_APP", nil, now)
	requireDomainClass(t, err, aman.ErrorUnauthorized)

	// A callsign correction resolves the same persisted FlightID through the
	// current strip/session binding.
	repository.state.Flights[0].CurrentCallsign = "SAS124"
	strips.strip.Callsign = "SAS124"
	require.NoError(t, service.ReportDirectTo(context.Background(), 42, "EKCH", "SAS124", "EKCH_B_APP", nil, now))
	require.Equal(t, aman.FlightID("flight-1"), repository.state.Flights[0].ID)
	require.Equal(t, aman.RouteFactCleared, repository.state.Flights[0].ActiveRouteFact.State)
}

func TestReportDirectToToleratesBoundedControllerClockSkew(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	cid := "1234567"
	repository := &memoryRepository{state: aman.AirportState{
		Airport: "EKCH", Revision: 7, Flights: []aman.AMANFlight{routedFlight(cid)},
	}}
	service, err := New(Dependencies{
		Repository: repository,
		Strips:     &stripReader{strip: &internalModels.Strip{Callsign: "SAS123", Session: 42, TrackingController: "EKCH_A_APP", VatsimCID: &cid}},
		Geometry:   geometry(now),
		Publisher:  &publisher{},
		Reconciler: &reconciler{},
		Now:        func() time.Time { return now },
		NewID:      func() string { return "fact" },
	})
	require.NoError(t, err)

	fix := "KEMAX"
	require.NoError(t, service.ReportDirectTo(context.Background(), 42, "EKCH", "SAS123", "EKCH_A_APP", &fix, now.Add(10*time.Second)))
	require.Equal(t, now, repository.state.Flights[0].ActiveRouteFact.ObservedAt)

	repository.state.Flights[0].ActiveRouteFact = nil
	err = service.ReportDirectTo(context.Background(), 42, "EKCH", "SAS123", "EKCH_A_APP", &fix, now.Add(maximumFutureClockSkew+time.Second))
	requireDomainClass(t, err, aman.ErrorInvalidArgument)
}

func TestReportSpeedCorrelatesWithoutChangingAMANInputs(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	cid := "1234567"
	repository := &memoryRepository{state: aman.AirportState{Airport: "EKCH", Revision: 7, Flights: []aman.AMANFlight{routedFlight(cid)}}}
	correlator := &correlator{}
	service, err := New(Dependencies{Repository: repository,
		Strips:   &stripReader{strip: &internalModels.Strip{Callsign: "SAS123", Session: 42, TrackingController: "EKCH_A_APP", VatsimCID: &cid}},
		Geometry: geometry(now), Publisher: &publisher{}, Reconciler: &reconciler{}, Correlator: correlator,
		Now: func() time.Time { return now }, NewID: func() string { return "speed-fact" }})
	require.NoError(t, err)
	require.NoError(t, service.ReportSpeed(context.Background(), 42, "EKCH", "SAS123", "EKCH_A_APP", "220 kt", now))
	require.Zero(t, repository.commits)
	require.Equal(t, aman.SequenceRevision(7), repository.state.Revision)
	require.Equal(t, coordinationrequest.ClearanceFact{Airport: "EKCH", FlightID: "flight-1", FactID: "speed-fact", Kind: coordinationrequest.KindSpeed,
		Value: "220 KT", Issuer: "EKCH_A_APP", ObservedAt: now}, correlator.fact)
}

type memoryRepository struct {
	state   aman.AirportState
	commits int
}

func (r *memoryRepository) LoadAirportState(context.Context, string) (aman.AirportState, error) {
	return r.state, nil
}
func (r *memoryRepository) Commit(_ context.Context, commit aman.StateCommit) (aman.CommitResult, error) {
	if commit.ExpectedRevision != r.state.Revision {
		return aman.CommitResult{}, &aman.DomainError{Class: aman.ErrorRevisionConflict, Message: "revision conflict"}
	}
	r.state = commit.State
	r.commits++
	return aman.CommitResult{State: r.state}, nil
}

type stripReader struct{ strip *internalModels.Strip }

func (r *stripReader) GetByCallsign(_ context.Context, session int32, callsign string) (*internalModels.Strip, error) {
	if r.strip == nil || r.strip.Session != session || r.strip.Callsign != callsign {
		return nil, errors.New("not found")
	}
	return r.strip, nil
}

type geometryReader struct {
	snapshot navdata.ActiveGeometrySnapshot
	route    navdata.RouteGeometry
}

func (r geometryReader) ActiveGeometrySnapshot(context.Context, navdata.AirportID) (navdata.ActiveGeometrySnapshot, error) {
	return r.snapshot, nil
}
func (r geometryReader) Route(context.Context, navdata.RouteKey) (navdata.RouteGeometry, error) {
	return r.route, nil
}

type publisher struct{ calls int }

func (p *publisher) PublishAMANState(context.Context, aman.AirportState) error { p.calls++; return nil }

type reconciler struct{ calls int }

func (r *reconciler) Reconcile(context.Context) { r.calls++ }

func geometrySnapshot(now time.Time) navdata.ActiveGeometrySnapshot {
	version := navdata.DatasetVersion{Cycle: "2608", SourceRevision: "revision-a", EffectiveFrom: now.Add(-24 * time.Hour), EffectiveUntil: now.Add(24 * time.Hour)}
	provenance := navdata.Provenance{SourceID: "fixture", SourceRevision: "revision-a", ImportedAt: now.Add(-time.Hour), EffectiveFrom: version.EffectiveFrom, EffectiveUntil: version.EffectiveUntil}
	return navdata.ActiveGeometrySnapshot{
		Manifest: navdata.ManifestCandidate{Airport: "EKCH", Version: version},
		Fixes: []navdata.Fix{
			{ID: "KEMAX", Position: navdata.Coordinate{LatitudeDeg: 55, LongitudeDeg: 12}, Provenance: provenance},
			{ID: "PASSED", Position: navdata.Coordinate{LatitudeDeg: 54.5, LongitudeDeg: 11.5}, Provenance: provenance},
			{ID: "OFFPATH", Position: navdata.Coordinate{LatitudeDeg: 56, LongitudeDeg: 13}, Provenance: provenance},
		},
	}
}

func geometry(now time.Time) geometryReader {
	snapshot := geometrySnapshot(now)
	from, passed, to := navdata.FixID("ENTRY"), navdata.FixID("PASSED"), navdata.FixID("KEMAX")
	fromPosition := navdata.Coordinate{LatitudeDeg: 54, LongitudeDeg: 11}
	passedPosition := navdata.Coordinate{LatitudeDeg: 54.5, LongitudeDeg: 11.5}
	toPosition := navdata.Coordinate{LatitudeDeg: 55, LongitudeDeg: 12}
	return geometryReader{snapshot: snapshot, route: navdata.RouteGeometry{
		Version: snapshot.Manifest.Version, Digest: "route-digest", Coverage: navdata.CoverageComplete,
		Legs: []navdata.ProcedureLeg{
			{ID: "ENTRY-PASSED", PathTerminator: navdata.PathTF, FromFix: &from, ToFix: &passed, FromPosition: &fromPosition, ToPosition: &passedPosition},
			{ID: "PASSED-KEMAX", PathTerminator: navdata.PathTF, FromFix: &passed, ToFix: &to, FromPosition: &passedPosition, ToPosition: &toPosition},
		},
	}}
}

func routedFlight(cid string) aman.AMANFlight {
	routeKey, feeder, group := "route-1", "FEEDER", aman.RunwayGroupID("GROUP")
	return aman.AMANFlight{
		ID: "flight-1", VATSIMCID: cid, CurrentCallsign: "SAS123", State: aman.StateAirborne,
		ActiveRouteKey: &routeKey, SelectedFeeder: &feeder, SelectedRunwayGroup: &group,
	}
}

func requireDomainClass(t *testing.T, err error, class aman.ErrorClass) {
	t.Helper()
	var domainErr *aman.DomainError
	require.Error(t, err)
	require.True(t, errors.As(err, &domainErr))
	require.Equal(t, class, domainErr.Class)
}
