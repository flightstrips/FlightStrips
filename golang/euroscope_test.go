package main

import (
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEuroscopeLoginEvent(t *testing.T) {

	websocketHandlerServer := Server{
		nil,
		nil,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		websocketHandlerServer.euroscopeEvents(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[len("http"):]

	// Connect to the WebSocket server
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	assert.NoError(t, err, "Failed to connect to WebSocket server")
	defer conn.Close()

}
