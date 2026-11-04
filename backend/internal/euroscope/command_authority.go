package euroscope

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events/euroscope"
	"strings"
)

type commandBinding struct {
	hub          *Hub
	client       *Client
	session      int32
	cid          string
	requireReady bool
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
	if !registered || !b.client.IsAuthenticated() || b.client.isClosed() || (b.client.transport != nil && b.client.transport.root.isClosed()) || b.client.session != b.session || b.client.GetCid() != b.cid || b.client.identitySnapshot().observer || (b.requireReady && !b.hub.IsSessionSynced(b.session)) {
		return shared.RejectAction("initiating own plugin is unavailable")
	}
	return nil
}
func (b *commandBinding) enqueue(event euroscope.OutgoingMessage) error {
	if err := b.Validate(); err != nil {
		return err
	}
	// Enqueue targets the captured pointer. CID reuse cannot redirect this work.
	if !b.client.Enqueue(event) {
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
		if client.session != session || client.GetCid() != cid || client.isClosed() || client.identitySnapshot().observer {
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
	binding := &commandBinding{hub: hub, client: selected, session: session, cid: cid, requireReady: ready}
	if err := binding.Validate(); err != nil {
		return nil, err
	}
	return binding, nil
}

func (hub *Hub) ResolveAutomaticCommandBinding(session int32, preferredCID string) (shared.CommandBinding, error) {
	if preferredCID != "" {
		return hub.captureCommandBinding(session, preferredCID, false)
	}
	master := hub.getMasterClient(session)
	if master == nil || master.session != session {
		return nil, shared.RejectAction("no eligible automatic executor in session")
	}
	binding := &commandBinding{hub: hub, client: master, session: session, cid: master.GetCid()}
	if err := binding.Validate(); err != nil {
		return nil, err
	}
	return binding, nil
}
