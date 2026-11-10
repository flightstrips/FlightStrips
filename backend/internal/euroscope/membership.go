package euroscope

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	es "FlightStrips/pkg/events/euroscope"
	"google.golang.org/protobuf/proto"
)

// A physical transport owns authentication and one writer. Only its logical
// memberships enter the operational hub registry or refresh controller rows.
type pluginTransport struct {
	root             *Client
	instance         string
	generation       uint64
	cleanupOnce      sync.Once
	mu               sync.RWMutex
	coverageMu       sync.Mutex // control evidence and this instance's membership deltas
	takeovers        map[string]coverageTakeover
	members          map[string]*Client
	evidence         es.ControlEvent
	evidenceAt       time.Time
	evidenceRevision uint64 // monotonic even when two reports share a clock tick
	pendingLegacy    string // retry failed admission, never an intentional removal
}

// HasLiveSourceDemand includes physical control instances waiting to rejoin.
// Membership removal does not remove the transport's demand for staffing/radios.
func (hub *Hub) HasLiveSourceDemand() bool {
	hub.clientsMu.RLock()
	roots := make([]*Client, 0, len(hub.transports))
	for root := range hub.transports {
		roots = append(roots, root)
	}
	hub.clientsMu.RUnlock()
	for _, root := range roots {
		if root.isClosed() || !root.IsAuthenticated() || root.transport == nil {
			continue
		}
		root.transport.mu.RLock()
		live := root.transport.evidence.Environment == "LIVE"
		root.transport.mu.RUnlock()
		if live {
			return true
		}
	}
	return false
}

func newGeneration() uint64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return binary.LittleEndian.Uint64(b[:]) | 1
}

func (hub *Hub) controllerRowLock(airport, environment, callsign string) *sync.Mutex {
	key := strings.ToUpper(airport) + "\x00" + strings.ToUpper(environment) + "\x00" + strings.ToUpper(callsign)
	value, _ := hub.controllerRows.LoadOrStore(key, &sync.Mutex{})
	return value.(*sync.Mutex)
}
func (t *pluginTransport) snapshot() []*Client {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]*Client, 0, len(t.members))
	for _, c := range t.members {
		result = append(result, c)
	}
	return result
}
func (c *Client) wireContext() *es.MembershipContext {
	if c.membership != nil {
		scope := proto.Clone(c.membership).(*es.MembershipContext)
		scope.MasterEpoch = c.hub.election(c.session).epoch.Load()
		return scope
	}
	return &es.MembershipContext{InstanceId: c.transport.instance, TransportGeneration: c.transport.generation}
}

