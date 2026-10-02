package main

import (
	"FlightStrips/internal/testing/positionload"
	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// Full-duration load runs exercise every selected mix/pattern despite gate
// failures. A pattern result alone cannot qualify Task23: the runner also gates
// the separate kill/reconnect, disk boundary and fault/restore checks.
func TestPositionLoadNATS(t *testing.T) {
	if os.Getenv("NATS_TASK23") != "1" {
		t.Skip("explicit isolated Task23 qualification")
	}
	require.NotEmpty(t, os.Getenv("NATS_SERVER_BINARY"), "never use the default development brokers")
	smoke := os.Getenv("NATS_TASK23_SMOKE") == "1"
	parallel := positionLoadParallelism(t)
	slots := make(chan struct{}, parallel)
	for _, mix := range []struct {
		name     string
		arrivals int
	}{{"mixed", 100}, {"arrivals-heavy", 160}, {"departures-heavy", 40}} {
		for _, pattern := range []string{"even", "second-burst"} {
			label := mix.name + "/" + pattern
			if selected := os.Getenv("NATS_TASK23_PATTERN"); selected != "" && selected != mix.name+"-"+pattern {
				continue
			}
			t.Run(label, func(t *testing.T) {
				if parallel > 1 {
					t.Parallel()
				}
				slots <- struct{}{}
				// Release only after the fixture's child processes and stores close.
				t.Cleanup(func() { <-slots })
				runPositionLoad(t, mix.arrivals, pattern, smoke)
			})
		}
	}
}

func positionLoadParallelism(t *testing.T) int {
	t.Helper()
	value := os.Getenv("NATS_TASK23_PARALLEL")
	if value == "" {
		return 1
	}
	parallel, err := strconv.Atoi(value)
	require.NoError(t, err)
	require.GreaterOrEqual(t, parallel, 1)
	require.LessOrEqual(t, parallel, 6)
	return parallel
}

func runPositionLoad(t *testing.T, arrivals int, pattern string, smoke bool) {
	capture := positionload.New()
	collector := httptest.NewServer(capture)
	defer collector.Close()
	fleet := newLoadFleet(t, arrivals)
	navigation, provider := newAMANHTTPFixture(t)
	f := newEntrypointFixtureConfigured(t, true, map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": collector.URL, "OTEL_BSP_SCHEDULE_DELAY": "50", "OTEL_BSP_MAX_QUEUE_SIZE": "16384", "OTEL_METRIC_EXPORT_INTERVAL": "10000", "ENABLE_STAND_ASSIGNMENT": "true", "GRPLUGIN_ICAO_AIRCRAFT_JSON": "config/test/ICAO_Aircraft.json", "ENABLE_VATSIM": "true", "VATSIM_STATUS_URL": fleet.provider.URL + "/status", "VATSIM_POLL_INTERVAL": "5s", "NAVIGATION_SOURCE": "airacnet", "NAVIGATION_TERMINAL_GEOMETRY_PATH": navigation, "AMAN_MODE": "shadow", "AMAN_SOURCE_MODE": "euroscope", "AMAN_ENABLED_AIRPORTS": "EKCH", "TASK22_AIRAC_URL": provider + "/api/v1", "TASK22_WIND_URL": provider + "/wind"})
	profilePositionLoad(t, f)
	name := "LIVE"
	plugins := f.concurrentPlugins(name)
	ref := sessionFaultRef(f.session(name))
	f.await("master election", func() bool {
		return f.state(ref).Master.GetCid() != "" && len(f.state(ref).EntitiesByKind(pb.EntityKind_CONTROLLER)) == 2
	})
	f.assertOneMaster(ref, plugins)
	for _, p := range plugins {
		p.mu.Lock()
		p.discardSync = true
		p.mu.Unlock()
	}
	state := f.state(ref)
	master := 0
	if state.Master.Cid == "222222" {
		master = 1
	}
	strips := fleet.strips
	envelope := func(id string) *es.Envelope {
		return &es.Envelope{CommandId: id, SessionId: ref.GetSession().Id, OwnerEpoch: state.Owner.Epoch, MasterEpoch: state.Master.Epoch}
	}
	syncFrame := envelope(uuid.NewString())
	syncFrame.Event = &es.Envelope_Sync{Sync: &es.SyncEvent{Strips: strips, Runways: []*es.Runway{{Name: "22R", Departure: true}, {Name: "22L", Arrival: true}}}}
	sendEntrypointFrame(t, plugins[master].conn, syncFrame)
	f.await("all 200 strips and sync accepted", func() bool {
		s := f.state(ref)
		return len(s.EntitiesByKind(pb.EntityKind_STRIP)) == 200 && s.Sync != nil
	})
	fronts := []*loadFrontend{f.loadFrontend(0, name), f.loadFrontend(1, name)}
	lifecycle := f.watchLoadLifecycle(ref)
	lifecycleFinished := false
	defer func() {
		if !lifecycleFinished {
			_ = lifecycle.finish()
		}
	}()
	warmup, duration := 2*time.Minute, 15*time.Minute
	profiled := os.Getenv("NATS_TASK23_PROFILE_NODE") != ""
	if profiled {
		// Diagnostic sampling retains the full warmup, but never qualifies capacity.
		duration = time.Minute
	}
	if smoke {
		warmup, duration = 2*time.Second, 10*time.Second
	}
	start := time.Now()
	measureStart := start.Add(warmup)
	measureEnd := measureStart.Add(duration)
	binaryHash := hashFixtureBinary(t, f.binary)
	var measurements []map[string]any
	var abortedAt time.Time
	sent, controls, frontend := 0, 0, 0
	completed := false
	defer func() {
		if completed {
			return
		}
		if abortedAt.IsZero() {
			abortedAt = time.Now()
		}
		// The lifecycle cleanup was registered before this report, so join it
		// here before reading its counts. Monitor cleanup runs before this defer.
		observed := map[string]int{}
		if !lifecycleFinished {
			observed = lifecycle.finish()
			lifecycleFinished = true
		}
		windowEnd := abortedAt
		if windowEnd.After(measureEnd) {
			windowEnd = measureEnd
		}
		if windowEnd.Before(measureStart) {
			windowEnd = measureStart
		}
		data, err := json.MarshalIndent(map[string]any{
			"qualification": false, "load_pass": false, "full_duration": false, "planned_full_duration": !smoke && !profiled, "diagnostic_profiled": profiled,
			"arrival_count": arrivals, "pattern": pattern, "aborted": true, "elapsed": abortedAt.Sub(start).String(),
			"started_utc": start.UTC(), "aborted_utc": abortedAt.UTC(), "warmup": warmup.String(), "duration": duration.String(),
			"measure_start_utc": measureStart.UTC(), "measure_end_utc": windowEnd.UTC(),
			"result": capture.Report(measureStart, windowEnd, abortedAt, 0), "positions_sent": sent, "operational_sent": controls, "frontend_sent": frontend,
			"measurements": measurements, "lifecycle": observed, "puback": capture.PubAckReport(), "position_stages_ms": capture.StageReport(measureStart, windowEnd),
			"backend_binary_sha256": binaryHash, "concurrent_load_fixture_limit": positionLoadParallelism(t), "shared_host_parallel_load": positionLoadParallelism(t) > 1,
		}, "", "  ")
		if err != nil {
			t.Errorf("abort report: %v", err)
			return
		}
		if dir := os.Getenv("NATS_TASK23_OUTPUT"); dir != "" {
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Errorf("abort report directory: %v", err)
				return
			}
			filename := fmt.Sprintf("%d-%s.json", arrivals, pattern)
			if smoke {
				filename = "smoke-" + filename
			}
			if err := os.WriteFile(filepath.Join(dir, filename), data, 0600); err != nil {
				t.Errorf("abort report: %v", err)
			}
		}
		t.Logf("POSITION_LOAD_ABORT %s", data)
	}()
	monitor := f.startLoadMonitor()
	monitorFinished := false
	defer func() {
		if !monitorFinished {
			abortedAt = time.Now()
			measurements = monitor.finish(f)
			monitorFinished = true
		}
	}()
	send := func(frame *es.Envelope, due time.Time, position bool) {
		if delay := time.Until(due); delay > 0 {
			time.Sleep(delay)
		}
		at := time.Now()
		sendEntrypointFrame(t, plugins[master].conn, frame)
		capture.Send(positionload.Sent{ID: frame.CommandId, Due: due, At: at, Position: position})
	}
	// Controls share the plugin writer and are interleaved inside each burst.
	traffic := func(segment time.Time, seconds, rate int) {
		for n := 0; n < seconds*rate; n++ {
			due := segment.Add(positionload.Offset(n, rate, pattern))
			i := sent % 200
			cycle := sent / 200
			position, ground := fleet.position(i, cycle)
			if ground != "" {
				motion := envelope(uuid.NewString())
				motion.Event = &es.Envelope_GroundState{GroundState: &es.GroundStateEvent{Callsign: strips[i].Callsign, GroundState: ground}}
				send(motion, due, false)
				controls++
			}
			frame := envelope(uuid.NewString())
			frame.Event = &es.Envelope_AircraftPositionUpdate{AircraftPositionUpdate: position}
			send(frame, due, true)
			fleet.observe(i, position)
			sent++
			if (n+1)%(rate/5) == 0 {
				control := envelope(uuid.NewString())
				control.Event = &es.Envelope_Heading{Heading: &es.HeadingEvent{Callsign: strips[(i+1)%199].Callsign, Heading: int32(controls % 360)}}
				send(control, due, false)
				controls++
			}
			if n%rate == 0 {
				id := uuid.NewString()
				fronts[frontend%2].marked(id, frontend%2 == 0)
				frontend++
			}
		}
		if wait := time.Until(segment.Add(time.Duration(seconds) * time.Second)); wait > 0 {
			time.Sleep(wait)
		}
	}
	traffic(start, int((warmup+duration)/time.Second), 100)
	overloadStart := time.Now()
	traffic(overloadStart, 10, 200)
	overloadTarget := sent
	overloadEnd := overloadStart.Add(10 * time.Second)
	traffic(overloadEnd, 2, 100)
	// Exporting spans is asynchronous; this wait never changes their timestamps.
	time.Sleep(2 * time.Second)
	r := capture.Report(measureStart, measureEnd, overloadEnd, overloadTarget)
	observed := lifecycle.finish()
	lifecycleFinished = true
	if !smoke {
		for _, gate := range []string{"landing", "bay:TWY_ARR", "bay:AIRBORNE", "stand:DEPARTURE_BLOCK", "stand_assignment_removed"} {
			if observed[gate] == 0 {
				r.Failures = append(r.Failures, "missing original lifecycle assertion: "+gate)
			}
		}
	}
	measurements = monitor.finish(f)
	monitorFinished = true
	if r.PositionsSent != sent || r.OperationalSent != controls {
		r.Failures = append(r.Failures, "capture/sender count mismatch")
	}
	if pattern == "second-burst" && r.MaxBatchMS >= 1000 {
		r.Failures = append(r.Failures, "batch not completed before next second")
	}
	unexpectedFrontend := 0
	frontendAttempts, frontendRetries, frontendLogical := 0, 0, 0
	var frontendRetryEvidence []*loadFrontendAction
	var frontendActions []*loadFrontendAction
	for _, front := range fronts {
		frontendActions = append(frontendActions, front.finish()...)
	}
	// ActionResult may acknowledge RAM while the accepted tail is still pending.
	// Wait for this independent observer's durable history before evaluating
	// success; all timing windows and sender counts have already closed.
	durableDeadline := time.Now().Add(10 * time.Second)
	for {
		durable := true
		state, err := f.projection.ReadDurable(ref)
		if err != nil {
			durable = false
		} else {
			for _, action := range frontendActions {
				for _, attempt := range action.Attempts {
					if attempt.Status == pb.CommandOutcome_STATUS_UNSPECIFIED {
						continue // A missing reply remains a failure below.
					}
					outcome, err := state.LookupOutcome(attempt.ID)
					if err != nil || outcome == nil || outcome.CommittedStreamSequence == 0 {
						durable = false
					}
				}
			}
		}
		if durable {
			break
		}
		if time.Now().After(durableDeadline) {
			r.Failures = append(r.Failures, "frontend durable completion timed out")
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, action := range frontendActions {
		frontendLogical++
		frontendAttempts += len(action.Attempts)
		if len(action.Attempts) > 1 {
			frontendRetries += len(action.Attempts) - 1
			frontendRetryEvidence = append(frontendRetryEvidence, action)
		}
		valid := action.Err == "" && len(action.Attempts) > 0
		for i, attempt := range action.Attempts {
			// Even discarded conflicts must be real durable failures. RAM
			// success and socket delivery alone never satisfy this gate.
			outcome, err := f.state(ref).LookupOutcome(attempt.ID)
			if err != nil || outcome == nil || outcome.CommittedStreamSequence == 0 {
				valid = false
				continue
			}
			if i == len(action.Attempts)-1 {
				valid = valid && outcome.Status == pb.CommandOutcome_SUCCEEDED && f.outcome(0, attempt.ID) == "succeeded"
			} else {
				valid = valid && outcome.Status == pb.CommandOutcome_FAILED && outcome.ReasonCode == "REVISION_CONFLICT"
			}
		}
		if !valid {
			unexpectedFrontend++
			t.Logf("FRONTEND_LOGICAL_FAILURE logical_id=%s error=%s attempts=%+v", action.LogicalID, action.Err, action.Attempts)
		}
	}
	if frontendLogical != frontend {
		r.Failures = append(r.Failures, "frontend logical action count mismatch")
	}
	if unexpectedFrontend != 0 {
		r.Failures = append(r.Failures, "frontend action not durably successful")
	}
	report := map[string]any{"qualification": false, "full_duration": !smoke && !profiled, "diagnostic_profiled": profiled, "qualification_pending": []string{"runner combines all six full load results, backend recovery, disk and Task22 fault/restore results"}, "arrival_count": arrivals, "pattern": pattern, "warmup": warmup.String(), "duration": duration.String(), "result": r, "frontend_sent": frontend, "frontend_unexpected": unexpectedFrontend, "backend_binary_sha256": binaryHash, "topology": "one physical Windows host, 3 native NATS 2.15.0 file R3 encrypted stores, 2 compiled backend processes; loopback", "load_pass": len(r.Failures) == 0}
	report["measurements"] = measurements
	report["frontend_attempts"] = frontendAttempts
	report["frontend_revision_conflict_retries"] = frontendRetries
	report["frontend_retry_evidence"] = frontendRetryEvidence
	report["lifecycle"] = observed
	report["puback"] = capture.PubAckReport()
	report["position_stages_ms"] = capture.StageReport(measureStart, measureEnd)
	report["concurrent_load_fixture_limit"] = positionLoadParallelism(t)
	report["shared_host_parallel_load"] = positionLoadParallelism(t) > 1
	data, err := json.MarshalIndent(report, "", "  ")
	require.NoError(t, err)
	completed = true
	t.Logf("POSITION_LOAD %s", data)
	if dir := os.Getenv("NATS_TASK23_OUTPUT"); dir != "" {
		require.NoError(t, os.MkdirAll(dir, 0700))
		filename := fmt.Sprintf("%d-%s.json", arrivals, pattern)
		if smoke {
			filename = "smoke-" + filename
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, filename), data, 0600))
	}
	require.Empty(t, r.Failures, "full pattern fails its unchanged gates")
}
