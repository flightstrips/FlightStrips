package services

import (
	"FlightStrips/internal/shared"
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetrySerializableOperationRetriesSerializationFailureOnce(t *testing.T) {
	attempts := 0
	state := &shared.WebsocketMessageState{}
	err := retrySerializableOperation(shared.WithWebsocketMessageState(context.Background(), state), func() error {
		attempts++
		if attempts == 1 {
			return &pgconn.PgError{Code: "40001"}
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 2, attempts)
	assert.Equal(t, 1, state.DBRetries["serialization_conflict"])
}

func TestRetrySerializableOperationDoesNotRetryOtherFailures(t *testing.T) {
	attempts := 0
	want := errors.New("failed")
	err := retrySerializableOperation(context.Background(), func() error {
		attempts++
		return want
	})

	require.ErrorIs(t, err, want)
	assert.Equal(t, 1, attempts)
}