func (hub *Hub) ResolveMessage(root *Client, scope *es.MembershipContext, kind string) (*Client, error) {
	if kind == "login" {
		return nil, shared.RejectAction("membership identity is backend-owned; use transport control")
	}
	if root.transport == nil {
		return nil, shared.RejectAction("plugin transport identity is required")
	}
	if kind == "token" || kind == "control" {
		if scope != nil && (scope.InstanceId != root.transport.instance || scope.TransportGeneration != root.transport.generation || scope.MembershipId != "") {
			return nil, shared.RejectAction("wrong transport context")
		}
		return root, nil
	}
	if scope == nil {
		return nil, shared.RejectAction("membership context is required")
	}
	root.transport.mu.RLock()
	member := root.transport.members[scope.MembershipId]
	root.transport.mu.RUnlock()
	if member == nil || !member.matches(scope) {
		return nil, shared.RejectAction("stale or wrong-session membership")
	}
	return member, nil
}
func (c *Client) matches(scope *es.MembershipContext) bool {
	if c.membership == nil || scope == nil || c.isClosed() || c.transport.root.isClosed() {
		return false
	}
	want := c.membership
	return scope.InstanceId == want.InstanceId && scope.TransportGeneration == want.TransportGeneration && scope.MembershipId == want.MembershipId && scope.SessionId == want.SessionId && scope.Airport == want.Airport && scope.Environment == want.Environment && scope.Generation == want.Generation
}
func (c *Client) ApplyScopedMessage(ctx context.Context, scope *es.MembershipContext, kind string, payload []byte, origin es.CommandOrigin, executionID string, apply func(context.Context) error) error {
	if c.membership == nil {
		return apply(ctx)
	}
	// Position batches release their execution slot while awaiting SQL. A later
	// report must not occupy that slot while waiting behind removal or election.
	resume := shared.SuspendPositionExecution(ctx)
	c.applyMu.RLock()
	defer c.applyMu.RUnlock()
	if !c.matches(scope) || !c.IsAuthenticated() {
		resume()
		return shared.RejectAction("membership was removed or reconnected before application")
	}
	kind = strings.TrimPrefix(strings.ToLower(kind), "event_")
	election := c.hub.election(c.session)
	var release func()
	ctx, release = c.hub.enterSessionApplication(ctx, c.session)
	resume()
	defer release()
	if !c.matches(scope) || ctx.Err() != nil {
		return shared.RejectAction("membership is no longer available")
	}
	if c.validateExecutionEcho(kind, payload, origin, executionID) {
		ctx = context.WithValue(ctx, executionResultKey{}, true)
		if origin == es.CommandOrigin_COMMAND_ORIGIN_HUMAN {
			var err error
			ctx, err = shared.WithHumanCommand(ctx, &commandBinding{hub: c.hub, client: c, session: c.session, cid: c.GetCid()})
			if err != nil {
				return err
			}
		}
		return apply(ctx)
	}
	if executionID != "" || origin != es.CommandOrigin_COMMAND_ORIGIN_UNSPECIFIED {
		return shared.RejectAction("SDK result does not match an authorized execution")
	}
	if kind == "frequency_message_result" {
		return shared.RejectAction("frequency result has no matching membership request")
	}
	if !controllerMembershipEvent(kind) {
		if scope.MasterEpoch != election.epoch.Load() || (!trackedMembershipEvent(kind) && c.hub.getMasterClient(c.session) != c) {
			return shared.RejectAction("stale master observation")
		}
	}
	return apply(ctx)
}

var environmentName = regexp.MustCompile(`^(LIVE|SWEATBOX(?:_[A-Za-z0-9-]+)?|PLAYBACK_[A-Za-z0-9-]+)$`)

