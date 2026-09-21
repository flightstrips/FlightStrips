package constants

import "time"

// Shared constants for websocket timeouts
const (
	// Time allowed to write a message to the peer.
	WriteWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	PongWait = 60 * time.Second

	// Send pings early enough that a slow outbound data write cannot consume the
	// entire pong margin. Must remain less than PongWait.
	PingPeriod = PongWait / 2

	// How often to check whether the connected client's token is still valid.
	TokenCheckPeriod = 60 * time.Second
)

// Shared byte slices for message formatting
var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)
