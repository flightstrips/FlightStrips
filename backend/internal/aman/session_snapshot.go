package aman

import "context"

// SessionSnapshot contains server-read session inputs reused within one fact
// event. The repository refreshes it after each successful guarded write and
// invalidates it on a conflict. It never authorizes aircraft tracking.
type SessionSnapshot struct {
	ID             int32
	Airport        string
	Live           bool
	ArrivalRunways []string
}

type sessionSnapshotKey struct{}

// WithSessionSnapshot is for trusted persistence adapters, after a validated
// read. The provider must be safe to call concurrently and return detached data.
func WithSessionSnapshot(ctx context.Context, read func() (SessionSnapshot, bool)) context.Context {
	return context.WithValue(ctx, sessionSnapshotKey{}, read)
}

func ReadSessionSnapshot(ctx context.Context) (SessionSnapshot, bool) {
	read, ok := ctx.Value(sessionSnapshotKey{}).(func() (SessionSnapshot, bool))
	if !ok {
		return SessionSnapshot{}, false
	}
	snapshot, ok := read()
	if !ok || snapshot.ID != SessionID(ctx) {
		return SessionSnapshot{}, false
	}
	return snapshot, true
}
