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
	request, err := New(commandID, "EKCH", "flight-1", "EKCH_APP", "1234567", "EKCH_FMH", KindRouteDirect,
		Payload{RouteDirect: &RouteDirectPayload{DirectTo: "MONAK"}}, at)
	require.NoError(t, err)
	return request
}

func TestRequestKindsAndCommandDerivedIdentity(t *testing.T) {
	route := routeRequest(t, "command-1", testTime)
	require.Equal(t, RequestID("coordination-request/command-1"), route.ID)

	speed, err := New("command-2", "EKCH", "flight-1", "EKCH_APP", "1234567", "EKCH_FMH", KindSpeed,
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
	for _, state := range []State{StateAccepted, StateRejected} {
		t.Run(string(state), func(t *testing.T) {
			request := routeRequest(t, "command-"+string(state), testTime)
			reason := ""
			if state == StateRejected {
				reason = "unable"
			}
			resolved, err := request.Decide("decision-"+string(state), "7654321", "EKCH_APP", "EKCH_APP", state, reason, testTime.Add(time.Minute))
			require.NoError(t, err)
			require.Equal(t, state, resolved.State)
			require.Equal(t, resolved.UpdatedAt, *resolved.ResolvedAt)
			require.Equal(t, StatePending, resolved.Decision.BeforeState)
			_, err = resolved.Decide("another", "7654321", "EKCH_APP", "EKCH_APP", StateRejected, "unable", testTime.Add(2*time.Minute))
			require.Error(t, err, "terminal state must be immutable")
		})
	}
	request := routeRequest(t, "command-expired", testTime)
	expired, err := request.Transition(StateExpired, testTime.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, StateExpired, expired.State)
	_, err = expired.Decide("late", "7654321", "EKCH_APP", "EKCH_APP", StateAccepted, "", testTime.Add(2*time.Minute))
	require.Error(t, err)
	request = routeRequest(t, "command-superseded", testTime)
	superseded, err := request.Supersede("coordination-request/replacement", testTime.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, StateSuperseded, superseded.State)
	_, err = superseded.Decide("stale", "7654321", "EKCH_APP", "EKCH_APP", StateRejected, "unable", testTime.Add(2*time.Minute))
	require.Error(t, err)

	request = routeRequest(t, "command-invalid", testTime)
	_, err = request.Transition(StatePending, testTime.Add(time.Minute))
	require.Error(t, err)
	_, err = request.Decide("bad", "7654321", "EKCH_APP", "EKCH_APP", StateAccepted, "", testTime.Add(-time.Minute))
	require.Error(t, err)
}

func TestRequestJSONIgnoresAdditiveFields(t *testing.T) {
	var request Request
	err := json.Unmarshal([]byte(`{
        "id":"coordination-request/command-1","command_id":"command-1","airport":"EKCH",
        "flight_id":"flight-1","recipient_controller":"EKCH_APP","submitted_by":"1234567","submitted_role":"EKCH_FMH","kind":"route_direct","state":"pending",
        "payload":{"route_direct":{"direct_to":"MONAK","future_detail":"kept-by-new-writers"},"future_payload":true},
        "created_at":"2026-09-12T10:00:00Z","updated_at":"2026-09-12T10:00:00Z","future_top_level":42}`), &request)
	require.NoError(t, err)
	require.NoError(t, request.Validate())
	require.Equal(t, "MONAK", request.Payload.RouteDirect.DirectTo)
}

func TestRequestRecipientStatusIsRollingCompatible(t *testing.T) {
	legacy := routeRequest(t, "legacy", testTime)
	legacy.RecipientStatus = ""
	require.NoError(t, legacy.Validate())

	unassigned, err := New("unassigned", "EKCH", "flight-1", "", "1234567", "EKCH_FMH", KindSpeed,
		Payload{Speed: &SpeedPayload{Requested: "220 KT"}}, testTime)
	require.NoError(t, err)
	require.Equal(t, RecipientUnassigned, unassigned.RecipientStatus)

	raw, err := json.Marshal(unassigned)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"recipient_status":"unassigned"`)
}

func TestPendingRequestTransfersRecipientWithoutChangingIdentityOrContent(t *testing.T) {
	request := routeRequest(t, "transfer", testTime)
	transferred, err := request.TransferRecipient("es/2", 2, "EKCH_DEP", testTime.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, request.ID, transferred.ID)
	require.Equal(t, request.Payload, transferred.Payload)
	require.Equal(t, request.Kind, transferred.Kind)
	require.Equal(t, request.CreatedAt, transferred.CreatedAt)
	require.Equal(t, request.State, transferred.State)
	require.Equal(t, ControllerID("EKCH_DEP"), transferred.RecipientController)
	require.Equal(t, []RecipientTransfer{{OwnershipFact: "es/2", OwnershipRevision: 2, PreviousRecipient: "EKCH_APP",
		NewRecipient: "EKCH_DEP", TransferredAt: testTime.Add(time.Minute)}}, transferred.RecipientTransfers)

	unassigned, err := transferred.TransferRecipient("es/3", 3, "", testTime.Add(2*time.Minute))
	require.NoError(t, err)
	require.Equal(t, RecipientUnassigned, unassigned.RecipientStatus)
	require.Empty(t, unassigned.RecipientController)
}
