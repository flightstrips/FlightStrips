package aman

import (
	"context"
	"fmt"
)

type sessionContextKey struct{}

// WithSession binds server-derived session identity to AMAN reads, commands,
// observations and publications. Airport alone never identifies a live queue.
func WithSession(ctx context.Context, id int32) context.Context {
	return context.WithValue(ctx, sessionContextKey{}, id)
}

func SessionID(ctx context.Context) int32 {
	id, _ := ctx.Value(sessionContextKey{}).(int32)
	return id
}

func SessionAirportKey(ctx context.Context, airport string) string {
	if id := SessionID(ctx); id > 0 {
		return fmt.Sprintf("%d/%s", id, airport)
	}
	return airport
}
