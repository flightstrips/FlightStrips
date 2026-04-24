package frontend

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClientCanHandleMessage_ReadOnlyAllowsTokenOnly(t *testing.T) {
	client := &Client{readOnly: true}

	assert.NoError(t, client.CanHandleMessage("token"))
	assert.Error(t, client.CanHandleMessage("move"))
}

func TestClientCanHandleMessage_WritableAllowsMutations(t *testing.T) {
	client := &Client{readOnly: false}

	assert.NoError(t, client.CanHandleMessage("move"))
}
