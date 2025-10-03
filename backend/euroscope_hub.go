package main

import (
	"FlightStrips/database"
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgtype"
)

type EuroscopeMessage struct {
	session int32
	cid     string
	message []byte
}

type EuroscopeHub struct {
	server *Server

	clients map[*EuroscopeClient]bool

	register chan *EuroscopeClient

	unregister chan *EuroscopeClient

	send chan EuroscopeMessage

	master *EuroscopeClient
}

func NewEuroscopeHub(server *Server) *EuroscopeHub {
	hub := &EuroscopeHub{
		register:   make(chan *EuroscopeClient),
		unregister: make(chan *EuroscopeClient),
		clients:    make(map[*EuroscopeClient]bool),
		send:       make(chan EuroscopeMessage),
		server:     server,
		master:     nil,
	}

	return hub
}

func (hub *EuroscopeHub) Register(client *EuroscopeClient) {
	hub.register <- client
}

func (hub *EuroscopeHub) Unregister(client *EuroscopeClient) {
	hub.unregister <- client
}

func (hub *EuroscopeHub) OnRegister(client *EuroscopeClient) {
	if hub.master == nil {
		hub.master = client
		SendEuroscopeEvent(client, EuroscopeSessionInfoEvent{Role: SessionInfoMaster})
		return
	}

	SendEuroscopeEvent(client, EuroscopeSessionInfoEvent{Role: SessionInfoSlave})
}

func (hub *EuroscopeHub) OnUnregister(client *EuroscopeClient) {
	server := hub.server
	db := database.New(server.DBPool)
	params := database.SetControllerCidParams{
		Cid:      pgtype.Text{Valid: false},
		Callsign: client.callsign,
		Session:  client.session,
	}
	count, err := db.SetControllerCid(context.Background(), params)

	if err != nil || count != 1 {
		log.Printf("Failed to remove CID for client %s with CID %s. Error: %s", client.callsign, client.user.cid, err)
	}

	if hub.master != client {
		return
	}

	// No clients, no master can be assigned
	if len(hub.clients) == 0 {
		hub.master = nil
		return
	}

	// TODO better master selection. For now just use the next available client
	for newMaster := range hub.clients {
		hub.master = newMaster
		SendEuroscopeEvent(newMaster, EuroscopeSessionInfoEvent{Role: SessionInfoMaster})
		break
	}
}

func (hub *EuroscopeHub) SendGenerateSquawk(cid string, callsign string) {
	event := EuroscopeGenerateSquawkEvent{
		Callsign: callsign,
	}
	sendEuroscopeEventInternal(hub, cid, event)
}

func (hub *EuroscopeHub) SendGroundState(cid string, callsign string, state string) {
	event := EuroscopeGroundStateEvent{
		Callsign:    callsign,
		GroundState: state,
	}

	sendEuroscopeEventInternal(hub, cid, event)
}

func (hub *EuroscopeHub) SendClearedFlag(cid string, callsign string, flag bool) {
	event := EuroscopeClearedFlagEvent{
		Callsign: callsign,
		Cleared:  flag,
	}

	sendEuroscopeEventInternal(hub, cid, event)
}

func (hub *EuroscopeHub) SendStand(cid string, callsign string, stand string) {
	event := EuroscopeStandEvent{
		Callsign: callsign,
		Stand:    stand,
	}

	sendEuroscopeEventInternal(hub, cid, event)
}

func (hub *EuroscopeHub) SendRoute(cid string, callsign string, route string) {
	event := EuroscopeRouteEvent{
		Callsign: callsign,
		Route:    route,
	}

	sendEuroscopeEventInternal(hub, cid, event)
}

func (hub *EuroscopeHub) SendRemarks(cid string, callsign string, remarks string) {
	event := EuroscopeRemarksEvent{
		Callsign: callsign,
		Remarks:  remarks,
	}

	sendEuroscopeEventInternal(hub, cid, event)
}

func (hub *EuroscopeHub) SendSid(cid string, callsign string, sid string) {
	event := EuroscopeSidEvent{
		Callsign: callsign,
		Sid:      sid,
	}

	sendEuroscopeEventInternal(hub, cid, event)
}

func (hub *EuroscopeHub) SendAssignedSquawk(cid string, callsign string, squawk string) {
	event := EuroscopeAssignedSquawkEvent{
		Callsign: callsign,
		Squawk:   squawk,
	}

	sendEuroscopeEventInternal(hub, cid, event)
}

func (hub *EuroscopeHub) SendRunway(cid string, callsign string, runway string) {
	event := EuroscopeAircraftRunwayEvent{
		Callsign: callsign,
		Runway:   runway,
	}

	sendEuroscopeEventInternal(hub, cid, event)
}

func (hub *EuroscopeHub) SendClearedAltitude(cid string, callsign string, altitude int) {
	event := EuroscopeClearedAltitudeEvent{
		Callsign: callsign,
		Altitude: altitude,
	}

	sendEuroscopeEventInternal(hub, cid, event)
}

func (hub *EuroscopeHub) SendHeading(cid string, callsign string, heading int) {
	event := EuroscopeHeadingEvent{
		Callsign: callsign,
		Heading:  heading,
	}

	sendEuroscopeEventInternal(hub, cid, event)
}

func sendEuroscopeEventInternal[T EuroscopeSendEvent](hub *EuroscopeHub, cid string, event T) {
	eventSent := false
	for client := range hub.clients {
		if client.user.cid == cid {
			SendEuroscopeEvent(client, event)
			eventSent = true
			break
		}
	}

	if !eventSent {
		fmt.Printf("Failed to find a client with %s when trying to send ES event.", cid)
	}
}

func (hub *EuroscopeHub) Run() {
	for {
		select {
		case client := <-hub.register:
			hub.clients[client] = true
			hub.OnRegister(client)
		case client := <-hub.unregister:
			if _, ok := hub.clients[client]; ok {
				delete(hub.clients, client)
				client.Close()
				hub.server.FrontendHub.CidDisconnect(client.user.cid)
			}
			hub.OnUnregister(client)
		case message := <-hub.send:
			for client := range hub.clients {
				if client.user.cid == message.cid && client.session == message.session {
					client.send <- message.message
					break
				}
			}
		}
	}
}
