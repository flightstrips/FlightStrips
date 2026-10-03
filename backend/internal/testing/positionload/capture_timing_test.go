package positionload

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPairedTimingPreservesMonotonicSenderLagAfterWallClockCorrection(t *testing.T) {
	oldWallDeadline := time.Unix(100, 0)
	// The sender's monotonic At.Sub(Due) remains 3ms when Windows moves wall
	// time back by 1.9s. Serialized OTLP and the actual send use that new wall
	// clock; the originally constructed deadline retains its old wall time.
	lag := 3 * time.Millisecond
	sent := oldWallDeadline.Add(lag - 1900*time.Millisecond)
	done := Completion{Receipt: sent.Add(2 * time.Millisecond), End: sent.Add(7 * time.Millisecond)}
	require.Negative(t, done.End.Sub(oldWallDeadline))
	receipt, scheduled, sentRead, dueRead, valid := pairedTiming(lag, sent, done)
	require.True(t, valid)
	require.Equal(t, 5*time.Millisecond, receipt)
	require.Equal(t, 10*time.Millisecond, scheduled)
	require.Equal(t, 2*time.Millisecond, sentRead)
	require.Equal(t, 5*time.Millisecond, dueRead)
}

func TestReportRejectsNegativeTimingRatherThanQualifyingIt(t *testing.T) {
	for _, sample := range []struct {
		name               string
		sent, receipt, end time.Duration
		position           bool
	}{
		{"completion before receipt", 0, time.Millisecond, 0, true},
		{"receipt before send", time.Millisecond, 0, 2 * time.Millisecond, true},
		{"send before deadline", -time.Millisecond, 0, time.Millisecond, true},
		{"operational completion before receipt", 0, time.Millisecond, 0, false},
	} {
		t.Run(sample.name, func(t *testing.T) {
			start := time.Now()
			c := New()
			c.Send(Sent{ID: "valid", Due: start, At: start, Position: true})
			c.Done["valid"] = Completion{Receipt: start, End: start.Add(time.Millisecond), Position: true}
			c.Send(Sent{ID: "invalid", Due: start, At: start.Add(sample.sent), Position: sample.position})
			c.Done["invalid"] = Completion{Receipt: start.Add(sample.receipt), End: start.Add(sample.end), Position: sample.position}
			r := c.Report(start, start.Add(time.Second), start.Add(time.Second), 0)
			require.Equal(t, 2, r.Completed)
			require.Equal(t, 1, r.InvalidTimingSamples)
			require.Contains(t, r.Failures, "invalid negative latency measurements")
			require.Equal(t, 1., r.P95MS, "invalid data must not lower a percentile")
			require.Equal(t, 1., r.ScheduledP95MS)
		})
	}
}
