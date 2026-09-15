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

func TestDBOperationTracerCountsNestedSyncQueriesWithoutManualDoubleCounting(t *testing.T) {
	ctx, counter := shared.WithDBOperationCounter(context.Background())
	tracer := dbOperationTracer{}
	tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{}) // initial snapshot
	syncState := &shared.SyncState{DBOperations: 3}
	ctx = shared.WithSyncState(ctx, syncState)
	tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{})
	shared.AddDBOperations(ctx, 1)                              // legacy repository accounting
	tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{}) // nested AMAN query
	assert.Equal(t, 3, counter.Finish())
	assert.Equal(t, 4, syncState.DBOperations, "manual fallback remains separate from the authoritative count")
}
