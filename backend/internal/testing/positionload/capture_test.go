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
