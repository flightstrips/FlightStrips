package main

import (
	"FlightStrips/data"
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgtype"
)

type EuroscopeHub struct {
	BaseHub[*EuroscopeClient]

	Master *EuroscopeClient
}

func NewEuroscopeHub(server *Server) *EuroscopeHub {
	hub := &EuroscopeHub{
		BaseHub: BaseHub[*EuroscopeClient]{
			broadcast:  make(chan []byte),
			register:   make(chan *EuroscopeClient),
			unregister: make(chan *EuroscopeClient),
			clients:    make(map[*EuroscopeClient]bool),
			server:     server,
		},
		Master: nil,
	}

	return hub
}

func (hub *EuroscopeHub) OnRegister(client *EuroscopeClient) {
	if hub.Master == nil {
		hub.Master = client
		SendEuroscopeEvent(client, EuroscopeSessionInfoEvent{Role: SessionInfoMaster})
		return
	}

	SendEuroscopeEvent(client, EuroscopeSessionInfoEvent{Role: SessionInfoSlave})
}

func (hub *EuroscopeHub) OnUnregister(client *EuroscopeClient) {
	server := hub.server
	db := data.New(server.DBPool)
	params := data.SetControllerCidParams {
		Cid: pgtype.Text{ Valid: false },
		Callsign: client.callsign,
		Session: client.session,
	}
	count, err := db.SetControllerCid(context.Background(), params)

	if err != nil || count != 1 {
		log.Printf("Failed to remove CID for client %s with CID %s. Error: %s", client.callsign, client.user.cid, err)
	}

	if hub.Master != client {
		return
	}

	// No clients, no master can be assigned
	if len(hub.clients) == 0 {
		hub.Master = nil
		return
	}

	// TODO better master selection. For now just use the next available client
	for newMaster := range hub.clients {
		hub.Master = newMaster
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
			}
			h.OnUnregister(client)
		case message := <-h.broadcast:
			for client := range h.clients {
				client.Send(message)
			}
		}
	}
}
