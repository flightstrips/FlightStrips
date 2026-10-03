package main

import (
	"FlightStrips/internal/testing/positionload"
	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Five real owner-process deaths under 100/s traffic. Recovery includes fresh
// binary EuroScope login/sync, frontend reconnect and a durably accepted action
// and position. Broker state and all unaffected processes stay in place.
func TestPositionLoadBackendRecovery(t *testing.T) {
	if os.Getenv("NATS_TASK23") != "1" {
		t.Skip("explicit isolated Task23 qualification")
	}
	require.NotEmpty(t, os.Getenv("NATS_SERVER_BINARY"))
	capture := positionload.New()
	collector := httptest.NewServer(capture)
	defer collector.Close()
	f := newEntrypointFixtureConfigured(t, true, map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": collector.URL, "OTEL_BSP_SCHEDULE_DELAY": "50", "OTEL_BSP_MAX_QUEUE_SIZE": "16384"})
	name := "TASK23-RECOVERY-" + strings.ToUpper(uuid.NewString())
	ref := sessionFaultRef(0)
	strips := make([]*es.Strip, 200)
	for i := range strips {
		origin, destination := "EKCH", "ESSA"
		if i < 100 {
			origin, destination = destination, origin
		}
		strips[i] = &es.Strip{Callsign: fmt.Sprintf("SAS%03d", i), Origin: origin, Destination: destination, AircraftType: "B738", AssignedSquawk: "1001", Runway: "22L", GroundState: "TAXI", HasFp: true}
	}
	timings := []float64{}
	timeline := []map[string]any{}
	defer func() {
		data, err := json.MarshalIndent(map[string]any{"source": "compiled local owner process", "trials": timeline, "p95_ms": positionload.Percentile(timings, .95), "pass": len(timings) == 5 && positionload.Percentile(timings, .95) <= 15000 && !t.Failed()}, "", "  ")
		require.NoError(t, err)
		if dir := os.Getenv("NATS_TASK23_OUTPUT"); dir != "" {
			require.NoError(t, os.MkdirAll(dir, 0700))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "backend-recovery.json"), data, 0600))
		}
		t.Logf("LOAD_RECOVERY %s", data)
	}()
	for trial := 0; trial < 5; trial++ {
		plugins := f.concurrentPlugins(name)
		ref = sessionFaultRef(f.session(name))
		f.await("recovery trial master elected", func() bool { s, err := f.projection.Read(ref); return err == nil && s.Master.GetCid() != "" })
		f.assertOneMaster(ref, plugins)
		syncFleet := func() int {
			state := f.state(ref)
			master := 0
			if state.Master.Cid == "222222" {
				master = 1
			}
			sendEntrypointFrame(t, plugins[master].conn, &es.Envelope{SessionId: ref.GetSession().Id, CommandId: uuid.NewString(), OwnerEpoch: state.Owner.Epoch, MasterEpoch: state.Master.Epoch, Event: &es.Envelope_Sync{Sync: &es.SyncEvent{Strips: strips, Runways: []*es.Runway{{Name: "22L", Arrival: true}, {Name: "22R", Departure: true}}}}})
			f.await("fresh full operational sync", func() bool {
				s, err := f.projection.Read(ref)
				return err == nil && len(s.Indexes[pb.EntityKind_STRIP]) == 200 && s.Sync.GetMasterEpoch() == s.Master.GetEpoch() && s.Sync.GetConnectionId() == s.Master.GetConnectionId()
			})
			return master
		}
		master := syncFleet()
		for _, p := range plugins {
			p.mu.Lock()
			p.discardSync = true
			p.mu.Unlock()
		}
		front := f.front(master, name)
		state := f.state(ref)
		start := time.Now()
		for n := 0; n < 3000; n++ {
			due := start.Add(positionload.Offset(n, 100, "even"))
			if wait := time.Until(due); wait > 0 {
				time.Sleep(wait)
			}
			sendEntrypointFrame(t, plugins[master].conn, &es.Envelope{SessionId: ref.GetSession().Id, CommandId: uuid.NewString(), OwnerEpoch: state.Owner.Epoch, MasterEpoch: state.Master.Epoch, Event: &es.Envelope_AircraftPositionUpdate{AircraftPositionUpdate: &es.AircraftPositionUpdateEvent{Callsign: strips[n%200].Callsign, Lat: 55.63, Lon: 12.65, Altitude: 20}}})
			if (n+1)%20 == 0 {
				sendEntrypointFrame(t, plugins[master].conn, &es.Envelope{SessionId: ref.GetSession().Id, CommandId: uuid.NewString(), OwnerEpoch: state.Owner.Epoch, MasterEpoch: state.Master.Epoch, Event: &es.Envelope_Heading{Heading: &es.HeadingEvent{Callsign: strips[n%199].Callsign, Heading: int32(n % 360)}}})
			}
			if n%100 == 0 {
				s, err := f.projection.ReadEntity(ref, pb.EntityKind_STRIP, "SAS199")
				require.NoError(t, err)
				sendEntrypointFrame(t, front, markedCommand(uuid.NewString(), "SAS199", s.Revision, n%200 == 0))
			}
		}
		ownerNode := -1
		_, presence, err := f.projection.ObservationSnapshot(ref.GetSession().Id)
		require.NoError(t, err)
		for _, p := range presence {
			client := p.Value.GetClient()
			if client != nil && client.Kind == pb.ClientPresence_EUROSCOPE && client.NodeId == state.Owner.NodeId {
				if client.Cid == "111111" {
					ownerNode = 0
				} else if client.Cid == "222222" {
					ownerNode = 1
				}
			}
		}
		require.NotEqual(t, -1, ownerNode, "owner must map to a verified test-owned backend")
		ownerEpochBefore := state.Owner.Epoch
		killed := time.Now()
		f.apps[ownerNode].stop()
		_ = front.Close()
		for _, p := range plugins {
			_ = p.conn.Close()
		}
		survivor := 1 - ownerNode
		f.await("survivor claims a fresh owner epoch", func() bool {
			s, err := f.projection.Read(ref)
			return err == nil && s.Owner.GetEpoch() > state.Owner.Epoch && s.Owner.GetNodeId() != state.Owner.NodeId
		})
		plugins = []*faultSocket{f.plugin(survivor, name, "111111"), f.plugin(survivor, name, "222222")}
		f.await("reconnected clients elected", func() bool {
			s, err := f.projection.Read(ref)
			return err == nil && s.Master.GetOwnerEpoch() == s.Owner.GetEpoch() && s.Master.GetCid() != ""
		})
		f.assertOneMaster(ref, plugins)
		master = syncFleet()
		front = f.front(survivor, name)
		state = f.state(ref)
		id := uuid.NewString()
		sendEntrypointFrame(t, plugins[master].conn, &es.Envelope{SessionId: ref.GetSession().Id, CommandId: id, OwnerEpoch: state.Owner.Epoch, MasterEpoch: state.Master.Epoch, Event: &es.Envelope_AircraftPositionUpdate{AircraftPositionUpdate: &es.AircraftPositionUpdateEvent{Callsign: "SAS000", Lat: 55.63, Lon: 12.65, Altitude: 20}}})
		action := uuid.NewString()
		sendEntrypointFrame(t, front, markedCommand(action, "SAS199", state.Indexes[pb.EntityKind_STRIP]["SAS199"].Revision, trial%2 == 0))
		f.await("accepted post-reconnect position and frontend action", func() bool { _, ok := capture.Completion(id); return ok && f.outcome(survivor, action) == "succeeded" })
		completion, _ := capture.Completion(id)
		recovered := time.Now()
		// The frontend receipt query is a conservative final bound; the position
		// exporter wait does not shorten the accepted-position completion time.
		ms := float64(recovered.Sub(killed)) / float64(time.Millisecond)
		timings = append(timings, ms)
		timeline = append(timeline, map[string]any{"trial": trial, "killed_utc": killed.UTC(), "recovered_utc": recovered.UTC(), "position_completed_utc": completion.End.UTC(), "elapsed_ms": ms, "owner_epoch_before": ownerEpochBefore, "owner_epoch_after": state.Owner.Epoch, "killed_backend": ownerNode})
		_ = front.Close()
		for _, p := range plugins {
			_ = p.conn.Close()
		}
		f.restart(ownerNode)
	}
	require.LessOrEqual(t, positionload.Percentile(timings, .95), 15000., "backend kill plus client reconnect p95")
}
