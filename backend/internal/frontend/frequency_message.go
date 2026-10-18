package frontend

import (
	models "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events/euroscope"
	"FlightStrips/pkg/events/frontend"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type pushbackPreparer interface {
	PrepareFrontendPushback(context.Context, int32, string, string, string, string) error
}

type frequencyMessageRouter interface {
	FrequencyMessageSender(int32, string, string) func(euroscope.SendFrequencyMessageEvent) bool
}

func validateFrequencyMessage(req frontend.SendFrequencyMessageEvent) error {
	if len(req.RequestID) == 0 || len(req.RequestID) > 100 || len(req.Callsign) == 0 || len(req.Callsign) > 16 {
		return errors.New("invalid request ID or callsign")
	}
	for _, c := range req.Callsign {
		if !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return errors.New("invalid callsign")
		}
	}
	if !req.Clearance && !req.Prepare && strings.TrimSpace(req.Message) == "" {
		return errors.New("message is empty")
	}
	if len(req.Message) > 1000 || strings.ContainsAny(req.Message, "\r\n\x00") || strings.HasPrefix(strings.TrimSpace(req.Message), ".") {
		return errors.New("message must be a single line of text, not a command")
	}
	if req.Prepare && (req.Clearance || len(req.Actions) != 1) {
		return errors.New("preparation requires exactly one stand/runway/pushback action")
	}
	if len(req.Actions) > 4 {
		return errors.New("too many operational actions")
	}
	for _, raw := range req.Actions {
		var action struct {
			Type     frontend.EventType `json:"type"`
			Callsign string             `json:"callsign"`
		}
		if err := json.Unmarshal(raw, &action); err != nil {
			return err
		}
		if action.Callsign != req.Callsign {
			return errors.New("action callsign differs from message")
		}
		switch action.Type {
		case frontend.UpdateStripData, frontend.RunwayClearance, frontend.RunwayConfirmation:
			if action.Type == frontend.UpdateStripData {
				var fields map[string]json.RawMessage
				_ = json.Unmarshal(raw, &fields)
				for key := range fields {
					if key != "type" && key != "callsign" && key != "stand" {
						return errors.New("stand preparation cannot change other fields")
					}
				}
				if fields["stand"] == nil {
					return errors.New("stand is required")
				}
			}
			if !req.Prepare || len(req.Actions) != 1 {
				return errors.New("stand/runway actions require a preparation request")
			}
		case frontend.Move:
			if req.Prepare {
				var move frontend.MoveEvent
				if err := json.Unmarshal(raw, &move); err != nil {
					return err
				}
				if move.Bay != shared.BAY_PUSH {
					return errors.New("only pushback moves support preparation")
				}
				var fields map[string]json.RawMessage
				_ = json.Unmarshal(raw, &fields)
				for key := range fields {
					if key != "type" && key != "callsign" && key != "bay" {
						return errors.New("pushback preparation cannot change other fields")
					}
				}
			}
		case frontend.ReleasePoint, frontend.CoordinationTransferRequestType, frontend.CoordinationCancelTransferRequest, frontend.AcknowledgeUnexpectedChange:
			if req.Prepare {
				return errors.New("unsupported preparation action")
			}
		default:
			return errors.New("unsupported frequency-message action")
		}
	}
	return nil
}

