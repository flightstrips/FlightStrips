package frontend

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events/euroscope"
	"FlightStrips/pkg/events/frontend"
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5/pgtype"
)

type Message = shared.Message[frontend.EventType]

func handleGenerateSquawk(client *Client, message Message) error {
	var generateSquawk frontend.GenerateSquawkRequest
	err := message.JsonUnmarshal(&generateSquawk)
	if err != nil {
		return err
	}

	client.hub.server.GetEuroscopeHub().SendGenerateSquawk(client.session, client.GetCid(), generateSquawk.Callsign)
	return nil
}

func handleMove(client *Client, message Message) error {
	var move frontend.MoveEvent
	err := message.JsonUnmarshal(&move)
	if err != nil {
		return err
	}

	s := client.hub.server

	db := database.New(s.GetDatabasePool())
	strip, err := db.GetStrip(context.Background(), database.GetStripParams{Callsign: move.Callsign, Session: client.session})
	if err != nil {
		return err
	}

	if strip.Bay.String == move.Bay {
		return nil
	}

	if move.Bay == shared.BAY_NOT_CLEARED || move.Bay == shared.BAY_CLEARED {
		err = handleClearedBayUpdate(client, strip, move, db, s.GetEuroscopeHub())
	} else {
		err = handleGeneralBayUpdate(client, strip, move, db, s.GetEuroscopeHub())
	}

	if err != nil {
		return err
	}

	return client.hub.stripService.MoveToBay(context.Background(), client.session, move.Callsign, move.Bay, true)
}

