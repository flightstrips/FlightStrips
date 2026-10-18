package cdm

import (
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestViffPushRetry_StopsAfterFiveMinutesUntilProposalChanges(t *testing.T) {
	var tracker viffPushRetryTracker
	now := time.Date(2026, 10, 18, 14, 40, 16, 0, time.UTC)
	start := now
	state := viffPushState{Params: SetCdmDataParams{Callsign: "SAS1635", Tsat: "151000"}}
	attempt := tracker.begin(1, "SAS1635", state, now)
	for i := 0; ; i++ {
		require.Less(t, i, 20, "retry window must be bounded")
		result := tracker.complete(attempt, errViffUpdatePending, now)
		require.Equal(t, uint(i+1), result.failures)
		require.Equal(t, now.Sub(start), result.failingFor)
		if now.Sub(start) >= 5*time.Minute {
			require.Equal(t, 5*time.Minute, now.Sub(start), "last retry is capped at the deadline")
			require.Equal(t, slog.LevelWarn, result.level)
			require.Nil(t, result.next, "stop retrying after five minutes")
			break
		}
		require.Equal(t, slog.LevelDebug, result.level)
		require.NotNil(t, result.next)
		delay := min(30*time.Second, 5*time.Minute-now.Sub(start))
		if i == 0 {
			delay = 5 * time.Second
		} else if i == 1 {
			delay = 15 * time.Second
		}
		require.Equal(t, delay, result.next.notBefore.Sub(now))
		require.Nil(t, tracker.begin(1, "SAS1635", state, now), "retry wait retains sole ownership")
		now = result.next.notBefore
		attempt = tracker.prepare(result.next, now)
	}
	require.Nil(t, tracker.begin(1, "SAS1635", state, now.Add(24*time.Hour)), "sync must not restart an exhausted update")
	state.Params.Tsat = "152000"
	attempt = tracker.begin(1, "SAS1635", state, now)
	require.NotNil(t, attempt, "changed proposal starts a new retry window")
	result := tracker.complete(attempt, errViffUpdatePending, now)
	require.Equal(t, slog.LevelDebug, result.level)
	require.Zero(t, result.failingFor)
	require.Equal(t, uint(1), result.failures)
	attempt = tracker.prepare(result.next, result.next.notBefore)
	require.Nil(t, tracker.complete(attempt, nil, result.next.notBefore).next)
	require.Nil(t, tracker.begin(1, "SAS1635", state, now.Add(time.Hour)), "acknowledged state is deduplicated")

	state.Params.Tsat = "153000"
	attempt = tracker.begin(1, "SAS1635", state, now.Add(time.Hour))
	result = tracker.complete(attempt, errViffUpdatePending, now.Add(time.Hour))
	require.Equal(t, slog.LevelDebug, result.level, "success resets the failure period")
	require.Zero(t, result.failingFor)
}

func TestViffPushRetry_ProposalChangesDoNotExtendActiveRetryWindow(t *testing.T) {
	var tracker viffPushRetryTracker
	now := time.Now()
	start := now
	state := viffPushState{Params: SetCdmDataParams{Tsat: "120000"}}
	attempt := tracker.begin(1, "SAS1", state, now)
	result := tracker.complete(attempt, errViffUpdatePending, now)
	now = now.Add(5*time.Minute - time.Nanosecond)
	changed := state
	changed.Params.Tsat = "130000"
	require.Nil(t, tracker.begin(1, "SAS1", changed, now))
	attempt = tracker.prepare(result.next, now)
	require.Equal(t, changed, attempt.state)
	result = tracker.complete(attempt, errViffUpdatePending, now)
	require.Equal(t, slog.LevelDebug, result.level)
	require.Equal(t, now.Sub(start), result.failingFor)
	require.Equal(t, start.Add(5*time.Minute), result.next.notBefore, "proposal change does not extend the deadline")
	now = result.next.notBefore
	attempt = tracker.prepare(result.next, now)
	result = tracker.complete(attempt, errViffUpdatePending, now)
	require.Equal(t, slog.LevelWarn, result.level)
	require.Nil(t, result.next)
	require.Nil(t, tracker.begin(1, "SAS1", changed, now.Add(time.Hour)))
}

func TestViffPushRetry_AcknowledgementAtDeadlineSucceeds(t *testing.T) {
	var tracker viffPushRetryTracker
	start := time.Now()
	state := viffPushState{}
	attempt := tracker.begin(1, "SAS1", state, start)
	retry := tracker.complete(attempt, errViffUpdatePending, start).next
	attempt = tracker.prepare(retry, start.Add(5*time.Minute))
	result := tracker.complete(attempt, nil, start.Add(5*time.Minute))
	require.Nil(t, result.next)
	require.Equal(t, slog.LevelDebug, result.level)
	require.Nil(t, tracker.entries[attempt.key].exhausted)
	require.Equal(t, state, *tracker.entries[attempt.key].confirmed)
}

func TestViffPushRetry_ActualAPIFailureWarnsImmediately(t *testing.T) {
	var tracker viffPushRetryTracker
	now := time.Now()
	attempt := tracker.begin(1, "SAS1", viffPushState{}, now)
	result := tracker.complete(attempt, errors.New("HTTP 401"), now)
	require.Equal(t, slog.LevelWarn, result.level)
	require.NotNil(t, result.next)
}

func TestViffPushRetry_RefreshUsesActualPayloadWithoutExtendingDeadline(t *testing.T) {
	for _, accepted := range []bool{true, false} {
		t.Run(map[bool]string{true: "accepted", false: "rejected"}[accepted], func(t *testing.T) {
			var tracker viffPushRetryTracker
			start := time.Now()
			old := viffPushState{Params: SetCdmDataParams{Tobt: "120000", Tsat: "120000"}}
			latest := old
			latest.Params.Tobt = "120200"
			first := tracker.begin(1, "SAS1", old, start)
			retry := tracker.complete(first, errViffUpdatePending, start).next
			now := start.Add(5*time.Minute - time.Second)
			attempt := tracker.prepare(retry, now)
			attempt = tracker.refresh(attempt, latest)
			require.NotNil(t, attempt)
			if accepted {
				require.Nil(t, tracker.complete(attempt, nil, now).next)
				require.Nil(t, tracker.begin(1, "SAS1", latest, now))
				require.Equal(t, latest, *tracker.entries[attempt.key].confirmed)
				return
			}
			result := tracker.complete(attempt, errViffUpdatePending, now)
			require.Equal(t, now.Sub(start), result.failingFor)
			require.Equal(t, start.Add(5*time.Minute), result.next.notBefore)
			attempt = tracker.prepare(result.next, result.next.notBefore)
			result = tracker.complete(attempt, errViffUpdatePending, result.next.notBefore)
			require.Nil(t, result.next, "refreshing current values must not reset the five-minute budget")
			require.Equal(t, slog.LevelWarn, result.level)
		})
	}
}

func TestViffPushRetry_InFlightChangedProposalAndStaleCompletion(t *testing.T) {
	for _, firstErr := range []error{nil, errViffUpdatePending} {
		t.Run(map[bool]string{true: "success", false: "rejection"}[firstErr == nil], func(t *testing.T) {
			var tracker viffPushRetryTracker
			now := time.Now()
			first := viffPushState{Params: SetCdmDataParams{Tsat: "120000"}}
			second := viffPushState{Params: SetCdmDataParams{Tsat: "130000"}}
			latest := viffPushState{Params: SetCdmDataParams{Tsat: "140000"}}
			a := tracker.begin(1, "SAS1", first, now)
			require.Nil(t, tracker.begin(1, "SAS1", second, now))
			require.Nil(t, tracker.begin(1, "SAS1", latest, now))
			result := tracker.complete(a, firstErr, now)
			require.NotNil(t, result.next)
			require.Equal(t, latest, result.next.state)
			require.Equal(t, now, result.next.notBefore, "changed queued proposal is sent immediately")
			require.Nil(t, tracker.complete(a, nil, now).next, "stale completion cannot dispatch work")
			b := tracker.prepare(result.next, now)
			require.Nil(t, tracker.complete(b, nil, now).next)
			require.Nil(t, tracker.begin(1, "SAS1", latest, now))
			require.NotNil(t, tracker.begin(1, "SAS1", first, now), "latest acknowledgement replaces older state")
		})
	}
}

func TestViffPushRetry_RemovalBlocksWorkAndIgnoresCompletion(t *testing.T) {
	var tracker viffPushRetryTracker
	now := time.Now()
	state := viffPushState{Params: SetCdmDataParams{Tsat: "120000"}}
	attempt := tracker.begin(1, "SAS1", state, now)
	retry := tracker.complete(attempt, errViffUpdatePending, now).next
	tracker.removeSession(1)
	require.Nil(t, tracker.prepare(retry, now))
	require.Nil(t, tracker.complete(retry, nil, now).next)
	require.Empty(t, tracker.entries)
	require.Nil(t, tracker.begin(1, "SAS1", state, now))
	require.NotNil(t, tracker.begin(2, "SAS1", state, now))
}

func TestViffPushRetry_DiscardInvalidatesOldWorker(t *testing.T) {
	var tracker viffPushRetryTracker
	now := time.Now()
	state := viffPushState{}
	first := tracker.begin(1, "SAS1", state, now)
	tracker.discard(first)
	second := tracker.begin(1, "SAS1", state, now)
	require.NotNil(t, second)
	require.Nil(t, tracker.prepare(first, now))
	require.Nil(t, tracker.complete(first, nil, now).next)
	tracker.discard(first)
	require.Same(t, second, tracker.entries[first.key].inFlight)
}

func TestViffPushRetry_AuthoritativeExportInvalidatesStaleWorkers(t *testing.T) {
	var tracker viffPushRetryTracker
	now := time.Now()
	old := viffPushState{Params: SetCdmDataParams{Tsat: "120000"}}
	ready := viffPushState{Params: SetCdmDataParams{Tsat: "130000"}}
	attempt := tracker.begin(1, "SAS1", old, now)
	retry := tracker.complete(attempt, errViffUpdatePending, now).next
	tracker.begin(1, "SAS1", old, now)
	tracker.confirmExport(1, "SAS1", &ready)
	require.Nil(t, tracker.prepare(retry, now))
	require.Nil(t, tracker.complete(retry, nil, now).next)
	tracker.discard(retry)
	require.Nil(t, tracker.begin(1, "SAS1", ready, now), "READY's acknowledged state must not be resent")
	require.Equal(t, ready, *tracker.entries[retry.key].confirmed)
	require.NotNil(t, tracker.begin(1, "SAS1", old, now), "a later changed proposal is still allowed")
	tracker.removeSession(1)
	tracker.confirmExport(1, "SAS1", &ready)
	require.Empty(t, tracker.entries, "late READY completion must not recreate a removed session")
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
	next := tracker.complete(<-attempts, errViffUpdatePending, now).next
	require.NotNil(t, next)
	require.Nil(t, tracker.begin(1, "SAS1", next.state, now))
}

func TestViffPushRetry_PendingLatestRevertsToActiveProposal(t *testing.T) {
	var tracker viffPushRetryTracker
	now := time.Now()
	first := viffPushState{Params: SetCdmDataParams{Tsat: "120000"}}
	second := viffPushState{Params: SetCdmDataParams{Tsat: "130000"}}
	a := tracker.begin(1, "SAS1", first, now)
	tracker.begin(1, "SAS1", second, now)
	tracker.begin(1, "SAS1", first, now)
	require.Nil(t, tracker.complete(a, nil, now).next, "superseded pending proposal must not be sent")
}
