package euroscope

import (
	"FlightStrips/internal/airportworkflow"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	es "FlightStrips/pkg/events/euroscope"
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type serializedPresenceRepo struct {
	repository.ControllerRepository
	membershipControllerWriter
	detach           func(context.Context, string, int32, string) (int64, error)
	entered, release chan struct{}
}

func (r *serializedPresenceRepo) DetachMembership(ctx context.Context, cid string, session int32, callsign string) (int64, error) {
	close(r.entered)
	<-r.release
	return r.detach(ctx, cid, session, callsign)
}

func TestMembershipReconnectWaitsForPostgresDetach(t *testing.T) {
	backendWorkflowFixture()
	workflows, err := airportworkflow.NewRegistry(backendFixtureDefinitions, airportworkflow.EKCHDescriptor())
	require.NoError(t, err)
	pool, queries := testdata.SetupTestDB(t)
	require.NoError(t, queries.InsertAirport(t.Context(), "EKCH"))
	session, err := postgres.NewSessionRepository(pool).Create(t.Context(), &models.Session{Name: "LIVE", Airport: "EKCH"})
	require.NoError(t, err)
	actual := postgres.NewControllerRepository(pool)
	repo := &serializedPresenceRepo{ControllerRepository: actual, membershipControllerWriter: actual, detach: actual.DetachMembership, entered: make(chan struct{}), release: make(chan struct{})}
	hub := membershipTestHub()
	hub.airports = backendFixtureDefinitions
	hub.workflows = workflows
	hub.server = &testutil.MockServer{ControllerRepoVal: repo, FrontendHubVal: &presenceFrontend{}, StripRepoVal: &testutil.MockStripRepository{ListFn: func(context.Context, int32) ([]*models.Strip, error) { return nil, nil }}, GetOrCreateSessionFn: func(string, string) (shared.Session, error) { return shared.Session{Id: session}, nil }}
	oldRoot := membershipTestRoot(hub, "123", "old")
	defer oldRoot.Close()
	oldRoot.transport.evidence = es.ControlEvent{Environment: "LIVE", Callsign: "SAME", PrimaryFrequency: "118.000"}
	old, err := hub.JoinMembership(oldRoot, "EKCH")
	require.NoError(t, err)
	old.Close()
	cleanup := make(chan error, 1)
	go func() { cleanup <- hub.clearClientCid(old) }()
	<-repo.entered
	next := membershipTestRoot(hub, "123", "new")
	defer next.Close()
	next.transport.evidence = es.ControlEvent{Environment: "LIVE", Callsign: "SAME", PrimaryFrequency: "119.000"}
	joined := make(chan error, 1)
	go func() { _, err := hub.JoinMembership(next, "EKCH"); joined <- err }()
	select {
	case err := <-joined:
		t.Fatalf("new admission passed old detach: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(repo.release)
	require.NoError(t, <-cleanup)
	require.NoError(t, <-joined)
	row, err := actual.Get(t.Context(), "SAME", session)
	require.NoError(t, err)
	require.Equal(t, "123", *row.Cid)
	require.Equal(t, "119.000", row.Position)
	require.NotNil(t, row.LastSeenEuroscope)
}
