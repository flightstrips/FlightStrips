package frontend

import (
	"context"
	"testing"
	"time"

	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
)

func TestSendStripUpdateDoesNotPublishStripUnseenByEuroscope(t *testing.T) {
	hub := &Hub{
		server: &testutil.MockServer{
			StripRepoVal: &testutil.MockStripRepository{
				GetByCallsignFn: func(context.Context, int32, string) (*internalModels.Strip, error) {
					return &internalModels.Strip{Callsign: "PREFILE1", Bay: "DEP_HIDDEN"}, nil
				},
			},
		},
		send: make(chan internalMessage, 1),
	}

	hub.SendStripUpdate(42, "PREFILE1")

	select {
	case <-hub.send:
		t.Fatal("a strip not seen by EuroScope was published to the frontend")
	default:
	}
}

func TestSendStripUpdateDoesNotPublishPendingEuroscopeDisconnect(t *testing.T) {
	seenAt := time.Now().UTC()
	esHub := &snapshotPendingDisconnectHub{
		MockEuroscopeHub: &testutil.MockEuroscopeHub{},
		pending:          map[string]bool{"STALE1": true},
	}
	hub := &Hub{
		server: &testutil.MockServer{
			StripRepoVal: &testutil.MockStripRepository{
				GetByCallsignFn: func(context.Context, int32, string) (*internalModels.Strip, error) {
					return &internalModels.Strip{Callsign: "STALE1", EuroscopeSeenAt: &seenAt}, nil
				},
			},
			EuroscopeHubVal: esHub,
		},
		send: make(chan internalMessage, 1),
	}

	hub.SendStripUpdate(42, "STALE1")

	select {
	case <-hub.send:
		t.Fatal("a strip pending EuroScope disconnect was published to the frontend")
	default:
	}
}
