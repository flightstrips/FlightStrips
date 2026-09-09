package app

import (
	"FlightStrips/internal/shared"
	"context"

	"github.com/jackc/pgx/v5"
)

// dbOperationTracer counts actual Query, QueryRow and Exec calls for handlers
// that opt into database-boundary accounting.
type dbOperationTracer struct{}

func (dbOperationTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	shared.TraceDBOperation(ctx)
	return ctx
}

func (dbOperationTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
