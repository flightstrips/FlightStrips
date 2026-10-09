package euroscope

import (
	"context"
	"errors"
	"strings"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	outgoing "FlightStrips/pkg/events"
	events "FlightStrips/pkg/events/euroscope"
	"github.com/stretchr/testify/require"
)

func TestScratchPadEditsFromNonTrackingControllerPersistAndBroadcast(t *testing.T) {
	for _, text := range []string{"CALL OPS", "", "123456789012345"} {
		t.Run(text, func(t *testing.T) {
			repo := &testutil.MockStripRepository{UpdateFsScratchPadFn: func(_ context.Context, session int32, callsign, value string) (int64, error) {
				require.Equal(t, int32(42), session)
				require.Equal(t, "SAS123", callsign)
				require.Equal(t, text, value)
				return 1, nil
			}}
			hub := &Hub{send: make(chan internalMessage, 1), server: &testutil.MockServer{StripRepoVal: repo}}
			client := &Client{hub: hub, session: 42, callsign: "EKCH_B_APP"}
			event := &events.FsScratchPadEvent{Callsign: "SAS123", Text: text}
			require.NoError(t, handleFsScratchPad(context.Background(), client, Message{Message: mustMarshalMessage(t, event)}))
			broadcast := <-hub.send
			require.Equal(t, int32(42), broadcast.session)
			require.Nil(t, broadcast.cid, "the sender must receive the authoritative echo too")
			require.Equal(t, text, broadcast.message.(events.FsScratchPadEvent).Text)
		})
	}
}

func TestScratchPadRejectsObserverAndInvalidText(t *testing.T) {
	for _, test := range []struct {
		text     string
		observer bool
	}{
		{"NOTE", true}, {strings.Repeat("x", 16), false}, {"A\nB", false}, {"A\x00B", false},
	} {
		client := &Client{observer: test.observer}
		event := &events.FsScratchPadEvent{Callsign: "SAS123", Text: test.text}
		require.Error(t, handleFsScratchPad(context.Background(), client, Message{Message: mustMarshalMessage(t, event)}))
	}
}

func TestScratchPadDoesNotBroadcastFailedOrUnchangedWrites(t *testing.T) {
	for _, failure := range []error{nil, errors.New("database unavailable")} {
		hub := &Hub{send: make(chan internalMessage, 1), server: &testutil.MockServer{StripRepoVal: &testutil.MockStripRepository{
			UpdateFsScratchPadFn: func(context.Context, int32, string, string) (int64, error) { return 0, failure },
		}}}
		client := &Client{hub: hub, session: 42}
		err := handleFsScratchPad(context.Background(), client, Message{Message: mustMarshalMessage(t, &events.FsScratchPadEvent{Callsign: "SAS123", Text: "NOTE"})})
		require.ErrorIs(t, err, failure)
		require.Empty(t, hub.send)
	}
}

func TestScratchPadReconnectIncludesSavedTextAndEmptyValues(t *testing.T) {
	hub := &Hub{server: &testutil.MockServer{StripRepoVal: &testutil.MockStripRepository{
		ListFn: func(context.Context, int32) ([]*models.Strip, error) {
			// These planning strips do not require a EuroScope flight plan to cache shared text.
			return []*models.Strip{nil, {Callsign: "SAS123", FsScratchPad: "NOTE"}, {Callsign: "SAS456", FsScratchPad: ""}}, nil
		},
	}}}
	client := startQueuedTestClient(&Client{session: 42, send: make(chan outgoing.OutgoingMessage, 3)})
	hub.sendBackendSyncIfNeeded(client)
	event := (<-client.send).(events.FsScratchPadEvent)
	require.Equal(t, "NOTE", event.Text)
	require.Equal(t, "SAS123", event.Callsign)
	clearEvent := (<-client.send).(events.FsScratchPadEvent)
	require.Equal(t, "SAS456", clearEvent.Callsign)
	require.Empty(t, clearEvent.Text)
}
