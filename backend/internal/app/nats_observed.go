package app

import (
	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"context"
	"fmt"
)

func (r *natsRuntime) observedStrip(ctx context.Context, id int32, connection, cid string, frame *es.Envelope, strip *es.Strip) error {
	if strip.Eobt == "" {
		return nil
	}
	ref := sessionNATSRef(id)
	session, err := r.projection.ReadEntity(ref, pb.EntityKind_SESSION, fmt.Sprint(id))
	if err != nil {
		return err
	}
	seed := session.GetValue().GetSession()
	if seed == nil {
		return fmt.Errorf("observed session unavailable")
	}
	if strip.Origin != seed.Airport {
		return nil
	}
	entity, err := r.projection.ReadEntity(ref, pb.EntityKind_STRIP, strip.Callsign)
	if err != nil {
		return err
	}
	accepted := entity.GetValue().GetStrip()
	if accepted.Eobt != nil && accepted.Eobt.AsTime().UTC().Format("1504") == strip.Eobt {
		return nil
	}
	// Every aircraft in a sync has its own stable derived action identity.
	command, _ := cluster.ProviderEventCommandID("euroscope-cdm", connection, frame.CommandId+"/eobt/"+strip.Callsign)
	cdm, err := r.projection.ReadEntity(ref, pb.EntityKind_CDM_STATE, strip.Callsign)
	if err != nil {
		return err
	}
	revision := cdm.GetRevision()
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: command, Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "euroscope-cdm", SessionId: &id}, ExpectedEntityRevision: &revision, Command: &pb.CommandRequest_Client{Client: &pb.ClientCommand{Action: &pb.ClientCommand_Cdm{Cdm: &pb.CdmAction{Callsign: strip.Callsign, Change: &pb.CdmAction_SetEobt{SetEobt: &pb.SetTobt{HhmmUtc: strip.Eobt}}}}}}}
	return natsReply(r.deadlines.ExecuteClient(ctx, id, connection, cid, frame, request, true))
}
