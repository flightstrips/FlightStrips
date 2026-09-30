package euroscopebinary

import (
	"fmt"

	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	euroscope "FlightStrips/pkg/events/euroscope"
)

// EffectRenderer converts only reviewed typed effects to revision-2 frames.
// Private text is decrypted at the final socket boundary and is never logged.
func EffectRenderer(secrets cluster.EffectSecrets) func(int32, *pb.EffectRecord) (*euroscope.Envelope, error) {
	return func(sessionID int32, effect *pb.EffectRecord) (*euroscope.Envelope, error) {
		if effect == nil || effect.Status != pb.EffectRecord_DISPATCH_CLAIMED {
			return nil, fmt.Errorf("effect is not claimed")
		}
		frame := &euroscope.Envelope{CommandId: effect.CommandId, SessionId: sessionID,
			OwnerEpoch: effect.OwnerEpoch, MasterEpoch: effect.MasterEpoch}
		switch payload := effect.GetPayload().(type) {
		case *pb.EffectRecord_AmanHoldingEat:
			hold := payload.AmanHoldingEat
			if hold == nil || hold.Callsign == "" || hold.Hold == "" || hold.HoldType != "enroute" {
				return nil, fmt.Errorf("invalid AMAN holding effect")
			}
			frame.Event = &euroscope.Envelope_Hold{Hold: &euroscope.HoldEvent{Callsign: hold.Callsign, Hold: hold.Hold, HoldType: hold.HoldType, HoldEat: hold.Eat}}
		case *pb.EffectRecord_PrivateMessage:
			secret := payload.PrivateMessage
			if secret == nil || secret.Recipient == "" {
				return nil, fmt.Errorf("private message recipient missing")
			}
			body, err := secrets.OpenPrivateMessage(effect.CommandId, effect.TargetCid, secret.ObjectName, secret.Sha256)
			if err != nil {
				return nil, err
			}
			frame.Event = &euroscope.Envelope_SendPrivateMessage{SendPrivateMessage: &euroscope.SendPrivateMessageEvent{Callsign: secret.Recipient, Message: body}}
		case *pb.EffectRecord_Pdc:
			pdc := payload.Pdc
			if pdc == nil {
				return nil, fmt.Errorf("PDC effect missing")
			}
			switch pdc.Action {
			case "ISSUE":
				frame.Event = &euroscope.Envelope_IssuePdcClearance{IssuePdcClearance: &euroscope.IssuePdcClearanceEvent{Callsign: pdc.Callsign, Remarks: pdc.Clearance}}
			case "REVERT_TO_VOICE":
				frame.Event = &euroscope.Envelope_PdcRevertToVoice{PdcRevertToVoice: &euroscope.PdcRevertToVoiceEvent{Callsign: pdc.Callsign}}
			default:
				return nil, fmt.Errorf("unsupported PDC effect")
			}
		case *pb.EffectRecord_SetFlightPlan:
			fpl := payload.SetFlightPlan
			if fpl == nil {
				return nil, fmt.Errorf("flight plan effect missing")
			}
			switch fpl.Field {
			case "ROUTE":
				frame.Event = &euroscope.Envelope_Route{Route: &euroscope.RouteEvent{Callsign: fpl.Callsign, Route: fpl.Value}}
			case "REMARKS":
				frame.Event = &euroscope.Envelope_Remarks{Remarks: &euroscope.RemarksEvent{Callsign: fpl.Callsign, Remarks: fpl.Value}}
			case "SID":
				frame.Event = &euroscope.Envelope_Sid{Sid: &euroscope.SidEvent{Callsign: fpl.Callsign, Sid: fpl.Value}}
			case "RUNWAY":
				frame.Event = &euroscope.Envelope_AircraftRunway{AircraftRunway: &euroscope.AircraftRunwayEvent{Callsign: fpl.Callsign, Runway: fpl.Value}}
			case "EOBT":
				frame.Event = &euroscope.Envelope_Eobt{Eobt: &euroscope.EobtEvent{Callsign: fpl.Callsign, Eobt: fpl.Value}}
			case "AIRCRAFT_TYPE":
				frame.Event = &euroscope.Envelope_AircraftInfo{AircraftInfo: &euroscope.AircraftInfoEvent{Callsign: fpl.Callsign, AircraftType: fpl.Value}}
			case "STAND":
				frame.Event = &euroscope.Envelope_Stand{Stand: &euroscope.StandEvent{Callsign: fpl.Callsign, Stand: fpl.Value}}
			default:
				return nil, fmt.Errorf("unsupported flight plan effect")
			}
		case *pb.EffectRecord_Cdm:
			cdm := payload.Cdm
			if cdm == nil || cdm.Action != "SET_TOBT" {
				return nil, fmt.Errorf("unsupported CDM effect")
			}
			frame.Event = &euroscope.Envelope_CdmTobtUpdate{CdmTobtUpdate: &euroscope.CdmTobtUpdateEvent{Callsign: cdm.Callsign, Tobt: cdm.Value}}
		case *pb.EffectRecord_Coordination:
			coordination := payload.Coordination
			if coordination == nil || coordination.Action != "HANDOVER" {
				return nil, fmt.Errorf("unsupported coordination effect")
			}
			frame.Event = &euroscope.Envelope_CoordinationHandover{CoordinationHandover: &euroscope.CoordinationHandoverEvent{Callsign: coordination.Callsign, TargetCallsign: coordination.TargetCid}}
		default:
			return nil, fmt.Errorf("unsupported effect payload")
		}
		return frame, nil
	}
}