func handleClearedBayUpdate(client *Client, strip database.Strip, move frontend.MoveEvent, db *database.Queries, es shared.EuroscopeHub) error {
	isCleared := move.Bay == shared.BAY_CLEARED
	if strip.Cleared.Bool == isCleared {
		return nil
	}

	count, err := db.UpdateStripClearedFlagByID(
		context.Background(),
		database.UpdateStripClearedFlagByIDParams{
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

	es.SendClearedFlag(client.session, client.GetCid(), move.Callsign, isCleared)
	return nil
}

func handleGeneralBayUpdate(client *Client, strip database.Strip, move frontend.MoveEvent, db *database.Queries, es shared.EuroscopeHub) error {
	state := strip.State.String
	if strip.Origin == client.airport {
		groundState := shared.GetGroundState(move.Bay)
		if groundState != euroscope.GroundStateUnknown {
			state = groundState
		}
	}

	count, err := db.UpdateStripGroundStateByID(
		context.Background(),
		database.UpdateStripGroundStateByIDParams{
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
		es.SendGroundState(client.session, client.GetCid(), move.Callsign, state)
	}
	return nil
}

func handleStripUpdate(client *Client, message Message) error {
	var event frontend.UpdateStripDataEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}

	if event.Route != nil && event.Sid != nil {
		return errors.New("cannot update both route and sid at the same time")
	}

	s := client.hub.server

	db := database.New(s.GetDatabasePool())
	strip, err := db.GetStrip(context.Background(), database.GetStripParams{Callsign: event.Callsign, Session: client.session})
	if err != nil {
		return err
	}

	if event.Route != nil && strip.Route.String != *event.Route {
		s.GetEuroscopeHub().SendRoute(client.session, client.GetCid(), event.Callsign, *event.Route)
	}

	if event.Sid != nil && strip.Sid.String != *event.Sid {
		s.GetEuroscopeHub().SendSid(client.session, client.GetCid(), event.Callsign, *event.Sid)
	}

	if event.Stand != nil && strip.Stand.String != *event.Stand {
		s.GetEuroscopeHub().SendStand(client.session, client.GetCid(), event.Callsign, *event.Stand)
	}

	if event.Eobt != nil && strip.Eobt.String != *event.Eobt {
		// TODO add support
	}

	if event.Altitude != nil && strip.ClearedAltitude.Int32 != int32(*event.Altitude) {
		s.GetEuroscopeHub().SendClearedAltitude(client.session, client.GetCid(), event.Callsign, *event.Altitude)
	}

	if event.Heading != nil && strip.Heading.Int32 != int32(*event.Heading) {
		s.GetEuroscopeHub().SendHeading(client.session, client.GetCid(), event.Callsign, *event.Heading)
	}

	return nil
}

func handleCoordinationTransferRequest(client *Client, message Message) error {
	var req frontend.CoordinationTransferRequestEvent
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	position := client.position
	db := database.New(client.hub.server.GetDatabasePool())
	strip, err := db.GetStrip(context.Background(), database.GetStripParams{Callsign: req.Callsign, Session: client.session})
	if err != nil {
		return err
	}

	if !strip.Owner.Valid || strip.Owner.String != position {
		return errors.New("cannot transfer strip which is not assumed")
	}

	_, err = db.CreateCoordination(
		context.Background(),
		database.CreateCoordinationParams{
			Session:      client.session,
			StripID:      strip.ID,
			FromPosition: position,
			ToPosition:   req.To,
		},
	)
	if err != nil {
		return err
	}
	client.hub.SendCoordinationTransfer(client.session, req.Callsign, position, req.To)
	return nil
}

func handleCoordinationAssumeRequest(client *Client, message Message) error {
	var req frontend.CoordinationAssumeRequestEvent
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	db := database.New(client.hub.server.GetDatabasePool())

	strip, err := db.GetStrip(context.Background(), database.GetStripParams{Callsign: req.Callsign, Session: client.session})
	if err != nil {
		return err
	}
	// Strip is not owned by anyone assume it
	if !strip.Owner.Valid || strip.Owner.String == "" {
		err2 := setOwner(client, db, req, strip)
		if err2 != nil {
			return err2
		}
		return nil
	}
	coordination, err := db.GetCoordinationByStripID(context.Background(), database.GetCoordinationByStripIDParams{StripID: strip.ID, Session: client.session})
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

	nextOwners := strip.NextOwners
	index := slices.Index(nextOwners, client.position)
	if index >= 0 {
		nextOwners = nextOwners[index+1:]
	}

	previousOwners := append(strip.PreviousOwners, client.position)

	err = db.SetNextAndPreviousOwners(context.Background(), database.SetNextAndPreviousOwnersParams{
		Session:        client.session,
		Callsign:       strip.Callsign,
		NextOwners:     nextOwners,
		PreviousOwners: previousOwners,
	})
	if err != nil {
		return err
	}

	if err := setOwner(client, db, req, strip); err != nil {
		return err
	}

	client.hub.SendOwnersUpdate(client.session, strip.Callsign, nextOwners, previousOwners)
	return nil
}

func setOwner(client *Client, db *database.Queries, req frontend.CoordinationAssumeRequestEvent, strip database.Strip) error {
	count, err := db.SetStripOwner(context.Background(), database.SetStripOwnerParams{
		Owner: pgtype.Text{Valid: true, String: client.position}, Callsign: req.Callsign, Session: client.session, Version: strip.Version,
	})

	if err != nil {
		return err
	}

	if count != 1 {
		return errors.New("failed to set strip owner")
	}

	client.hub.SendCoordinationAssume(client.session, req.Callsign, client.position)
	return nil
}

func handleCoordinationRejectRequest(client *Client, message Message) error {
	var req frontend.CoordinationRejectRequestEvent
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	db := database.New(client.hub.server.GetDatabasePool())

	coordination, err := db.GetCoordinationByStripCallsign(context.Background(), database.GetCoordinationByStripCallsignParams{Callsign: req.Callsign, Session: client.session})
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
	client.hub.SendCoordinationReject(client.session, req.Callsign, client.position)
	return nil
}

func handleCoordinationFreeRequest(client *Client, message Message) error {
	var req frontend.CoordinationFreeRequestEvent
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	db := database.New(client.hub.server.GetDatabasePool())

	strip, err := db.GetStrip(context.Background(), database.GetStripParams{Callsign: req.Callsign, Session: client.session})
	if err != nil {
		return err
	}

	if strip.Owner.String != client.position {
		return errors.New("cannot free strip which is not owned by you")
	}

	previousOwners := append(strip.PreviousOwners, client.position)

	if err := db.SetPreviousOwners(context.Background(), database.SetPreviousOwnersParams{
		Session:        client.session,
		Callsign:       strip.Callsign,
		PreviousOwners: previousOwners,
	}); err != nil {
		return err
	}

	count, err := db.SetStripOwner(context.Background(), database.SetStripOwnerParams{Owner: pgtype.Text{Valid: false}, Callsign: req.Callsign, Session: client.session, Version: strip.Version})

	if err != nil {
		return err
	}

	if count != 1 {
		return errors.New("failed to set strip owner")
	}

	client.hub.SendCoordinationFree(client.session, req.Callsign)
	client.hub.SendOwnersUpdate(client.session, strip.Callsign, strip.NextOwners, previousOwners)

	return nil
}
