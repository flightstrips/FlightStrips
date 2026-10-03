package cluster

import (
	"FlightStrips/internal/shared"
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
	masterObservation := false
	requireSync := true
	switch envelope.GetEvent().(type) {
	case *euroscope.Envelope_Sync:
		masterObservation, requireSync = true, false
	case *euroscope.Envelope_AircraftPositionUpdate, *euroscope.Envelope_AircraftDisconnect,
		*euroscope.Envelope_StripUpdate,
		*euroscope.Envelope_Squawk, *euroscope.Envelope_RequestedAltitude,
		*euroscope.Envelope_ClearedAltitude, *euroscope.Envelope_CommunicationType,
		*euroscope.Envelope_GroundState, *euroscope.Envelope_ClearedFlag,
		*euroscope.Envelope_Heading, *euroscope.Envelope_Stand,
		*euroscope.Envelope_Route, *euroscope.Envelope_Remarks,
		*euroscope.Envelope_AircraftInfo, *euroscope.Envelope_AircraftInfoRemarks,
		*euroscope.Envelope_Sid, *euroscope.Envelope_AircraftRunway,
		*euroscope.Envelope_AssignedSquawk:
		masterObservation = true
	}
	if masterObservation {
		if p == nil || sessionID < 1 || connectionID == "" || cid == "" || envelope.MasterEpoch == 0 {
			return fmt.Errorf("invalid socket authority")
		}
		if err := p.sessionReadHealth(sessionRef(sessionID)); err != nil {
			return err
		}
		p.mu.RLock()
		defer p.mu.RUnlock()
		if err := p.healthLocked(); err != nil {
			return err
		}
		state := p.acceptedStateLocked(fmt.Sprintf("fs.v1.state.session.%d", sessionID))
		if envelope.SessionId != sessionID || state == nil || state.Owner == nil || envelope.OwnerEpoch != state.Owner.Epoch {
			p.staleEpochs.Add(1)
			return fmt.Errorf("master observation is missing current session terms")
		}
		return p.requireMasterInboundLocked(sessionID, connectionID, cid, envelope.MasterEpoch, requireSync)
	}
	if err := p.RequireLiveSocket(sessionID, connectionID, cid, pb.ClientPresence_EUROSCOPE); err != nil {
		return err
	}
	state, err := p.Read(sessionRef(sessionID))
	if err != nil {
		return err
	}
	if result := envelope.GetCommandResult(); result != nil {
		effect, err := state.LookupEffect(result.CommandId)
		if err != nil {
			return err
		}
		if envelope.SessionId != sessionID || envelope.CommandId != result.CommandId || effect == nil ||
			effect.TargetCid != cid || effect.DispatchConnectionId == nil ||
			envelope.OwnerEpoch != effect.OwnerEpoch || envelope.MasterEpoch != effect.MasterEpoch ||
			(effect.Status != pb.EffectRecord_DISPATCH_CLAIMED && effect.Status != pb.EffectRecord_EXECUTED && effect.Status != pb.EffectRecord_FAILED) {
			return fmt.Errorf("result does not match a committed dispatch claim")
		}
		return nil
	}
	if state.Owner == nil || (envelope.OwnerEpoch != 0 && state.Owner.Epoch != envelope.OwnerEpoch) {
		p.staleEpochs.Add(1)
		return fmt.Errorf("stale session owner epoch")
	}
	switch envelope.GetEvent().(type) {
	case *euroscope.Envelope_Runway:
		if envelope.SessionId != sessionID || envelope.OwnerEpoch != state.Owner.Epoch {
			return fmt.Errorf("runway report is missing current session terms")
		}
		return nil
	case *euroscope.Envelope_Sync:
		if envelope.SessionId != sessionID || envelope.OwnerEpoch != state.Owner.Epoch {
			return fmt.Errorf("sync is missing current session terms")
		}
		return p.RequireMasterInbound(sessionID, connectionID, cid, envelope.MasterEpoch, false)
	case *euroscope.Envelope_AircraftPositionUpdate,
		*euroscope.Envelope_AircraftDisconnect,
		*euroscope.Envelope_StripUpdate,
		*euroscope.Envelope_Squawk, *euroscope.Envelope_RequestedAltitude,
		*euroscope.Envelope_ClearedAltitude, *euroscope.Envelope_CommunicationType,
		*euroscope.Envelope_GroundState, *euroscope.Envelope_ClearedFlag,
		*euroscope.Envelope_Heading, *euroscope.Envelope_Stand,
		*euroscope.Envelope_Route, *euroscope.Envelope_Remarks,
		*euroscope.Envelope_AircraftInfo, *euroscope.Envelope_AircraftInfoRemarks,
		*euroscope.Envelope_Sid, *euroscope.Envelope_AircraftRunway,
		*euroscope.Envelope_AssignedSquawk:
		if envelope.SessionId != sessionID || envelope.OwnerEpoch != state.Owner.Epoch {
			return fmt.Errorf("master observation is missing current session terms")
		}
		return p.RequireMasterInbound(sessionID, connectionID, cid, envelope.MasterEpoch, true)
	case *euroscope.Envelope_Hold:
		controller := state.Indexes[pb.EntityKind_CONTROLLER][cid].GetValue().GetController()
		strip := state.Indexes[pb.EntityKind_STRIP][envelope.GetHold().Callsign].GetValue().GetStrip()
		if controller == nil || controller.Observer || strip == nil {
			return fmt.Errorf("hold requires an operational controller and strip")
		}
		if shared.IsTrackingController(controller.Callsign, strip.TrackingController) {
			return nil
		}
		return p.RequireMasterInbound(sessionID, connectionID, cid, envelope.MasterEpoch, true)
	case *euroscope.Envelope_TrackingControllerChanged:
		controller := state.Indexes[pb.EntityKind_CONTROLLER][cid].GetValue().GetController()
		if controller == nil || controller.Observer {
			return fmt.Errorf("tracking requires an operational controller")
		}
		return nil
	case *euroscope.Envelope_Token, *euroscope.Envelope_Login:
		return fmt.Errorf("authentication frames are not operational observations")
	default:
		// Other operational frames can originate from a tracking controller
		// or an observer where the domain permits it. Their authenticated
		// socket and owner epoch still have to be current.
		return nil
	}
}
