// Package positionload captures real compiled-server OTLP completion spans.
// Diagnostic reports are JSON; all application sockets and NATS values remain
// their existing typed binary Protobuf. No benchmark acknowledgment is added.
package positionload

import (
	collector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type Sent struct {
	ID       string
	Due, At  time.Time
	Position bool
}
type Completion struct {
	ID                       string
	Receipt, End             time.Time
	Position, Failed         bool
	ErrorType, ErrorReason   string
	DispatchMS, ProcessingMS float64
}
type StageSample struct {
	Start time.Time
	MS    float64
}
type Capture struct {
	mu                       sync.Mutex
	Sent                     []Sent
	Done                     map[string]Completion
	PubAck                   map[string][]float64
	Stages                   map[string][]StageSample
	Duplicates, DecodeErrors int
}

func New() *Capture            { return &Capture{Done: map[string]Completion{}, PubAck: map[string][]float64{}} }
func (c *Capture) Send(s Sent) { c.mu.Lock(); defer c.mu.Unlock(); c.Sent = append(c.Sent, s) }
func (c *Capture) Count() int  { c.mu.Lock(); defer c.mu.Unlock(); return len(c.Done) }
func (c *Capture) Completion(id string) (Completion, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.Done[id]
	return v, ok
}
func (c *Capture) PubAckReport() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string]any{}
	for name, samples := range c.PubAck {
		out[name] = map[string]any{"count": len(samples), "p95_ms": Percentile(samples, .95), "p99_ms": Percentile(samples, .99)}
	}
	return out
}

