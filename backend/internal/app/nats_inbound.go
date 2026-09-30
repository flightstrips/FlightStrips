package app

import (
	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"context"
	"fmt"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

func (r *natsRuntime) inbound(ctx context.Context, id int32, connection, cid string, frame *es.Envelope) error {
	state, err := r.projection.Read(sessionNATSRef(id))
	if err != nil {
		return err
	}
	controller := state.Indexes[pb.EntityKind_CONTROLLER][cid].GetValue().GetController()
	if controller == nil || controller.Observer {
		return fmt.Errorf("operational controller required")
	}
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: frame.CommandId, Aggregate: state.Ref, Actor: &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: cid, SessionId: &id}}
	cdm := &pb.CdmAction{}
	client := &pb.ClientCommand{}
	master := false
	switch v := frame.Event.(type) {
	case *es.Envelope_CdmTobtUpdate:
		cdm.Callsign = v.CdmTobtUpdate.Callsign
		cdm.Change = &pb.CdmAction_SetTobt{SetTobt: &pb.SetTobt{HhmmUtc: v.CdmTobtUpdate.Tobt}}
	case *es.Envelope_CdmAsrtToggle:
		cdm.Callsign = v.CdmAsrtToggle.Callsign
		cdm.Change = &pb.CdmAction_SetAsrt{SetAsrt: &pb.SetTobt{HhmmUtc: v.CdmAsrtToggle.Asrt}}
	case *es.Envelope_CdmTsacUpdate:
		cdm.Callsign = v.CdmTsacUpdate.Callsign
		cdm.Change = &pb.CdmAction_SetTsac{SetTsac: &pb.SetTobt{HhmmUtc: v.CdmTsacUpdate.Tsac}}
	case *es.Envelope_CdmDeiceUpdate:
		cdm.Callsign = v.CdmDeiceUpdate.Callsign
		cdm.Change = &pb.CdmAction_SetDeice{SetDeice: &pb.SetCdmDeice{Code: v.CdmDeiceUpdate.DeiceType}}
	case *es.Envelope_CdmManualCtot:
		cdm.Callsign = v.CdmManualCtot.Callsign
		clock, e := socketClock(v.CdmManualCtot.Ctot)
		if e != nil {
			return e
		}
		cdm.Change = &pb.CdmAction_SetCtot{SetCtot: &pb.SetCdmCtot{Value: clock}}
	case *es.Envelope_CdmCtotRemove:
		cdm.Callsign = v.CdmCtotRemove.Callsign
		cdm.Change = &pb.CdmAction_RemoveCtot{RemoveCtot: &pb.RemoveCdmCtot{}}
	case *es.Envelope_CdmReady:
		cdm.Callsign = v.CdmReady.Callsign
		cdm.Change = &pb.CdmAction_SetReady{SetReady: &pb.SetCdmReady{Ready: true}}
	case *es.Envelope_Eobt:
		cdm.Callsign = v.Eobt.Callsign
		cdm.Change = &pb.CdmAction_SetEobt{SetEobt: &pb.SetTobt{HhmmUtc: v.Eobt.Eobt}}
		master = true
		request.Actor.Kind = pb.Actor_SYSTEM
		request.Actor.Id = "euroscope-cdm"
	case *es.Envelope_IssuePdcClearance:
		if r.pdc == nil {
			return fmt.Errorf("PDC disabled")
		}
		action := &pb.PdcAction{Callsign: v.IssuePdcClearance.Callsign, Change: &pb.PdcAction_Issue{Issue: &pb.IssuePdc{RequestRemarks: v.IssuePdcClearance.Remarks}}}
		client.Action = &pb.ClientCommand_Pdc{Pdc: action}
		revision := state.Indexes[pb.EntityKind_PDC_SEQUENCE][action.Callsign].GetRevision()
		request.ExpectedEntityRevision = &revision
	case *es.Envelope_PdcRevertToVoice:
		if r.pdc == nil {
			return fmt.Errorf("PDC disabled")
		}
		action := &pb.PdcAction{Callsign: v.PdcRevertToVoice.Callsign, Change: &pb.PdcAction_RevertToVoice{RevertToVoice: &pb.RevertPdcToVoice{}}}
		client.Action = &pb.ClientCommand_Pdc{Pdc: action}
		revision := state.Indexes[pb.EntityKind_PDC_SEQUENCE][action.Callsign].GetRevision()
		request.ExpectedEntityRevision = &revision
	case *es.Envelope_SendPrivateMessage:
		client.Action = &pb.ClientCommand_Message{Message: &pb.MessageAction{Send: &pb.MessageAction_PrivateMessage{PrivateMessage: &pb.PrivateMessage{TargetCid: v.SendPrivateMessage.Callsign, Text: v.SendPrivateMessage.Message}}}}
	case *es.Envelope_CoordinationReceived:
		if err = r.projection.RequireMasterInbound(id, connection, cid, frame.MasterEpoch, true); err != nil {
			return err
		}
		value := v.CoordinationReceived
		from, to := "", ""
		for _, e := range state.EntitiesByKind(pb.EntityKind_CONTROLLER) {
			c := e.Value.GetController()
			if c.Callsign == value.SourceControllerCallsign {
				from = c.Cid
			}
			if c.Callsign == value.ControllerCallsign {
				to = c.Cid
			}
		}
		old := state.Indexes[pb.EntityKind_STRIP][value.Callsign]
		if old == nil || to == "" {
			return fmt.Errorf("handover strip or recipient unavailable")
		}
		revision := old.Revision
		request.ExpectedEntityRevision = &revision
		request.Actor.Kind = pb.Actor_SYSTEM
		request.Actor.Id = "euroscope-coordination"
		request.Command = &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: value.Callsign, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Coordination{Coordination: &pb.Coordination{Callsign: value.Callsign, FromCid: from, ToCid: to, Status: "TRANSFER", FromEuroscope: true, EuroscopeHandoverCid: to}}}}}}}
		return natsReply(r.deadlines.ExecuteClient(ctx, id, connection, cid, frame, request, true))
	case *es.Envelope_AmanRouteFact:
		return r.inboundAMAN(ctx, id, connection, cid, controller, frame)
	default:
		return r.deadlines.AdmitOperational(ctx, id, connection, cid, frame)
	}
	if cdm.Change != nil {
		revision := state.Indexes[pb.EntityKind_CDM_STATE][cdm.Callsign].GetRevision()
		request.ExpectedEntityRevision = &revision
		client.Action = &pb.ClientCommand_Cdm{Cdm: cdm}
	}
	request.Command = &pb.CommandRequest_Client{Client: client}
	return natsReply(r.deadlines.ExecuteClient(ctx, id, connection, cid, frame, request, master))
}

func socketClock(value string) (*timestamppb.Timestamp, error) {
	parsed, err := time.Parse("1504", value)
	if err != nil {
		return nil, fmt.Errorf("invalid UTC clock")
	}
	now := time.Now().UTC()
	at := time.Date(now.Year(), now.Month(), now.Day(), parsed.Hour(), parsed.Minute(), 0, 0, time.UTC)
	if at.Before(now.Add(-12 * time.Hour)) {
		at = at.Add(24 * time.Hour)
	}
	return timestamppb.New(at), nil
}
