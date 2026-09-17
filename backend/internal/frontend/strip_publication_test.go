package frontend

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	events "FlightStrips/pkg/events/frontend"
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type publicationRepository struct {
	*testutil.MockStripRepository
	snapshot *models.StripPublicationSnapshot
}

func (r *publicationRepository) GetStripPublicationSnapshot(context.Context, int32, string) (*models.StripPublicationSnapshot, error) {
	return r.snapshot, nil
}

func TestStripPublicationUsesSnapshotAndPreservesPayload(t *testing.T) {
	seen := time.Now().UTC()
	heading := int32(90)
	snapshot := &models.StripPublicationSnapshot{
		Strip:       &models.Strip{ID: 3, Session: 42, Callsign: "PUB1", Heading: &heading, EuroscopeSeenAt: &seen},
		Session:     &models.Session{ID: 42},
		Assignments: []*models.StandAssignment{{SessionID: 42, Callsign: "PUB1", Stand: "A1", Direction: "DEPARTURE", Stage: "RESERVED"}},
	}
	hub := &Hub{server: &standAllocationPublishingServer{assignments: &standAssignmentSnapshotRepository{}, MockServer: &testutil.MockServer{
		StripRepoVal: &publicationRepository{MockStripRepository: &testutil.MockStripRepository{GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
			t.Fatal("unexpected strip reread")
			return nil, nil
		}}, snapshot: snapshot},
		ComputeNextDisplayForStripContextFn: func(ctx context.Context, strip *models.Strip, session int32) (*models.NextDisplay, error) {
			require.Same(t, snapshot, shared.StripPublication(ctx, session))
			return &models.NextDisplay{Label: "TWR", Frequency: "118.100"}, nil
		},
	}}, send: make(chan internalMessage, 1)}
	hub.SendStripUpdateContext(context.Background(), 42, "PUB1")
	message := <-hub.send
	event, ok := message.message.(events.StripUpdateEvent)
	require.True(t, ok)
	require.NotNil(t, event.Strip.StandAssignment)
	require.Equal(t, "A1", event.Strip.StandAssignment.Stand)
	require.Equal(t, MapStripToFrontendModelWithClx(snapshot.Strip, hub.makeClxValidationContext(42)).Heading, event.Strip.Heading)
	// Publication-only state must not leak to the caller or another session.
	require.Nil(t, shared.StripPublication(context.Background(), 42))
	require.Nil(t, shared.StripPublication(shared.WithStripPublication(context.Background(), snapshot), 43))
}
