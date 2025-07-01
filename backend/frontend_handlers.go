package main

import (
	"FlightStrips/data"
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgtype"
	"log"
)

func (s *Server) frontendeventhandlerGoARound(event Event) (err error) {
	var goAround GoAroundEventPayload
	payload := event.Payload.(string)
	err = json.Unmarshal([]byte(payload), &goAround)
	if err != nil {
		log.Println("Error unmarshalling goAround event")
		return err
	}

	_, err = json.Marshal(event)
	if err != nil {
		return err
	}

	// TODO Go Around is an event to send to all FrontEndClients
	//s.FrontendHub.broadcast <- bEvent

	return nil
}

func (s *Server) frontendEventGenerateSquawk(client *FrontendClient, message []byte) error {
	var generateSquawk FrontendGenerateSquawkEvent
	err := json.Unmarshal(message, &generateSquawk)
	if err != nil {
		return err
	}

	s.EuroscopeHub.SendGenerateSquawk(client.user.cid, generateSquawk.Callsign)
	return nil
}

func (s *Server) frontendEventHandlerMove(client *FrontendClient, message []byte) error {
	var move FrontendMoveEvent
	err := json.Unmarshal(message, &move)
	if err != nil {
		return err
	}

	db := data.New(s.DBPool)
	strip, err := db.GetStrip(context.Background(), data.GetStripParams{Callsign: move.Callsign, Session: client.session})
	if err != nil {
		return err
	}

	if strip.Bay.String == move.Bay {
		return nil
	}

	if move.Bay == BAY_NOT_CLEARED || move.Bay == BAY_CLEARED {
		err = handleClearedBayUpdate(client, strip, move, db, s)
	} else {
		err = handleGeneralBayUpdate(client, strip, move, db, s)
	}

	if err != nil {
		return err
	}

	s.FrontendHub.SendBayEvent(client.session, move.Callsign, move.Bay)

	return nil
}

func handleClearedBayUpdate(client *FrontendClient, strip data.Strip, move FrontendMoveEvent, db *data.Queries, s *Server) error {
	isCleared := move.Bay == BAY_CLEARED
	if strip.Cleared.Bool == isCleared {
		return nil
	}

	count, err := db.UpdateStripClearedFlagByID(
		context.Background(),
		data.UpdateStripClearedFlagByIDParams{
			Callsign: move.Callsign,
			Session:  client.session,
			Cleared:  pgtype.Bool{Valid: true, Bool: isCleared},
			Bay:      pgtype.Text{Valid: true, String: move.Bay},
		})

	if err != nil {
		return err
	}

	if count != 1 {
		return errors.New("failed to update strip cleared flag")
	}

	s.EuroscopeHub.SendClearedFlag(client.user.cid, move.Callsign, isCleared)
	return nil

}

func handleGeneralBayUpdate(client *FrontendClient, strip data.Strip, move FrontendMoveEvent, db *data.Queries, s *Server) error {
	state := strip.State.String
	if strip.Origin == client.airport {
		groundState := GetGroundState(move.Bay)
		if groundState != EuroscopeGroundStateUnknown {
			state = groundState
		}
	}

	count, err := db.UpdateStripGroundStateByID(
		context.Background(),
		data.UpdateStripGroundStateByIDParams{
			Callsign: move.Callsign,
			Session:  client.session,
			Bay:      pgtype.Text{Valid: true, String: move.Bay},
			State:    pgtype.Text{Valid: true, String: state},
		})

	if err != nil {
		return err
	}

	if count != 1 {
		return errors.New("failed to update strip bay/ground state")
	}

	if state != strip.State.String {
		s.EuroscopeHub.SendGroundState(client.user.cid, move.Callsign, state)
	}
	return nil
}

func (s *Server) frontendEventHandlerStripUpdate(client *FrontendClient, message []byte) error {
	var event FrontendUpdateStripDataEvent
	err := json.Unmarshal(message, &event)
	if err != nil {
		return err
	}

	if event.Route != nil && event.Sid != nil {
		return errors.New("cannot update both route and sid at the same time")
	}

	db := data.New(s.DBPool)
	strip, err := db.GetStrip(context.Background(), data.GetStripParams{Callsign: event.Callsign, Session: client.session})
	if err != nil {
		return err
	}

	if event.Route != nil && strip.Route.String != *event.Route {
		s.EuroscopeHub.SendRoute(client.user.cid, event.Callsign, *event.Route)
	}

	if event.Sid != nil && strip.Sid.String != *event.Sid {
		s.EuroscopeHub.SendSid(client.user.cid, event.Callsign, *event.Sid)
	}

	if event.Stand != nil && strip.Stand.String != *event.Stand {
		s.EuroscopeHub.SendStand(client.user.cid, event.Callsign, *event.Stand)
	}

	if event.Eobt != nil && strip.Eobt.String != *event.Eobt {
		// TODO add support
	}

	if event.Altitude != nil && strip.ClearedAltitude.Int32 != int32(*event.Altitude) {
		s.EuroscopeHub.SendClearedAltitude(client.user.cid, event.Callsign, *event.Altitude)
	}

	if event.Heading != nil && strip.Heading.Int32 != int32(*event.Heading) {
		s.EuroscopeHub.SendHeading(client.user.cid, event.Callsign, *event.Heading)
	}

	return nil
}

