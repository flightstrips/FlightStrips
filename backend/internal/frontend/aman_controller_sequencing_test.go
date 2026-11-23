package frontend

import (
	"context"
	"testing"

	"FlightStrips/internal/aman"
	internalModels "FlightStrips/internal/models"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"github.com/stretchr/testify/require"
)

type fixedAMANState struct{ state frontendEvents.AMANState }

func (s fixedAMANState) CurrentAMANState(context.Context, string) (frontendEvents.AMANStateEvent, error) {
	return frontendEvents.AMANStateEvent{Version: frontendEvents.AMANWireVersion, Data: s.state}, nil
}

func stringRef(value string) *string { return &value }

func sequencingState() frontendEvents.AMANState {
	flight := func(callsign, family string) frontendEvents.AMANFlight {
		return frontendEvents.AMANFlight{Callsign: callsign, STARFamily: stringRef(family)}
	}
	return frontendEvents.AMANState{
		Airport: "EKCH",
		Flights: []frontendEvents.AMANFlight{flight("SAS1", "TUDLO"), flight("SAS2", "TUDLO"), flight("SAS3", "MONAK"), flight("SAS4", "TESPI")},
		TimelineConfig: &frontendEvents.AMANTimelineConfig{Mappings: []frontendEvents.AMANTimelineMapping{
			{ID: 1, Left: stringRef("TUDLO"), Right: stringRef("MONAK")},
			{ID: 2, Left: stringRef("TESPI"), Right: stringRef("TIDVU")},
			{ID: 3, Left: stringRef("ERNOV"), Right: nil},
		}},
	}
}

func newSequencingTestHub(t *testing.T, callsign string, online []*internalModels.Controller) (*Hub, *Client, *recordingAMANCommandService) {
	t.Helper()
	hub, client, service := newForcedFMPTestHub(t, callsign, &online)
	hub.amanStateProvider = fixedAMANState{state: sequencingState()}
	return hub, client, service
}

func sendSequence(t *testing.T, hub *Hub, client *Client, eventType frontendEvents.EventType, data string) {
	t.Helper()
	payload := `{"type":"` + string(eventType) + `","version":1,"data":{"command_id":"seq-1","expected_revision":7,` + data + `}}`
	require.NoError(t, hub.handlers.Handle(context.Background(), client, Message{Type: eventType, Message: []byte(payload)}))
}

func TestControllerSequencingFamiliesFollowACCView(t *testing.T) {
	available := map[string]bool{"TUDLO": true, "MONAK": true, "TESPI": true, "TIDVU": true, "ERNOV": true}
	all := map[string]bool{"EKDK_B_CTR": true, "EKDK_D_CTR": true, "EKDK_E_CTR": true, "EKDK_I_CTR": true}
	tests := []struct {
		position string
		online   map[string]bool
		want     []string
	}{
		{"EKDK_D_CTR", all, []string{"TUDLO"}},
		{"EKDK_D_CTR", map[string]bool{}, []string{"MONAK", "TESPI", "TUDLO"}},
		{"EKCH_W_APP", map[string]bool{}, []string{"ERNOV", "TESPI", "TUDLO"}},
		{"EKDK_UC_CTR", map[string]bool{}, []string{"MONAK", "TESPI", "TUDLO"}},
		{"EKCH_P_APP", map[string]bool{}, []string{"ERNOV", "MONAK", "TESPI", "TIDVU", "TUDLO"}},
		{"EKCH_A_TWR", map[string]bool{}, nil},
	}
	for _, test := range tests {
		got := controllerSequencingFamilies(test.position, test.online, available)
		families := []string{}
		for family := range got {
			families = append(families, family)
		}
		require.ElementsMatch(t, test.want, families, test.position)
	}
}

func TestControllerCanSwapFlightsOnOwnSTARFamily(t *testing.T) {
	hub, client, service := newSequencingTestHub(t, "EKDK_D_CTR", []*internalModels.Controller{
		{Callsign: "EKDK_B_CTR"}, {Callsign: "EKDK_E_CTR"}, {Callsign: "EKDK_I_CTR"},
	})

	sendSequence(t, hub, client, frontendEvents.AMANSwapFlightsType, `"callsign":"SAS1","other_callsign":"SAS2"`)

	require.Equal(t, "swap", service.operation)
	require.Equal(t, "EKDK_D_CTR", service.auth.Role)
	require.True(t, service.auth.ControllerSequencing)
	require.False(t, service.auth.HasFMPAuthority(), "sequencing never grants FMP authority")
}

