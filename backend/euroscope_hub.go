package main

import (
	"FlightStrips/data"
	"context"
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

func (h *EuroscopeHub) Register(client *EuroscopeClient) {
	h.register <- client
}

func (h *EuroscopeHub) Unregister(client *EuroscopeClient) {
	h.unregister <- client
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
	db := data.New(server.DBPool)
	params := data.SetControllerCidParams{
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

func (h *EuroscopeHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			h.OnRegister(client)
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.Close()
				h.server.FrontendHub.CidDisconnect(client.user.cid)
			}
			h.OnUnregister(client)
		case message := <-h.send:
			for client := range h.clients {
				if client.user.cid == message.cid && client.session == message.session {
					client.send <- message.message
					break
				}
			}
		}
	}
}
