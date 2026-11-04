package euroscope

import (
	"context"
	"sync"
	"sync/atomic"
)

// Each session orders its own authority changes against source application.
// Full sync can block this session's election, never another airport's work.
type sessionElection struct {
	gate   sync.RWMutex
	infoMu sync.Mutex
	epoch  atomic.Uint64
}

func (hub *Hub) election(session int32) *sessionElection {
	value, _ := hub.sessionElections.LoadOrStore(session, &sessionElection{})
	return value.(*sessionElection)
}

func mayPreemptMaster(client *Client) bool {
	return client.definition == nil || client.definition.PreemptHigherPriority()
}

// Commands and typed controller facts keep the authenticated membership even
// when it is not master. Their handlers retain their existing actor/aircraft
// checks; ordinary reconciliation must come from the current master epoch.
func controllerMembershipEvent(kind string) bool {
	switch kind {
	case "cdm_tobt_update", "cdm_asrt_toggle", "cdm_tsac_update", "cdm_deice_update", "cdm_manual_ctot", "cdm_ctot_remove", "cdm_ready", "issue_pdc_clearance", "pdc_revert_to_voice", "send_private_message", "fs_scratch_pad":
		return true
	default:
		return false
	}
}

// MembershipReady is a control/discovery projection, not a heartbeat. A member
// must finish backend restoration/role announcement and its session must have
// a current-epoch complete master inventory before accepting new human edits.
func (hub *Hub) MembershipReady(client *Client) bool {
	return client != nil && !client.isClosed() && client.announced.Load() && client.roleEligible.Load() && hub.IsSessionSynced(client.session)
}

func trackedMembershipEvent(kind string) bool {
	return kind == "runway" || kind == "hold" || kind == "aman_route_fact" || kind == "coordination_received"
}

type sessionApplicationKey struct{}

func (hub *Hub) enterSessionApplication(ctx context.Context, session int32) (context.Context, func()) {
	election := hub.election(session)
	if previous, ok := ctx.Value(sessionApplicationKey{}).(*sessionElection); ok && previous == election {
		return ctx, func() {}
	}
	election.gate.RLock()
	return context.WithValue(ctx, sessionApplicationKey{}, election), election.gate.RUnlock
}
