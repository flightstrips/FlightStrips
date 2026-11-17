package frontend

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/services"
	"FlightStrips/internal/testutil"
	"FlightStrips/internal/vatsim"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// First operational promotion must resolve hard stand conflicts even when
// the feed skips intermediate airborne reports.
// Exercises the real lifecycle, PostgreSQL writes, publication enrichment,
// durable controller validation and EuroScope stand command dispatch.
func TestAirportPromotionResolvesSoftReservations(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := t.Context()
	stands, err := sat.LoadStandCapabilities(strings.NewReader(`
STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30
STAND:EKCH:A2:N055.37.42.710:E012.38.33.451:30
`))
	require.NoError(t, err)
	fallbacks := map[string]any{}
	for _, name := range []string{"airliner_default", "business_vip", "cargo", "military", "military_helicopter", "helicopter", "ga_private", "unknown"} {
		fallbacks[name] = map[string]any{"stands": map[string]any{"tier1": map[string]int{"A1": 100}}}
	}
	fallbackJSON, err := json.Marshal(fallbacks)
	require.NoError(t, err)
	policy, err := sat.LoadAirlineAssignment(strings.NewReader(`{
  "rules":[{"id":"sas","callsigns":["SAS"],"stands":{"tier1":{"A1":100,"A2":100}}}],
  "stand_groups":{},"fallback_rules":`+string(fallbackJSON)+`}`), stands)
	require.NoError(t, err)

	for _, scenario := range []struct {
		name                  string
		skipped, full, parked bool
	}{
		{name: "intermediate_report"},
		{name: "skipped_intermediate_report", skipped: true},
		{name: "wait_for_capacity_and_retry", skipped: true, full: true},
		{name: "preserve_physically_observed_stand", skipped: true, parked: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
			clock := func() time.Time { return now }
			session := testdata.SeedTestSessionNamedWithSectors(t, queries, scenario.name, nil)
			strips := postgres.NewStripRepository(pool)
			assignments := postgres.NewStandAssignmentRepository(pool)
			sessions := postgres.NewSessionRepository(pool)
			allocations, err := services.NewStandAllocationService(pool, strips, assignments, stands, policy,
				services.WithStandAllocationClock(clock), services.WithStandAllocationRandom(func() float64 { return 0 }))
			require.NoError(t, err)
			lifecycle, err := services.NewArrivalLifecycleService(allocations, assignments, strips, sessions, stands, nil, nil, sat.NewAirportCountryRegistry(),
				services.WithArrivalLifecycleClock(clock))
			require.NoError(t, err)
			esHub := &testutil.MockEuroscopeHub{GetMasterCidFn: func(int32) string { return "MASTER" }}
			hub := &Hub{
				send: make(chan internalMessage, 32), standCapabilities: stands,
				server: &standAllocationPublishingServer{
					MockServer:  &testutil.MockServer{StripRepoVal: strips, SessionRepoVal: sessions, EuroscopeHubVal: esHub},
					assignments: assignments,
				},
			}
			validation, err := services.NewStripValidationService(services.StripValidationDependencies{
				Strips: strips, Statuses: strips, Publisher: &testutil.MockFrontendHub{},
			})
			require.NoError(t, err)
			hub.SetValidationService(validation)
			allocations.SetPublisher(hub.PublishStandAllocation)
			for _, callsign := range []string{"SAS970", "SAS971", "SAS972"} {
				require.NoError(t, queries.InsertStrip(ctx, database.InsertStripParams{
					Callsign: callsign, Session: session, Origin: "ESSA", Destination: "EKCH", AircraftType: ptr("A320"),
					Bay: "ARR_HIDDEN", CdmData: []byte(`{"canonical":{}}`), NextOwners: []byte(`[]`), PreviousOwners: []byte(`[]`),
				}))
			}
			load := func(callsign string) *models.Strip {
				strip, err := strips.GetByCallsign(ctx, session, callsign)
				require.NoError(t, err)
				return strip
			}
			flight := vatsim.ArrivalFlightInfo{Callsign: "SAS970", CID: "1001", Online: true, Origin: "ESSA", Destination: "EKCH", AircraftType: "A320", Revision: 1}
			_, err = strips.UpdateArrivalETA(ctx, session, "SAS970", models.ArrivalETA{Time: now.Add(5 * time.Minute), Source: "FILED", CalculatedAt: now})
			require.NoError(t, err)
			require.NoError(t, lifecycle.ProcessArrival(ctx, session, load("SAS970"), flight))
			estimate, err := assignments.GetAssignment(ctx, session, "SAS970")
			require.NoError(t, err)
			require.Equal(t, services.StageEstimated, estimate.Stage)
			require.Equal(t, "A1", estimate.Stand)
			require.Nil(t, estimate.ConflictReason)
			require.Empty(t, load("SAS970").Stand)

			// An actual operational arrival now holds the estimate's soft stand.
			actions := services.NewStandActionService(allocations, assignments, strips, nil, nil, nil)
			_, err = actions.AssignManually(ctx, session, "EKCH", "GND", "SAS971", "A1", 0)
			require.NoError(t, err)
			if scenario.full {
				_, err = actions.AssignManually(ctx, session, "EKCH", "GND", "SAS972", "A2", 0)
				require.NoError(t, err)
			}
			// Leave it ASSIGNED: it is a hard reservation with unknown release time.
			if !scenario.skipped {
				lat, lon, altitude := 55.8, 12.642625, int32(8000)
				_, err = strips.UpdateAircraftPosition(ctx, session, "SAS970", &lat, &lon, &altitude, "FINAL", nil)
				require.NoError(t, err)
				flight.Revision++
				require.NoError(t, lifecycle.ProcessArrival(ctx, session, load("SAS970"), flight))
			}
			now = now.Add(time.Minute)
			// Taxiing in the airport area, outside either stand's position radius.
			lat, lon, altitude := 55.6285306, 12.644625, int32(0)
			state := "TAXI"
			if scenario.parked {
				lon, state = 12.642625, "PARK"
			}
			_, err = strips.UpdateAircraftPosition(ctx, session, "SAS970", &lat, &lon, &altitude, state, nil)
			require.NoError(t, err)
			if scenario.parked {
				_, err = strips.UpdateGroundState(ctx, session, "SAS970", &state, "ARR_HIDDEN", nil)
				require.NoError(t, err)
			}
			flight.Revision++
			require.NoError(t, lifecycle.ProcessArrival(ctx, session, load("SAS970"), flight))
			promoted, err := assignments.GetAssignment(ctx, session, "SAS970")
			require.NoError(t, err)
			if scenario.full {
				require.Equal(t, services.StageEstimated, promoted.Stage)
				require.NotNil(t, promoted.ConflictReason)
				require.Empty(t, load("SAS970").Stand)
				require.Empty(t, esHub.Stands, "a blocked promotion must never send an occupied stand to EuroScope")
				now = now.Add(5 * time.Minute)
				require.NoError(t, lifecycle.ReleaseExpired(ctx))
				require.Empty(t, load("SAS970").Stand)
				other, err := assignments.GetAssignment(ctx, session, "SAS972")
				require.NoError(t, err)
				require.NoError(t, allocations.ReleaseAssignment(ctx, other))
				flight.Revision++
				require.NoError(t, lifecycle.ProcessArrival(ctx, session, load("SAS970"), flight))
				promoted, err = assignments.GetAssignment(ctx, session, "SAS970")
				require.NoError(t, err)
			}
			require.Equal(t, services.StageConfirmed, promoted.Stage)
			wantStand := "A2"
			if scenario.parked {
				wantStand = "A1"
			}
			require.Equal(t, wantStand, promoted.Stand)
			require.NotNil(t, load("SAS970").Stand)
			require.Equal(t, wantStand, *load("SAS970").Stand)
			require.Nil(t, promoted.ConflictReason)
			require.NotNil(t, promoted.ExpiresAt)
			require.True(t, promoted.ExpiresAt.Equal(now.Add(30*time.Minute)), "promotion must retain the airport-area expiry")
			var lastSnapshot frontendEvents.StandStatusSnapshotEvent
			for len(hub.send) > 0 {
				message := <-hub.send
				if snapshot, ok := message.message.(frontendEvents.StandStatusSnapshotEvent); ok {
					lastSnapshot = snapshot
				}
			}
			require.Len(t, lastSnapshot.Assignments, 2)
			for _, assignment := range lastSnapshot.Assignments {
				if scenario.parked {
					require.Len(t, assignment.BlockedBy, 1, "physical occupancy stays fixed and reports the overlap")
				} else {
					require.Empty(t, assignment.BlockedBy)
				}
			}
			status := load("SAS970").ValidationStatus
			if scenario.parked {
				require.NotNil(t, status)
				require.True(t, status.Active)
			} else {
				require.Nil(t, status)
			}
			require.NotEmpty(t, esHub.Stands)
			require.Equal(t, wantStand, esHub.Stands[len(esHub.Stands)-1].Stand)
			for _, command := range esHub.Stands {
				if command.Callsign == "SAS970" {
					require.Equal(t, wantStand, command.Stand, "never send the conflicting estimate to EuroScope")
				}
			}
		})
	}
}
