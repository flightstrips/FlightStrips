package frontend

import (
	"testing"

	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events"
	fe "FlightStrips/pkg/events/frontend"
	"github.com/stretchr/testify/require"
)

type aircraftSelectionSource struct {
	boardRecoverySource
	selection fe.AircraftSelection
	cid       string
	board     events.BoardContext
}

func (s *aircraftSelectionSource) CurrentAircraftSelection(cid string, board events.BoardContext) *fe.AircraftSelection {
	s.cid, s.board = cid, board
	selection := s.selection
	return &selection
}

func TestInitialSnapshotRestoresOwnAircraftSelectionOnConnectAndReconnect(t *testing.T) {
	hub := newAMANInitialTestHub(t, nil)
	source := &aircraftSelectionSource{boardRecoverySource: boardRecoverySource{available: true}}
	source.HasActiveClientForAirportFn = func(string) bool { return true }
	hub.server.(*testutil.MockServer).EuroscopeHubVal = source
	board := events.BoardContext{SessionID: 42, Airport: "EKCH", Environment: "LIVE", InstanceID: "own", MembershipID: "member", TransportGeneration: 1, Generation: 2, DefinitionRevision: "revision", SubscriptionID: "tab"}
	callsign := "SAS123"
	for _, selection := range []fe.AircraftSelection{{Callsign: &callsign, Revision: 3}, {Revision: 7}} {
		source.selection = selection
		client := startQueuedTestClient(&Client{
			hub: hub, selection: membershipSelection{board: &board, session: 42, instance: "own", membership: "member"},
			session: 42, sessionName: "LIVE", airport: "EKCH", callsign: "EKCH_A_TWR", position: "118.105", user: validFrontendUser("1234567"),
		})
		hub.OnRegister(client)
		event := waitForOutgoingMessage(t, client.send).(fe.BoundMessage).Message.(fe.InitialEvent)
		require.Equal(t, &selection, event.AircraftSelection)
		require.Equal(t, "1234567", source.cid)
		require.Equal(t, board, source.board)
	}
}
