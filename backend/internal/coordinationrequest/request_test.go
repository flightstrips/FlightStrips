package coordinationrequest

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var testTime = time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)

func routeRequest(t *testing.T, commandID string, at time.Time) Request {
	t.Helper()
	request, err := New(commandID, "EKCH", "flight-1", "EKCH_APP", KindRouteDirect,
		Payload{RouteDirect: &RouteDirectPayload{DirectTo: "MONAK"}}, at)
	require.NoError(t, err)
	return request
}

func TestRequestKindsAndCommandDerivedIdentity(t *testing.T) {
	route := routeRequest(t, "command-1", testTime)
	require.Equal(t, RequestID("coordination-request/command-1"), route.ID)

	speed, err := New("command-2", "EKCH", "flight-1", "EKCH_APP", KindSpeed,
		Payload{Speed: &SpeedPayload{Requested: "220 KT"}}, testTime)
	require.NoError(t, err)
	require.Equal(t, KindSpeed, speed.Kind)
}

func TestRequestValidationRejectsIncompleteOrMismatchedAggregates(t *testing.T) {
	valid := routeRequest(t, "command-1", testTime)
	tests := map[string]func(*Request){
		"derived identity": func(r *Request) { r.ID = "request-elsewhere" },
		"flight identity":  func(r *Request) { r.FlightID = "" },
		"recipient owner":  func(r *Request) { r.RecipientController = " EKCH_APP" },
		"UTC timestamps":   func(r *Request) { r.CreatedAt = testTime.In(time.FixedZone("CET", 3600)) },
		"matching payload": func(r *Request) { r.Payload.Speed = &SpeedPayload{Requested: "220 KT"} },
		"pending resolved": func(r *Request) { resolved := testTime; r.ResolvedAt = &resolved },
		"terminal resolved": func(r *Request) {
			r.State = StateAccepted
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			request := valid
			mutate(&request)
			require.Error(t, request.Validate())
		})
	}
}

func TestRequestAllowsOnlyPendingToTerminalTransitions(t *testing.T) {
	for _, state := range []State{StateAccepted, StateRejected, StateSuperseded, StateExpired} {
		t.Run(string(state), func(t *testing.T) {
			request := routeRequest(t, "command-"+string(state), testTime)
			resolved, err := request.Transition(state, testTime.Add(time.Minute))
			require.NoError(t, err)
			require.Equal(t, state, resolved.State)
			require.Equal(t, resolved.UpdatedAt, *resolved.ResolvedAt)
			_, err = resolved.Transition(StateRejected, testTime.Add(2*time.Minute))
			require.Error(t, err, "terminal state must be immutable")
		})
	}

	request := routeRequest(t, "command-invalid", testTime)
	_, err := request.Transition(StatePending, testTime.Add(time.Minute))
	require.Error(t, err)
	_, err = request.Transition(StateAccepted, testTime.Add(-time.Minute))
	require.Error(t, err)
}

func TestRequestJSONIgnoresAdditiveFields(t *testing.T) {
	var request Request
	err := json.Unmarshal([]byte(`{
        "id":"coordination-request/command-1","command_id":"command-1","airport":"EKCH",
        "flight_id":"flight-1","recipient_controller":"EKCH_APP","kind":"route_direct","state":"pending",
        "payload":{"route_direct":{"direct_to":"MONAK","future_detail":"kept-by-new-writers"},"future_payload":true},
        "created_at":"2026-09-12T10:00:00Z","updated_at":"2026-09-12T10:00:00Z","future_top_level":42}`), &request)
	require.NoError(t, err)
	require.NoError(t, request.Validate())
	require.Equal(t, "MONAK", request.Payload.RouteDirect.DirectTo)
}
