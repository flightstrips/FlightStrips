package operational

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"github.com/stretchr/testify/require"
)

func TestRunwayGapCommandsAuthorizePersistMergeRetryAndRemove(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	existing := aman.RunwayGap{
		ID: "old-gap", Start: now.Add(5 * time.Minute), End: now.Add(8 * time.Minute),
		Label: "inspection", CreatedAt: now.Add(-time.Hour), CreatedBy: "old-controller",
	}
	repository := &memoryRepository{has: true, state: aman.AirportState{
		Airport: "EKCH", Revision: 7, GeneratedAt: now.Add(-time.Minute), PolicyVersion: "gap-v1", Mode: aman.ModeAuthoritative,
		RunwayGroups: []aman.RunwayGroupPolicy{{ID: "north", ActiveRatePerHour: 20, Gaps: []aman.RunwayGap{existing}}},
	}}
	publisher := &recordingPublisher{}
	actions := runwayGapActions(t, repository, publisher, now.Add(time.Second))
	auth := aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKCH_FMH", ReceivedAt: now}
	slots := uint32(2)
	create := aman.CreateRunwayGapCommand{
		Metadata: aman.CommandMetadata{CommandID: "create-gap", ExpectedRevision: 7}, RunwayGroupID: "north",
		Interval: aman.RunwayGapIntervalInput{Start: now, SlotCount: &slots}, Label: "approach stop",
	}

	created, err := actions.CreateRunwayGap(ctx, auth, create)
	require.NoError(t, err)
	require.True(t, created.Changed)
	require.Equal(t, aman.SequenceRevision(8), created.CurrentRevision)
	require.Len(t, repository.state.RunwayGroups[0].Gaps, 1)
	gap := repository.state.RunwayGroups[0].Gaps[0]
	require.Equal(t, aman.RunwayGapID("create-gap"), gap.ID)
	require.Equal(t, now, gap.Start)
	require.Equal(t, now.Add(8*time.Minute), gap.End)
	require.Equal(t, auth.Actor, gap.CreatedBy)
	require.Equal(t, auth.ReceivedAt, gap.CreatedAt)
	require.Len(t, repository.commits[0].AuditRecords, 1)
	require.Equal(t, "aman.create_runway_gap", repository.commits[0].AuditRecords[0].Category)
	var createAudit map[string]any
	require.NoError(t, json.Unmarshal(repository.commits[0].AuditRecords[0].Payload, &createAudit))
	require.Equal(t, []any{"old-gap"}, createAudit["replaced_ids"])
	require.NotNil(t, createAudit["before_interval"])
	require.NotNil(t, createAudit["after_interval"])

	restarted := runwayGapActions(t, repository, publisher, now.Add(2*time.Second))
	retried, err := restarted.CreateRunwayGap(ctx, auth, create)
	require.NoError(t, err)
	require.True(t, retried.Duplicate)
	require.False(t, retried.Changed)
	require.Equal(t, created.Outcome, retried.Outcome)
	require.Len(t, repository.commits, 1, "restart retry must not create another persisted GAP or audit")

	_, err = restarted.RemoveRunwayGap(ctx, auth, aman.RemoveRunwayGapCommand{
		Metadata: aman.CommandMetadata{CommandID: "stale-remove", ExpectedRevision: 7}, RunwayGroupID: "north", GapID: gap.ID,
	})
	requireDomainErrorClass(t, err, aman.ErrorRevisionConflict)
	require.Len(t, repository.state.RunwayGroups[0].Gaps, 1)

	removed, err := restarted.RemoveRunwayGap(ctx, auth, aman.RemoveRunwayGapCommand{
		Metadata: aman.CommandMetadata{CommandID: "remove-gap", ExpectedRevision: 8}, RunwayGroupID: "north", GapID: gap.ID,
	})
	require.NoError(t, err)
	require.True(t, removed.Changed)
	require.Equal(t, aman.SequenceRevision(9), removed.CurrentRevision)
	require.Empty(t, repository.state.RunwayGroups[0].Gaps)
	require.Len(t, repository.commits[1].AuditRecords, 1)
	var removeAudit map[string]any
	require.NoError(t, json.Unmarshal(repository.commits[1].AuditRecords[0].Payload, &removeAudit))
	require.NotNil(t, removeAudit["before_interval"])
	require.Nil(t, removeAudit["after_interval"])
	require.Equal(t, []any{"create-gap"}, removeAudit["removed_ids"])
}

func TestRunwayGapCommandsRejectNonFMPBeforeStateMutation(t *testing.T) {
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	repository := &memoryRepository{has: true, state: aman.AirportState{
		Airport: "EKCH", Revision: 1, GeneratedAt: now, PolicyVersion: "gap-v1", Mode: aman.ModeAuthoritative,
		RunwayGroups: []aman.RunwayGroupPolicy{{ID: "north", ActiveRatePerHour: 20}},
	}}
	actions := runwayGapActions(t, repository, &recordingPublisher{}, now)
	end := now.Add(time.Minute)
	_, err := actions.CreateRunwayGap(context.Background(), aman.CommandContext{
		Airport: "EKCH", Actor: "7654321", Role: "EKCH_TWR", ReceivedAt: now,
	}, aman.CreateRunwayGapCommand{
		Metadata: aman.CommandMetadata{CommandID: "spoof-attempt", ExpectedRevision: 1}, RunwayGroupID: "north",
		Interval: aman.RunwayGapIntervalInput{Start: now, End: &end}, Label: "approach stop",
	})
	requireDomainErrorClass(t, err, aman.ErrorUnauthorized)
	require.Empty(t, repository.commits)
}

func TestRunwayGapPayloadHasNoAuditIdentityFields(t *testing.T) {
	typeOf := reflect.TypeFor[aman.CreateRunwayGapCommand]()
	for _, forbidden := range []string{"Airport", "Actor", "Role", "CreatedAt", "CreatedBy", "ReceivedAt"} {
		_, present := typeOf.FieldByName(forbidden)
		require.Falsef(t, present, "runway GAP payload must not accept server-owned %s", forbidden)
	}
}

func runwayGapActions(t *testing.T, repository *memoryRepository, publisher *recordingPublisher, recordedAt time.Time) *sequence.ActionService {
	t.Helper()
	coordinator, err := sequence.NewCoordinator(sequence.CoordinatorDependencies{
		States: repository, Outcomes: repository, Committer: repository, Publisher: publisher, Now: func() time.Time { return recordedAt },
	})
	require.NoError(t, err)
	actions, err := sequence.NewActionService(coordinator, &Service{deps: Dependencies{FMPRoles: []string{"EKCH_FMH"}}})
	require.NoError(t, err)
	return actions
}

func requireDomainErrorClass(t *testing.T, err error, class aman.ErrorClass) {
	t.Helper()
	var domain *aman.DomainError
	require.ErrorAs(t, err, &domain)
	require.Equal(t, class, domain.Class)
}
