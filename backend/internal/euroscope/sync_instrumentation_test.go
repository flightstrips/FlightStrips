package euroscope

import (
	"testing"
	"time"

	"FlightStrips/internal/metrics"
	"FlightStrips/internal/shared"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncFollowUpWorkCountsScheduledWork(t *testing.T) {
	state := &shared.SyncState{
		RouteRecalcStrips:   map[string]struct{}{"SAS123": {}, "DLH456": {}},
		BayUpdates:          map[string]string{"SAS123": "TAXI"},
		PdcValidationStrips: map[string]struct{}{"SAS123": {}},
		StripUpdates:        map[string]struct{}{"SAS123": {}, "DLH456": {}, "KLM789": {}},
		SquawkValidation:    true,
		LandingValidation:   false,
		CdmRecalculation:    true,
	}

	work := syncFollowUpWork(state)

	assert.Equal(t, 2, work[metrics.SyncWorkRouteRecalc])
	assert.Equal(t, 1, work[metrics.SyncWorkBayUpdate])
	assert.Equal(t, 1, work[metrics.SyncWorkPdcValidation])
	assert.Equal(t, 3, work[metrics.SyncWorkStripUpdate])
	assert.Equal(t, 1, work[metrics.SyncWorkSquawkValidation])
	assert.Equal(t, 0, work[metrics.SyncWorkLandingValidation])
	assert.Equal(t, 1, work[metrics.SyncWorkCdmRecalculation])
}

// A sync that changed nothing must still report the work it scheduled, because a
// recalculation triggered by an unchanged heartbeat is exactly the wasted work
// this instrumentation exists to expose.
func TestSyncFollowUpWorkReportsRecalculationWithoutChanges(t *testing.T) {
	state := &shared.SyncState{CdmRecalculation: true}

	work := syncFollowUpWork(state)

	assert.Equal(t, 0, state.ChangedStrips)
	assert.Equal(t, 0, state.ChangedControllers)
	assert.Equal(t, 1, work[metrics.SyncWorkCdmRecalculation])
}

func TestSyncFollowUpWorkHandlesMissingState(t *testing.T) {
	assert.Nil(t, syncFollowUpWork(nil))
}

func TestSyncFollowUpWorkIsEmptyForAnIdleSync(t *testing.T) {
	work := syncFollowUpWork(&shared.SyncState{})

	for kind, count := range work {
		assert.Zerof(t, count, "expected no %s work for an idle sync", kind)
	}
}

func TestSyncPhaseTimingsRecordEveryObservedPhase(t *testing.T) {
	timings := &syncPhaseTimings{}

	started := timings.begin()
	timings.observe(metrics.SyncPhaseStrips, started)
	timings.observe(metrics.SyncPhaseFinalize, timings.begin())

	require.Len(t, timings.entries, 2)
	assert.Equal(t, metrics.SyncPhaseStrips, timings.entries[0].phase)
	assert.Equal(t, metrics.SyncPhaseFinalize, timings.entries[1].phase)
	for _, entry := range timings.entries {
		assert.GreaterOrEqual(t, entry.duration, time.Duration(0))
	}
}

func TestBoolToCount(t *testing.T) {
	assert.Equal(t, 1, boolToCount(true))
	assert.Equal(t, 0, boolToCount(false))
}
