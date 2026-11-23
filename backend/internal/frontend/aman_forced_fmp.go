package frontend

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/config"
	events "FlightStrips/pkg/events/frontend"
)

// forcedFMPRegistry caches the forced FMP grant of each session and writes
// changes through to the optional store, which keeps grants across restarts.
type forcedFMPRegistry struct {
	mu     sync.Mutex
	store  aman.ForcedFMPStore
	loaded map[int32]bool
	grants map[int32]aman.ForcedFMPGrant
}

func newForcedFMPRegistry() *forcedFMPRegistry {
	return &forcedFMPRegistry{loaded: map[int32]bool{}, grants: map[int32]aman.ForcedFMPGrant{}}
}

func (r *forcedFMPRegistry) setStore(store aman.ForcedFMPStore) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.store = store
	r.loaded = map[int32]bool{}
	r.grants = map[int32]aman.ForcedFMPGrant{}
}

func (r *forcedFMPRegistry) get(ctx context.Context, session int32) (aman.ForcedFMPGrant, bool) {
	if r == nil {
		return aman.ForcedFMPGrant{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.loaded[session] && r.store != nil {
		grant, ok, err := r.store.LoadForcedFMP(ctx, session)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to load forced FMP grant", slog.Int("session", int(session)), slog.Any("error", err))
			return aman.ForcedFMPGrant{}, false
		}
		if ok {
			r.grants[session] = grant
		}
	}
	r.loaded[session] = true
	grant, ok := r.grants[session]
	return grant, ok
}

func (r *forcedFMPRegistry) set(ctx context.Context, grant aman.ForcedFMPGrant) error {
	if r == nil {
		return errors.New("forced FMP registry is not configured")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.store != nil {
		if err := r.store.SaveForcedFMP(ctx, grant); err != nil {
			return err
		}
	}
	r.grants[grant.SessionID] = grant
	r.loaded[grant.SessionID] = true
	return nil
}

// clearIf removes the session's grant when match accepts it and reports the
// removed grant.
func (r *forcedFMPRegistry) clearIf(ctx context.Context, session int32, match func(aman.ForcedFMPGrant) bool) (aman.ForcedFMPGrant, bool, error) {
	grant, ok := r.get(ctx, session)
	if !ok || !match(grant) {
		return aman.ForcedFMPGrant{}, false, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if current, ok := r.grants[session]; !ok || current != grant {
		return aman.ForcedFMPGrant{}, false, nil
	}
	if r.store != nil {
		if err := r.store.ClearForcedFMP(ctx, session); err != nil {
			return aman.ForcedFMPGrant{}, false, err
		}
	}
	delete(r.grants, session)
	return grant, true, nil
}

// SetAMANForcedFMPStore makes forced FMP grants survive a backend restart.
func (hub *Hub) SetAMANForcedFMPStore(store aman.ForcedFMPStore) {
	hub.forcedFMP.setStore(store)
}

// forcedFMPEligibleRole reports whether a role may force FMP: approach and
// area (ACC) control positions that are not themselves FMP.
func forcedFMPEligibleRole(role string) bool {
	if role == "" || aman.IsFMPRole(role) {
		return false
	}
	if position, err := config.GetPositionByName(role); err == nil {
		section := strings.ToUpper(strings.TrimSpace(position.Section))
		return section == "APP" || section == "CTR"
	}
	upper := strings.ToUpper(strings.TrimSpace(role))
	return strings.HasSuffix(upper, "_APP") || strings.HasSuffix(upper, "_CTR")
}

func sameCallsign(a, b string) bool {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	return a != "" && strings.EqualFold(a, b)
}

func (hub *Hub) holdsForcedFMP(identity clientIdentity) bool {
	if identity.session == WaitingForEuroscopeConnectionSessionId || identity.readOnly {
		return false
	}
	grant, ok := hub.forcedFMP.get(context.Background(), identity.session)
	return ok && sameCallsign(grant.Callsign, identity.callsign)
}

// fmpOnline reports whether a controller on an FMP position is logged on in
// the session.
func (hub *Hub) fmpOnline(ctx context.Context, session int32) bool {
	if hub.server == nil {
		return false
	}
	repository := hub.server.GetControllerRepository()
	if repository == nil {
		return false
	}
	controllers, err := repository.ListBySession(ctx, session)
	if err != nil {
		slog.WarnContext(ctx, "Failed to list controllers for FMP presence", slog.Int("session", int(session)), slog.Any("error", err))
		return false
	}
	for _, controller := range controllers {
		if controller != nil && !controller.Observer && hub.controllerIsFMP(controller.Callsign, controller.Position) {
			return true
		}
	}
	return false
}

func (hub *Hub) controllerIsFMP(callsign, position string) bool {
	return aman.IsFMPRole(hub.amanRoleForIdentity(clientIdentity{callsign: callsign, position: position}))
}

func (hub *Hub) amanFMPAuthorityEvent(ctx context.Context, client *Client, fmpOnline bool) events.AMANFMPAuthorityEvent {
	identity := client.identitySnapshot()
	data := events.AMANFMPAuthority{FMPOnline: fmpOnline}
	if grant, ok := hub.forcedFMP.get(ctx, identity.session); ok && identity.session != WaitingForEuroscopeConnectionSessionId {
		holder := grant.Callsign
		data.ForcedHolder = &holder
	}
	data.HasFMPAuthority = !identity.readOnly && hub.hasAMANFMPAuthorityForIdentity(client, identity)
	data.Forced = data.HasFMPAuthority && hub.holdsForcedFMP(identity)
	data.CanForce = hub.amanMutations && client.IsAuthenticated() && !identity.readOnly && !fmpOnline && !data.Forced &&
		identity.session != WaitingForEuroscopeConnectionSessionId && forcedFMPEligibleRole(hub.amanRoleForIdentity(identity))
	return events.AMANFMPAuthorityEvent{Type: events.AMANFMPAuthorityType, Version: events.AMANWireVersion, Data: data}
}

func (hub *Hub) sendAMANFMPAuthority(ctx context.Context, client *Client) {
	client.Enqueue(hub.amanFMPAuthorityEvent(ctx, client, hub.fmpOnline(ctx, client.session)))
}

// refreshAMANFMPAuthority asks the hub loop to resend authority and
// coordination state to every client of the session.
func (hub *Hub) refreshAMANFMPAuthority(session int32) {
	select {
	case hub.amanAuthorityRefresh <- session:
	default:
		slog.Warn("Dropping duplicate AMAN FMP authority refresh", slog.Int("session", int(session)))
	}
}

// publishAMANFMPAuthority runs on the hub loop, which owns hub.clients.
func (hub *Hub) publishAMANFMPAuthority(ctx context.Context, session int32) {
	fmpOnline := hub.fmpOnline(ctx, session)
	for client := range hub.clients {
		if client.session != session || client.initialPending {
			continue
		}
		client.Enqueue(hub.amanFMPAuthorityEvent(ctx, client, fmpOnline))
		if hub.amanCoordination != nil {
			hub.sendAMANCoordinationSnapshot(ctx, client)
		}
	}
}

// resetForcedFMPOnControllerOnline ends a forced grant when an FMP logs on.
func (hub *Hub) resetForcedFMPOnControllerOnline(session int32, callsign, position string) {
	if !hub.controllerIsFMP(callsign, position) {
		return
	}
	hub.clearForcedFMP(session, "fmp_online", callsign, func(aman.ForcedFMPGrant) bool { return true })
}

// resetForcedFMPOnControllerOffline ends a forced grant when its holder logs off.
func (hub *Hub) resetForcedFMPOnControllerOffline(session int32, callsign string) {
	hub.clearForcedFMP(session, "holder_offline", callsign, func(grant aman.ForcedFMPGrant) bool { return sameCallsign(grant.Callsign, callsign) })
}

func (hub *Hub) clearForcedFMP(session int32, reason, callsign string, match func(aman.ForcedFMPGrant) bool) {
	ctx := context.Background()
	grant, cleared, err := hub.forcedFMP.clearIf(ctx, session, match)
	if err != nil {
		slog.Error("Failed to clear forced FMP grant", slog.Int("session", int(session)), slog.String("reason", reason), slog.Any("error", err))
		return
	}
	if !cleared {
		return
	}
	slog.Info("Forced FMP ended", slog.Int("session", int(session)), slog.String("airport", grant.Airport), slog.String("holder", grant.Callsign), slog.String("reason", reason), slog.String("trigger", callsign))
	hub.refreshAMANFMPAuthority(session)
}

func handleAMANForceFMP(ctx context.Context, client *Client, message Message) error {
	var wire events.AMANForcedFMPMessage
	if err := decodeAMANMessage(message, events.AMANForceFMPType, &wire); err != nil {
		return rejectDecodedAMAN(ctx, client, commandIDFromMessage(message), err)
	}
	hub := client.hub
	identity := client.messageIdentity(ctx)
	auth, err := hub.coordinationContextForIdentity(client, identity)
	if err != nil {
		return rejectDecodedAMAN(ctx, client, wire.Data.CommandID, err)
	}
	if identity.session == WaitingForEuroscopeConnectionSessionId {
		return rejectDecodedAMAN(ctx, client, wire.Data.CommandID, &aman.DomainError{Class: aman.ErrorUnauthorized, Message: "FORCE FMP requires a EuroScope session"})
	}
	if aman.IsFMPRole(auth.Role) {
		return rejectDecodedAMAN(ctx, client, wire.Data.CommandID, &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: "FMP positions already hold FMP authority"})
	}
	if !forcedFMPEligibleRole(auth.Role) {
		return rejectDecodedAMAN(ctx, client, wire.Data.CommandID, &aman.DomainError{Class: aman.ErrorUnauthorized, Message: "Only APP and ACC controllers can force FMP"})
	}
	if hub.fmpOnline(ctx, identity.session) {
		return rejectDecodedAMAN(ctx, client, wire.Data.CommandID, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "FMP is online; FORCE FMP is only available while FMP is not manned"})
	}
	previous, hadPrevious := hub.forcedFMP.get(ctx, identity.session)
	grant := aman.ForcedFMPGrant{SessionID: identity.session, Airport: identity.airport, Callsign: strings.TrimSpace(identity.callsign), CID: client.GetCid(), GrantedAt: auth.ReceivedAt}
	if err := hub.forcedFMP.set(ctx, grant); err != nil {
		slog.ErrorContext(ctx, "Failed to store forced FMP grant", slog.Any("error", err))
		return rejectDecodedAMAN(ctx, client, wire.Data.CommandID, &aman.DomainError{Class: aman.ErrorDependencyUnavailable, Message: "FORCE FMP could not be saved"})
	}
	attrs := []any{slog.Int("session", int(identity.session)), slog.String("airport", identity.airport), slog.String("holder", grant.Callsign), slog.String("actor", grant.CID)}
	if hadPrevious && !sameCallsign(previous.Callsign, grant.Callsign) {
		attrs = append(attrs, slog.String("previous_holder", previous.Callsign))
	}
	slog.InfoContext(ctx, "Forced FMP granted", attrs...)
	hub.refreshAMANFMPAuthority(identity.session)
	return nil
}

func handleAMANReleaseForcedFMP(ctx context.Context, client *Client, message Message) error {
	var wire events.AMANForcedFMPMessage
	if err := decodeAMANMessage(message, events.AMANReleaseForcedFMPType, &wire); err != nil {
		return rejectDecodedAMAN(ctx, client, commandIDFromMessage(message), err)
	}
	hub := client.hub
	identity := client.messageIdentity(ctx)
	if !client.IsAuthenticated() || identity.session == WaitingForEuroscopeConnectionSessionId {
		return rejectDecodedAMAN(ctx, client, wire.Data.CommandID, &aman.DomainError{Class: aman.ErrorUnauthorized, Message: "Releasing FORCE FMP requires a EuroScope session"})
	}
	grant, cleared, err := hub.forcedFMP.clearIf(ctx, identity.session, func(grant aman.ForcedFMPGrant) bool {
		return sameCallsign(grant.Callsign, identity.callsign)
	})
	if err != nil {
		slog.ErrorContext(ctx, "Failed to release forced FMP grant", slog.Any("error", err))
		return rejectDecodedAMAN(ctx, client, wire.Data.CommandID, &aman.DomainError{Class: aman.ErrorDependencyUnavailable, Message: "FORCE FMP could not be released"})
	}
	if !cleared {
		return rejectDecodedAMAN(ctx, client, wire.Data.CommandID, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "You do not hold FORCE FMP"})
	}
	slog.InfoContext(ctx, "Forced FMP released", slog.Int("session", int(identity.session)), slog.String("airport", grant.Airport), slog.String("holder", grant.Callsign))
	hub.refreshAMANFMPAuthority(identity.session)
	return nil
}
