package euroscope

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClientCanHandleMessage_ObserverAllowsRunwayValidation(t *testing.T) {
	client := &Client{observer: true}

	assert.NoError(t, client.CanHandleMessage("token"))
	assert.NoError(t, client.CanHandleMessage("login"))
	assert.NoError(t, client.CanHandleMessage("runway"))
	assert.Error(t, client.CanHandleMessage("sync"))
}

func TestClientCanHandleMessage_ActiveClientAllowsTelemetry(t *testing.T) {
	client := &Client{observer: false}

	assert.NoError(t, client.CanHandleMessage("sync"))
}
