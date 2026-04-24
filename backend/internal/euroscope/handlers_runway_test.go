package euroscope

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	esEvents "FlightStrips/pkg/events/euroscope"
	frontendEvents "FlightStrips/pkg/events/frontend"
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
		send:         make(chan internalMessage, 10),
		master:       make(map[int32]*Client),
		runwayStates: make(map[string]*clientRunwayState),
	}
}

// buildTestClient returns a Client attached to hub with the given session.
func buildTestClient(hub *Hub, session int32, cid, callsign string) *Client {
	return &Client{
		hub:      hub,
		session:  session,
		user:     shared.NewAuthenticatedUser(cid, 0, nil),
		callsign: callsign,
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
	client := buildTestClient(hub, 1, "cid-master", "EKCH_A_TWR")

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
	master := buildTestClient(hub, 1, "cid-master", "EKCH_A_TWR")
	slave := buildTestClient(hub, 1, "cid-slave", "EKCH_D_TWR")

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

func TestApplyOrValidateRunways_SlaveMismatch_TargetsFrontendAndAlertsOnceUntilResolved(t *testing.T) {
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

	frontendHub := &testutil.MockFrontendHub{}
	mockServer := &testutil.MockServer{
		SessionRepoVal: sessionRepo,
		FrontendHubVal: frontendHub,
	}

	hub := buildTestHub(mockServer, &testutil.NoOpStripService{})
	master := buildTestClient(hub, 1, "cid-master", "EKCH_A_TWR")
	slave := buildTestClient(hub, 1, "cid-slave", "EKCH_D_TWR")
	hub.master[1] = master

	mismatchRunways := []esEvents.SyncRunway{
		{Name: "22L", Departure: true},
		{Name: "04R", Arrival: true},
	}

	err := applyOrValidateRunways(context.Background(), slave, mismatchRunways)
	require.NoError(t, err)
	require.Len(t, frontendHub.SentMessages, 1)

	runwayEvent, ok := frontendHub.SentMessages[0].Message.(frontendEvents.RunwayConfigurationEvent)
	require.True(t, ok)
	assert.Equal(t, int32(1), frontendHub.SentMessages[0].Session)
	assert.Equal(t, "cid-slave", frontendHub.SentMessages[0].Cid)
	assert.Equal(t, []string{"04L"}, runwayEvent.RunwaySetup.Departure)
	assert.Equal(t, []string{"22R"}, runwayEvent.RunwaySetup.Arrival)
	assert.True(t, runwayEvent.RunwaySetup.DepartureMismatch)
	assert.True(t, runwayEvent.RunwaySetup.ArrivalMismatch)

	departureMismatch, arrivalMismatch := hub.GetRunwayMismatchStatus(1, "cid-slave")
	assert.True(t, departureMismatch)
	assert.True(t, arrivalMismatch)

	alert := readRunwayMismatchAlert(t, hub)
	assert.Equal(t, []string{"04L"}, alert.ExpectedDeparture)
	assert.Equal(t, []string{"22R"}, alert.ExpectedArrival)
	assert.Equal(t, []string{"22L"}, alert.CurrentDeparture)
	assert.Equal(t, []string{"04R"}, alert.CurrentArrival)

	err = applyOrValidateRunways(context.Background(), slave, mismatchRunways)
	require.NoError(t, err)
	require.Len(t, frontendHub.SentMessages, 1)
	assertNoRunwayAlertQueued(t, hub)

	err = applyOrValidateRunways(context.Background(), slave, []esEvents.SyncRunway{
		{Name: "04L", Departure: true},
		{Name: "22R", Arrival: true},
	})
	require.NoError(t, err)
	require.Len(t, frontendHub.SentMessages, 2)

	resolvedEvent, ok := frontendHub.SentMessages[1].Message.(frontendEvents.RunwayConfigurationEvent)
	require.True(t, ok)
	assert.False(t, resolvedEvent.RunwaySetup.DepartureMismatch)
	assert.False(t, resolvedEvent.RunwaySetup.ArrivalMismatch)
	assertNoRunwayAlertQueued(t, hub)

	departureMismatch, arrivalMismatch = hub.GetRunwayMismatchStatus(1, "cid-slave")
	assert.False(t, departureMismatch)
	assert.False(t, arrivalMismatch)

	err = applyOrValidateRunways(context.Background(), slave, mismatchRunways)
	require.NoError(t, err)
	require.Len(t, frontendHub.SentMessages, 3)
	assert.Equal(t, readRunwayMismatchAlert(t, hub), readRunwayMismatchAlertPayload(mismatchRunways))
}

func TestApplyOrValidateRunways_ObserverMismatch_TargetsFrontendAndAlerts(t *testing.T) {
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

	frontendHub := &testutil.MockFrontendHub{}
	mockServer := &testutil.MockServer{
		SessionRepoVal: sessionRepo,
		FrontendHubVal: frontendHub,
	}

	hub := buildTestHub(mockServer, &testutil.NoOpStripService{})
	master := buildTestClient(hub, 1, "cid-master", "EKCH_A_TWR")
	observer := buildTestClient(hub, 1, "cid-observer", "EKCH_OBS")
	observer.observer = true
	hub.master[1] = master

	err := applyOrValidateRunways(context.Background(), observer, []esEvents.SyncRunway{
		{Name: "22L", Departure: true},
		{Name: "04R", Arrival: true},
	})
	require.NoError(t, err)
	require.Len(t, frontendHub.SentMessages, 1)

	runwayEvent, ok := frontendHub.SentMessages[0].Message.(frontendEvents.RunwayConfigurationEvent)
	require.True(t, ok)
	assert.Equal(t, "cid-observer", frontendHub.SentMessages[0].Cid)
	assert.True(t, runwayEvent.RunwaySetup.DepartureMismatch)
	assert.True(t, runwayEvent.RunwaySetup.ArrivalMismatch)

	departureMismatch, arrivalMismatch := hub.GetRunwayMismatchStatus(1, "cid-observer")
	assert.True(t, departureMismatch)
	assert.True(t, arrivalMismatch)

	alert := readRunwayMismatchAlert(t, hub)
	assert.Equal(t, []string{"04L"}, alert.ExpectedDeparture)
	assert.Equal(t, []string{"22R"}, alert.ExpectedArrival)
	assert.Equal(t, []string{"22L"}, alert.CurrentDeparture)
	assert.Equal(t, []string{"04R"}, alert.CurrentArrival)
}

func TestApplyOrValidateRunways_ObserverWithoutMasterDoesNotFlagMismatch(t *testing.T) {
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

	frontendHub := &testutil.MockFrontendHub{}
	mockServer := &testutil.MockServer{
		SessionRepoVal: sessionRepo,
		FrontendHubVal: frontendHub,
	}

	hub := buildTestHub(mockServer, &testutil.NoOpStripService{})
	observer := buildTestClient(hub, 1, "cid-observer", "EKCH_OBS")
	observer.observer = true

	err := applyOrValidateRunways(context.Background(), observer, []esEvents.SyncRunway{
		{Name: "22L", Departure: true},
		{Name: "04R", Arrival: true},
	})
	require.NoError(t, err)
	assert.Empty(t, frontendHub.SentMessages)
	assertNoRunwayAlertQueued(t, hub)

	departureMismatch, arrivalMismatch := hub.GetRunwayMismatchStatus(1, "cid-observer")
	assert.False(t, departureMismatch)
	assert.False(t, arrivalMismatch)
}

func readRunwayMismatchAlert(t *testing.T, hub *Hub) esEvents.RunwayMismatchAlertEvent {
	t.Helper()

	select {
	case message := <-hub.send:
		alert, ok := message.message.(esEvents.RunwayMismatchAlertEvent)
		require.True(t, ok, "expected runway mismatch alert, got %T", message.message)
		return alert
	default:
		t.Fatal("expected runway mismatch alert to be queued")
		return esEvents.RunwayMismatchAlertEvent{}
	}
}

func assertNoRunwayAlertQueued(t *testing.T, hub *Hub) {
	t.Helper()

	select {
	case message := <-hub.send:
		t.Fatalf("unexpected queued message: %T", message.message)
	default:
	}
}

func readRunwayMismatchAlertPayload(runways []esEvents.SyncRunway) esEvents.RunwayMismatchAlertEvent {
	currentDeparture := make([]string, 0)
	currentArrival := make([]string, 0)
	for _, runway := range runways {
		if runway.Departure {
			currentDeparture = append(currentDeparture, runway.Name)
		}
		if runway.Arrival {
			currentArrival = append(currentArrival, runway.Name)
		}
	}

	return esEvents.RunwayMismatchAlertEvent{
		ExpectedDeparture: []string{"04L"},
		ExpectedArrival:   []string{"22R"},
		CurrentDeparture:  currentDeparture,
		CurrentArrival:    currentArrival,
	}
}
