package main

import (
	"FlightStrips/internal/testing/positionload"
	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Full-duration load runs exercise every selected mix/pattern despite gate
// failures. This initial harness is not complete Task23 qualification: lifecycle,
// kill/reconnect and disk/restore acceptance remain pending.
func TestPositionLoadNATS(t *testing.T) {
	if os.Getenv("NATS_TASK23") != "1" {
		t.Skip("explicit isolated Task23 qualification")
	}
	require.NotEmpty(t, os.Getenv("NATS_SERVER_BINARY"), "never use the default development brokers")
	smoke := os.Getenv("NATS_TASK23_SMOKE") == "1"
	for _, mix := range []struct {
		name     string
		arrivals int
	}{{"mixed", 100}, {"arrivals-heavy", 160}, {"departures-heavy", 40}} {
		for _, pattern := range []string{"even", "second-burst"} {
			label := mix.name + "/" + pattern
			if selected := os.Getenv("NATS_TASK23_PATTERN"); selected != "" && selected != mix.name+"-"+pattern {
				continue
			}
			t.Run(label, func(t *testing.T) { runPositionLoad(t, mix.arrivals, pattern, smoke) })
		}
	}
}

func runPositionLoad(t *testing.T, arrivals int, pattern string, smoke bool) {
	capture := positionload.New()
	collector := httptest.NewServer(capture)
	defer collector.Close()
	f := newEntrypointFixtureConfigured(t, false, map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": collector.URL, "OTEL_BSP_SCHEDULE_DELAY": "50", "OTEL_BSP_MAX_QUEUE_SIZE": "16384", "OTEL_METRIC_EXPORT_INTERVAL": "10000"})
	name := "TASK23-" + strings.ToUpper(uuid.NewString())
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
	strips := make([]*es.Strip, 200)
	for i := range strips {
		origin, destination, runway := "EKCH", "ESSA", "22R"
		if i < arrivals {
			origin, destination, runway = "ESSA", "EKCH", "22L"
		}
		strips[i] = &es.Strip{Callsign: fmt.Sprintf("SAS%03d", i), Origin: origin, Destination: destination, AircraftType: "B738", Route: "SOK KEMAX", AssignedSquawk: "1001", Runway: runway, HasFp: true, Cleared: true, GroundState: "TAXI"}
	}
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
	fronts := []*websocket.Conn{f.front(0, name), f.front(1, name)}
	warmup, duration := 2*time.Minute, 15*time.Minute
	if smoke {
		warmup, duration = 2*time.Second, 10*time.Second
	}
	start := time.Now()
	monitor := f.startLoadMonitor()
	monitorFinished := false
	defer func() {
		if !monitorFinished {
			_ = monitor.finish(f)
		}
	}()
	measureStart := start.Add(warmup)
	measureEnd := measureStart.Add(duration)
	sent, controls, frontend := 0, 0, 0
	frontIDs := []string{}
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
			lat, lon, alt := 55.63+float64(i%20)*.0001, 12.65+float64(cycle%50)*.00002, int64(20)
			if i < arrivals {
				lat, lon, alt = 55.85-float64(cycle%100)*.0003, 12.95-float64(cycle%100)*.0003, 5000-int64(cycle%100)*20
			}
			frame := envelope(uuid.NewString())
			frame.Event = &es.Envelope_AircraftPositionUpdate{AircraftPositionUpdate: &es.AircraftPositionUpdateEvent{Callsign: strips[i].Callsign, Lat: lat, Lon: lon, Altitude: alt}}
			send(frame, due, true)
			sent++
			if (n+1)%(rate/5) == 0 {
				control := envelope(uuid.NewString())
				control.Event = &es.Envelope_Heading{Heading: &es.HeadingEvent{Callsign: strips[(i+1)%199].Callsign, Heading: int32(controls % 360)}}
				send(control, due, false)
				controls++
			}
			if n%rate == 0 {
				id := uuid.NewString()
				frontIDs = append(frontIDs, id)
				// This action uses the accepted revision of a separate control strip.
				// Position observations do not change the durable strip revision.
				revision := f.state(ref).Indexes[pb.EntityKind_STRIP]["SAS199"].Revision
				command := faultCommand(id, &pb.ClientCommand{Action: &pb.ClientCommand_Strip{Strip: &pb.StripAction{Callsign: "SAS199", Change: &pb.StripAction_SetMarked{SetMarked: &pb.SetMarked{Marked: frontend%2 == 0}}}}}, &revision)
				sendEntrypointFrame(t, fronts[frontend%2], command)
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
	measurements := monitor.finish(f)
	monitorFinished = true
	if r.PositionsSent != sent || r.OperationalSent != controls {
		r.Failures = append(r.Failures, "capture/sender count mismatch")
	}
	if pattern == "second-burst" && r.MaxBatchMS >= 1000 {
		r.Failures = append(r.Failures, "batch not completed before next second")
	}
	unexpectedFrontend := 0
	for _, id := range frontIDs {
		if f.outcome(0, id) != "succeeded" {
			unexpectedFrontend++
		}
	}
	if unexpectedFrontend != 0 {
		r.Failures = append(r.Failures, "frontend action not durably successful")
	}
	report := map[string]any{"qualification": false, "full_duration": !smoke, "qualification_pending": []string{"original landing/bay/stand lifecycle assertions", "backend-kill recovery including binary client reconnect", "disk thresholds and off-cluster restore", "affected Task22 fault rerun"}, "arrival_count": arrivals, "pattern": pattern, "warmup": warmup.String(), "duration": duration.String(), "result": r, "frontend_sent": frontend, "frontend_unexpected": unexpectedFrontend, "backend_binary_sha256": hashFixtureBinary(t, f.binary), "topology": "one physical Windows host, 3 native NATS 2.15.0 file R3 stores, 2 compiled backend processes; loopback", "load_latency_pass": len(r.Failures) == 0}
	report["measurements"] = measurements
	report["puback"] = capture.PubAckReport()
	data, err := json.MarshalIndent(report, "", "  ")
	require.NoError(t, err)
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
