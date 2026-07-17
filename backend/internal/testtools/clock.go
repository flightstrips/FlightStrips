package testtools

import (
	"sync"
	"time"
)

type Clock struct {
	mu  sync.RWMutex
	now time.Time
}

func NewClock() *Clock {
	return &Clock{now: time.Now().UTC()}
}

func (c *Clock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.now
}

func (c *Clock) Advance(duration time.Duration) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(duration)
	return c.now
}

func (c *Clock) Reset() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = time.Now().UTC()
	return c.now
}
