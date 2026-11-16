package frontend

import (
	"context"
	"strings"
	"time"

	"FlightStrips/internal/shared"
	es "FlightStrips/pkg/events/euroscope"
	fe "FlightStrips/pkg/events/frontend"
)

// Only the authenticated socket's captured own binding can route call controls.
// The browser supplies neither a CID nor a destination host/plugin.
func handleVacsRequest(ctx context.Context, client *Client, message Message) error {
	var req fe.VacsRequestEvent
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	reject := func(reason string) error {
		client.Enqueue(fe.VacsResultEvent{RequestID: req.RequestID, Error: reason})
		return nil
	}
	if len(req.RequestID) == 0 || len(req.RequestID) > 64 || strings.TrimSpace(req.RequestID) != req.RequestID || len(req.CallID) > 256 || len(req.ClientID) > 256 {
		return reject("Invalid VACS request")
	}
	switch req.Action {
	case "status":
		if req.CallID != "" || req.ClientID != "" {
			return reject("Invalid VACS status request")
		}
	case "dial":
		if req.ClientID == "" || req.CallID != "" {
			return reject("Select a VACS controller")
		}
	case "answer", "reject", "cancel", "end":
		if req.CallID == "" || req.ClientID != "" {
			return reject("Select the current VACS call")
		}
	default:
		return reject("Unsupported VACS action")
	}
	if req.Action != "status" {
		client.vacsRequestMu.Lock()
		now := time.Now()
		for id, expiry := range client.vacsRequests {
			if now.After(expiry) {
				delete(client.vacsRequests, id)
			}
		}
		_, duplicate := client.vacsRequests[req.RequestID]
		if duplicate || len(client.vacsRequests) >= 256 {
			client.vacsRequestMu.Unlock()
			return reject("Duplicate or excessive VACS actions; no action was resent")
		}
		if client.vacsRequests == nil {
			client.vacsRequests = map[string]time.Time{}
		}
		client.vacsRequests[req.RequestID] = now.Add(5 * time.Minute)
		client.vacsRequestMu.Unlock()
	}
	binding, ok := shared.HumanCommand(ctx)
	if !ok {
		return reject("Connect your own EuroScope plugin to use VACS")
	}
	relay, ok := binding.(interface {
		SendVacsRequest(es.VacsRequestEvent, func(fe.VacsResultEvent)) error
	})
	if !ok {
		return reject("Update the EuroScope plugin to use VACS controls")
	}
	identity := client.messageIdentity(ctx)
	err := relay.SendVacsRequest(es.VacsRequestEvent{RequestId: req.RequestID, Action: req.Action, CallId: req.CallID, ClientId: req.ClientID}, func(result fe.VacsResultEvent) {
		if client.identitySnapshot() == identity {
			client.Enqueue(result)
		}
	})
	if err != nil {
		return reject(err.Error())
	}
	return nil
}
