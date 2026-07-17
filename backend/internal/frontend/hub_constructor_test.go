package frontend

import (
	"FlightStrips/internal/services"
	"FlightStrips/internal/shared"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewHubRejectsMissingRequiredDependencies(t *testing.T) {
	_, err := NewHub(HubDependencies{})
	require.EqualError(t, err, "frontend hub requires strip service")

	_, err = NewHub(HubDependencies{Strips: services.NewStripService(nil)})
	require.EqualError(t, err, "frontend hub requires authentication service")
}

func TestHubOmitsPDCHandlersUntilFeatureRegistration(t *testing.T) {
	hub := &Hub{handlers: shared.NewMessageHandlers[frontendEvents.EventType, *Client]()}
	err := hub.handlers.Handle(context.Background(), nil, shared.Message[frontendEvents.EventType]{
		Type: frontendEvents.IssuePdcClearance,
	})
	require.EqualError(t, err, "no handler for event type: issue_pdc_clearance")
}
