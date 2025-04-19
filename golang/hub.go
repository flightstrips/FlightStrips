package main

type Hub[T WebsocketClient] interface {
	Register(client T)
	Unregister(client T)
}