func handleControlEvent(ctx context.Context, root *Client, message Message) error {
	var event es.ControlEvent
	if err := message.ProtoUnmarshal(&event); err != nil {
		return err
	}
	// Admission already canonicalizes session names. Keep control evidence in
	// that same namespace for replacement attachment and simulation providers.
	event.Environment = strings.ToUpper(event.Environment)
	if !environmentName.MatchString(event.Environment) {
		return shared.RejectAction("invalid physical-instance environment")
	}
	t := root.transport
	if t == nil || root.membership != nil || root.isClosed() {
		return shared.RejectAction("control update requires physical transport")
	}
	t.coverageMu.Lock()
	defer t.coverageMu.Unlock()
	defer root.hub.notifyCoverage()
	t.mu.Lock()
	previous := t.evidence
	pendingLegacy := t.pendingLegacy
	t.evidence = *proto.Clone(&event).(*es.ControlEvent)
	t.evidenceAt = time.Now().UTC()
	t.evidenceRevision++
	t.mu.Unlock()
	identityChanged := previous.Environment != "" && (previous.Environment != event.Environment || previous.Callsign != event.Callsign || previous.Observer != event.Observer)
	if identityChanged {
		for _, c := range t.snapshot() {
			root.hub.RemoveMembership(c, "physical identity changed")
		}
	}
	root.updateIdentity(event.PrimaryFrequency, event.Callsign, event.Observer, event.LocalIp)
	validFrequency := strings.TrimSpace(event.PrimaryFrequency) != "" && event.PrimaryFrequency != "199.998"
	for _, c := range t.snapshot() {
		if c.definition != nil && c.definition.MembershipPolicy() == "legacy-single-airport/v1" && (c.airport != event.RelevantAirport || !validFrequency || event.Callsign == "") {
			root.hub.RemoveMembership(c, "legacy association no longer available")
			continue
		}
		before := c.identitySnapshot()
		if validFrequency && before.position != event.PrimaryFrequency {
			c.applyMu.Lock()
			if !c.isClosed() {
				if err := root.hub.updateMembershipPosition(ctx, c, &event); err != nil {
					c.applyMu.Unlock()
					return err
				}
			}
			c.applyMu.Unlock()
			root.hub.reconsiderMasterAfterLogin(c)
			if err := root.hub.server.UpdateLayouts(c.session); err != nil {
				return err
			}
			root.hub.server.GetFrontendHub().CidOnline(c.session, c.GetCid())
		} else if before.localIP != event.LocalIp {
			c.updateIdentity(before.position, before.callsign, before.observer, event.LocalIp)
		}
		root.hub.setClientLocalIP(c.session, c.GetCid(), event.LocalIp)
	}
	changed := previous.Environment == "" || previous.Environment != event.Environment || previous.Callsign != event.Callsign || previous.PrimaryFrequency != event.PrimaryFrequency || previous.Observer != event.Observer || previous.RelevantAirport != event.RelevantAirport
	// Only legacy association is selected by this existing plugin hint. New
	// responsibilities/takeover decisions belong to the MA-04 coordinator.
	if (changed || pendingLegacy == event.RelevantAirport) && validFrequency && event.Callsign != "" {
		if definition, ok := root.hub.airports.Lookup(event.RelevantAirport); ok && definition.MembershipPolicy() == "legacy-single-airport/v1" {
			_, err := root.hub.JoinMembership(root, event.RelevantAirport)
			t.mu.Lock()
			if err != nil {
				t.pendingLegacy = event.RelevantAirport
			} else {
				t.pendingLegacy = ""
			}
			t.mu.Unlock()
			return err
		}
	}
	if !validFrequency || event.RelevantAirport != pendingLegacy {
		t.mu.Lock()
		t.pendingLegacy = ""
		t.mu.Unlock()
	}
	return nil
}

// JoinMembership is a backend coordinator operation, never a client-selected
// session or CID. Definitions/workflows were bound once during startup.
func (hub *Hub) JoinMembership(root *Client, airport string) (*Client, error) {
	if root == nil || root.transport == nil || root.membership != nil || !root.IsAuthenticated() || root.isClosed() {
		return nil, shared.RejectAction("transport is unavailable")
	}
	if hub.airports == nil || hub.workflows == nil {
		return nil, shared.RejectAction("airport registry unavailable")
	}
	definition, ok := hub.airports.Lookup(airport)
	if !ok {
		return nil, shared.RejectAction("airport is not enabled")
	}
	workflow, ok := hub.workflows.Lookup(airport)
	if !ok {
		return nil, shared.RejectAction("airport workflow is not enabled")
	}
	t := root.transport
	t.mu.Lock()
	defer t.mu.Unlock()
	if root.isClosed() || root.IsDraining() {
		return nil, shared.RejectAction("transport is unavailable")
	}
	for _, member := range t.members {
		if member.airport == airport {
			return member, nil
		}
	}
	evidence := t.evidence
	if !environmentName.MatchString(evidence.Environment) || evidence.Callsign == "" {
		return nil, shared.RejectAction("transport evidence is not ready")
	}
	login := &es.LoginEvent{Airport: airport, Connection: evidence.Environment, Position: evidence.PrimaryFrequency, Callsign: evidence.Callsign, Range: evidence.Range, Observer: evidence.Observer, LocalIp: evidence.LocalIp}
	rowLock := hub.controllerRowLock(airport, evidence.Environment, evidence.Callsign)
	rowLock.Lock()
	defer rowLock.Unlock()
	if !root.IsAuthenticated() || root.isClosed() || root.IsDraining() {
		return nil, shared.RejectAction("transport is unavailable")
	}
	event, session, err := hub.admitMembershipController(context.Background(), login, root.userSnapshot())
	if err != nil {
		return nil, err
	}
	generation := newGeneration()
	scope := &es.MembershipContext{InstanceId: t.instance, TransportGeneration: t.generation, MembershipId: fmt.Sprintf("%x", generation), SessionId: session, Airport: airport, Environment: event.Connection, Generation: generation}
	member := &Client{transport: t, membership: scope, session: session, sessionName: event.Connection, send: make(chan events.OutgoingMessage, clientSendQueueSize), closed: make(chan struct{}), hub: hub, user: root.userSnapshot(), position: event.Position, callsign: event.Callsign, airport: airport, version: root.version, observer: event.Observer, localIP: event.LocalIp, definition: definition, workflow: workflow}
	member.frequencyMessages.Store(root.frequencyMessages.Load())
	t.members[scope.MembershipId] = member
	hub.clientsMu.Lock()
	hub.clients[member] = true
	hub.clientsMu.Unlock()
	if !event.Observer {
		hub.markPendingOnlineOrchestration(session, event.Callsign)
	}
	hub.OnRegister(member)
	hub.notifyCoverage()
	return member, nil
}