func handleSendFrequencyMessage(ctx context.Context, client *Client, message Message) error {
	var req frontend.SendFrequencyMessageEvent
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	var applied int32
	var inspectedStrip *models.Strip
	reject := func(err error) error {
		blocked := false
		if inspectedStrip != nil {
			if current, readErr := client.hub.server.GetStripRepository().GetByCallsign(ctx, client.session, req.Callsign); readErr == nil {
				blocked = current.IsValidationLocked()
			}
		}
		client.Enqueue(frontend.FrequencyMessageResultEvent{RequestID: req.RequestID, Callsign: req.Callsign, Error: err.Error(), AppliedActions: applied, ValidationBlocked: blocked})
		return nil
	}
	if err := validateFrequencyMessage(req); err != nil {
		return reject(err)
	}
	if _, loaded := client.frequencyRequests.LoadOrStore(req.RequestID, struct{}{}); loaded {
		return reject(errors.New("duplicate message request; not resent"))
	}
	router, ok := client.hub.server.GetEuroscopeHub().(frequencyMessageRouter)
	if !ok && !req.Prepare {
		return reject(errors.New("frequency messages are unavailable"))
	}
	var send func(euroscope.SendFrequencyMessageEvent) bool
	if ok {
		send = router.FrequencyMessageSender(client.session, client.GetCid(), client.position)
	}
	if send == nil && !req.Prepare {
		return reject(errors.New("connect an updated EuroScope plugin on your primary frequency"))
	}
	strip, err := client.hub.server.GetStripRepository().GetByCallsign(ctx, client.session, req.Callsign)
	if err != nil {
		return reject(err)
	}
	inspectedStrip = strip
	if !strip.IsTextOnly() {
		return reject(errors.New("aircraft is not text-only"))
	}
	// Action handlers enforce validation and permit supported corrective actions
	// and TSAT pushback retries; a blanket batch lock would prevent both.
	if req.Clearance {
		if _, err := buildTextClearance(strip); err != nil {
			return reject(err)
		}
	}
	for _, raw := range req.Actions {
		var action struct {
			Type frontend.EventType `json:"type"`
		}
		_ = json.Unmarshal(raw, &action)
		if req.Prepare && action.Type == frontend.Move {
			preparer, ok := client.hub.stripService.(pushbackPreparer)
			if !ok {
				return reject(errors.New("pushback preparation is unavailable"))
			}
			if err := preparer.PrepareFrontendPushback(ctx, client.session, req.Callsign, client.GetCid(), client.airport, client.position); err != nil {
				return reject(err)
			}
			continue // Validation only: no operational action was applied.
		}
		if err := client.hub.handlers.Handle(ctx, client, Message{Type: action.Type, Message: raw}); err != nil {
			return reject(err)
		}
		applied++
	}
	if req.Prepare {
		strip, err = client.hub.server.GetStripRepository().GetByCallsign(ctx, client.session, req.Callsign)
		if err != nil {
			return reject(err)
		}
		client.Enqueue(frontend.FrequencyMessageResultEvent{RequestID: req.RequestID, Callsign: req.Callsign, Prepared: true, AppliedActions: applied, Stand: stringPtrValue(strip.Stand), RunwayConfirmed: strip.RunwayConfirmed})
		return nil
	}
	if req.Clearance {
		strip, err = client.hub.server.GetStripRepository().GetByCallsign(ctx, client.session, req.Callsign)
		if err != nil {
			return reject(err)
		}
		if !strip.Cleared {
			return reject(errors.New("clearance was not accepted"))
		}
		req.Message, err = buildTextClearance(strip)
		if err != nil {
			return reject(err)
		}
	}
	if !send(euroscope.SendFrequencyMessageEvent{RequestId: req.RequestID, Callsign: req.Callsign, Message: strings.TrimSpace(req.Message), PrimaryFrequency: client.position, AppliedActions: applied}) {
		return reject(errors.New("EuroScope disconnected before command submission"))
	}
	return nil
}

func buildTextClearance(strip *models.Strip) (string, error) {
	if strip.Destination == "" || strip.ClearedAltitude == nil || *strip.ClearedAltitude <= 0 || stringPtrValue(strip.AssignedSquawk) == "" {
		return "", errors.New("destination, initial climb and assigned squawk are required")
	}
	sid := stringPtrValue(strip.Sid)
	straightAhead := strings.EqualFold(sid, "SA") || strings.HasPrefix(strings.ToUpper(sid), "SA ")
	if straightAhead {
		sid = ""
	}
	route := "Radar vectors"
	if strings.TrimSpace(stringPtrValue(strip.Route)) != "" {
		route = "Flight plan route"
	}
	if sid != "" {
		route = sid + " Departure"
	}
	// EuroScope's assigned altitude is stored in feet. Keep the unit explicit.
	text := fmt.Sprintf("Cleared to %s, via %s, Initial climb %d feet, Squawk %s.", strip.Destination, route, *strip.ClearedAltitude, stringPtrValue(strip.AssignedSquawk))
	if straightAhead {
		text += " After departure, Climb straight ahead."
	} else if sid == "" && strip.Heading != nil {
		if *strip.Heading > 0 {
			text += fmt.Sprintf(" After departure, Fly heading %03d.", *strip.Heading)
		}
	}
	return text, nil
}
