package alb

import (
	pkgAlb "FlightStrips/pkg/events/alb"
	"encoding/json"
	"log/slog"
)

func handleLogin(client *Client, msg []byte) {
	var event pkgAlb.LoginEvent
	if err := json.Unmarshal(msg, &event); err != nil {
		slog.Warn("ALB failed to parse login event", slog.Any("error", err))
		return
	}
	client.callsign = event.Callsign
	slog.Info("ALB client login", slog.String("callsign", client.callsign))
}

func handleQuery(client *Client, msg []byte) {
	var event pkgAlb.QueryEvent
	if err := json.Unmarshal(msg, &event); err != nil {
		slog.Warn("ALB failed to parse query event", slog.Any("error", err))
		return
	}

	if event.Subtype != "EventSlot" {
		slog.Debug("ALB unhandled query subtype", slog.String("subtype", event.Subtype))
		return
	}

	response := pkgAlb.ResponseEvent{
		Type:     pkgAlb.Response,
		Subtype:  "EventSlot",
		Callsign: event.Callsign,
		Dest:     event.Dest,
		Accepted: true,
		Plt:      event.Elt,
	}

	bytes, err := response.Marshal()
	if err != nil {
		slog.Error("ALB failed to marshal response event", slog.Any("error", err))
		return
	}

	select {
	case client.send <- bytes:
	default:
		slog.Warn("ALB send buffer full, dropping EventSlot response", slog.String("callsign", client.callsign))
	}
}

func handleA2A(_ *Client, hub *Hub, msg []byte) {
	var event pkgAlb.A2AEvent
	if err := json.Unmarshal(msg, &event); err != nil {
		slog.Warn("ALB failed to parse a2a event", slog.Any("error", err))
		return
	}
	hub.BroadcastA2A(event)
}
