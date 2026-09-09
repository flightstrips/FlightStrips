package app

import (
	"FlightStrips/internal/shared"
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
)

func TestDBOperationTracerCountsAutomaticMessageOperations(t *testing.T) {
	state := &shared.WebsocketMessageState{AutoCountDBOperations: true}
	ctx := shared.WithWebsocketMessageState(context.Background(), state)
	tracer := dbOperationTracer{}

	tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{})
	shared.AddDBOperations(ctx, 1)

	assert.Equal(t, 1, state.DBOperations, "manual accounting must not double-count traced queries")
}
