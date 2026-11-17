package operational

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"runtime/pprof"
	"strings"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/routefact"
	"FlightStrips/internal/coordinationrequest"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// Count actual SQL, including BEGIN/COMMIT/ROLLBACK, across the whole synchronous
// event. Context propagation and all repositories use the instrumented pool.
type routeFactSQLRecorder struct {
	mu      sync.Mutex
	queries []string
	sqlTime time.Duration
}

type routeFactSQLStart struct{}

func (r *routeFactSQLRecorder) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	r.mu.Lock()
	r.queries = append(r.queries, data.SQL)
	r.mu.Unlock()
	return context.WithValue(ctx, routeFactSQLStart{}, time.Now())
}
func (r *routeFactSQLRecorder) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	started, _ := ctx.Value(routeFactSQLStart{}).(time.Time)
	r.mu.Lock()
	r.sqlTime += time.Since(started)
	r.mu.Unlock()
}
func (r *routeFactSQLRecorder) reset() { r.mu.Lock(); r.queries = nil; r.sqlTime = 0; r.mu.Unlock() }

type routeFactSessions struct{ pool *pgxpool.Pool }

func (s routeFactSessions) AMANSessions(ctx context.Context) ([]Session, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,airport,name='LIVE' FROM sessions`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Session, error) {
		var session Session
		err := row.Scan(&session.ID, &session.Airport, &session.Live)
		return session, err
	})
}
func (s routeFactSessions) ActiveArrivalRunway(ctx context.Context, _ string) (string, error) {
	if snapshot, ok := aman.ReadSessionSnapshot(ctx); ok && len(snapshot.ArrivalRunways) == 1 {
		return snapshot.ArrivalRunways[0], nil
	}
	var airport string
	err := s.pool.QueryRow(ctx, `SELECT airport FROM sessions WHERE id=$1`, aman.SessionID(ctx)).Scan(&airport)
	return "22L", err
}

type routeFactCorrelator struct {
	repository *coordinationrequest.Repository
}

// Exercise the same persistence scope check and event projections as the app
// transport, without opening websocket clients. Their SQL is part of the event.
type routeFactPublisher struct {
	recordingPublisher
	pool     *pgxpool.Pool
	geometry GeometryCache
	health   func(context.Context) aman.TechnicalHealth
}

func (p *routeFactPublisher) PublishAMANState(ctx context.Context, state aman.AirportState) error {
	if session, ok := aman.ReadSessionSnapshot(ctx); ok {
		if session.Airport != state.Airport {
			return fmt.Errorf("publication session airport mismatch")
		}
	} else {
		session, err := postgres.NewSessionRepository(p.pool).GetByID(ctx, state.SessionID)
		if err != nil {
			return err
		}
		if session.Airport != state.Airport {
			return fmt.Errorf("publication session airport mismatch")
		}
	}
	health := p.health(ctx)
	event, err := frontendEvents.NewAMANStateEvent(state, health.EffectiveMode, health)
	if err != nil {
		return err
	}
	snapshot, err := p.geometry.ActiveGeometrySnapshot(ctx, navdata.AirportID(state.Airport))
	if err != nil {
		return err
	}
	event.Data.TimelineConfig = frontendEvents.ProjectAMANTimelineConfig(snapshot.TerminalVersion, snapshot.TimelineMappings)
	if _, err := euroscopeEvents.NewAMANGainLossEvent(state); err != nil {
		return err
	}
	return p.recordingPublisher.PublishAMANState(ctx, state)
}

func (c routeFactCorrelator) ObserveClearance(ctx context.Context, fact coordinationrequest.ClearanceFact) (coordinationrequest.CommitResult, error) {
	return c.repository.CorrelateAccepted(ctx, fact)
}

func TestRouteFactFullEventSQLBudget(t *testing.T) {
	for _, flights := range []int{1, 132, 2} {
		t.Run(fmt.Sprintf("%d_arrivals", flights), func(t *testing.T) {
			basePool, queries := testdata.SetupTestDB(t)
			session := testdata.SeedTestSession(t, queries)
			_, err := basePool.Exec(t.Context(), `UPDATE sessions SET active_runways='{"arrival_runways":["22L"],"departure_runways":[]}'::jsonb WHERE id=$1`, session)
			require.NoError(t, err)
			other := testdata.SeedTestSessionNamedWithSectors(t, queries, "OTHER", nil)
			config := basePool.Config().Copy()
			recorder := &routeFactSQLRecorder{}
			config.ConnConfig.Tracer = recorder
			pool, err := pgxpool.NewWithConfig(t.Context(), config)
			require.NoError(t, err)
			t.Cleanup(pool.Close)
			ctx := aman.WithSession(t.Context(), session)
			now := time.Date(2026, 11, 16, 20, 16, 7, 0, time.UTC)
			prototype, _, observation, _ := arrivalChangeFixture(now)
			geometry := prototype.deps.Geometry.(arrivalChangeGeometry)
			nav := postgres.NewNavigationCache(pool)
			key := persistRouteFactGeometry(t, ctx, nav, geometry, observation)
			baseRoute, err := nav.Route(ctx, key)
			require.NoError(t, err)
			repository := postgres.NewAMANRepository(pool)
			publisher := &routeFactPublisher{pool: pool, geometry: nav}
			deps := prototype.deps
			deps.Repository, deps.Geometry, deps.Publisher = repository, nav, publisher
			deps.Materializer = fixedNavigation{key: key}
			deps.Now = func() time.Time { return now }
			deps.Airports = []string{"EKCH"}
			deps.Sessions = routeFactSessions{pool}
			// Map the real session runway to the fixture's AMAN group.
			deps.Terminal.RunwayGroups[0].Runways = []navdata.RunwayID{"22L"}
			deps.Terminal.RunwayGroups[0].Aliases = []aman.RunwayGroupID{"22L"}
			deps.Runways = routeFactSessions{pool}
			service, err := New(deps)
			require.NoError(t, err)
			publisher.health = service.TechnicalHealth
			state := service.initialState("EKCH", now)
			state.SessionID = session
			for i := 0; i < flights; i++ {
				o := observation
				o.Callsign = fmt.Sprintf("SAS%d", 100+i)
				// Distinct immutable routes exercise batching across every arrival,
				// rather than obtaining a low count by sharing one route-cache key.
				o.FiledRoute = stringPointer(fmt.Sprintf("DCT ORIGIN DCT ENTRY TEST%d", i))
				query := navdata.RouteQuery{Version: geometry.version, Origin: navdata.AirportID(o.Origin), Destination: "EKCH", FiledRoute: *o.FiledRoute, RunwayGroup: &geometry.path.RunwayGroup}
				route := baseRoute
				route.Digest, err = navdata.RouteGeometryDigest(query, route)
				require.NoError(t, err)
				flightKey, err := nav.PutRoute(ctx, navdata.RouteCandidate{Query: query, Geometry: route, ResolverVersion: routeResolverVersion, SchemaVersion: navdata.CanonicalSchemaVersion, CreatedAt: now})
				require.NoError(t, err)
				service.deps.Materializer = fixedNavigation{key: flightKey}
				require.NoError(t, service.Observe(ctx, o))
				flight, err := service.reconcileFlight(ctx, state, newFlight(o, now), o, now)
				require.NoError(t, err)
				require.NotNil(t, flight.Prediction)
				require.True(t, flight.Prediction.Publishable)
				require.NotNil(t, flight.RouteProgress)
				state.Flights = append(state.Flights, flight)
			}
			service.deps.Materializer = unavailableNavigation{}
			if flights == 2 {
				lost := &state.Flights[1]
				require.NotNil(t, lost.Lifecycle)
				lost.DataStatus = aman.DataDisconnected
				lost.Lifecycle.Absence = &aman.AbsenceState{MissingSince: now.Add(-10 * time.Minute), RemovalDueAt: timePointer(now.Add(-5 * time.Minute))}
				missing := *lost.LatestObservation
				missing.Missing, missing.SourceStatus = true, aman.DataDisconnected
				lost.LatestObservation = &missing
				require.NoError(t, service.Observe(ctx, missing))
			}
			state.Revision = 1
			service.resequence(&state, now)
			_, err = repository.Commit(ctx, aman.StateCommit{State: state})
			require.NoError(t, err)
			otherState := state
			otherState.SessionID = other
			_, err = repository.Commit(aman.WithSession(ctx, other), aman.StateCommit{State: otherState})
			require.NoError(t, err)
			strips := postgres.NewStripRepository(pool)
			require.NoError(t, strips.Create(ctx, &models.Strip{Callsign: "SAS100", Session: session, TrackingController: "EKCH_A_APP", Origin: "ESSA", Destination: "EKCH"}))
			coordination := coordinationrequest.NewRepository(pool)
			request, err := coordinationrequest.New("direct-request", "EKCH", "SAS100", "EKCH_A_APP", "123", "EKCH_FMP", coordinationrequest.KindRouteDirect,
				coordinationrequest.Payload{RouteDirect: &coordinationrequest.RouteDirectPayload{DirectTo: "END"}}, now.Add(-2*time.Minute))
			require.NoError(t, err)
			request, err = request.Decide("accept-direct", "456", "EKCH_A_APP", "EKCH_A_APP", coordinationrequest.StateAccepted, "", now.Add(-time.Minute))
			require.NoError(t, err)
			require.NoError(t, coordination.Save(ctx, request))
			if flights == 2 {
				pending, err := coordinationrequest.New("lost-request", "EKCH", "SAS101", "EKCH_A_APP", "123", "EKCH_FMP", coordinationrequest.KindSpeed,
					coordinationrequest.Payload{Speed: &coordinationrequest.SpeedPayload{Requested: "220 KT"}}, now.Add(-time.Minute))
				require.NoError(t, err)
				require.NoError(t, coordination.Save(ctx, pending))
			}
			facts, err := routefact.New(routefact.Dependencies{Repository: repository, Strips: strips, Geometry: nav, Publisher: publisher, Reconciler: service,
				Correlator: routeFactCorrelator{coordination}, Now: func() time.Time { return now }, NewID: func() string { return "sql-budget-direct" }})
			require.NoError(t, err)
			target := "END"
			recorder.reset()
			stopProfile := func() {}
			if path := os.Getenv("FLIGHTSTRIPS_ROUTEFACT_CPU_PROFILE"); path != "" {
				profile, err := os.Create(path)
				require.NoError(t, err)
				require.NoError(t, pprof.StartCPUProfile(profile))
				stopProfile = sync.OnceFunc(func() {
					pprof.StopCPUProfile()
					require.NoError(t, profile.Close())
				})
				t.Cleanup(stopProfile)
			}
			started := time.Now()
			require.NoError(t, facts.ReportDirectTo(ctx, session, "EKCH", "SAS100", "EKCH_A_APP", &target, now))
			elapsed := time.Since(started)
			stopProfile()
			statements := append([]string(nil), recorder.queries...)
			counts := make(map[string]int)
			for _, sql := range statements {
				kind := strings.Fields(sql)[0]
				if strings.Contains(sql, "aman_nav_") {
					kind = "navigation"
				}
				counts[kind]++
			}
			t.Logf("full event: arrivals=%d SQL=%d elapsed=%s SQL_time=%s categories=%v", flights, len(statements), elapsed, recorder.sqlTime, counts)
			require.Equal(t, 1, counts["navigation"], "snapshot and all cached routes share one statement")
			require.LessOrEqual(t, len(statements), 10, "includes both durable commits, publication scope checks, and clearance correlation")
			updated, err := repository.LoadAirportState(ctx, "EKCH")
			require.NoError(t, err)
			require.EqualValues(t, 3, updated.Revision, "fact and prediction remain separate durable revisions")
			flight := stateFlight(t, updated, "SAS100")
			require.Equal(t, "sql-budget-direct", flight.ActiveRouteFact.ID)
			require.Equal(t, "sql-budget-direct", flight.RouteProgress.RouteFactID)
			require.True(t, flight.Prediction.Publishable)
			require.True(t, flight.Prediction.RawTETA.Before(state.Flights[0].Prediction.RawTETA))
			require.Len(t, publisher.states, 2, "fact and full synchronous replan are published")
			published, err := json.Marshal(publisher.states)
			require.NoError(t, err)
			t.Logf("published state fingerprint: arrivals=%d SHA256=%x", flights, sha256.Sum256(published))
			require.NoError(t, updated.Validate())
			requests, err := coordination.ReplayAirport(ctx, "EKCH")
			require.NoError(t, err)
			if flights == 2 {
				require.Len(t, requests, 2)
				require.Equal(t, coordinationrequest.StateExpired, requests[1].State)
				require.NotNil(t, requests[1].Expiry)
				audits, err := repository.ListAuditRecords(ctx, "EKCH")
				require.NoError(t, err)
				require.NotEmpty(t, audits, "removal diagnostics are persisted atomically without extra statements")
			} else {
				require.Len(t, requests, 1)
			}
			require.Equal(t, "sql-budget-direct", requests[0].Clearance.FactID, "accepted request keeps the authoritative direct-to audit")
			unchanged, err := repository.LoadAirportState(aman.WithSession(ctx, other), "EKCH")
			require.NoError(t, err)
			require.Equal(t, otherState, unchanged, "route fact must not reconcile other sessions")
			recorder.reset()
			started = time.Now()
			require.NoError(t, facts.ReportDirectTo(ctx, session, "EKCH", "SAS100", "EKCH_A_APP", &target, now))
			t.Logf("repeat event: arrivals=%d SQL=%d elapsed=%s", flights, len(recorder.queries), time.Since(started))
			require.LessOrEqual(t, len(recorder.queries), 8)
			recorder.reset()
			started = time.Now()
			require.NoError(t, facts.ReportDirectTo(ctx, session, "EKCH", "SAS100", "EKCH_A_APP", nil, now))
			t.Logf("clear event: arrivals=%d SQL=%d elapsed=%s", flights, len(recorder.queries), time.Since(started))
			require.LessOrEqual(t, len(recorder.queries), 10)
		})
	}
}

func persistRouteFactGeometry(t *testing.T, ctx context.Context, cache interface {
	PutAirportFragment(context.Context, navdata.CandidateAirportFragment) (string, error)
	PutFixFragment(context.Context, navdata.CandidateFixFragment) (string, error)
	PutProcedureFragment(context.Context, navdata.CandidateProcedureFragment) (string, error)
	PutTerminalFragment(context.Context, navdata.CandidateTerminalFragment) (string, error)
	ActivateManifest(context.Context, navdata.ManifestCandidate) (int64, error)
	PutRoute(context.Context, navdata.RouteCandidate) (navdata.RouteKey, error)
}, geometry arrivalChangeGeometry, observation aman.FlightObservation) navdata.RouteKey {
	t.Helper()
	version := geometry.version
	imported, validated := version.EffectiveFrom, version.EffectiveFrom.Add(time.Minute)
	provenance := navdata.Provenance{SourceID: "fixture", SourceRevision: version.SourceRevision, ImportedAt: imported, EffectiveFrom: version.EffectiveFrom, EffectiveUntil: version.EffectiveUntil}
	digest := func(payload any) string {
		d, err := navdata.CanonicalFragmentDigest(navdata.CanonicalSchemaVersion, version, provenance, payload)
		require.NoError(t, err)
		return d
	}
	airport := navdata.Airport{ID: "EKCH", Name: "Copenhagen", Position: navdata.Coordinate{LatitudeDeg: 55.618, LongitudeDeg: 12.656}, Provenance: provenance}
	runways := []navdata.Runway{}
	airportDigest, err := cache.PutAirportFragment(ctx, navdata.CandidateAirportFragment{SchemaVersion: navdata.CanonicalSchemaVersion, Version: version, Airport: airport, Runways: runways, Provenance: provenance, ImportedAt: imported, ValidatedAt: &validated, State: navdata.ValidationValidated, Digest: digest(struct {
		Airport navdata.Airport
		Runways []navdata.Runway
	}{airport, runways})})
	require.NoError(t, err)
	fixes := append([]navdata.Fix(nil), geometry.fixes...)
	for i := range fixes {
		fixes[i].Provenance = provenance
	}
	fixDigest, err := cache.PutFixFragment(ctx, navdata.CandidateFixFragment{SchemaVersion: navdata.CanonicalSchemaVersion, Version: version, Fixes: fixes, Coverage: navdata.CoverageComplete, Provenance: provenance, ImportedAt: imported, ValidatedAt: &validated, State: navdata.ValidationValidated, Digest: digest(struct {
		Fixes    []navdata.Fix
		Coverage navdata.Coverage
	}{fixes, navdata.CoverageComplete})})
	require.NoError(t, err)
	digests := []string{}
	for _, kind := range []navdata.ProcedureKind{navdata.ProcedureSID, navdata.ProcedureSTAR, navdata.ProcedureApproach} {
		procedures := []navdata.Procedure{}
		d, err := cache.PutProcedureFragment(ctx, navdata.CandidateProcedureFragment{SchemaVersion: navdata.CanonicalSchemaVersion, Version: version, Airport: "EKCH", Kind: kind, Procedures: procedures, Coverage: navdata.CoverageComplete, Provenance: provenance, ImportedAt: imported, ValidatedAt: &validated, State: navdata.ValidationValidated, Digest: digest(struct {
			Airport    navdata.AirportID
			Kind       navdata.ProcedureKind
			Procedures []navdata.Procedure
			Coverage   navdata.Coverage
		}{"EKCH", kind, procedures, navdata.CoverageComplete})})
		require.NoError(t, err)
		digests = append(digests, d)
	}
	path := geometry.path
	path.Provenance, path.Coverage = provenance, navdata.CoverageComplete
	path.Digest = "fixture-terminal"
	path.HoldingIDs = nil
	terminal := navdata.CandidateTerminalFragment{SchemaVersion: navdata.CanonicalSchemaVersion, Version: version, Airport: "EKCH", ConfigVersion: "test", Paths: []navdata.TerminalPath{path}, Provenance: provenance, ImportedAt: imported, ValidatedAt: &validated, State: navdata.ValidationValidated}
	encoded, err := navdata.MarshalTerminalFragmentPayload(terminal)
	require.NoError(t, err)
	terminal.Digest = digest(json.RawMessage(encoded))
	terminalDigest, err := cache.PutTerminalFragment(ctx, terminal)
	require.NoError(t, err)
	_, err = cache.ActivateManifest(ctx, navdata.ManifestCandidate{Airport: "EKCH", Version: version, AirportDigest: airportDigest, FixDigest: fixDigest, ProcedureDigests: digests, TerminalDigest: terminalDigest})
	require.NoError(t, err)
	group := path.RunwayGroup
	query := navdata.RouteQuery{Version: version, Origin: navdata.AirportID(observation.Origin), Destination: "EKCH", FiledRoute: *observation.FiledRoute, RunwayGroup: &group}
	route := geometry.route
	route.Provenance = provenance
	route.Digest, err = navdata.RouteGeometryDigest(query, route)
	require.NoError(t, err)
	key, err := cache.PutRoute(ctx, navdata.RouteCandidate{Query: query, Geometry: route, ResolverVersion: routeResolverVersion, SchemaVersion: navdata.CanonicalSchemaVersion, CreatedAt: validated})
	require.NoError(t, err)
	return key
}
