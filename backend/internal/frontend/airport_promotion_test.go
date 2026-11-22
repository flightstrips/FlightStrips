package frontend

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/services"
	"FlightStrips/internal/testutil"
	"FlightStrips/internal/vatsim"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"context"
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
		name                          string
		skipped, full, parked, pinned bool
		reassign                      string
	}{
		{name: "intermediate_report"},
		{name: "skipped_intermediate_report", skipped: true},
		{name: "wait_for_capacity_and_retry", skipped: true, full: true},
		{name: "preserve_physically_observed_stand", skipped: true, parked: true},
		{name: "warn_and_wait_for_pinned_stand", skipped: true, pinned: true},
		{name: "manually_reassign_from_pin_warning", skipped: true, pinned: true, reassign: "manual"},
		{name: "automatically_reassign_from_pin_warning", skipped: true, pinned: true, reassign: "auto"},
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
				_, err := queries.InsertStrip(ctx, database.InsertStripParams{
					Callsign: callsign, Session: session, Origin: "ESSA", Destination: "EKCH", AircraftType: ptr("A320"),
					Bay: "ARR_HIDDEN", CdmData: []byte(`{"canonical":{}}`), NextOwners: []byte(`[]`), PreviousOwners: []byte(`[]`),
				})
				require.NoError(t, err)
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
			if scenario.pinned {
				// Legacy controller pins can still be at the ESTIMATED stage.
				estimate.Manual, estimate.Source = true, "MANUAL"
				rows, err := assignments.UpdateAssignment(ctx, estimate)
				require.NoError(t, err)
				require.EqualValues(t, 1, rows)
			}

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
			var expiryWhileWaiting *time.Time
			if scenario.full || scenario.pinned {
				require.Equal(t, services.StageEstimated, promoted.Stage)
				require.NotNil(t, promoted.ConflictReason)
				require.Empty(t, load("SAS970").Stand)
				require.Empty(t, esHub.Stands, "a blocked promotion must never send an occupied stand to EuroScope")
				blocker := "SAS972"
				if scenario.pinned {
					require.Equal(t, "A1", promoted.Stand)
					expiryWhileWaiting = promoted.ExpiresAt
					require.NotNil(t, expiryWhileWaiting)
					status := load("SAS970").ValidationStatus
					require.NotNil(t, status)
					require.True(t, status.Active)
					require.Contains(t, status.Message, "A1")
					require.Contains(t, status.Message, "SAS971")
					var payload models.StandAssignmentActionPayload
					require.NotNil(t, status.CustomAction)
					require.NoError(t, json.Unmarshal(status.CustomAction.Payload, &payload))
					require.Equal(t, promoted.Version, payload.AssignmentVersion)
					require.Equal(t, promoted.ID, payload.AssignmentID)
					if scenario.reassign != "" {
						var result *services.StandAllocationResult
						if scenario.reassign == "manual" {
							result, err = actions.AssignManually(ctx, session, "EKCH", "GND", "SAS970", "A2", payload.AssignmentVersion, &payload.AssignmentID)
						} else {
							result, err = actions.Allocate(ctx, session, "EKCH", "GND", "SAS970", payload.AssignmentVersion, &payload.AssignmentID)
						}
						require.NoError(t, err)
						require.Equal(t, "A2", result.Assignment.Stand)
						require.Equal(t, services.StageAssigned, result.Assignment.Stage)
						require.Nil(t, load("SAS970").ValidationStatus)
						return
					}
					require.NoError(t, validation.AcknowledgeValidationStatus(ctx, session, "SAS970", status.ActivationKey, status.OwningPosition))
					rows, err := assignments.UpdateAssignment(ctx, promoted)
					require.NoError(t, err)
					require.EqualValues(t, 1, rows)
					promoted.Version++
					require.NoError(t, allocations.PublishAssignment(ctx, *promoted))
					refreshed := load("SAS970").ValidationStatus
					require.NotNil(t, refreshed)
					require.False(t, refreshed.Active, "refreshing the action must preserve acknowledgement")
					require.Equal(t, status.ActivationKey, refreshed.ActivationKey)
					require.NoError(t, json.Unmarshal(refreshed.CustomAction.Payload, &payload))
					require.Equal(t, promoted.Version, payload.AssignmentVersion)
					require.Equal(t, promoted.ID, payload.AssignmentID)
					blocker = "SAS971"
				}
				now = now.Add(5 * time.Minute)
				require.NoError(t, lifecycle.ReleaseExpired(ctx))
				require.Empty(t, load("SAS970").Stand)
				other, err := assignments.GetAssignment(ctx, session, blocker)
				require.NoError(t, err)
				require.NoError(t, allocations.ReleaseAssignment(ctx, other))
				flight.Revision++
				require.NoError(t, lifecycle.ProcessArrival(ctx, session, load("SAS970"), flight))
				promoted, err = assignments.GetAssignment(ctx, session, "SAS970")
				require.NoError(t, err)
			}
			require.Equal(t, services.StageConfirmed, promoted.Stage)
			wantStand := "A2"
			if scenario.parked || scenario.pinned {
				wantStand = "A1"
			}
			require.Equal(t, wantStand, promoted.Stand)
			require.NotNil(t, load("SAS970").Stand)
			require.Equal(t, wantStand, *load("SAS970").Stand)
			require.Nil(t, promoted.ConflictReason)
			require.NotNil(t, promoted.ExpiresAt)
			wantExpiry := now.Add(30 * time.Minute)
			if expiryWhileWaiting != nil {
				wantExpiry = *expiryWhileWaiting
			}
			require.True(t, promoted.ExpiresAt.Equal(wantExpiry), "promotion must retain the airport-area expiry")
			var lastSnapshot frontendEvents.StandStatusSnapshotEvent
			for len(hub.send) > 0 {
				message := <-hub.send
				if snapshot, ok := message.message.(frontendEvents.StandStatusSnapshotEvent); ok {
					lastSnapshot = snapshot
				}
			}
			if scenario.pinned {
				require.Len(t, lastSnapshot.Assignments, 1)
			} else {
				require.Len(t, lastSnapshot.Assignments, 2)
			}
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

func TestStandPublicationUsesCurrentBlockersAndHiddenReplacement(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	for _, scenario := range []string{"blocker leaves", "blocker arrives", "unblocked hidden replacement"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := t.Context()
			session := testdata.SeedTestSessionNamedWithSectors(t, queries, t.Name(), nil)
			strips := postgres.NewStripRepository(pool)
			assignments := postgres.NewStandAssignmentRepository(pool)
			observed := &standPublicationReadHook{StandAssignmentRepository: assignments}
			const inbound, blocker = "SAS970", "SAS971"
			testdata.SeedTestStrip(t, queries, session, inbound)
			testdata.SeedTestStrip(t, queries, session, blocker)
			hub := &Hub{
				send: make(chan internalMessage, 32),
				server: &standAllocationPublishingServer{
					MockServer:  &testutil.MockServer{StripRepoVal: strips, SessionRepoVal: postgres.NewSessionRepository(pool)},
					assignments: observed,
				},
			}
			validation, err := services.NewStripValidationService(services.StripValidationDependencies{
				Strips: strips, Statuses: strips, Publisher: &testutil.MockFrontendHub{},
			})
			require.NoError(t, err)
			hub.SetValidationService(validation)
			warning := func() *models.ValidationStatus {
				t.Helper()
				strip, err := strips.GetByCallsign(ctx, session, inbound)
				require.NoError(t, err)
				return strip.ValidationStatus
			}
			target := &models.StandAssignment{
				SessionID: session, Callsign: inbound, Stand: "A1", Direction: "ARRIVAL",
				Stage: services.StageAssigned, Source: "AUTOMATIC",
			}
			if scenario == "unblocked hidden replacement" {
				target.Stage, target.Source, target.Manual = services.StageEstimated, "MANUAL", true
				target.ConflictReason = ptr("Pinned arrival promotion is waiting for stand A1. Stand is occupied")
				require.NoError(t, assignments.CreateAssignment(ctx, target))
				require.NoError(t, hub.PublishStandAllocation(ctx, services.StandAllocationResult{Assignment: *target}))
				require.NotNil(t, warning())

				deleted, err := assignments.DeleteAssignment(ctx, session, target.ID, target.Version)
				require.NoError(t, err)
				require.EqualValues(t, 1, deleted)
				replacement := *target
				replacement.ConflictReason = nil
				require.NoError(t, assignments.CreateAssignment(ctx, &replacement))
				require.NotEqual(t, target.ID, replacement.ID)

				require.NoError(t, hub.PublishStandAllocation(ctx, services.StandAllocationResult{Assignment: *target, Removed: true}))
				require.Nil(t, warning(), "the current hidden estimate must clear the removed pin's warning")
				var lastSnapshot *frontendEvents.StandStatusSnapshotEvent
				for len(hub.send) > 0 {
					if snapshot, ok := (<-hub.send).message.(frontendEvents.StandStatusSnapshotEvent); ok {
						lastSnapshot = &snapshot
					}
				}
				require.NotNil(t, lastSnapshot)
				require.Empty(t, lastSnapshot.Assignments)
				return
			}

			require.NoError(t, assignments.CreateAssignment(ctx, target))
			initial, moved := "A1", "A2"
			if scenario == "blocker arrives" {
				initial, moved = moved, initial
			}
			other := &models.StandAssignment{
				SessionID: session, Callsign: blocker, Stand: initial, Direction: "DEPARTURE",
				Stage: services.StageDepartureBlock, Source: "OBSERVED",
			}
			require.NoError(t, assignments.CreateAssignment(ctx, other))
			require.NoError(t, hub.PublishStandAllocation(ctx, services.StandAllocationResult{Assignment: *other}))
			require.Equal(t, scenario == "blocker leaves", warning() != nil)

			observed.afterList = func() {
				other.Stand = moved
				rows, err := assignments.UpdateAssignment(ctx, other)
				require.NoError(t, err)
				require.EqualValues(t, 1, rows)
				other.Version++
				require.NoError(t, hub.PublishStandAllocation(ctx, services.StandAllocationResult{Assignment: *other}))
				require.Equal(t, scenario == "blocker arrives", warning() != nil)
			}
			require.NoError(t, hub.PublishStandAllocation(ctx, services.StandAllocationResult{Assignment: *target}))

			status := warning()
			if scenario == "blocker arrives" {
				require.NotNil(t, status, "a delayed unblocked snapshot must preserve the new warning")
				require.Contains(t, status.Message, blocker)
			} else {
				require.Nil(t, status, "a delayed blocked snapshot must preserve the cleared warning")
			}
			current, err := assignments.GetAssignment(ctx, session, inbound)
			require.NoError(t, err)
			require.Equal(t, target.Revision(), current.Revision(), "only the blocker was changed")
			var lastSnapshot *frontendEvents.StandStatusSnapshotEvent
			for len(hub.send) > 0 {
				if snapshot, ok := (<-hub.send).message.(frontendEvents.StandStatusSnapshotEvent); ok {
					lastSnapshot = &snapshot
				}
			}
			require.NotNil(t, lastSnapshot)
			require.Len(t, lastSnapshot.Assignments, 2)
			callsigns := make([]string, 0, len(lastSnapshot.Assignments))
			for _, assignment := range lastSnapshot.Assignments {
				callsigns = append(callsigns, assignment.Callsign)
			}
			require.ElementsMatch(t, []string{inbound, blocker}, callsigns)
			for _, assignment := range lastSnapshot.Assignments {
				if assignment.Callsign == blocker {
					require.Equal(t, moved, assignment.Stand, "the last snapshot must contain the latest committed stand")
					require.Equal(t, other.Version, assignment.Version)
				}
			}
		})
	}
}

type standPublicationReadHook struct {
	repository.StandAssignmentRepository
	afterList func()
}

func (r *standPublicationReadHook) ListAssignments(ctx context.Context, session int32) ([]*models.StandAssignment, error) {
	assignments, err := r.StandAssignmentRepository.ListAssignments(ctx, session)
	if err == nil && r.afterList != nil {
		hook := r.afterList
		r.afterList = nil
		hook()
	}
	return assignments, err
}
