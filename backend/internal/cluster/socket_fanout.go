package cluster

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

// LocalSessionSocket is the candidate socket boundary. Both frontend and
// EuroScope sockets get the same committed projection changes on their own
// backend; protocol adapters render them to their respective binary frames.
type LocalSessionSocket struct {
	OnInitial func(session, airport *Aggregate) error
	OnDelta   func(*pb.FrontendDelta) error
	OnRole    func(role string, masterEpoch, ownerEpoch uint64) error
	OnEffect  func(*pb.EffectRecord) error
	Close     func()
}

type socketEntry struct {
	presence  *pb.ClientPresence
	socket    LocalSessionSocket
	cancel    context.CancelFunc
	delivered map[string]bool
}

type SessionFanout struct {
	NC         *nats.Conn
	Projection *Projection
	NodeID     string
	mu         sync.Mutex
	sockets    map[string]*socketEntry
}

// Attach takes both checkpoints before publishing presence. Deltas produced
// while OnInitial writes are buffered by the projection's listeners.
func (f *SessionFanout) Attach(ctx context.Context, lease ClientPresenceLease, socket LocalSessionSocket) (func(), error) {
	if f == nil || f.Projection == nil || f.NodeID == "" || lease.Client == nil ||
		lease.Client.NodeId != f.NodeID || socket.OnInitial == nil || socket.OnDelta == nil || socket.OnRole == nil {
		return nil, fmt.Errorf("invalid socket fanout")
	}
	if err := lease.validate(); err != nil {
		return nil, err
	}
	sessionID := lease.Client.SessionId
	session, sessionDeltas, stopSession, err := f.Projection.SubscribeInitial(sessionRef(sessionID))
	if err != nil {
		return nil, err
	}
	seed := session.Entities[fmt.Sprint(sessionID)]
	if seed == nil || seed.GetValue().GetSession() == nil || seed.GetValue().GetSession().Tombstoned {
		stopSession()
		return nil, fmt.Errorf("session is not active")
	}
	airport := seed.GetValue().GetSession().Airport
	airportRef := &pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: airport}}}
	airportState, airportDeltas, stopAirport, err := f.Projection.SubscribeInitial(airportRef)
	if err != nil {
		stopSession()
		return nil, err
	}
	stop := func() { stopSession(); stopAirport() }
	if err := socket.OnInitial(session, airportState); err != nil {
		stop()
		return nil, err
	}
	if err := socket.OnRole(socketRole(lease.Client, session.Master), masterEpoch(session.Master), ownerEpoch(session.Owner)); err != nil {
		stop()
		return nil, err
	}
	socketCtx, cancel := context.WithCancel(ctx)
	entry := &socketEntry{presence: proto.Clone(lease.Client).(*pb.ClientPresence), socket: socket, cancel: cancel, delivered: map[string]bool{}}
	f.mu.Lock()
	if f.sockets == nil {
		f.sockets = map[string]*socketEntry{}
	}
	if f.sockets[entry.presence.ConnectionId] != nil {
		f.mu.Unlock()
		cancel()
		stop()
		return nil, fmt.Errorf("socket generation already registered")
	}
	f.sockets[entry.presence.ConnectionId] = entry
	f.mu.Unlock()
	if _, err := lease.Renew(socketCtx); err != nil {
		f.detach(entry.presence.ConnectionId, entry)
		stop()
		return nil, err
	}
	var once sync.Once
	var jobs sync.WaitGroup
	closeSocket := func() {
		once.Do(func() {
			cancel()
			f.detach(entry.presence.ConnectionId, entry)
			stop()
			_ = lease.KV.Delete("client." + entry.presence.ConnectionId)
			if socket.Close != nil {
				socket.Close()
			}
		})
	}
	jobs.Add(2)
	go func() {
		defer jobs.Done()
		if err := lease.Run(socketCtx); err != nil && socketCtx.Err() == nil {
			slog.WarnContext(socketCtx, "session socket delivery failed", "stage", "presence_renewal", "error_type", fmt.Sprintf("%T", err))
			closeSocket()
		}
	}()
	go func() {
		defer jobs.Done()
		defer closeSocket()
		lastSession, lastAirport := session.Revision, airportState.Revision
		lastMaster := session.Master
		lastOwner := ownerEpoch(session.Owner)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		updateRole := func() error {
			term, master, err := f.Projection.ReadSessionTerms(sessionID)
			if err != nil {
				return err
			}
			owner := ownerEpoch(term)
			if !proto.Equal(lastMaster, master) || lastOwner != owner {
				if err := socket.OnRole(socketRole(lease.Client, master), masterEpoch(master), owner); err != nil {
					return err
				}
				lastMaster, lastOwner = master, owner
			}
			return nil
		}
		for {
			select {
			case <-socketCtx.Done():
				return
			case <-ticker.C:
				if err := updateRole(); err != nil {
					slog.WarnContext(socketCtx, "session socket delivery failed", "stage", "role_refresh", "error_type", fmt.Sprintf("%T", err))
					return
				}
			case delta, ok := <-sessionDeltas:
				if !ok {
					slog.WarnContext(socketCtx, "session socket delivery failed", "stage", "session_listener_closed")
					return
				}
				if err := deliverDelta(socket.OnDelta, delta, &lastSession); err != nil {
					slog.WarnContext(socketCtx, "session socket delivery failed", "stage", "session_delta", "error_type", fmt.Sprintf("%T", err))
					return
				}
				if err := updateRole(); err != nil {
					slog.WarnContext(socketCtx, "session socket delivery failed", "stage", "role_after_delta", "error_type", fmt.Sprintf("%T", err))
					return
				}
			case delta, ok := <-airportDeltas:
				if !ok {
					slog.WarnContext(socketCtx, "session socket delivery failed", "stage", "airport_listener_closed")
					return
				}
				if err := deliverDelta(socket.OnDelta, delta, &lastAirport); err != nil {
					slog.WarnContext(socketCtx, "session socket delivery failed", "stage", "airport_delta", "error_type", fmt.Sprintf("%T", err))
					return
				}
			}
		}
	}()
	return func() { closeSocket(); jobs.Wait() }, nil
}

