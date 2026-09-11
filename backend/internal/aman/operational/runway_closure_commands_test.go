package operational

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"FlightStrips/internal/aman/terminal"
	"github.com/stretchr/testify/require"
)

func TestRunwayClosureCommandsNormalizeRetryAcrossRestartAndRemove(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	rateAt := now.Add(-time.Hour)
	group := aman.RunwayGroupID("north")
	anchor := gapCommandFlight("ANCHOR", group, now, 1, aman.StateStable, aman.FreezeNone)
	repository := &memoryRepository{has: true, state: aman.AirportState{
		Airport: "EKCH", Revision: 7, GeneratedAt: now.Add(-time.Minute), PolicyVersion: "closure-v1", Mode: aman.ModeAuthoritative,
		Flights: []aman.AMANFlight{anchor}, RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, ActiveRatePerHour: 60, RateEffectiveAt: &rateAt}},
	}}
	auth := aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKCH_FMH", ReceivedAt: now}
	end := now.Add(20 * time.Minute)
	explicitStart := now.Add(10 * time.Minute)
	explicit := aman.CreateRunwayClosureCommand{
		Metadata: aman.CommandMetadata{CommandID: "closure-absolute", ExpectedRevision: 7},
		Interval: aman.RunwayClosureIntervalInput{RunwayGroupID: group, Start: &explicitStart, End: &end}, Reason: "inspection",
	}
	actions := runwayGapActions(t, repository, &recordingPublisher{}, now.Add(time.Second))
	created, err := actions.CreateRunwayClosure(ctx, auth, explicit)
	require.NoError(t, err)
	require.True(t, created.Changed)
	require.Equal(t, aman.SequenceRevision(8), created.CurrentRevision)
	require.Equal(t, explicitStart, repository.state.RunwayGroups[0].Closures[0].Start)
	require.Equal(t, auth.Actor, repository.state.RunwayGroups[0].Closures[0].CreatedBy)

	for _, retryActions := range []*sequence.ActionService{actions, runwayGapActions(t, repository, &recordingPublisher{}, now.Add(2*time.Second))} {
		retry, retryErr := retryActions.CreateRunwayClosure(ctx, auth, explicit)
		require.NoError(t, retryErr)
		require.True(t, retry.Duplicate)
		require.Equal(t, created.Outcome, retry.Outcome)
	}
	require.Len(t, repository.commits, 1)

	anchorID := anchor.ID
	after := aman.CreateRunwayClosureCommand{
		Metadata: aman.CommandMetadata{CommandID: "closure-after", ExpectedRevision: 8},
		Interval: aman.RunwayClosureIntervalInput{RunwayGroupID: group, AfterFlightID: &anchorID}, Reason: "works",
	}
	afterCreated, err := actions.CreateRunwayClosure(ctx, auth, after)
	require.NoError(t, err)
	require.Equal(t, aman.SequenceRevision(9), afterCreated.CurrentRevision)
	require.Equal(t, now.Add(time.Minute), repository.state.RunwayGroups[0].Closures[0].Start)
	require.Nil(t, repository.state.RunwayGroups[0].Closures[0].End)
	var afterAudit map[string]any
	require.NoError(t, json.Unmarshal(repository.commits[1].AuditRecords[0].Payload, &afterAudit))
	require.Equal(t, "after_aircraft", afterAudit["input_provenance"].(map[string]any)["kind"])
	require.Equal(t, "ANCHOR", afterAudit["input_provenance"].(map[string]any)["anchor_flight_id"])

	_, err = actions.RemoveRunwayClosure(ctx, auth, aman.RemoveRunwayClosureCommand{
		Metadata: aman.CommandMetadata{CommandID: "stale-remove", ExpectedRevision: 8}, RunwayGroupID: group,
		ClosureID: "closure-absolute", Reason: "cancelled",
	})
	requireDomainErrorClass(t, err, aman.ErrorRevisionConflict)
	removed, err := actions.RemoveRunwayClosure(ctx, auth, aman.RemoveRunwayClosureCommand{
		Metadata: aman.CommandMetadata{CommandID: "remove-closure", ExpectedRevision: 9}, RunwayGroupID: group,
		ClosureID: "closure-absolute", Reason: "runway reopened",
	})
	require.NoError(t, err)
	require.True(t, removed.Changed)
	require.Len(t, repository.state.RunwayGroups[0].Closures, 1)
	var removeAudit map[string]any
	require.NoError(t, json.Unmarshal(repository.commits[2].AuditRecords[0].Payload, &removeAudit))
	require.Equal(t, "runway reopened", removeAudit["removal_reason"])
	require.Equal(t, aman.SequenceRevision(10), repository.commits[2].AuditRecords[0].Revision)
}

