package cdm

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestViffPushRetry_FiveRetriesThenStopsUntilProposalChanges(t *testing.T) {
	var tracker viffPushRetryTracker
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	state := viffPushState{Params: SetCdmDataParams{Callsign: "SAS1", Tsat: "120000"}}
	for i, delay := range []time.Duration{time.Second, time.Second, 3 * time.Second, 5 * time.Second, 10 * time.Second} {
		attempt := tracker.begin(1, "SAS1", state, now)
		require.NotNil(t, attempt, "attempt %d", i)
		tracker.complete(attempt, false, now)
		require.Nil(t, tracker.begin(1, "SAS1", state, now.Add(delay-time.Nanosecond)))
		now = now.Add(delay)
	}
	attempt := tracker.begin(1, "SAS1", state, now)
	require.NotNil(t, attempt)
	tracker.complete(attempt, false, now)
	require.Nil(t, tracker.begin(1, "SAS1", state, now.Add(24*time.Hour)), "exhausted proposal never retries again")
	state.Params.Tsat = "130000"
	attempt = tracker.begin(1, "SAS1", state, now)
	require.NotNil(t, attempt, "changed proposal starts a fresh retry budget")
	tracker.complete(attempt, true, now)
	require.Nil(t, tracker.begin(1, "SAS1", state, now.Add(time.Hour)), "confirmed state is deduplicated")
	state.Params.Tsat = "140000"
	attempt = tracker.begin(1, "SAS1", state, now)
	require.NotNil(t, attempt, "success clears cooldown")
	tracker.complete(attempt, false, now)
	require.Nil(t, tracker.begin(1, "SAS1", state, now.Add(time.Second-time.Nanosecond)))
	require.NotNil(t, tracker.begin(1, "SAS1", state, now.Add(time.Second)), "new proposal uses the first retry delay")
}

func TestViffPushRetry_ChangedProposalClearsCooldown(t *testing.T) {
	var tracker viffPushRetryTracker
	now := time.Now()
	state := viffPushState{Params: SetCdmDataParams{Tsat: "120000"}}
	attempt := tracker.begin(1, "SAS1", state, now)
	tracker.complete(attempt, false, now)
	require.Nil(t, tracker.begin(1, "SAS1", state, now))
	state.Params.Tsat = "130000"
	attempt = tracker.begin(1, "SAS1", state, now)
	require.NotNil(t, attempt, "fresh data is exported immediately")
	tracker.complete(attempt, false, now)
	require.Nil(t, tracker.begin(1, "SAS1", state, now.Add(time.Second-time.Nanosecond)))
	require.NotNil(t, tracker.begin(1, "SAS1", state, now.Add(time.Second)))
}

func TestViffPushRetry_InFlightChangedProposalAndStaleCompletion(t *testing.T) {
	var tracker viffPushRetryTracker
	now := time.Now()
	first := viffPushState{Params: SetCdmDataParams{Tsat: "120000"}}
	second := viffPushState{Params: SetCdmDataParams{Tsat: "130000"}}
	a := tracker.begin(1, "SAS1", first, now)
	require.NotNil(t, a)
	require.Nil(t, tracker.begin(1, "SAS1", second, now), "changed proposal cannot race active request")
	b := tracker.complete(a, true, now)
	require.NotNil(t, b)
	require.Equal(t, second, b.state)
	tracker.complete(a, false, now) // delayed duplicate completion cannot overwrite b
	tracker.complete(b, true, now)
	require.Nil(t, tracker.begin(1, "SAS1", second, now))
	require.NotNil(t, tracker.begin(1, "SAS1", first, now), "latest acknowledged proposal replaces older state")
}

func TestViffPushRetry_RemovalBlocksWorkAndIgnoresCompletion(t *testing.T) {
	var tracker viffPushRetryTracker
	now := time.Now()
	state := viffPushState{Params: SetCdmDataParams{Tsat: "120000"}}
	attempt := tracker.begin(1, "SAS1", state, now)
	require.NotNil(t, attempt)
	changed := state
	changed.Params.Tsat = "130000"
	require.Nil(t, tracker.begin(1, "SAS1", changed, now))
	tracker.removeSession(1)
	require.Nil(t, tracker.complete(attempt, true, now), "removal discards queued proposals")
	require.Empty(t, tracker.entries)
	require.Nil(t, tracker.begin(1, "SAS1", state, now), "removed session cannot recreate retry state")
	require.NotNil(t, tracker.begin(2, "SAS1", state, now), "another session is independent")
}

func TestViffPushRetry_ConcurrentProposalsStartOneRequest(t *testing.T) {
	var tracker viffPushRetryTracker
	var wg sync.WaitGroup
	attempts := make(chan *viffPushAttempt, 32)
	now := time.Now()
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			state := viffPushState{Params: SetCdmDataParams{Tsat: now.Add(time.Duration(i) * time.Minute).Format("150405")}}
			if attempt := tracker.begin(1, "SAS1", state, now); attempt != nil {
				attempts <- attempt
			}
		}(i)
	}
	wg.Wait()
	require.Len(t, attempts, 1)
	attempt := <-attempts
	next := tracker.complete(attempt, false, now)
	if next != nil {
		require.Nil(t, tracker.begin(1, "SAS1", next.state, now), "pending proposal is reserved as the sole active request")
	}
}

func TestViffPushRetry_FailedExportDispatchesLatestPendingProposal(t *testing.T) {
	var tracker viffPushRetryTracker
	now := time.Now()
	first := viffPushState{Params: SetCdmDataParams{Tsat: "120000"}}
	second := viffPushState{Params: SetCdmDataParams{Tsat: "130000"}}
	latest := viffPushState{Params: SetCdmDataParams{Tsat: "140000"}}
	a := tracker.begin(1, "SAS1", first, now)
	require.Nil(t, tracker.begin(1, "SAS1", second, now))
	require.Nil(t, tracker.begin(1, "SAS1", latest, now))
	b := tracker.complete(a, false, now)
	require.NotNil(t, b, "pending proposal does not depend on another sync or recalculation")
	require.Equal(t, latest, b.state, "only the latest proposal is dispatched")
	require.Nil(t, tracker.complete(a, true, now), "stale completion cannot dispatch duplicate work")
	require.Nil(t, tracker.complete(b, false, now))
	require.Nil(t, tracker.begin(1, "SAS1", latest, now.Add(time.Second-time.Nanosecond)))
	require.NotNil(t, tracker.begin(1, "SAS1", latest, now.Add(time.Second)), "pending proposal has a fresh retry budget")
}

func TestViffPushRetry_PendingLatestRevertsToActiveProposal(t *testing.T) {
	var tracker viffPushRetryTracker
	now := time.Now()
	first := viffPushState{Params: SetCdmDataParams{Tsat: "120000"}}
	second := viffPushState{Params: SetCdmDataParams{Tsat: "130000"}}
	a := tracker.begin(1, "SAS1", first, now)
	tracker.begin(1, "SAS1", second, now)
	tracker.begin(1, "SAS1", first, now)
	require.Nil(t, tracker.complete(a, true, now), "superseded pending proposal must not be sent")
}
