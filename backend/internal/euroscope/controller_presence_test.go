package euroscope

import (
	"FlightStrips/internal/airportworkflow"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	es "FlightStrips/pkg/events/euroscope"
	"context"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"testing"
	"time"
)

func TestMembershipControllerPresenceRetainsOperationalProjection(t *testing.T) {
	for _, observerFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "operational_first", true: "observer_first"}[observerFirst], func(t *testing.T) {
			backendWorkflowFixture()
			workflows, err := airportworkflow.NewRegistry(backendFixtureDefinitions, airportworkflow.EKCHDescriptor())
			require.NoError(t, err)
			pool, queries := testdata.SetupTestDB(t)
			require.NoError(t, queries.InsertAirport(t.Context(), "EKCH"))
			sessions := postgres.NewSessionRepository(pool)
			session, err := sessions.Create(t.Context(), &models.Session{Name: "LIVE", Airport: "EKCH"})
			require.NoError(t, err)
			_, err = pool.Exec(t.Context(), "UPDATE sessions SET created_at=$1 WHERE id=$2", time.Now().UTC().Add(-time.Hour), session)
			require.NoError(t, err)
			repo := postgres.NewControllerRepository(pool)
			hub := membershipTestHub()
			hub.airports = backendFixtureDefinitions
			hub.workflows = workflows
			hub.server = &testutil.MockServer{ControllerRepoVal: repo, FrontendHubVal: &presenceFrontend{}, StripRepoVal: &testutil.MockStripRepository{ListFn: func(context.Context, int32) ([]*models.Strip, error) { return nil, nil }}, GetOrCreateSessionFn: func(string, string) (shared.Session, error) { return shared.Session{Id: session}, nil }}
			operational := membershipTestRoot(hub, "123", "operational-instance")
			defer operational.Close()
			operational.transport.evidence = es.ControlEvent{Environment: "LIVE", Callsign: "SAME", PrimaryFrequency: "118.000"}
			observer := membershipTestRoot(hub, "123", "observer-instance")
			defer observer.Close()
			observer.transport.evidence = es.ControlEvent{Environment: "LIVE", Callsign: "SAME", PrimaryFrequency: "119.000", Observer: true}
			roots := []*Client{operational, observer}
			if observerFirst {
				roots = []*Client{observer, operational}
			}
			members := map[*Client]*Client{}
			for _, root := range roots {
				members[root], err = hub.JoinMembership(root, "EKCH")
				require.NoError(t, err)
			}
			op, obs := members[operational], members[observer]
			require.False(t, op.identitySnapshot().observer)
			require.True(t, obs.identitySnapshot().observer)
			checkOperational := func() {
				row, err := repo.Get(t.Context(), "SAME", session)
				require.NoError(t, err)
				require.False(t, row.Observer)
				require.Equal(t, "118.000", row.Position)
				require.Equal(t, "123", *row.Cid)
				require.NotNil(t, row.LastSeenEuroscope)
			}
			checkOperational()
			past := time.Now().UTC().Add(-time.Minute)
			_, err = pool.Exec(t.Context(), "UPDATE controllers SET last_seen_euroscope=$1 WHERE session=$2", past, session)
			require.NoError(t, err)
			payload, err := proto.Marshal(&es.ControlEvent{Environment: "LIVE", Callsign: "SAME", PrimaryFrequency: "120.000", RelevantAirport: "EKCH", Observer: true})
			require.NoError(t, err)
			require.NoError(t, handleControlEvent(t.Context(), observer, Message{Message: payload}))
			require.Equal(t, "120.000", obs.identitySnapshot().position)
			require.NoError(t, observer.HandlePong())
			checkOperational()
			row, err := repo.Get(t.Context(), "SAME", session)
			require.NoError(t, err)
			require.WithinDuration(t, past, *row.LastSeenEuroscope, time.Microsecond)
			require.NoError(t, operational.HandlePong())
			threshold := time.Now().UTC().Add(-5 * time.Minute)
			expired, err := sessions.GetExpiredSessions(t.Context(), &threshold)
			require.NoError(t, err)
			require.Empty(t, expired)
			row, err = repo.Get(t.Context(), "SAME", session)
			require.NoError(t, err)
			lastSeen := *row.LastSeenEuroscope
			operational.SetUser(shared.NewAuthenticatedUser("123", 0, &jwt.Token{Claims: jwt.MapClaims{"exp": float64(time.Now().Add(-time.Minute).Unix())}}))
			require.Error(t, operational.HandlePong())
			require.Error(t, op.HandlePong())
			require.False(t, op.IsAuthenticated())
			row, err = repo.Get(t.Context(), "SAME", session)
			require.NoError(t, err)
			require.Equal(t, lastSeen, *row.LastSeenEuroscope)
			operational.SetUser(observer.userSnapshot())
			require.True(t, op.IsAuthenticated())
			hub.RemoveMembership(op, "coverage removed")
			row, err = repo.Get(t.Context(), "SAME", session)
			require.NoError(t, err)
			require.True(t, row.Observer)
			require.Equal(t, "120.000", row.Position)
			require.Nil(t, row.LastSeenEuroscope)
			require.NoError(t, observer.HandlePong())
			expired, err = sessions.GetExpiredSessions(t.Context(), &threshold)
			require.NoError(t, err)
			require.Len(t, expired, 1)
			require.Equal(t, session, expired[0].ID)
			hub.RemoveMembership(obs, "coverage removed")
			row, err = repo.Get(t.Context(), "SAME", session)
			require.NoError(t, err)
			require.Nil(t, row.Cid)
			require.Nil(t, row.LastSeenEuroscope)
		})
	}
}
func TestControlOnlyPongRejectsExpiredPhysicalAuthentication(t *testing.T) {
	root := membershipTestRoot(membershipTestHub(), "123", "instance")
	defer root.Close()
	root.SetUser(shared.NewAuthenticatedUser("123", 0, &jwt.Token{Claims: jwt.MapClaims{"exp": float64(time.Now().Add(-time.Minute).Unix())}}))
	require.Error(t, root.HandlePong())
}

type presenceFrontend struct{ testutil.MockFrontendHub }

func (*presenceFrontend) CidOnline(int32, string) {}
