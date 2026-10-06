package cdm

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestRecalcDebouncer_CoalescesBurstIntoSingleRun(t *testing.T) {
	t.Parallel()

	debouncer := newRecalcDebouncer(25 * time.Millisecond)
	var runs atomic.Int32
	done := make(chan struct{})

	run := func() {
		if runs.Add(1) == 1 {
			close(done)
		}
	}

	debouncer.Schedule("EKCH", run)
	time.Sleep(5 * time.Millisecond)
	debouncer.Schedule("EKCH", run)
	time.Sleep(5 * time.Millisecond)
	debouncer.Schedule("EKCH", run)

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for debounced run")
	}

	time.Sleep(60 * time.Millisecond)
	if got := runs.Load(); got != 1 {
		t.Fatalf("expected exactly 1 run, got %d", got)
	}
}

func TestRecalcDebouncer_RerunsWhenTriggeredDuringActiveRun(t *testing.T) {
	t.Parallel()

	debouncer := newRecalcDebouncer(20 * time.Millisecond)
	var runs atomic.Int32
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondDone := make(chan struct{})

	run := func() {
		if runs.Add(1) == 1 {
			close(firstStarted)
			<-releaseFirst
			return
		}
		close(secondDone)
	}

	debouncer.Schedule("EKCH", run)

	select {
	case <-firstStarted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for first run to start")
	}

	debouncer.Schedule("EKCH", run)
	close(releaseFirst)

	select {
	case <-secondDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for second run")
	}

	if got := runs.Load(); got != 2 {
		t.Fatalf("expected exactly 2 runs, got %d", got)
	}
}

func TestRecalcDebounceKey_IsSessionScoped(t *testing.T) {
	t.Parallel()

	first := recalcDebounceKey(7, "EKCH")
	second := recalcDebounceKey(8, "EKCH")

	if first == second {
		t.Fatalf("expected distinct debounce keys for different sessions, both were %q", first)
	}
}

func TestRecalcDebouncerCancelSessionDropsPendingAndRerun(t *testing.T) {
	d := newRecalcDebouncer(10 * time.Millisecond)
	started := make(chan struct{})
	release := make(chan struct{})
	var runs atomic.Int32
	d.Schedule("7:EKCH", func() {
		if runs.Add(1) == 1 {
			close(started)
			<-release
		}
	})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("run did not start")
	}
	d.Schedule("7:EKCH", func() { t.Error("unexpected replacement callback") })
	d.Schedule("7:EKBI", func() { runs.Add(1) })
	d.CancelPrefix("7:")
	close(release)
	other := make(chan struct{})
	d.Schedule("8:EKCH", func() { close(other) })
	select {
	case <-other:
	case <-time.After(time.Second):
		t.Fatal("another session's work was cancelled")
	}
	time.Sleep(30 * time.Millisecond)
	if runs.Load() != 1 {
		t.Fatalf("cancelled work ran: %d", runs.Load())
	}
}

func TestRecalcDebouncerCancelledRunningJobCannotConsumeReplacement(t *testing.T) {
	d := newRecalcDebouncer(time.Millisecond)
	started := make(chan struct{})
	release := make(chan struct{})
	d.Schedule("7:EKCH", func() { close(started); <-release })
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("run did not start")
	}
	d.CancelPrefix("7:")
	d.interval = 20 * time.Millisecond
	replacement := make(chan struct{})
	d.Schedule("7:EKCH", func() { close(replacement) })
	close(release)
	select {
	case <-replacement:
	case <-time.After(time.Second):
		t.Fatal("old running job consumed its replacement")
	}
}
