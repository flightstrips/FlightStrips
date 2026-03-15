package euroscope

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	esEvents "FlightStrips/pkg/events/euroscope"
	pkgModels "FlightStrips/pkg/models"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTestHub returns a minimal Hub wired with the given server and strip service.
func buildTestHub(server *testutil.MockServer, ss *testutil.NoOpStripService) *Hub {
	return &Hub{
		server:       server,
		stripService: ss,
		master:       make(map[int32]*Client),
	}
}

// buildTestClient returns a Client attached to hub with the given session.
func buildTestClient(hub *Hub, session int32) *Client {
	return &Client{
		hub:     hub,
		session: session,
	}
}

func TestApplyOrValidateRunways_Master_CallsUpdateLayouts(t *testing.T) {
	var updateLayoutsCalled bool

	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
			return &models.Session{ID: id, Airport: "EKCH"}, nil
		},
	}

	mockServer := &testutil.MockServer{
		SessionRepoVal: sessionRepo,
		FrontendHubVal: &testutil.MockFrontendHub{},
		UpdateLayoutsFn: func(_ int32) error {
			updateLayoutsCalled = true
			return nil
		},
	}

	hub := buildTestHub(mockServer, &testutil.NoOpStripService{})
	client := buildTestClient(hub, 1)

	// Register client as master for session 1.
	hub.master[1] = client

	err := applyOrValidateRunways(context.Background(), client, []esEvents.SyncRunway{
		{Name: "04L", Departure: true},
		{Name: "22R", Arrival: true},
	})
	require.NoError(t, err)
	assert.True(t, updateLayoutsCalled, "UpdateLayouts must be called after a runway change by master client")
}

func TestApplyOrValidateRunways_Slave_DoesNotCallUpdateLayouts(t *testing.T) {
	var updateLayoutsCalled bool

	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
			return &models.Session{
				ID:      id,
				Airport: "EKCH",
				ActiveRunways: pkgModels.ActiveRunways{
					DepartureRunways: []string{"04L"},
					ArrivalRunways:   []string{"22R"},
				},
			}, nil
		},
	}

	mockServer := &testutil.MockServer{
		SessionRepoVal: sessionRepo,
		FrontendHubVal: &testutil.MockFrontendHub{},
		UpdateLayoutsFn: func(_ int32) error {
			updateLayoutsCalled = true
			return nil
		},
	}

	hub := buildTestHub(mockServer, &testutil.NoOpStripService{})
	master := buildTestClient(hub, 1)
	slave := buildTestClient(hub, 1)

	// Only master is registered; slave is a different pointer.
	hub.master[1] = master

	// Slave sends the same runways that are already active (no mismatch warning, just early return).
	err := applyOrValidateRunways(context.Background(), slave, []esEvents.SyncRunway{
		{Name: "04L", Departure: true},
		{Name: "22R", Arrival: true},
	})
	require.NoError(t, err)
	assert.False(t, updateLayoutsCalled, "UpdateLayouts must NOT be called for a slave client")
}