func (hub *Hub) RemoveMembership(member *Client, reason string) {
	if member == nil || member.membership == nil {
		return
	}
	member.applyMu.Lock()
	select {
	case <-member.closed:
		member.applyMu.Unlock()
		return
	default:
	}
	member.Enqueue(es.MembershipRemovedEvent{Reason: reason})
	member.Close()
	member.aircraftSelectionMu.Lock()
	member.aircraftSelection = nil
	member.aircraftSelectionMu.Unlock()
	t := member.transport
	t.mu.Lock()
	delete(t.members, member.membership.MembershipId)
	t.mu.Unlock()
	hub.clientsMu.Lock()
	delete(hub.clients, member)
	hub.clientsMu.Unlock()
	member.applyMu.Unlock()
	hub.OnUnregister(member)
	hub.notifyCoverage()
	if notifier, ok := hub.server.GetFrontendHub().(interface {
		PluginMembershipDisconnected(int32, string, string, string)
	}); ok {
		notifier.PluginMembershipDisconnected(member.session, member.GetCid(), member.membership.InstanceId, member.membership.MembershipId)
		return
	}
	for _, other := range hub.clientsSnapshot() {
		if other.session == member.session && other.GetCid() == member.GetCid() && !other.isClosed() {
			return
		}
	}
	if notifier, ok := hub.server.GetFrontendHub().(interface{ MembershipDisconnected(int32, string) }); ok {
		notifier.MembershipDisconnected(member.session, member.GetCid())
	}
}
func (hub *Hub) removeTransport(root *Client) {
	for _, member := range root.transport.snapshot() {
		hub.RemoveMembership(member, "transport disconnected")
	}
	hub.clientsMu.Lock()
	delete(hub.transports, root)
	hub.clientsMu.Unlock()
}

func (hub *Hub) membershipDisconnected(session int32, cid string) {
	if notifier, ok := hub.server.GetFrontendHub().(interface{ MembershipDisconnected(int32, string) }); ok {
		notifier.MembershipDisconnected(session, cid)
		return
	}
	// Narrow fakes predate the scoped notification; production frontend implements it.
	hub.server.GetFrontendHub().CidDisconnect(cid)
}

func (hub *Hub) scheduleTransportRemoval(root *Client) {
	root.Close()
	root.transport.cleanupOnce.Do(func() { go hub.removeTransport(root) })
}
