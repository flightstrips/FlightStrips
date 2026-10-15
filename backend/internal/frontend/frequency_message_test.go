package frontend

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	pkgEvents "FlightStrips/pkg/events"
	"FlightStrips/pkg/events/euroscope"
	events "FlightStrips/pkg/events/frontend"
	"context"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestFrequencyMessageValidation(t *testing.T) {
	valid := events.SendFrequencyMessageEvent{RequestID: "request", Callsign: "SAS123", Message: "Taxi via TWY A."}
	require.NoError(t, validateFrequencyMessage(valid))
	for name, modify := range map[string]func(*events.SendFrequencyMessageEvent){
		"command":   func(r *events.SendFrequencyMessageEvent) { r.Message = " .msg SAS123 hello" },
		"multiline": func(r *events.SendFrequencyMessageEvent) { r.Message = "hello\nworld" },
		"null":      func(r *events.SendFrequencyMessageEvent) { r.Message = "hello\x00world" },
		"empty":     func(r *events.SendFrequencyMessageEvent) { r.Message = " " },
		"long":      func(r *events.SendFrequencyMessageEvent) { r.Message = strings.Repeat("x", 1001) },
		"other aircraft": func(r *events.SendFrequencyMessageEvent) {
			r.Actions = []json.RawMessage{[]byte(`{"type":"move","callsign":"BAW123","bay":"PUSH"}`)}
		},
		"unrestricted update": func(r *events.SendFrequencyMessageEvent) {
			r.Prepare = true
			r.Actions = []json.RawMessage{[]byte(`{"type":"update_strip_data","callsign":"SAS123","stand":"A18","sid":"BAD"}`)}
		},
		"unsupported": func(r *events.SendFrequencyMessageEvent) {
			r.Actions = []json.RawMessage{[]byte(`{"type":"issue_pdc_clearance_request","callsign":"SAS123"}`)}
		},
		"prepare without action": func(r *events.SendFrequencyMessageEvent) { r.Prepare = true },
	} {
		t.Run(name, func(t *testing.T) { r := valid; modify(&r); require.Error(t, validateFrequencyMessage(r)) })
	}
	r := valid
	r.Message = ""
	r.Clearance = true
	require.NoError(t, validateFrequencyMessage(r))
	r.Clearance = false
	r.Prepare = true
	r.Actions = []json.RawMessage{[]byte(`{"type":"runway_confirmation","callsign":"SAS123"}`)}
	require.NoError(t, validateFrequencyMessage(r))
}

func TestTextClearanceUsesAcceptedValues(t *testing.T) {
	altitude := int32(4000)
	heading := int32(90)
	squawk := "0421"
	sid := "VEDAR4A"
	route := "DCT VEDAR"
	strip := &models.Strip{Destination: "EGLL", ClearedAltitude: &altitude, AssignedSquawk: &squawk, Sid: &sid, Route: &route, Heading: &heading}
	message, err := buildTextClearance(strip)
	require.NoError(t, err)
	require.Equal(t, "Cleared to EGLL, via VEDAR4A Departure, Initial climb 4000 feet, Squawk 0421.", message)
	strip.Sid = nil
	message, err = buildTextClearance(strip)
	require.NoError(t, err)
	require.Contains(t, message, "Flight plan route")
	require.Contains(t, message, "Fly heading 090.")
	strip.Route = nil
	message, err = buildTextClearance(strip)
	require.NoError(t, err)
	require.Contains(t, message, "Radar vectors")
	sid = "SA 3000'"
	strip.Sid = &sid
	message, err = buildTextClearance(strip)
	require.NoError(t, err)
	require.Contains(t, message, "Climb straight ahead.")
	require.NotContains(t, message, "Fly heading")
	require.NotContains(t, message, "SA 3000' Departure")
	strip.AssignedSquawk = nil
	_, err = buildTextClearance(strip)
	require.Error(t, err)
}

// The real move service owns validation exceptions and timing re-evaluation.
// These tests ensure frequency batches reach it and honor its result.
type frequencyTestRouter struct {
	testutil.MockEuroscopeHub
	sent []euroscope.SendFrequencyMessageEvent
}