func deliverDelta(deliver func(*pb.FrontendDelta) error, delta *pb.FrontendDelta, last *uint64) error {
	if delta == nil || delta.AggregateRevision <= *last {
		return nil
	}
	if delta.AggregateRevision != *last+1 {
		return fmt.Errorf("projection delta gap")
	}
	if err := deliver(delta); err != nil {
		return err
	}
	*last = delta.AggregateRevision
	return nil
}

func socketRole(client *pb.ClientPresence, term *pb.MasterTerm) string {
	if client.Observer {
		return "observer"
	}
	if term != nil && term.ConnectionId == client.ConnectionId && term.Cid == client.Cid {
		return "master"
	}
	return "slave"
}
func masterEpoch(term *pb.MasterTerm) uint64 {
	if term != nil {
		return term.Epoch
	}
	return 0
}
func ownerEpoch(term *pb.OwnerTerm) uint64 {
	if term != nil {
		return term.Epoch
	}
	return 0
}

func (f *SessionFanout) detach(connectionID string, entry *socketEntry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sockets[connectionID] == entry {
		delete(f.sockets, connectionID)
	}
}

// ServeTargeted accepts only typed, already committed dispatch claims.
// The sender never treats a socket callback as proof of durable command success.
func (f *SessionFanout) ServeTargeted(ctx context.Context) error {
	if f == nil || f.NC == nil || f.Projection == nil || f.NodeID == "" {
		return fmt.Errorf("targeted fanout unavailable")
	}
	_, closeSub, err := SubscribeJoined(f.NC, "fs.v1.delivery."+f.NodeID, func(message *nats.Msg) {
		request := &pb.EffectDeliveryRequest{}
		response := &pb.EffectDeliveryReply{}
		if len(message.Data) > 0 && len(message.Data) <= MaxStateBytes && pb.UnmarshalStrict(message.Data, request) == nil {
			effect := request.GetEffect()
			if effect != nil {
				response.CommandId = effect.CommandId
				if effect.DispatchConnectionId != nil && request.ConnectionId == *effect.DispatchConnectionId &&
					request.ClaimStreamSequence > 0 {
					wait, cancel := context.WithTimeout(ctx, time.Second)
					err := f.Projection.WaitApplied(wait, request.ClaimStreamSequence)
					cancel()
					if err == nil && f.deliverLocal(request.SessionId, effect) == nil {
						response.Accepted = true
					}
				}
			}
		}
		if message.Reply != "" {
			data, _ := proto.Marshal(response)
			_ = f.NC.Publish(message.Reply, data)
		}
	})
	if err != nil {
		return err
	}
	defer closeSub()
	err = FlushSubscription(ctx, f.NC)
	if err != nil {
		return err
	}
	<-ctx.Done()
	return ctx.Err()
}

