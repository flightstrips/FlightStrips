package frontend

import (
	"FlightStrips/internal/models"
	events "FlightStrips/pkg/events/frontend"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFrontendCommunicationStateIsCanonical(t *testing.T) {
	for input, expected := range map[string]string{"t": "T", "T": "T", "r": "R", "v": "V", "?": "", "": ""} {
		t.Run(input, func(t *testing.T) {
			payload := MapStripToFrontendModel(&models.Strip{Callsign: "SAS909", CommunicationType: &input})
			require.Equal(t, expected, payload.CommunicationType)
			hub := &Hub{send: make(chan internalMessage, 1)}
			hub.SendCommunicationTypeEvent(96, "SAS909", input)
			update := (<-hub.send).message.(events.CommunicationTypeEvent)
			require.Equal(t, expected, update.CommunicationType)
			require.Equal(t, payload.CommunicationType == "T", (&models.Strip{CommunicationType: &input}).IsTextOnly())
		})
	}
}
