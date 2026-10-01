package cluster

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// FlushSubscription waits for the broker to register subscriptions. Worker
// contexts can be unbounded; each flush has its own deadline, and temporary
// transport loss must not permanently stop a recovering runtime.
func FlushSubscription(ctx context.Context, nc *nats.Conn) error {
	for ctx.Err() == nil {
		attempt, cancel := context.WithTimeout(ctx, time.Second)
		err := nc.FlushWithContext(attempt)
		cancel()
		if err == nil {
			return nil
		}
		if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, nats.ErrTimeout) &&
			!errors.Is(err, nats.ErrConnectionReconnecting) {
			return err
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
	return ctx.Err()
}

// SubscribeJoined stops new callback admissions and joins every admitted
// callback before returning from close. Unsubscribe alone does not join an
// in-flight NATS callback.
func SubscribeJoined(nc *nats.Conn, subject string, callback nats.MsgHandler) (*nats.Subscription, func(), error) {
	var mu sync.Mutex
	var jobs sync.WaitGroup
	stopping := false
	sub, err := nc.Subscribe(subject, func(msg *nats.Msg) {
		mu.Lock()
		if stopping {
			mu.Unlock()
			return
		}
		jobs.Add(1)
		mu.Unlock()
		defer jobs.Done()
		callback(msg)
	})
	if err != nil {
		return nil, nil, err
	}
	var once sync.Once
	return sub, func() {
		once.Do(func() {
			mu.Lock()
			stopping = true
			mu.Unlock()
			_ = sub.Unsubscribe()
			jobs.Wait()
		})
	}, nil
}