// StageReport limits diagnostic stages to the measured window when supplied.
// Warmup and overload samples must not distort steady-state percentiles.
func (c *Capture) StageReport(window ...time.Time) map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string]any{}
	for name, samples := range c.Stages {
		values := make([]float64, 0, len(samples))
		for _, sample := range samples {
			if len(window) == 2 && (sample.Start.Before(window[0]) || !sample.Start.Before(window[1])) {
				continue
			}
			values = append(values, sample.MS)
		}
		if name == "position.wait_rechecks" {
			out[name] = map[string]any{"count": len(values), "unit": "count", "p95": Percentile(values, .95), "p99": Percentile(values, .99)}
		} else {
			out[name] = map[string]any{"count": len(values), "p95_ms": Percentile(values, .95), "p99_ms": Percentile(values, .99)}
		}
	}
	return out
}
func (c *Capture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/traces" {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(200)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	req := &collector.ExportTraceServiceRequest{}
	if err != nil || proto.Unmarshal(body, req) != nil {
		c.mu.Lock()
		c.DecodeErrors++
		c.mu.Unlock()
		w.WriteHeader(400)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, rs := range req.ResourceSpans {
		for _, ss := range rs.ScopeSpans {
			for _, s := range ss.Spans {
				if strings.HasPrefix(s.Name, "euroscope.position.") {
					if c.Stages == nil {
						c.Stages = make(map[string][]StageSample)
					}
					at := time.Unix(0, int64(s.StartTimeUnixNano))
					c.Stages[s.Name+".total_ms"] = append(c.Stages[s.Name+".total_ms"], StageSample{Start: at, MS: float64(s.EndTimeUnixNano-s.StartTimeUnixNano) / float64(time.Millisecond)})
					for _, a := range s.Attributes {
						if a.Key == "position.wait_rechecks" {
							c.Stages[a.Key] = append(c.Stages[a.Key], StageSample{Start: at, MS: float64(a.Value.GetIntValue())})
						}
						if strings.HasPrefix(a.Key, "position.") && strings.HasSuffix(a.Key, "_ms") {
							c.Stages[a.Key] = append(c.Stages[a.Key], StageSample{Start: at, MS: a.Value.GetDoubleValue()})
						}
					}
				}
				if s.Name == "nats.positions.puback" || s.Name == "nats.state.puback" {
					c.PubAck[s.Name] = append(c.PubAck[s.Name], float64(s.EndTimeUnixNano-s.StartTimeUnixNano)/float64(time.Millisecond))
				}
				if s.Name != "euroscope.receipt_to_completion" {
					continue
				}
				v := Completion{Receipt: time.Unix(0, int64(s.StartTimeUnixNano)), End: time.Unix(0, int64(s.EndTimeUnixNano)), Failed: s.Status.GetCode() == tracepb.Status_STATUS_CODE_ERROR}
				for _, a := range s.Attributes {
					if a.Key == "command_id" {
						v.ID = a.Value.GetStringValue()
					}
					if a.Key == "position" {
						v.Position = a.Value.GetBoolValue()
					}
					if a.Key == "error_type" {
						v.ErrorType = a.Value.GetStringValue()
					}
					if a.Key == "error_reason" {
						v.ErrorReason = a.Value.GetStringValue()
					}
					if a.Key == "dispatch_wait_ms" {
						v.DispatchMS = a.Value.GetDoubleValue()
					}
					if a.Key == "processing_ms" {
						v.ProcessingMS = a.Value.GetDoubleValue()
					}
				}
				if v.ID == "" {
					continue
				}
				if _, ok := c.Done[v.ID]; ok {
					c.Duplicates++
				}
				c.Done[v.ID] = v
			}
		}
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(200)
}

func Offset(n, rate int, pattern string) time.Duration {
	if pattern == "second-burst" {
		return time.Duration(n/rate) * time.Second
	}
	return time.Duration(n) * time.Second / time.Duration(rate)
}
func Percentile(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	v = append([]float64(nil), v...)
	sort.Float64s(v)
	return v[int(math.Ceil(float64(len(v))*p))-1]
}

type Report struct {
	DispatchP95MS, DispatchP99MS, ProcessingP95MS, ProcessingP99MS                                                                                 float64
	FailureReasons                                                                                                                                 map[string]int
	OperationalP95MS, OperationalP99MS                                                                                                             float64
	ScheduledGroupP95MS                                                                                                                            []float64
	Sent, Completed, PositionsSent, PositionsCompleted, OperationalSent, OperationalCompleted, UnexpectedErrors, Missing, Duplicates, DecodeErrors int
	P95MS, P99MS, ScheduledP95MS, ScheduledP99MS, SenderP99LagMS, MaxBatchMS, DrainMS                                                              float64
	MaxBacklog                                                                                                                                     int
	BacklogByMinute                                                                                                                                []int
	Failures                                                                                                                                       []string
}

func (c *Capture) Report(start, end, overloadEnd time.Time, overloadTarget int) Report {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := Report{Sent: len(c.Sent), Duplicates: c.Duplicates, DecodeErrors: c.DecodeErrors}
	var receipt, scheduled, sender, operations, dispatch, processing []float64
	groups := make([][]float64, 5)
	type change struct {
		at    time.Time
		delta int
	}
	var changes []change
	batches := map[int64]time.Time{}
	for _, s := range c.Sent {
		if s.Position {
			r.PositionsSent++
		} else {
			r.OperationalSent++
		}
		d, ok := c.Done[s.ID]
		if !ok {
			r.Missing++
			continue
		}
		r.Completed++
		if d.Failed {
			r.UnexpectedErrors++
			if r.FailureReasons == nil {
				r.FailureReasons = make(map[string]int)
			}
			reason := d.ErrorReason
			if reason == "" {
				reason = "unclassified"
			}
			r.FailureReasons[reason+":"+d.ErrorType]++
		}
		if s.Position {
			r.PositionsCompleted++
			changes = append(changes, change{s.At, 1}, change{d.End, -1})
			if !s.Due.Before(start) && s.Due.Before(end) {
				dispatch = append(dispatch, d.DispatchMS)
				processing = append(processing, d.ProcessingMS)
				receipt = append(receipt, float64(d.End.Sub(d.Receipt))/float64(time.Millisecond))
				scheduled = append(scheduled, float64(d.End.Sub(s.Due))/float64(time.Millisecond))
				group := ((r.PositionsSent - 1) % 100) / 20
				groups[group] = append(groups[group], float64(d.End.Sub(s.Due))/float64(time.Millisecond))
				sender = append(sender, float64(s.At.Sub(s.Due))/float64(time.Millisecond))
				key := s.Due.UnixNano()
				if d.End.After(batches[key]) {
					batches[key] = d.End
				}
			}
			if r.PositionsSent <= overloadTarget && d.End.After(overloadEnd) {
				r.DrainMS = max(r.DrainMS, float64(d.End.Sub(overloadEnd))/float64(time.Millisecond))
			}
		} else {
			r.OperationalCompleted++
			if !s.Due.Before(start) && s.Due.Before(end) {
				operations = append(operations, float64(d.End.Sub(d.Receipt))/float64(time.Millisecond))
			}
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].at.Before(changes[j].at) })
	depth := 0
	minute := start.Add(time.Minute)
	for _, v := range changes {
		for !v.at.Before(minute) && !minute.After(end) {
			r.BacklogByMinute = append(r.BacklogByMinute, depth)
			minute = minute.Add(time.Minute)
		}
		depth += v.delta
		r.MaxBacklog = max(r.MaxBacklog, depth)
	}
	for due, last := range batches {
		r.MaxBatchMS = max(r.MaxBatchMS, float64(last.Sub(time.Unix(0, due)))/float64(time.Millisecond))
	}
	r.OperationalP95MS, r.OperationalP99MS = Percentile(operations, .95), Percentile(operations, .99)
	for _, group := range groups {
		r.ScheduledGroupP95MS = append(r.ScheduledGroupP95MS, Percentile(group, .95))
	}
	r.P95MS, r.P99MS = Percentile(receipt, .95), Percentile(receipt, .99)
	r.DispatchP95MS, r.DispatchP99MS = Percentile(dispatch, .95), Percentile(dispatch, .99)
	r.ProcessingP95MS, r.ProcessingP99MS = Percentile(processing, .95), Percentile(processing, .99)
	r.ScheduledP95MS, r.ScheduledP99MS = Percentile(scheduled, .95), Percentile(scheduled, .99)
	r.SenderP99LagMS = Percentile(sender, .99)
	if len(receipt) == 0 {
		r.Failures = append(r.Failures, "no steady position samples")
	}
	if r.Missing != 0 || r.Completed != r.Sent {
		r.Failures = append(r.Failures, "sent/completed mismatch")
	}
	if r.UnexpectedErrors+r.Duplicates+r.DecodeErrors != 0 {
		r.Failures = append(r.Failures, "unexpected errors or duplicate/decode failures")
	}
	if r.P95MS > 20 || r.P99MS > 50 {
		r.Failures = append(r.Failures, "receipt-to-completion latency gate")
	}
	if r.ScheduledP95MS > 20 || r.ScheduledP99MS > 50 {
		r.Failures = append(r.Failures, "sender-deadline completion latency gate")
	}
	if r.DrainMS > 2000 {
		r.Failures = append(r.Failures, "overload drain exceeds two seconds")
	}
	if len(r.BacklogByMinute) > 1 && r.BacklogByMinute[len(r.BacklogByMinute)-1] > r.BacklogByMinute[0]+100 {
		r.Failures = append(r.Failures, "steady backlog grows beyond one second of traffic")
	}
	return r
}
