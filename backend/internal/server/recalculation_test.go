package server

import (
	"testing"
	"time"
)

func TestSessionRecalcLockManager_SerializesSameSession(t *testing.T) {
	var manager sessionRecalcLockManager

	releaseFirst := manager.lock(42)
	secondAcquired := make(chan struct{})

	go func() {
		releaseSecond := manager.lock(42)
		close(secondAcquired)
		releaseSecond()
	}()

	select {
	case <-secondAcquired:
		t.Fatal("second lock should wait while the first session lock is held")
	case <-time.After(50 * time.Millisecond):
	}

	releaseFirst()

	select {
	case <-secondAcquired:
	case <-time.After(time.Second):
		t.Fatal("second lock did not acquire after first lock released")
	}
}

func TestSessionRecalcLockManager_AllowsDifferentSessionsInParallel(t *testing.T) {
	var manager sessionRecalcLockManager

	releaseFirst := manager.lock(42)
	defer releaseFirst()

	secondAcquired := make(chan struct{})
	go func() {
		releaseSecond := manager.lock(43)
		close(secondAcquired)
		releaseSecond()
	}()

	select {
	case <-secondAcquired:
	case <-time.After(time.Second):
		t.Fatal("different session lock should not be blocked")
	}
}
