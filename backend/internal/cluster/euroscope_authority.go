package cluster

import (
	"fmt"

	pb "FlightStrips/pkg/events/cluster"
	euroscope "FlightStrips/pkg/events/euroscope"
)

// ValidateEuroScopeInbound is called after token/login authentication and
// before any candidate adapter handles a frame. The trusted socket generation,
// session and CID come from connection state, never from the envelope.
func (p *Projection) ValidateEuroScopeInbound(sessionID int32, connectionID, cid string, envelope *euroscope.Envelope) error {
	if envelope == nil || envelope.GetEvent() == nil ||
		(envelope.SessionId != 0 && envelope.SessionId != sessionID) {
		return fmt.Errorf("invalid EuroScope envelope authority")
	}
	if err := p.RequireLiveSocket(sessionID, connectionID, cid, pb.ClientPresence_EUROSCOPE); err != nil {
		return err
	}
	state, err := p.Read(sessionRef(sessionID))
	if err != nil {
		return err
	}
	if result := envelope.GetCommandResult(); result != nil {
		effect := state.Effects[result.CommandId]
		if envelope.SessionId != sessionID || envelope.CommandId != result.CommandId || effect == nil ||
			effect.TargetCid != cid || effect.DispatchConnectionId == nil ||
			envelope.OwnerEpoch != effect.OwnerEpoch || envelope.MasterEpoch != effect.MasterEpoch ||
			(effect.Status != pb.EffectRecord_DISPATCH_CLAIMED && effect.Status != pb.EffectRecord_EXECUTED && effect.Status != pb.EffectRecord_FAILED) {
			return fmt.Errorf("result does not match a committed dispatch claim")
		}
		return nil
	}
	if state.Owner == nil || (envelope.OwnerEpoch != 0 && state.Owner.Epoch != envelope.OwnerEpoch) {
		return fmt.Errorf("stale session owner epoch")
	}
	switch envelope.GetEvent().(type) {
	case *euroscope.Envelope_Sync:
		if envelope.SessionId != sessionID || envelope.OwnerEpoch != state.Owner.Epoch {
			return fmt.Errorf("sync is missing current session terms")
		}
		return p.RequireMasterInbound(sessionID, connectionID, cid, envelope.MasterEpoch, false)
	case *euroscope.Envelope_AircraftPositionUpdate,
		*euroscope.Envelope_AircraftDisconnect,
		*euroscope.Envelope_StripUpdate:
		if envelope.SessionId != sessionID || envelope.OwnerEpoch != state.Owner.Epoch {
			return fmt.Errorf("master observation is missing current session terms")
		}
		return p.RequireMasterInbound(sessionID, connectionID, cid, envelope.MasterEpoch, true)
	case *euroscope.Envelope_Token, *euroscope.Envelope_Login:
		return fmt.Errorf("authentication frames are not operational observations")
	default:
		// Other operational frames can originate from a tracking controller
		// or an observer where the domain permits it. Their authenticated
		// socket and owner epoch still have to be current.
		return nil
	}
}
