package euroscope

import (
	"context"
	"testing"

	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events"
	es "FlightStrips/pkg/events/euroscope"
	fe "FlightStrips/pkg/events/frontend"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type selectionFrontend struct {
	testutil.MockFrontendHub
	cid    string
	board  events.BoardContext
	events []fe.AircraftSelectionEvent
}

func (f *selectionFrontend) SendToPluginMembership(cid string, board events.BoardContext, message fe.OutgoingMessage) {
	f.cid, f.board = cid, board
	f.events = append(f.events, message.(fe.AircraftSelectionEvent))
}

func TestAircraftSelectionRoutesAndRestoresExactMembership(t *testing.T) {
	hub := membershipTestHub()
	target := &selectionFrontend{}
	hub.server.(*testutil.MockServer).FrontendHubVal = target
	root := membershipTestRoot(hub, "123", "own")
	member := membershipTestMember(hub, root, 1, "EKCH", "LIVE", "a")
	board := frontendTestBoard(t, member)
	require.Nil(t, hub.CurrentAircraftSelection("123", board))
	for index, callsign := range []string{"SAS123", "UNRELATED", ""} {
		message := Message{Message: mustMarshalMessage(t, &es.AircraftSelectionEvent{Callsign: callsign})}
		require.NoError(t, handleAircraftSelection(t.Context(), member, message))
		snapshot := hub.CurrentAircraftSelection("123", board)
		require.EqualValues(t, index+1, snapshot.Revision)
		if callsign == "" {
			require.Nil(t, snapshot.Callsign)
		} else {
			require.Equal(t, callsign, *snapshot.Callsign)
		}
		require.Equal(t, "123", target.cid)
		require.Equal(t, board.InstanceID, target.board.InstanceID)
		require.Equal(t, board.MembershipID, target.board.MembershipID)
		require.Equal(t, board.TransportGeneration, target.board.TransportGeneration)
		require.Equal(t, *snapshot, target.events[index].AircraftSelection)
	}
	for _, mutate := range []func(*events.BoardContext){
		func(b *events.BoardContext) { b.SessionID++ },
		func(b *events.BoardContext) { b.InstanceID = "other" },
		func(b *events.BoardContext) { b.MembershipID = "other" },
		func(b *events.BoardContext) { b.Airport = "EKBI" },
		func(b *events.BoardContext) { b.Environment = "PLAYBACK_test" },
		func(b *events.BoardContext) { b.Generation++ },
		func(b *events.BoardContext) { b.TransportGeneration++ },
	} {
		wrong := board
		mutate(&wrong)
		require.Nil(t, hub.CurrentAircraftSelection("123", wrong))
	}
	require.Nil(t, hub.CurrentAircraftSelection("456", board))
	hub.RemoveMembership(member, "disconnected")
	require.Nil(t, member.currentAircraftSelection())
	require.Nil(t, hub.CurrentAircraftSelection("123", board))
	replacement := membershipTestMember(hub, root, 1, "EKCH", "LIVE", "a")
	require.Nil(t, hub.CurrentAircraftSelection("123", frontendTestBoard(t, replacement)))
}

func TestAircraftSelectionAcceptsNonMasterAndObserverAndRejectsStaleScope(t *testing.T) {
	hub := membershipTestHub()
	root := membershipTestRoot(hub, "123", "own")
	member := membershipTestMember(hub, root, 1, "EKCH", "LIVE", "a")
	hub.master[1] = nil
	for _, observer := range []bool{false, true} {
		member.observer = observer
		require.NoError(t, member.CanHandleMessage("aircraft_selection"))
		require.NoError(t, member.ApplyScopedMessage(t.Context(), member.wireContext(), "aircraft_selection", nil, es.CommandOrigin_COMMAND_ORIGIN_UNSPECIFIED, "", func(context.Context) error { return nil }))
	}
	stale := proto.Clone(member.wireContext()).(*es.MembershipContext)
	stale.Generation++
	called := false
	require.Error(t, member.ApplyScopedMessage(t.Context(), stale, "aircraft_selection", nil, es.CommandOrigin_COMMAND_ORIGIN_UNSPECIFIED, "", func(context.Context) error { called = true; return nil }))
	require.False(t, called)
}

func TestAircraftSelectionProtobufRoundTrip(t *testing.T) {
	for _, callsign := range []string{"SAS123", ""} {
		wire, err := (es.AircraftSelectionEvent{Callsign: callsign}).Marshal()
		require.NoError(t, err)
		var envelope es.Envelope
		require.NoError(t, proto.Unmarshal(wire, &envelope))
		require.Equal(t, callsign, envelope.GetAircraftSelection().GetCallsign())
		require.EqualValues(t, es.AircraftSelection, envelope.ProtoReflect().WhichOneof(envelope.ProtoReflect().Descriptor().Oneofs().ByName("event")).Number())
	}
}
