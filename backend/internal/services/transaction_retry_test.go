package services

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetrySerializableOperationRetriesSerializationFailureOnce(t *testing.T) {
	attempts := 0
	err := retrySerializableOperation(func() error {
		attempts++
		if attempts == 1 {
			return &pgconn.PgError{Code: "40001"}
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 2, attempts)
}

func TestRetrySerializableOperationDoesNotRetryOtherFailures(t *testing.T) {
	attempts := 0
	want := errors.New("failed")
	err := retrySerializableOperation(func() error {
		attempts++
		return want
	})

	require.ErrorIs(t, err, want)
	assert.Equal(t, 1, attempts)
}
