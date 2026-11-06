package euroscope

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"FlightStrips/internal/shared"
	es "FlightStrips/pkg/events/euroscope"
	fe "FlightStrips/pkg/events/frontend"
	"google.golang.org/protobuf/proto"
)

type MembershipInfo struct {
	fe.BoardDefinition
	Callsign            string `json:"callsign"`
	Position            string `json:"position"`
	InstanceID          string `json:"instance_id"`
	MembershipID        string `json:"membership_id"`
	TransportGeneration uint64 `json:"transport_generation,string"`
	Generation          uint64 `json:"generation,string"`
	SessionID           int32  `json:"session_id"`
	Airport             string `json:"airport"`
	Environment         string `json:"environment"`
	Role                string `json:"role"`
	ReadOnly            bool   `json:"read_only"`
	MasterEpoch         uint64 `json:"master_epoch,string"`
	Ready               bool   `json:"ready"`
}
type PluginPresence struct {
	Root                *Client          `json:"-"`
	InstanceID          string           `json:"instance_id"`
	TransportGeneration uint64           `json:"transport_generation,string"`
	CID                 string           `json:"-"`
	Evidence            *es.ControlEvent `json:"control"`
	EvidenceAt          time.Time        `json:"evidence_at"`
	EvidenceRevision    uint64           `json:"-"`
	Memberships         []MembershipInfo `json:"memberships"`
}

// ControlPresence is for trusted in-process coverage coordination. It creates
// no controller rows and has no upstream acquisition worker of its own.
func (hub *Hub) ControlPresence() []PluginPresence {
	hub.clientsMu.RLock()
	roots := make([]*Client, 0, len(hub.transports))
	for root := range hub.transports {
		roots = append(roots, root)
	}
	hub.clientsMu.RUnlock()
	result := make([]PluginPresence, 0, len(roots))
	for _, root := range roots {
		if root.isClosed() || !root.IsAuthenticated() {
			continue
		}
		t := root.transport
		t.mu.RLock()
		evidence := proto.Clone(&t.evidence).(*es.ControlEvent)
		at := t.evidenceAt
		revision := t.evidenceRevision
		t.mu.RUnlock()
		presence := PluginPresence{Root: root, InstanceID: t.instance, TransportGeneration: t.generation, CID: root.GetCid(), Evidence: evidence, EvidenceAt: at, EvidenceRevision: revision, Memberships: []MembershipInfo{}}
		for _, member := range t.snapshot() {
			if member.isClosed() {
				continue
			}
			scope := member.wireContext()
			role := "slave"
			if member.identitySnapshot().observer {
				role = "observer"
			} else if hub.getMasterClient(member.session) == member {
				role = "master"
			}
			board := member.boardDefinition()
			identity := member.identitySnapshot()
			ready := hub.MembershipReady(member)
			presence.Memberships = append(presence.Memberships, MembershipInfo{InstanceID: t.instance, MembershipID: scope.MembershipId, TransportGeneration: t.generation, Generation: scope.Generation, SessionID: scope.SessionId, Airport: scope.Airport, Environment: scope.Environment, Role: role, Ready: ready, ReadOnly: role == "observer" || !ready, MasterEpoch: scope.MasterEpoch, BoardDefinition: board, Callsign: identity.callsign, Position: identity.position})
		}
		sort.Slice(presence.Memberships, func(i, j int) bool { return presence.Memberships[i].SessionID < presence.Memberships[j].SessionID })
		result = append(result, presence)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].InstanceID < result[j].InstanceID })
	return result
}

func (hub *Hub) DiscoverMemberships(user shared.AuthenticatedUser) ([]PluginPresence, error) {
	if !user.IsValid() {
		return nil, shared.RejectAction("authentication is required")
	}
	result := []PluginPresence{}
	for _, presence := range hub.ControlPresence() {
		if presence.CID == user.GetCid() {
			presence.Root = nil
			result = append(result, presence)
		}
	}
	return result, nil
}

// MembershipDiscovery authorizes against the bearer token's CID. Query/body
// identity claims never select another controller's sessions.
func (hub *Hub) MembershipDiscovery(auth shared.AuthenticationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || auth == nil {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		user, err := auth.Validate(parts[1])
		if err != nil {
			http.Error(w, "invalid authentication", http.StatusUnauthorized)
			return
		}
		transports, err := hub.DiscoverMemberships(user)
		if err != nil {
			http.Error(w, "invalid authentication", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Transports []PluginPresence `json:"transports"`
		}{transports})
	}
}

func (c *Client) boardDefinition() fe.BoardDefinition {
	board := fe.BoardDefinition{Actions: []string{}}
	if c.definition != nil {
		board.DefinitionRevision = c.definition.Revision()
		board.View = c.definition.ViewID()
		board.Capabilities = fe.BoardCapabilities{
			CDM:       c.definition.ServiceEnabled("cdm"),
			Stands:    c.definition.ServiceEnabled("stand_assignment"),
			TextPilot: c.definition.ServiceEnabled("text_pilot"),
			PDC:       c.definition.ServiceEnabled("pdc"),
			AMAN:      c.definition.ServiceEnabled("aman"),
		}
	}
	if c.workflow != nil {
		board.Actions = c.workflow.ActionCatalog()
	}
	return board
}
