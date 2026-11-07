package euroscope

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fixedGraceStripService struct{ aircraftAliveStripService }

func (*fixedGraceStripService) AircraftDisconnectDelay(context.Context, int32) (time.Duration, bool, error) {
	return 20 * time.Millisecond, true, nil
}

func TestWorkflowDisconnectGraceSurvivesRepeatedMissingObservations(t *testing.T) {
	service := &fixedGraceStripService{}
	hub := newAircraftDisconnectTestHub(service)
	stop, done := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() {
		close(stop)
		<-done
		hub.cancelAircraftDisconnect(42, "MISSING")
	})
	hub.scheduleAircraftDisconnect(42, "MISSING", time.Second)
	go func() {
		defer close(done)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				hub.scheduleAircraftDisconnect(42, "MISSING", time.Second)
			}
		}
	}()
	require.Eventually(t, func() bool { return service.deleteCalls.Load() > 0 }, time.Second, time.Millisecond, "continued missing observations must not reset the airport grace")
}
