package main

import (
	"FlightStrips/internal/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"google.golang.org/protobuf/types/known/timestamppb"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestServerNATSStandRace(t *testing.T) {
	if os.Getenv("NATS_TASK22") != "1" {
		t.Skip("requires explicit Task22 disposable fixture")
	}
	f := newEntrypointFixture(t, true)
	f.changeEnvironment(map[string]string{"ENABLE_STAND_ASSIGNMENT": "true", "GRPLUGIN_ICAO_AIRCRAFT_JSON": "config/test/ICAO_Aircraft.json"})
	name, ref, _ := f.seededSession()
	f.seedEntity(ref, "SAS124", &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: &pb.Strip{Callsign: "SAS124", Bay: "CLEARED", Departure: "EKCH", Destination: "ESSA", AircraftType: "A320", AssignedSquawk: "1002"}}})
	fronts := []*websocket.Conn{f.front(0, name), f.front(1, name)}
	ids := []string{uuid.NewString(), uuid.NewString()}
	zero := uint64(0)
	var writes sync.WaitGroup
	for i, callsign := range []string{"SAS123", "SAS124"} {
		writes.Add(1)
		go func(i int, callsign string) {
			defer writes.Done()
			sendEntrypointFrame(t, fronts[i], faultCommand(ids[i], &pb.ClientCommand{Action: &pb.ClientCommand_Stand{Stand: &pb.StandAction{Callsign: callsign, Stand: "A17", Change: &pb.StandAction_Manual{Manual: &pb.ManualStand{}}}}}, &zero))
		}(i, callsign)
	}
	writes.Wait()
	f.await("stand race durable outcomes", func() bool { return f.state(ref).Ledger[ids[0]] != nil && f.state(ref).Ledger[ids[1]] != nil })
	state := f.state(ref)
	success, failed := 0, 0
	for _, id := range ids {
		outcome := state.Ledger[id]
		if outcome.Status == pb.CommandOutcome_SUCCEEDED {
			success++
		} else if outcome.Status == pb.CommandOutcome_FAILED {
			failed++
		}
		t.Logf("STAND_RACE id=%s status=%s reason=%s sequence=%d", id, outcome.Status, outcome.ReasonCode, outcome.CommittedStreamSequence)
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, failed)
	require.Len(t, state.StandAssignmentsByStand["A17"], 1)
	for _, id := range ids {
		f.await("stand race JSON parity", func() bool { return f.outcome(0, id) != "" && f.outcome(0, id) == f.outcome(1, id) })
	}
}

func (f *entrypointFixture) changeEnvironment(values map[string]string) {
	f.t.Helper()
	for _, p := range f.apps {
		p.stop()
	}
	f.env = replaceFixtureEnv(f.env, values)
	for i := range f.apps {
		f.apps[i] = startFixtureProcess(f.t, f.binary, f.backend, f.env, "-addr", f.addresses[i])
	}
	f.ready()
}

func TestServerNATSAMANRace(t *testing.T) {
	if os.Getenv("NATS_TASK22") != "1" {
		t.Skip("requires explicit Task22 disposable fixture")
	}
	filename, provider := newAMANHTTPFixture(t)
	f := newEntrypointFixture(t, true)
	f.changeEnvironment(map[string]string{"NAVIGATION_SOURCE": "airacnet", "NAVIGATION_TERMINAL_GEOMETRY_PATH": filename, "AMAN_MODE": "authoritative", "AMAN_SOURCE_MODE": "euroscope", "AMAN_ENABLED_AIRPORTS": "EKCH", "AMAN_RECONCILIATION_INTERVAL": "1s", "AMAN_SURVEILLANCE_INTERVAL": "1s", "TASK22_AIRAC_URL": provider + "/api/v1", "TASK22_WIND_URL": provider + "/wind"})
	name := "TASK22-AMAN-" + strings.ToUpper(uuid.NewString())
	plugins := []*faultSocket{f.plugin(0, name, "111111", "EKCH_FMP"), f.plugin(1, name, "222222")}
	ref := sessionFaultRef(f.session(name))
	airport := &pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: "EKCH"}}}
	f.await("elected master", func() bool { state, err := f.projection.Read(ref); return err == nil && state.Master.GetCid() != "" })
	state := f.state(ref)
	master := 0
	if state.Master.Cid == "222222" {
		master = 1
	}
	sendEntrypointFrame(t, plugins[master].conn, &es.Envelope{SessionId: ref.GetSession().Id, CommandId: uuid.NewString(), OwnerEpoch: state.Owner.Epoch, MasterEpoch: state.Master.Epoch, Event: &es.Envelope_Sync{Sync: &es.SyncEvent{Strips: []*es.Strip{{Callsign: "SAS123", Origin: "EHAM", Destination: "EKCH", AircraftType: "A320", AssignedSquawk: "1001", Runway: "22L", Star: "SOK1P", Route: "DCT SOK", HasFp: true, Position: &es.Position{Lat: 55.3, Lon: 11.3, Altitude: 12000}, TrackingController: "EKCH_FMP"}}, Runways: []*es.Runway{{Name: "22L", Arrival: true}, {Name: "22R", Departure: true}}}}})
	f.await("real provider policy produces authoritative AMAN", func() bool {
		state, err := f.projection.Read(airport)
		if err != nil {
			return false
		}
		board, err := cluster.ReadAmanBoard(state)
		return err == nil && board.Airport.GetAuthoritative() && board.Airport.GetHealth().GetReady() && len(board.Flights) == 1
	})
	fronts := []*websocket.Conn{f.front(0, name), f.front(1, name)}
	board, err := cluster.ReadAmanBoard(f.state(airport))
	require.NoError(t, err)
	ids := []string{uuid.NewString(), uuid.NewString()}
	at := timestamppb.New(time.Now().Add(time.Minute))
	var writes sync.WaitGroup
	for node := 0; node < 2; node++ {
		writes.Add(1)
		go func(node int) {
			defer writes.Done()
			sendEntrypointFrame(t, fronts[node], faultCommand(ids[node], &pb.ClientCommand{Action: &pb.ClientCommand_Aman{Aman: &pb.AmanAction{ExpectedAirportRevision: board.Airport.Revision, Change: &pb.AmanAction_SetRate{SetRate: &pb.AmanRate{RunwayGroupId: "SOUTH", ArrivalsPerHour: uint32(18 + node*6), EffectiveAt: at}}}}}, nil))
		}(node)
	}
	writes.Wait()
	f.await("both AMAN raced outcomes durable", func() bool { return f.state(airport).Ledger[ids[0]] != nil && f.state(airport).Ledger[ids[1]] != nil })
	accepted := f.state(airport)
	success, conflict := 0, 0
	for _, id := range ids {
		outcome := accepted.Ledger[id]
		if outcome.Status == pb.CommandOutcome_SUCCEEDED {
			success++
		}
		if outcome.Status == pb.CommandOutcome_FAILED && outcome.ReasonCode == "REVISION_CONFLICT" {
			conflict++
		}
		t.Logf("AMAN_RACE id=%s status=%s reason=%s sequence=%d airport_revision=%d", id, outcome.Status, outcome.ReasonCode, outcome.CommittedStreamSequence, outcome.AggregateRevision)
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, conflict)
	require.NotEmpty(t, accepted.EntitiesByKind(pb.EntityKind_AMAN_AUDIT))
	for _, id := range ids {
		f.await("AMAN JSON outcomes agree across processes", func() bool { return f.outcome(0, id) != "" && f.outcome(0, id) == f.outcome(1, id) })
	}
}