func (s *Server) frontendEventHandlerCoordinationTransferRequest(client *FrontendClient, message []byte) error {
	var req CoordinationTransferRequestEvent
	if err := json.Unmarshal(message, &req); err != nil {
		return err
	}
	position := client.position
	db := data.New(s.DBPool)
	strip, err := db.GetStrip(context.Background(), data.GetStripParams{Callsign: req.Callsign, Session: client.session})
	if err != nil {
		return err
	}

	if !strip.Owner.Valid || strip.Owner.String != position {
		return errors.New("cannot transfer strip which is not assumed")
	}

	_, err = db.CreateCoordination(
		context.Background(),
		data.CreateCoordinationParams{
			Session:      client.session,
			StripID:      strip.ID,
			FromPosition: position,
			ToPosition:   req.To,
		},
	)
	if err != nil {
		return err
	}
	s.FrontendHub.SendCoordinationTransfer(client.session, req.Callsign, position, req.To)
	return nil
}

func (s *Server) frontendEventHandlerCoordinationAssumeRequest(client *FrontendClient, message []byte) error {
	var req CoordinationAssumeRequestEvent
	if err := json.Unmarshal(message, &req); err != nil {
		return err
	}
	db := data.New(s.DBPool)

	strip, err := db.GetStrip(context.Background(), data.GetStripParams{Callsign: req.Callsign, Session: client.session})
	if err != nil {
		return err
	}
	// Strip is not owned by anyone assume it
	if !strip.Owner.Valid || strip.Owner.String == "" {
		err2 := SetOwner(client, db, req, strip, s)
		if err2 != nil {
			return err2
		}
		return nil
	}
	coordination, err := db.GetCoordinationByStripID(context.Background(), data.GetCoordinationByStripIDParams{StripID: strip.ID, Session: client.session})
	if err != nil {
		return err
	}

	if coordination.ToPosition != client.position {
		return errors.New("cannot assume strip which is not transferred to you")
	}

	_, err = db.DeleteCoordinationByID(context.Background(), coordination.ID)
	if err != nil {
		return err
	}
	err = SetOwner(client, db, req, strip, s)
	return err
}

func SetOwner(client *FrontendClient, db *data.Queries, req CoordinationAssumeRequestEvent, strip data.Strip, s *Server) error {
	count, err := db.SetStripOwner(context.Background(), data.SetStripOwnerParams{
		Owner: pgtype.Text{Valid: true, String: client.position}, Callsign: req.Callsign, Session: client.session, Version: strip.Version,
	})

	if err != nil {
		return err
	}

	if count != 1 {
		return errors.New("failed to set strip owner")
	}

	s.FrontendHub.SendCoordinationAssume(client.session, req.Callsign, client.position)
	return nil
}

func (s *Server) frontendEventHandlerCoordinationRejectRequest(client *FrontendClient, message []byte) error {
	var req CoordinationRejectRequestEvent
	if err := json.Unmarshal(message, &req); err != nil {
		return err
	}
	db := data.New(s.DBPool)

	coordination, err := db.GetCoordinationByStripCallsign(context.Background(), data.GetCoordinationByStripCallsignParams{Callsign: req.Callsign, Session: client.session})
	if err != nil {
		return err
	}

	if coordination.ToPosition != client.position {
		return errors.New("cannot reject strip which is not transferred to you")
	}

	_, err = db.DeleteCoordinationByID(context.Background(), coordination.ID)
	if err != nil {
		return err
	}
	s.FrontendHub.SendCoordinationReject(client.session, req.Callsign, client.position)
	return nil
}

func (s *Server) frontendEventHandlerCoordinationFreeRequest(client *FrontendClient, message []byte) error {
	var req CoordinationFreeRequestEvent
	if err := json.Unmarshal(message, &req); err != nil {
		return err
	}
	db := data.New(s.DBPool)

	strip, err := db.GetStrip(context.Background(), data.GetStripParams{Callsign: req.Callsign, Session: client.session})
	if err != nil {
		return err
	}

	if strip.Owner.String != client.position {
		return errors.New("cannot free strip which is not owned by you")
	}

	count, err := db.SetStripOwner(context.Background(), data.SetStripOwnerParams{Owner: pgtype.Text{Valid: false}, Callsign: req.Callsign, Session: client.session, Version: strip.Version})

	if err != nil {
		return err
	}

	if count != 1 {
		return errors.New("failed to set strip owner")
	}

	s.FrontendHub.SendCoordinationFree(client.session, req.Callsign)

	return nil
}