func (r *frequencyTestRouter) FrequencyMessageSender(_ int32, _, _ string) func(euroscope.SendFrequencyMessageEvent) bool {
	return func(e euroscope.SendFrequencyMessageEvent) bool { r.sent = append(r.sent, e); return true }
}
func TestFrequencyMessageRetriesLockedPushThroughActionHandler(t *testing.T) {
	text := "T"
	strip := &models.Strip{Callsign: "SAS123", CommunicationType: &text, ValidationStatus: &models.ValidationStatus{Active: true, IssueType: "TSAT PUSHBACK"}}
	repo := &testutil.MockStripRepository{GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return strip, nil }}
	router := &frequencyTestRouter{}
	server := mockServerWithStripRepo(repo)
	server.EuroscopeHubVal = router
	service := &spyStripService{moveFrontendStripErr: errors.New("pushback is outside the TSAT window")}
	hub := buildFrontendTestHub(server, service)
	hub.handlers = shared.NewMessageHandlers[events.EventType, *Client]()
	hub.handlers.Add(events.Move, handleMove)
	client := buildFrontendTestClient(hub, 42, "EKCH")
	client.SetUser(shared.NewAuthenticatedUser("1234567", 0, nil))
	client.send = make(chan pkgEvents.OutgoingMessage, 2)
	client.closed = make(chan struct{})
	req := events.SendFrequencyMessageEvent{RequestID: "attempt-1", Callsign: "SAS123", Message: "Push and start approved, Release point J4.", Actions: []json.RawMessage{[]byte(`{"type":"move","callsign":"SAS123","bay":"PUSH"}`)}}
	require.NoError(t, handleSendFrequencyMessage(context.Background(), client, marshalMessage(t, req)))
	require.Len(t, service.moveFrontendStripCalls, 1)
	result := (<-client.send).(events.FrequencyMessageResultEvent)
	require.Contains(t, result.Error, "outside the TSAT window")
	require.Zero(t, result.AppliedActions)
	require.Empty(t, router.sent)
	// Timing has become valid; the strip still carries its original warning
	// until the move service re-evaluates it.
	service.moveFrontendStripErr = nil
	req.RequestID = "attempt-2"
	require.NoError(t, handleSendFrequencyMessage(context.Background(), client, marshalMessage(t, req)))
	require.Len(t, service.moveFrontendStripCalls, 2)
	require.Len(t, router.sent, 1)
	require.Equal(t, int32(1), router.sent[0].AppliedActions)
}
func TestFrequencyMessageRespectsActionValidationRejection(t *testing.T) {
	text := "T"
	strip := &models.Strip{Callsign: "SAS123", CommunicationType: &text, ValidationStatus: &models.ValidationStatus{Active: true, IssueType: "TAXIWAY TYPE"}}
	repo := &testutil.MockStripRepository{GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return strip, nil }}
	router := &frequencyTestRouter{}
	server := mockServerWithStripRepo(repo)
	server.EuroscopeHubVal = router
	service := &spyStripService{moveFrontendStripErr: errors.New("strip is locked by an active validation")}
	hub := buildFrontendTestHub(server, service)
	hub.handlers = shared.NewMessageHandlers[events.EventType, *Client]()
	hub.handlers.Add(events.Move, handleMove)
	client := buildFrontendTestClient(hub, 42, "EKCH")
	client.SetUser(shared.NewAuthenticatedUser("1234567", 0, nil))
	client.send = make(chan pkgEvents.OutgoingMessage, 1)
	client.closed = make(chan struct{})
	req := events.SendFrequencyMessageEvent{RequestID: "locked-move", Callsign: "SAS123", Message: "Taxi via TWY A.", Actions: []json.RawMessage{[]byte(`{"type":"move","callsign":"SAS123","bay":"TAXI"}`)}}
	require.NoError(t, handleSendFrequencyMessage(context.Background(), client, marshalMessage(t, req)))
	require.Len(t, service.moveFrontendStripCalls, 1)
	require.Empty(t, router.sent)
	result := (<-client.send).(events.FrequencyMessageResultEvent)
	require.Equal(t, "strip is locked by an active validation", result.Error)
	require.Zero(t, result.AppliedActions)
}

func TestFrequencyMessageAcceptsTextTypeInEitherCase(t *testing.T) {
	for _, commType := range []string{"T", "t", "R", "r", "V", "v", ""} {
		t.Run(commType, func(t *testing.T) {
			strip := &models.Strip{Callsign: "SAS123", CommunicationType: &commType}
			repo := &testutil.MockStripRepository{GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return strip, nil }}
			router := &frequencyTestRouter{}
			server := mockServerWithStripRepo(repo)
			server.EuroscopeHubVal = router
			hub := buildFrontendTestHub(server, &noOpStripService{})
			client := buildFrontendTestClient(hub, 42, "EKCH")
			client.SetUser(shared.NewAuthenticatedUser("1234567", 0, nil))
			client.send = make(chan pkgEvents.OutgoingMessage, 1)
			client.closed = make(chan struct{})
			req := events.SendFrequencyMessageEvent{RequestID: "case-check", Callsign: "SAS123", Message: "Readback correct."}
			require.NoError(t, handleSendFrequencyMessage(context.Background(), client, marshalMessage(t, req)))
			if commType == "T" || commType == "t" {
				require.Len(t, router.sent, 1)
			} else {
				require.Empty(t, router.sent)
				result := (<-client.send).(events.FrequencyMessageResultEvent)
				require.Equal(t, "aircraft is not text-only", result.Error)
			}
		})
	}
}
