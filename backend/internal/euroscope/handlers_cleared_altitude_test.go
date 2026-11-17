package euroscope

import (
	"context"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	es "FlightStrips/pkg/events/euroscope"

	"github.com/stretchr/testify/require"
)

func TestClearedAltitudeUsesTrackingControllerForHoldingAircraft(t *testing.T) {
	for _, test := range []struct {
		name, sender, tracker, hold string
		master, observer, allowed   bool
	}{
		{"tracking slave", "EKDK_N_CTR", "EKDK_N_CTR", "OLPIB", false, false, true},
		{"tracking master", "EKDK_N_CTR", "EKDK_N_CTR", "OLPIB", true, false, true},
		{"other master holding", "EKDK_FMP", "EKDK_N_CTR", "OLPIB", true, false, false},
		{"other master without hold", "EKDK_FMP", "EKDK_N_CTR", "", true, false, true},
		{"untracked master", "EKDK_FMP", "", "OLPIB", true, false, true},
		{"other slave", "EKDK_V_CTR", "EKDK_N_CTR", "OLPIB", false, false, false},
		{"observer", "EKDK_N_CTR", "EKDK_N_CTR", "OLPIB", false, true, false},
		{"normalized tracker", "ekdk_n_ctr", " EKDK_N_CTR ", "OLPIB", false, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &clearedAltitudeService{}
			hub := &Hub{stripService: service, master: make(map[int32]*Client), server: &testutil.MockServer{
				StripRepoVal: &testutil.MockStripRepository{GetByCallsignFn: func(_ context.Context, session int32, callsign string) (*models.Strip, error) {
					return &models.Strip{Session: session, Callsign: callsign, TrackingController: test.tracker, Hold: test.hold}, nil
				}},
			}}
			client := &Client{hub: hub, session: 42, callsign: test.sender, observer: test.observer}
			if test.master {
				hub.master[42] = client
			}
			err := handleClearedAltitude(context.Background(), client, Message{Message: mustMarshalMessage(t, &es.ClearedAltitudeEvent{Callsign: "GAF1AB", Altitude: 23000})})
			if test.allowed {
				require.NoError(t, err)
				require.Equal(t, []int32{23000}, service.altitudes)
			} else {
				require.Error(t, err)
				require.Empty(t, service.altitudes)
			}
		})
	}
}

func TestTrackingSlaveClearedAltitudePassesMembershipAdmission(t *testing.T) {
	hub := membershipTestHub()
	service := &clearedAltitudeService{}
	hub.stripService = service
	hub.server.(*testutil.MockServer).StripRepoVal = &testutil.MockStripRepository{
		GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
			return &models.Strip{Callsign: "GAF1AB", TrackingController: "EKDK_N_CTR", Hold: "OLPIB"}, nil
		},
	}
	tracker := membershipTestMember(hub, membershipTestRoot(hub, "123", "tracker"), 42, "EKCH", "LIVE", "tracker")
	tracker.callsign = "EKDK_N_CTR"
	master := membershipTestMember(hub, membershipTestRoot(hub, "456", "master"), 42, "EKCH", "LIVE", "master")
	master.callsign = "EKDK_FMP"
	handlers := shared.NewMessageHandlers[es.EventType, *Client]()
	handlers.Add(es.ClearedAltitude, handleClearedAltitude)
	scope := tracker.wireContext()
	require.NoError(t, handlers.Handle(t.Context(), tracker, authorityMessage(t, es.ClearedAltitude, &es.ClearedAltitudeEvent{Callsign: "GAF1AB", Altitude: 23000}, scope)))
	require.Equal(t, []int32{23000}, service.altitudes)
	scope.MasterEpoch++
	require.Error(t, handlers.Handle(t.Context(), tracker, authorityMessage(t, es.ClearedAltitude, &es.ClearedAltitudeEvent{Callsign: "GAF1AB", Altitude: 9000}, scope)))
	require.Equal(t, []int32{23000}, service.altitudes)
	// A master's unsolicited local level cannot undo the accepted tracker fact.
	require.Error(t, handlers.Handle(t.Context(), master, authorityMessage(t, es.ClearedAltitude, &es.ClearedAltitudeEvent{Callsign: "GAF1AB", Altitude: 9000}, master.wireContext())))
	require.Equal(t, []int32{23000}, service.altitudes)
	// An explicitly requested SDK edit still completes through its bound result.
	event := es.ClearedAltitudeEvent{Callsign: "GAF1AB", Altitude: 22000}
	id, err := master.registerExecution(event, es.CommandOrigin_COMMAND_ORIGIN_HUMAN, master.wireContext().MasterEpoch)
	require.NoError(t, err)
	message := authorityMessage(t, es.ClearedAltitude, &event, master.wireContext())
	message.Origin, message.ExecutionID = es.CommandOrigin_COMMAND_ORIGIN_HUMAN, id
	require.NoError(t, handlers.Handle(t.Context(), master, message))
	require.Equal(t, []int32{23000, 22000}, service.altitudes)
}

type clearedAltitudeService struct {
	noOpStripService
	altitudes []int32
}

func (s *clearedAltitudeService) UpdateClearedAltitude(_ context.Context, _ int32, _ string, altitude int32) error {
	s.altitudes = append(s.altitudes, altitude)
	return nil
}
