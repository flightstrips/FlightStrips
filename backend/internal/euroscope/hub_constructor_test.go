package euroscope

import (
	"FlightStrips/internal/services"
	"FlightStrips/internal/shared"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewHubRejectsMissingRequiredDependencies(t *testing.T) {
	stripService := services.NewStripService(nil)
	controllerService := services.NewControllerService(nil)

	_, err := NewHub(HubDependencies{})
	require.EqualError(t, err, "EuroScope hub requires strip service")

	_, err = NewHub(HubDependencies{Strips: stripService})
	require.EqualError(t, err, "EuroScope hub requires controller service")

	_, err = NewHub(HubDependencies{Strips: stripService, Controllers: controllerService})
	require.EqualError(t, err, "EuroScope hub requires authentication service")
}

func TestHubOmitsPDCHandlersUntilFeatureRegistration(t *testing.T) {
	hub := &Hub{handlers: shared.NewMessageHandlers[euroscopeEvents.EventType, *Client]()}
	err := hub.handlers.Handle(context.Background(), nil, shared.Message[euroscopeEvents.EventType]{
		Type: euroscopeEvents.IssuePdcClearance,
	})
	require.EqualError(t, err, "no handler for event type: issue_pdc_clearance")
}
