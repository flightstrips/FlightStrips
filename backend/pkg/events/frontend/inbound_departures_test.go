package frontend

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInboundDepartureWireSnapshots(t *testing.T) {
	event := InboundDeparturesEvent{SessionID: 42, Airport: "EKCH", Enabled: true, UpdatedAt: "2026-10-13T20:43:00Z", Flights: []InboundDeparture{{Callsign: "FBT08F", Departure: "LFLL", Arrival: "EKCH", Ctot: "1906", Atot: "1924", Status: "late", DeviationSeconds: 1080, OutsideWindowSeconds: 480}}}
	payload, err := event.Marshal()
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(payload, &parsed))
	require.Equal(t, "inbound_departures", parsed["type"])
	require.EqualValues(t, 42, parsed["session_id"])
	flight := parsed["flights"].([]any)[0].(map[string]any)
	require.EqualValues(t, 480, flight["outside_window_seconds"])
	initial := InitialEvent{InboundDepartures: &event}
	payload, err = initial.Marshal()
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(payload, &parsed))
	require.Equal(t, "initial", parsed["type"])
	require.Equal(t, "EKCH", parsed["inbound_departures"].(map[string]any)["airport"])
}