func TestRunwayClosureCommandsRejectSpoofableAuthority(t *testing.T) {
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	start := now.Add(time.Minute)
	repository := &memoryRepository{has: true, state: aman.AirportState{
		Airport: "EKCH", Revision: 1, GeneratedAt: now, PolicyVersion: "closure-v1", Mode: aman.ModeAuthoritative,
		RunwayGroups: []aman.RunwayGroupPolicy{{ID: "north", ActiveRatePerHour: 20}},
	}}
	_, err := runwayGapActions(t, repository, &recordingPublisher{}, now).CreateRunwayClosure(context.Background(), aman.CommandContext{
		Airport: "EKCH", Actor: "attacker", Role: "EKCH_TWR", ReceivedAt: now,
	}, aman.CreateRunwayClosureCommand{
		Metadata: aman.CommandMetadata{CommandID: "spoof", ExpectedRevision: 1},
		Interval: aman.RunwayClosureIntervalInput{RunwayGroupID: "north", Start: &start}, Reason: "spoof",
	})
	requireDomainErrorClass(t, err, aman.ErrorUnauthorized)
	require.Empty(t, repository.commits)

	for _, commandType := range []reflect.Type{reflect.TypeFor[aman.CreateRunwayClosureCommand](), reflect.TypeFor[aman.RemoveRunwayClosureCommand]()} {
		for _, forbidden := range []string{"Airport", "Actor", "Role", "CreatedAt", "CreatedBy", "ReceivedAt"} {
			_, present := commandType.FieldByName(forbidden)
			require.Falsef(t, present, "closure payload must not accept server-owned %s", forbidden)
		}
	}
}

func TestCreateRunwayClosureReplaysDeterministically(t *testing.T) {
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	start, end := now.Add(time.Minute), now.Add(5*time.Minute)
	initial := aman.AirportState{
		Airport: "EKCH", Revision: 3, GeneratedAt: now, PolicyVersion: "closure-v1", Mode: aman.ModeAuthoritative,
		RunwayGroups: []aman.RunwayGroupPolicy{{ID: "north", ActiveRatePerHour: 20}},
	}
	command := aman.CreateRunwayClosureCommand{
		Metadata: aman.CommandMetadata{CommandID: "deterministic", ExpectedRevision: 3},
		Interval: aman.RunwayClosureIntervalInput{RunwayGroupID: "north", Start: &start, End: &end}, Reason: "inspection",
	}
	auth := aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKCH_FMH", ReceivedAt: now}
	commits := make([][]byte, 0, 2)
	for range 2 {
		repository := &memoryRepository{has: true, state: cloneGapState(t, initial)}
		result, err := runwayGapActions(t, repository, &recordingPublisher{}, now.Add(time.Second)).CreateRunwayClosure(context.Background(), auth, command)
		require.NoError(t, err)
		require.Equal(t, aman.SequenceRevision(4), result.CurrentRevision)
		encoded, marshalErr := json.Marshal(repository.commits[0])
		require.NoError(t, marshalErr)
		commits = append(commits, encoded)
	}
	require.Equal(t, commits[0], commits[1])
}

func TestReconciliationExpiresFiniteClosuresAtExclusiveEndExactlyOnce(t *testing.T) {
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	created := now.Add(-time.Hour)
	ended, future := now, now.Add(time.Minute)
	repository := &memoryRepository{has: true, state: aman.AirportState{
		Airport: "EKCH", Revision: 4, GeneratedAt: now.Add(-time.Second), PolicyVersion: policyVersion, Mode: aman.ModeShadow,
		RunwayGroups: []aman.RunwayGroupPolicy{{ID: "ARRIVAL-22", Selected: true, ActiveRatePerHour: 20, RateEffectiveAt: &created, Closures: []aman.RunwayClosure{
			{ID: "ended", Start: created, End: &ended, Reason: "ended", CreatedAt: created, CreatedBy: "creator"},
			{ID: "future", Start: created, End: &future, Reason: "future", CreatedAt: created, CreatedBy: "creator"},
			{ID: "indefinite", Start: created, Reason: "indefinite", CreatedAt: created, CreatedBy: "creator"},
		}}},
	}}
	service, err := New(Dependencies{
		Repository: repository, Materializer: unavailableNavigation{}, Geometry: unavailableGeometry{}, Wind: unavailableWind{},
		Publisher: &recordingPublisher{}, Terminal: terminal.Configuration{Airport: "EKCH", ConfigVersion: "test", RunwayGroups: []terminal.RunwayGroup{{ID: "ARRIVAL-22"}}},
		Airports: []string{"EKCH"}, Mode: aman.ModeShadow, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	require.NoError(t, service.reconcileAirport(context.Background(), "EKCH"))
	require.Equal(t, []aman.RunwayClosureID{"future", "indefinite"}, closureIDs(repository.state.RunwayGroups[0].Closures))
	require.Len(t, repository.commits, 1)
	require.Equal(t, "aman.expire_runway_closure", repository.commits[0].AuditRecords[0].Category)
	require.Equal(t, aman.SequenceRevision(5), repository.commits[0].AuditRecords[0].Revision)

	require.NoError(t, service.reconcileAirport(context.Background(), "EKCH"))
	require.Len(t, repository.commits, 1, "an expired closure must not be committed or audited again")
	now = future
	require.NoError(t, service.reconcileAirport(context.Background(), "EKCH"))
	require.Equal(t, []aman.RunwayClosureID{"indefinite"}, closureIDs(repository.state.RunwayGroups[0].Closures))
	require.Len(t, repository.commits, 2)
	require.Len(t, repository.commits[1].AuditRecords, 1)
}

func closureIDs(closures []aman.RunwayClosure) []aman.RunwayClosureID {
	ids := make([]aman.RunwayClosureID, len(closures))
	for index := range closures {
		ids[index] = closures[index].ID
	}
	return ids
}
