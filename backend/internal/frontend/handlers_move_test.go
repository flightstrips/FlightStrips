package frontend

import (
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	pkgFrontend "FlightStrips/pkg/events/frontend"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildFrontendTestHub(server *testutil.MockServer, ss shared.StripService) *Hub {
	return &Hub{
		server:       server,
		stripService: ss,
	}
}

func buildFrontendTestClient(hub *Hub, session int32, airport string) *Client {
	return &Client{
		hub:     hub,
		session: session,
		airport: airport,
	}
}

func marshalMessage(t *testing.T, v interface{}) Message {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return shared.Message[pkgFrontend.EventType]{Message: data}
}

func mockServerWithStripRepo(repo *testutil.MockStripRepository) *testutil.MockServer {
	ms := &testutil.MockServer{
		FrontendHubVal: &testutil.MockFrontendHub{},
	}
	ms.StripRepoVal = repo
	return ms
}

type moveFrontendStripCall struct {
	session        int32
	callsign       string
	targetBay      string
	cid            string
	airport        string
	clientPosition string
}

type spyStripService struct {
	testutil.NoOpStripService
	moveFrontendStripCalls []moveFrontendStripCall
	moveFrontendStripErr   error
}

func (s *spyStripService) MoveFrontendStrip(_ context.Context, session int32, callsign string, targetBay string, cid string, airport string, clientPosition string) error {
	s.moveFrontendStripCalls = append(s.moveFrontendStripCalls, moveFrontendStripCall{
		session:        session,
		callsign:       callsign,
		targetBay:      targetBay,
		cid:            cid,
		airport:        airport,
		clientPosition: clientPosition,
	})
	return s.moveFrontendStripErr
}

func TestHandleMove_DelegatesToStripService(t *testing.T) {
	spy := &spyStripService{}
	hub := buildFrontendTestHub(&testutil.MockServer{FrontendHubVal: &testutil.MockFrontendHub{}}, spy)
	client := buildFrontendTestClient(hub, 17, "EKCH")
	client.position = "EKCH_DEL"
	client.SetUser(shared.NewAuthenticatedUser("1234567", 0, nil))

	msg := marshalMessage(t, pkgFrontend.MoveEvent{
		Callsign: "SAS123",
		Bay:      shared.BAY_CLEARED,
	})

	err := handleMove(context.Background(), client, msg)
	require.NoError(t, err)
	require.Len(t, spy.moveFrontendStripCalls, 1)
	assert.Equal(t, moveFrontendStripCall{
		session:        17,
		callsign:       "SAS123",
		targetBay:      shared.BAY_CLEARED,
		cid:            "1234567",
		airport:        "EKCH",
		clientPosition: "EKCH_DEL",
	}, spy.moveFrontendStripCalls[0])
}

func TestHandleMove_PropagatesStripServiceError(t *testing.T) {
	spy := &spyStripService{moveFrontendStripErr: assert.AnError}
	hub := buildFrontendTestHub(&testutil.MockServer{FrontendHubVal: &testutil.MockFrontendHub{}}, spy)
	client := buildFrontendTestClient(hub, 1, "EKCH")
	client.SetUser(shared.NewAuthenticatedUser("1234567", 0, nil))

	msg := marshalMessage(t, pkgFrontend.MoveEvent{
		Callsign: "SAS123",
		Bay:      shared.BAY_PUSH,
	})

	err := handleMove(context.Background(), client, msg)
	require.ErrorIs(t, err, assert.AnError)
}