func TestControllerCannotSequenceOtherSTARFamilies(t *testing.T) {
	online := []*internalModels.Controller{{Callsign: "EKDK_B_CTR"}, {Callsign: "EKDK_E_CTR"}, {Callsign: "EKDK_I_CTR"}}
	tests := []struct {
		name string
		kind frontendEvents.EventType
		data string
	}{
		{"swap with another family", frontendEvents.AMANSwapFlightsType, `"callsign":"SAS1","other_callsign":"SAS3"`},
		{"move behind another family", frontendEvents.AMANMoveFlightType, `"callsign":"SAS1","runway_group_id":"ARRIVAL-22L","after_callsign":"SAS4"`},
		{"lock another family", frontendEvents.AMANLockFlightType, `"callsign":"SAS3"`},
		{"insert onto another family", frontendEvents.AMANInsertFlightAfterType, `"callsign":"SAS1","after_callsign":"SAS2","star_family":"MONAK"`},
		{"unknown flight", frontendEvents.AMANUnlockFlightType, `"callsign":"NOPE"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			hub, client, service := newSequencingTestHub(t, "EKDK_D_CTR", online)
			sendSequence(t, hub, client, test.kind, test.data)
			require.Empty(t, service.operation)
			rejection := lastRejection(t, client)
			require.Equal(t, string(aman.ErrorUnauthorized), rejection.Data.Code)
			require.Contains(t, rejection.Data.Message, "TUDLO")
		})
	}
}

func TestControllerSequencingAllowsOrderCommandsOnly(t *testing.T) {
	hub, client, service := newSequencingTestHub(t, "EKDK_D_CTR", nil)

	for kind, data := range map[frontendEvents.EventType]string{
		frontendEvents.AMANMoveFlightType:        `"callsign":"SAS1","runway_group_id":"ARRIVAL-22L","before_callsign":"SAS2"`,
		frontendEvents.AMANInsertFlightAfterType: `"callsign":"SAS1","after_callsign":"SAS2"`,
		frontendEvents.AMANPlaceFlightAtTimeType: `"callsign":"SAS1","runway_group_id":"ARRIVAL-22L","slot_time":"2026-11-23T12:30:00Z","allow_gap":true`,
		frontendEvents.AMANLockFlightType:        `"callsign":"SAS1"`,
		frontendEvents.AMANUnlockFlightType:      `"callsign":"SAS1"`,
	} {
		service.operation = ""
		sendSequence(t, hub, client, kind, data)
		require.NotEmpty(t, service.operation, kind)
	}

	service.operation = ""
	sendSequence(t, hub, client, frontendEvents.AMANExpediteFlightType, `"callsign":"SAS1"`)
	require.Empty(t, service.operation, "expedite stays FMP-only")
	require.Equal(t, string(aman.ErrorUnauthorized), lastRejection(t, client).Data.Code)
}

func TestControllerWithoutSTARFamiliesCannotSequence(t *testing.T) {
	hub, client, service := newSequencingTestHub(t, "EKCH_A_TWR", nil)

	sendSequence(t, hub, client, frontendEvents.AMANSwapFlightsType, `"callsign":"SAS1","other_callsign":"SAS2"`)

	require.Empty(t, service.operation)
	require.Contains(t, lastRejection(t, client).Data.Message, "FMP authority is required")
}

func TestControllerSequencingRejectsObservers(t *testing.T) {
	hub, client, service := newSequencingTestHub(t, "EKDK_D_CTR", nil)
	client.readOnly = true

	sendSequence(t, hub, client, frontendEvents.AMANSwapFlightsType, `"callsign":"SAS1","other_callsign":"SAS2"`)

	require.Empty(t, service.operation)
}

func TestFMPSequencesAnyFamily(t *testing.T) {
	hub, client, service := newSequencingTestHub(t, "EKDK_FMP", nil)

	sendSequence(t, hub, client, frontendEvents.AMANSwapFlightsType, `"callsign":"SAS1","other_callsign":"SAS3"`)

	require.Equal(t, "swap", service.operation)
	require.False(t, service.auth.ControllerSequencing)
}
