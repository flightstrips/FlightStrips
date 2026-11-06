package frontend

import (
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	fe "FlightStrips/pkg/events/frontend"
	pkgmodels "FlightStrips/pkg/models"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDisabledAirportTextPilotRejectsBeforeOwnPluginOrStripActions(t *testing.T) {
	server := mockServerWithStripRepo(&testutil.MockStripRepository{})
	service := &spyStripService{}
	hub := buildFrontendTestHub(server, service)
	hub.SetTextPilotInputs(func(string) *pkgmodels.TextPilotInputs { return nil })
	client := startQueuedTestClient(buildFrontendTestClient(hub, 1, "AAAA"))
	client.SetUser(shared.NewAuthenticatedUser("123", 0, nil))
	binding := &selectedFrequencyBinding{identityTestBinding: identityTestBinding{session: 1, cid: "123"}}
	ctx, err := shared.WithHumanCommand(t.Context(), binding)
	require.NoError(t, err)
	req := fe.SendFrequencyMessageEvent{RequestID: "disabled", Callsign: "SAS123", Message: "Hold position"}
	require.NoError(t, handleSendFrequencyMessage(ctx, client, marshalMessage(t, req)))
	require.Empty(t, binding.sent)
	require.Empty(t, service.moveFrontendStripCalls)
	result := (<-client.send).(fe.FrequencyMessageResultEvent)
	require.Contains(t, result.Error, "disabled or unavailable")
}
