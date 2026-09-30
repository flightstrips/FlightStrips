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
	state, err := r.projection.Read(sessionNATSRef(id))
	if err != nil {
		return err
	}
	seed := state.Indexes[pb.EntityKind_SESSION][fmt.Sprint(id)].Value.GetSession()
	if strip.Origin != seed.Airport {
		return nil
	}
	accepted := state.Indexes[pb.EntityKind_STRIP][strip.Callsign].Value.GetStrip()
	if accepted.Eobt != nil && accepted.Eobt.AsTime().UTC().Format("1504") == strip.Eobt {
		return nil
	}
	// Every aircraft in a sync has its own stable derived action identity.
	command, _ := cluster.ProviderEventCommandID("euroscope-cdm", connection, frame.CommandId+"/eobt/"+strip.Callsign)
	revision := state.Indexes[pb.EntityKind_CDM_STATE][strip.Callsign].GetRevision()
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: command, Aggregate: state.Ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "euroscope-cdm", SessionId: &id}, ExpectedEntityRevision: &revision, Command: &pb.CommandRequest_Client{Client: &pb.ClientCommand{Action: &pb.ClientCommand_Cdm{Cdm: &pb.CdmAction{Callsign: strip.Callsign, Change: &pb.CdmAction_SetEobt{SetEobt: &pb.SetTobt{HhmmUtc: strip.Eobt}}}}}}}
	return natsReply(r.deadlines.ExecuteClient(ctx, id, connection, cid, frame, request, true))
}