// SendToCID resolves the current node and socket generation from fresh
// FS_PRESENCE, then sends one Protobuf EffectRecord. No retry is made after
// an ambiguous NATS reply; the effect lifecycle records unknown separately.
func (f *SessionFanout) SendToCID(ctx context.Context, sessionID int32, effect *pb.EffectRecord) error {
	if f == nil || f.Projection == nil || f.NC == nil || effect == nil || effect.TargetCid == "" ||
		effect.Status != pb.EffectRecord_DISPATCH_CLAIMED {
		return fmt.Errorf("invalid targeted effect")
	}
	selected, err := f.resolveCID(sessionID, effect)
	if err != nil {
		return err
	}
	state, err := f.Projection.ReadDurable(sessionRef(sessionID))
	if err != nil || state.Effects[effect.CommandId] == nil ||
		!proto.Equal(state.Effects[effect.CommandId], effect) {
		return fmt.Errorf("effect dispatch claim is not applied locally")
	}
	if selected.NodeId == f.NodeID {
		return f.deliverLocal(sessionID, effect)
	}
	data, err := proto.Marshal(&pb.EffectDeliveryRequest{SessionId: sessionID,
		ConnectionId: selected.ConnectionId, Effect: effect, ClaimStreamSequence: state.StreamSequence})
	if err != nil || len(data) > MaxStateBytes {
		return fmt.Errorf("invalid or oversized targeted effect")
	}
	wait, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	reply, err := f.NC.RequestWithContext(wait, "fs.v1.delivery."+selected.NodeId, data)
	if err != nil {
		return err
	}
	response := &pb.EffectDeliveryReply{}
	if pb.UnmarshalStrict(reply.Data, response) != nil ||
		response.CommandId != effect.CommandId || !response.Accepted {
		return fmt.Errorf("target socket did not accept effect")
	}
	return nil
}

func (f *SessionFanout) resolveCID(sessionID int32, effect *pb.EffectRecord) (*pb.ClientPresence, error) {
	if f == nil || f.Projection == nil || effect == nil || effect.TargetCid == "" || effect.DispatchConnectionId == nil {
		return nil, fmt.Errorf("invalid CID target")
	}
	_, entries, err := f.Projection.ObservationSnapshot(sessionID)
	if err != nil {
		return nil, err
	}
	nodes := make(map[string]bool)
	for _, entry := range entries {
		if node := entry.Value.GetNode(); node != nil && node.Ready {
			nodes[node.NodeId] = true
		}
	}
	var selected *pb.ClientPresence
	for _, entry := range entries {
		client := entry.Value.GetClient()
		if client == nil || client.Kind != pb.ClientPresence_EUROSCOPE || client.SessionId != sessionID ||
			client.Cid != effect.TargetCid || client.ConnectedAt == nil || !nodes[client.NodeId] {
			continue
		}
		if effect.DispatchConnectionId != nil && client.ConnectionId != *effect.DispatchConnectionId {
			continue
		}
		if selected == nil || selected.ConnectedAt.AsTime().Before(client.ConnectedAt.AsTime()) ||
			selected.ConnectedAt.AsTime().Equal(client.ConnectedAt.AsTime()) && selected.ConnectionId < client.ConnectionId {
			selected = client
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("target CID has no claimed live generation")
	}
	if effect.DispatchConnectionId == nil || *effect.DispatchConnectionId != selected.ConnectionId {
		return nil, fmt.Errorf("dispatch generation mismatch")
	}
	return proto.Clone(selected).(*pb.ClientPresence), nil
}

func (f *SessionFanout) deliverLocal(sessionID int32, effect *pb.EffectRecord) error {
	if f.NC == nil || f.NC.Status() != nats.CONNECTED || effect.DispatchConnectionId == nil || effect.Status != pb.EffectRecord_DISPATCH_CLAIMED ||
		effect.ResultDeadline == nil || !time.Now().Before(effect.ResultDeadline.AsTime()) {
		return fmt.Errorf("effect is not claimed for a socket")
	}
	connectionID := *effect.DispatchConnectionId
	f.mu.Lock()
	entry := f.sockets[connectionID]
	if entry == nil || entry.presence.SessionId != sessionID || entry.presence.Cid != effect.TargetCid || entry.socket.OnEffect == nil ||
		entry.delivered[effect.CommandId] {
		f.mu.Unlock()
		return fmt.Errorf("target socket is unavailable or already delivered")
	}
	f.mu.Unlock()
	state, err := f.Projection.ReadDurable(sessionRef(sessionID))
	if err != nil {
		return err
	}
	committed := state.Effects[effect.CommandId]
	if committed == nil || !proto.Equal(committed, effect) || state.Owner == nil ||
		committed.OwnerEpoch != state.Owner.Epoch || !time.Now().Before(state.Owner.GetLeaseUntil().AsTime()) {
		return fmt.Errorf("effect dispatch claim is not committed")
	}
	if err := f.Projection.RequireLiveSocket(sessionID, connectionID, entry.presence.Cid, pb.ClientPresence_EUROSCOPE); err != nil {
		return err
	}
	if effect.MasterEpoch != 0 && (state.Master == nil || state.Master.Epoch != effect.MasterEpoch) {
		return fmt.Errorf("effect master epoch changed")
	}
	f.mu.Lock()
	if f.sockets[connectionID] != entry || entry.delivered[effect.CommandId] {
		f.mu.Unlock()
		return fmt.Errorf("target socket changed")
	}
	// Sending is irreversible. Mark before callback so a duplicate NATS
	// request cannot invoke it again after a lost reply.
	entry.delivered[effect.CommandId] = true
	f.mu.Unlock()
	return entry.socket.OnEffect(proto.Clone(effect).(*pb.EffectRecord))
}
