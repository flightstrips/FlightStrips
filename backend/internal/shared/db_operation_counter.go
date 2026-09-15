package shared

import (
	"context"
	"sync"
)

type dbOperationCounterKey struct{}

// DBOperationCounter measures queries during a handler's lifetime. Detached
// follow-up work may inherit its context, so updates and completion are safe
// across goroutines and queries after Finish are ignored.
type DBOperationCounter struct {
	mu         sync.Mutex
	operations int
	finished   bool
}

func WithDBOperationCounter(ctx context.Context) (context.Context, *DBOperationCounter) {
	counter := &DBOperationCounter{}
	return context.WithValue(ctx, dbOperationCounterKey{}, counter), counter
}

func (c *DBOperationCounter) add() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.finished {
		c.operations++
	}
}

func (c *DBOperationCounter) Finish() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.finished = true
	return c.operations
}
