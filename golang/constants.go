package main

import "time"

// Shared constants for websocket timeouts
const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
)

// Shared byte slices for message formatting
var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)
