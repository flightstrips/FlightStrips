package positionload

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestReportRejectsSocketWaitingAndMissingCompletion(t *testing.T) {
	start := time.Now()
	c := New()
	for i := 0; i < 100; i++ {
		id := string(rune(i + 1))
		due := start.Add(time.Duration(i) * time.Millisecond)
		c.Send(Sent{ID: id, Due: due, At: due, Position: true})
		c.Done[id] = Completion{ID: id, Receipt: due.Add(60 * time.Millisecond), End: due.Add(61 * time.Millisecond), Position: true}
	}
	r := c.Report(start, start.Add(time.Second), start.Add(time.Second), 100)
	require.Equal(t, 1., r.P95MS)
	require.Equal(t, 61., r.ScheduledP95MS)
	require.Contains(t, r.Failures, "sender-deadline completion latency gate")
	delete(c.Done, string(rune(1)))
	r = c.Report(start, start.Add(time.Second), start.Add(time.Second), 100)
	require.Equal(t, 100, r.Sent)
	require.Equal(t, 99, r.Completed)
	require.Equal(t, 1, r.Missing)
	require.Contains(t, r.Failures, "sent/completed mismatch")
}
func TestReportUsesCompletionTimeForOverloadDrain(t *testing.T) {
	start := time.Now()
	c := New()
	end := start.Add(time.Second)
	c.Send(Sent{ID: "x", Due: start, At: start, Position: true})
	c.Done["x"] = Completion{ID: "x", Receipt: start, End: end.Add(2100 * time.Millisecond), Position: true}
	r := c.Report(start, end, end, 1)
	require.Equal(t, 2100., r.DrainMS)
	require.Contains(t, r.Failures, "overload drain exceeds two seconds")
}
func TestReportSchedule(t *testing.T) {
	for _, rate := range []int{100, 200} {
		for n := 0; n < rate*3; n++ {
			require.Equal(t, time.Duration(n/rate)*time.Second, Offset(n, rate, "second-burst"))
			require.Equal(t, time.Duration(n)*time.Second/time.Duration(rate), Offset(n, rate, "even"))
		}
	}
}

func TestReportIncludesClassifiedFailureWithoutChangingGate(t *testing.T) {
	start := time.Now()
	c := New()
	c.Send(Sent{ID: "failed", Due: start, At: start, Position: true})
	c.Done["failed"] = Completion{ID: "failed", Receipt: start, End: start.Add(time.Millisecond), Position: true, Failed: true, ErrorType: "*nats.APIError", ErrorReason: "nats_revision_conflict"}
	r := c.Report(start, start.Add(time.Second), start.Add(time.Second), 1)
	require.Equal(t, 1, r.FailureReasons["nats_revision_conflict:*nats.APIError"])
	require.Equal(t, 1, r.UnexpectedErrors)
	require.Contains(t, r.Failures, "unexpected errors or duplicate/decode failures")
}

func TestStageReportExcludesWarmupAndOverload(t *testing.T) {
	start := time.Now()
	end := start.Add(time.Minute)
	c := New()
	c.Stages = map[string][]StageSample{"position.validation_ms": {
		{Start: start.Add(-time.Second), MS: 1000},
		{Start: start, MS: 1},
		{Start: end.Add(-time.Second), MS: 2},
		{Start: end, MS: 2000},
	}}
	measured := c.StageReport(start, end)["position.validation_ms"].(map[string]any)
	require.Equal(t, 2, measured["count"])
	require.Equal(t, 2., measured["p99_ms"])
	require.Equal(t, 4, c.StageReport()["position.validation_ms"].(map[string]any)["count"])
}
