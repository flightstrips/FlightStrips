package cluster

import (
	"sync"

	"github.com/nats-io/nats.go"
)

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
