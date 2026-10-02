package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"errors"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
	"sync"
	"testing"
	"time"
)

func authorityProjection() (*Projection, *es.Envelope) {
	now := time.Now()
	state := NewAggregate(sessionRef(1))
	state.Owner = &pb.OwnerTerm{NodeId: "node-a", Epoch: 2}
	state.Master = &pb.MasterTerm{ConnectionId: "connection", Cid: "controller", Epoch: 3, OwnerEpoch: 2}
	state.Sync = &pb.SessionSync{ConnectionId: "connection", MasterEpoch: 3, CompletedAt: timestamppb.New(now)}
	p := &Projection{states: map[string]*Aggregate{"fs.v1.state.session.1": state}, started: true, startedAt: now.Add(-time.Minute), checked: now, positionReady: true, presenceReady: true, syncFresh: map[string]bool{"fs.v1.state.session.1": true}, presence: map[string]KVPresence{}}
	putTestPresence(p, &pb.ClientPresence{ConnectionId: "connection", NodeId: "node-a", SessionId: 1, Cid: "controller", Kind: pb.ClientPresence_EUROSCOPE})
	frame := &es.Envelope{SessionId: 1, OwnerEpoch: 2, MasterEpoch: 3, Event: &es.Envelope_AircraftPositionUpdate{AircraftPositionUpdate: &es.AircraftPositionUpdateEvent{Callsign: "SAS1", Lat: 55, Lon: 12}}}
	return p, frame
}
func TestEuroScopeMasterAuthorityRejectsEveryInvalidAuthorityComponent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Projection, *es.Envelope)
	}{
		{"quorum unhealthy", func(p *Projection, _ *es.Envelope) { p.healthErr = errors.New("quorum unavailable") }},
		{"watcher unhealthy", func(p *Projection, _ *es.Envelope) { p.observationErr = errors.New("watcher failed") }},
		{"metadata stale", func(p *Projection, _ *es.Envelope) { p.checked = time.Now().Add(-3 * time.Second) }},
		{"stale owner", func(_ *Projection, f *es.Envelope) { f.OwnerEpoch-- }},
		{"stale master", func(_ *Projection, f *es.Envelope) { f.MasterEpoch-- }},
		{"master belongs to stale owner", func(p *Projection, _ *es.Envelope) { p.states["fs.v1.state.session.1"].Master.OwnerEpoch-- }},
		{"missing sync", func(p *Projection, _ *es.Envelope) { p.states["fs.v1.state.session.1"].Sync = nil }},
		{"replayed sync", func(p *Projection, _ *es.Envelope) { p.syncFresh["fs.v1.state.session.1"] = false }},
		{"expired client presence", func(p *Projection, _ *es.Envelope) {
			v := p.presence["client.connection"]
			v.Observed = time.Now().Add(-11 * time.Second)
			p.presence["client.connection"] = v
		}},
		{"expired node presence", func(p *Projection, _ *es.Envelope) {
			v := p.presence["node.node-a"]
			v.Observed = time.Now().Add(-11 * time.Second)
			p.presence["node.node-a"] = v
		}},
		{"wrong CID", func(p *Projection, _ *es.Envelope) { p.presence["client.connection"].Value.GetClient().Cid = "other" }},
		{"wrong socket generation", func(p *Projection, _ *es.Envelope) { p.states["fs.v1.state.session.1"].Master.ConnectionId = "other" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, f := authorityProjection()
			require.NoError(t, p.ValidateEuroScopeInbound(1, "connection", "controller", f))
			tc.mutate(p, f)
			require.Error(t, p.ValidateEuroScopeInbound(1, "connection", "controller", f))
		})
	}
}
func TestEuroScopeSyncStillRequiresCurrentMasterWithoutPriorSync(t *testing.T) {
	p, f := authorityProjection()
	p.states["fs.v1.state.session.1"].Sync = nil
	f.Event = &es.Envelope_Sync{Sync: &es.SyncEvent{}}
	require.NoError(t, p.ValidateEuroScopeInbound(1, "connection", "controller", f))
	f.MasterEpoch--
	require.Error(t, p.ValidateEuroScopeInbound(1, "connection", "controller", f))
}

func TestEuroScopeAuthorityDoesNotCombineDifferentPublishedTerms(t *testing.T) {
	p, f := authorityProjection()
	// Neither coherent publication authorizes this mixed owner/master pair.
	f.OwnerEpoch = 2
	f.MasterEpoch = 4
	stop := make(chan struct{})
	var jobs sync.WaitGroup
	jobs.Add(1)
	go func() {
		defer jobs.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			for _, term := range []struct{ owner, master uint64 }{{2, 3}, {3, 4}} {
				p.mu.Lock()
				state := p.states["fs.v1.state.session.1"]
				state.Owner = &pb.OwnerTerm{NodeId: "node-a", Epoch: term.owner}
				state.Master = &pb.MasterTerm{ConnectionId: "connection", Cid: "controller", Epoch: term.master, OwnerEpoch: term.owner}
				state.Sync = &pb.SessionSync{ConnectionId: "connection", MasterEpoch: term.master, CompletedAt: timestamppb.Now()}
				p.mu.Unlock()
			}
		}
	}()
	defer func() { close(stop); jobs.Wait() }()
	for i := 0; i < 2000; i++ {
		require.Error(t, p.ValidateEuroScopeInbound(1, "connection", "controller", f), "mixed terms must never acquire authority")
	}
}
