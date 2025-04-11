package main

type EuroscopeHub struct {
	BaseHub[*EuroscopeClient]

	master *EuroscopeClient
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
		master: nil,
	}

	return hub
}

func (hub *EuroscopeHub) Register(client *EuroscopeClient) {
	hub.BaseHub.Register(client)

	// TODO select new master if relevant
}

func (hub *EuroscopeHub) Unregister(client *EuroscopeClient) {
	hub.BaseHub.Unregister(client)

	if hub.master != client {
		return
	}


	// No clients, no master can be assigned
	if len(hub.clients) == 0 {
		hub.master = nil
		return
	}

	// TODO select new master if any clients left
}
