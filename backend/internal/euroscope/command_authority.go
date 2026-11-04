package euroscope

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events/euroscope"
	"strings"
)

type commandBinding struct {
	hub           *Hub
	client        *Client
	session       int32
	cid           string
	requireReady  bool
	automatic     bool
	masterEpoch   uint64
	requireMaster bool
}

func (b *commandBinding) Session() int32 { return b.session }
func (b *commandBinding) CID() string    { return b.cid }
func (b *commandBinding) Same(other shared.CommandBinding) bool {
	o, ok := other.(*commandBinding)
	return ok && b.hub == o.hub && b.client == o.client && b.session == o.session && b.cid == o.cid
}
func (b *commandBinding) Validate() error {
	b.hub.clientsMu.RLock()
	registered := b.hub.clients[b.client]
	b.hub.clientsMu.RUnlock()
	if (b.automatic && b.hub.election(b.session).epoch.Load() != b.masterEpoch) || (b.requireMaster && b.hub.getMasterClient(b.session) != b.client) {
		return shared.RejectAction("automatic master executor changed")
	}
	if !registered || !b.client.IsAuthenticated() || b.client.isClosed() || (b.client.transport != nil && b.client.transport.root.isClosed()) || b.client.session != b.session || b.client.GetCid() != b.cid || b.client.identitySnapshot().observer || (b.client.membership != nil && (!b.client.announced.Load() || !b.client.roleEligible.Load())) || (b.requireReady && !b.hub.IsSessionSynced(b.session)) {
		return shared.RejectAction("initiating own plugin is unavailable")
	}
	return nil
}
func (b *commandBinding) enqueue(event euroscope.OutgoingMessage) error {
	if err := b.Validate(); err != nil {
		return err
	}
	// Enqueue targets the captured instance/member. The wire captures authority
	// here; a later election never relabels a queued operation with a new epoch.
	if b.client.membership != nil {
		scope := b.client.wireContext()
		if b.automatic && scope.MasterEpoch != b.masterEpoch {
			return shared.RejectAction("automatic master executor changed")
		}
		origin := euroscope.CommandOrigin_COMMAND_ORIGIN_HUMAN
		if b.automatic {
			origin = euroscope.CommandOrigin_COMMAND_ORIGIN_AUTOMATIC
		}
		id, err := b.client.registerExecution(event, origin, scope.MasterEpoch)
		if err != nil {
			return err
		}
		outgoing := euroscope.NewExecutionMessage(event, origin, id)
		if !b.client.transport.root.Enqueue(euroscope.NewScopedMessage(scope, outgoing)) {
			return shared.RejectAction("initiating own plugin disconnected")
		}
	} else if !b.client.Enqueue(event) {
		return shared.RejectAction("initiating own plugin disconnected")
	}

	return nil
}
func (b *commandBinding) Send(event euroscope.OutgoingMessage) error {
	if err := b.Validate(); err != nil {
		return err
	}
	if request, ok := event.(euroscope.GenerateSquawkEvent); ok {
		b.hub.squawkThrottle.Enqueue(b.session, queuedSquawkRequest{cid: b.cid, callsign: request.Callsign, binding: b})
		return nil
	}
	return b.enqueue(event)
}

func (hub *Hub) CaptureCommandBinding(session int32, cid string) (shared.CommandBinding, error) {
	return hub.captureCommandBinding(session, cid, true)
}
func (hub *Hub) captureCommandBinding(session int32, cid string, ready bool) (shared.CommandBinding, error) {
	return hub.captureMembershipBinding(session, cid, "", "", ready)
}

// CaptureMembershipBinding is available to authenticated session selection. The
// CID always comes from authentication; instance/member IDs only narrow it.
func (hub *Hub) CaptureMembershipBinding(session int32, cid, instance, membership string) (shared.CommandBinding, error) {
	return hub.captureMembershipBinding(session, cid, instance, membership, true)
}
func (hub *Hub) captureMembershipBinding(session int32, cid, instance, membership string, ready bool) (shared.CommandBinding, error) {
	if session <= 0 || strings.TrimSpace(cid) == "" {
		return nil, shared.RejectAction("own plugin binding is required")
	}
	var selected *Client
	for _, client := range hub.clientsSnapshot() {
		if instance != "" && (client.membership == nil || client.membership.InstanceId != instance) {
			continue
		}
		if membership != "" && (client.membership == nil || client.membership.MembershipId != membership) {
			continue
		}
		if client.session != session || client.GetCid() != cid || client.isClosed() || client.identitySnapshot().observer || (client.membership != nil && (!client.announced.Load() || !client.roleEligible.Load())) {
			continue
		}
		if selected != nil {
			return nil, shared.RejectAction("own plugin binding is ambiguous")
		}
		selected = client
	}
	if selected == nil {
		return nil, shared.RejectAction("own plugin is disconnected")
	}
	binding := &commandBinding{hub: hub, client: selected, session: session, cid: cid, requireReady: ready, automatic: !ready}
	if binding.automatic {
		binding.masterEpoch = hub.election(session).epoch.Load()
	}
	if err := binding.Validate(); err != nil {
		return nil, err
	}
	return binding, nil
}

func (hub *Hub) ResolveAutomaticCommandBinding(session int32, preferredCID string) (shared.CommandBinding, error) {
	master := hub.getMasterClient(session)
	if preferredCID != "" && (master == nil || master.GetCid() != preferredCID) {
		return hub.captureCommandBinding(session, preferredCID, false)
	}
	if master == nil || master.session != session {
		return nil, shared.RejectAction("no eligible automatic executor in session")
	}
	binding := &commandBinding{hub: hub, client: master, session: session, cid: master.GetCid(), automatic: true, requireMaster: true, masterEpoch: hub.election(session).epoch.Load()}
	if err := binding.Validate(); err != nil {
		return nil, err
	}
	return binding, nil
}
