package frontend

import (
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events/euroscope"
	"FlightStrips/pkg/events/frontend"
	"context"
	"errors"
	"slices"
)

type Message = shared.Message[frontend.EventType]

func handleGenerateSquawk(ctx context.Context, client *Client, message Message) error {
	var generateSquawk frontend.GenerateSquawkRequest
	err := message.JsonUnmarshal(&generateSquawk)
	if err != nil {
		return err
	}

	client.hub.server.GetEuroscopeHub().SendGenerateSquawk(client.session, client.GetCid(), generateSquawk.Callsign)
	return nil
}

func handleMove(ctx context.Context, client *Client, message Message) error {
	var move frontend.MoveEvent
	err := message.JsonUnmarshal(&move)
	if err != nil {
		return err
	}

	s := client.hub.server
	stripRepo := s.GetStripRepository()

	strip, err := stripRepo.GetByCallsign(ctx, client.session, move.Callsign)
	if err != nil {
		return err
	}

	if strip.Bay == move.Bay {
		return nil
	}

	if move.Bay == shared.BAY_NOT_CLEARED || move.Bay == shared.BAY_CLEARED {
		err = handleClearedBayUpdate(ctx, client, strip, move, stripRepo, s.GetEuroscopeHub())
	} else {
		err = handleGeneralBayUpdate(ctx, client, strip, move, stripRepo, s.GetEuroscopeHub())
	}

	if err != nil {
		return err
	}

	return client.hub.stripService.MoveToBay(ctx, client.session, move.Callsign, move.Bay, true)
}

func handleClearedBayUpdate(ctx context.Context, client *Client, strip *internalModels.Strip, move frontend.MoveEvent, stripRepo shared.StripRepository, es shared.EuroscopeHub) error {
	isCleared := move.Bay == shared.BAY_CLEARED
	if strip.Cleared == isCleared {
		return nil
	}

	count, err := stripRepo.UpdateClearedFlag(
		ctx,
		client.session,
		move.Callsign,
		isCleared,
		move.Bay,
		nil)

	if err != nil {
		return err
	}

	if count != 1 {
		return errors.New("failed to update strip cleared flag")
	}

	es.SendClearedFlag(client.session, client.GetCid(), move.Callsign, isCleared)
	return nil
}

func handleGeneralBayUpdate(ctx context.Context, client *Client, strip *internalModels.Strip, move frontend.MoveEvent, stripRepo shared.StripRepository, es shared.EuroscopeHub) error {
	state := strip.State
	if strip.Origin == client.airport {
		groundState := shared.GetGroundState(move.Bay)
		if groundState != euroscope.GroundStateUnknown {
			state = &groundState
		}
	}

	count, err := stripRepo.UpdateGroundState(
		ctx,
		client.session,
		move.Callsign,
		state,
		move.Bay,
		nil)

	if err != nil {
		return err
	}

	if count != 1 {
		return errors.New("failed to update strip bay/ground state")
	}

	if state != strip.State && state != nil {
		es.SendGroundState(client.session, client.GetCid(), move.Callsign, *state)
	}
	return nil
}

func handleStripUpdate(ctx context.Context, client *Client, message Message) error {
	var event frontend.UpdateStripDataEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}

	if event.Route != nil && event.Sid != nil {
		return errors.New("cannot update both route and sid at the same time")
	}

	s := client.hub.server
	stripRepo := s.GetStripRepository()

	strip, err := stripRepo.GetByCallsign(ctx, client.session, event.Callsign)
	if err != nil {
		return err
	}

	if event.Route != nil && strip.Route != event.Route {
		s.GetEuroscopeHub().SendRoute(client.session, client.GetCid(), event.Callsign, *event.Route)
	}

	if event.Sid != nil && strip.Sid != event.Sid {
		s.GetEuroscopeHub().SendSid(client.session, client.GetCid(), event.Callsign, *event.Sid)
	}

	if event.Stand != nil && strip.Stand != event.Stand {
		s.GetEuroscopeHub().SendStand(client.session, client.GetCid(), event.Callsign, *event.Stand)
	}

	if event.Eobt != nil && strip.Eobt != event.Eobt {
		// TODO add support
	}

	if event.Altitude != nil && strip.ClearedAltitude != event.Altitude {
		s.GetEuroscopeHub().SendClearedAltitude(client.session, client.GetCid(), event.Callsign, *event.Altitude)
	}

	if event.Heading != nil && strip.Heading != event.Heading {
		s.GetEuroscopeHub().SendHeading(client.session, client.GetCid(), event.Callsign, *event.Heading)
	}

	return nil
}

func handleCoordinationTransferRequest(ctx context.Context, client *Client, message Message) error {
	var req frontend.CoordinationTransferRequestEvent
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	position := client.position
	s := client.hub.server
	stripRepo := s.GetStripRepository()
	coordRepo := s.GetCoordinationRepository()
	
	strip, err := stripRepo.GetByCallsign(ctx, client.session, req.Callsign)
	if err != nil {
		return err
	}

	if strip.Owner == nil || *strip.Owner != position {
		return errors.New("cannot transfer strip which is not assumed")
	}

	coord := &internalModels.Coordination{
		Session:      client.session,
		StripID:      strip.ID,
		FromPosition: position,
		ToPosition:   req.To,
	}
	
	err = coordRepo.Create(ctx, coord)
	if err != nil {
		return err
	}
	client.hub.SendCoordinationTransfer(client.session, req.Callsign, position, req.To)
	return nil
}

func handleCoordinationAssumeRequest(ctx context.Context, client *Client, message Message) error {
	var req frontend.CoordinationAssumeRequestEvent
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	s := client.hub.server
	stripRepo := s.GetStripRepository()
	coordRepo := s.GetCoordinationRepository()

	strip, err := stripRepo.GetByCallsign(ctx, client.session, req.Callsign)
	if err != nil {
		return err
	}
	// Strip is not owned by anyone assume it
	if strip.Owner == nil || *strip.Owner == "" {
		err2 := setOwner(ctx, client, stripRepo, req, strip)
		if err2 != nil {
			return err2
		}
		return nil
	}
	coordination, err := coordRepo.GetByStripID(ctx, client.session, strip.ID)
	if err != nil {
		return err
	}

	if coordination.ToPosition != client.position {
		return errors.New("cannot assume strip which is not transferred to you")
	}

	err = coordRepo.Delete(ctx, coordination.ID)
	if err != nil {
		return err
	}

	nextOwners := strip.NextOwners
	index := slices.Index(nextOwners, client.position)
	if index >= 0 {
		nextOwners = nextOwners[index+1:]
	}

	previousOwners := append(strip.PreviousOwners, client.position)

	err = stripRepo.SetNextAndPreviousOwners(ctx, client.session, strip.Callsign, nextOwners, previousOwners)
	if err != nil {
		return err
	}

	if err := setOwner(ctx, client, stripRepo, req, strip); err != nil {
		return err
	}

	client.hub.SendOwnersUpdate(client.session, strip.Callsign, nextOwners, previousOwners)
	return nil
}

func setOwner(ctx context.Context, client *Client, stripRepo shared.StripRepository, req frontend.CoordinationAssumeRequestEvent, strip *internalModels.Strip) error {
	count, err := stripRepo.SetOwner(ctx, client.session, req.Callsign, &client.position, strip.Version)

	if err != nil {
		return err
	}

	if count != 1 {
		return errors.New("failed to set strip owner")
	}

	client.hub.SendCoordinationAssume(client.session, req.Callsign, client.position)
	return nil
}

func handleCoordinationRejectRequest(ctx context.Context, client *Client, message Message) error {
	var req frontend.CoordinationRejectRequestEvent
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	s := client.hub.server
	coordRepo := s.GetCoordinationRepository()

	coordination, err := coordRepo.GetByStripCallsign(ctx, client.session, req.Callsign)
	if err != nil {
		return err
	}

	if coordination.ToPosition != client.position {
		return errors.New("cannot reject strip which is not transferred to you")
	}

	err = coordRepo.Delete(ctx, coordination.ID)
	if err != nil {
		return err
	}
	client.hub.SendCoordinationReject(client.session, req.Callsign, client.position)
	return nil
}

func handleCoordinationFreeRequest(ctx context.Context, client *Client, message Message) error {
	var req frontend.CoordinationFreeRequestEvent
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	s := client.hub.server
	stripRepo := s.GetStripRepository()

	strip, err := stripRepo.GetByCallsign(ctx, client.session, req.Callsign)
	if err != nil {
		return err
	}

	if strip.Owner == nil || *strip.Owner != client.position {
		return errors.New("cannot free strip which is not owned by you")
	}

	previousOwners := append(strip.PreviousOwners, client.position)

	if err := stripRepo.SetPreviousOwners(ctx, client.session, strip.Callsign, previousOwners); err != nil {
		return err
	}

	count, err := stripRepo.SetOwner(ctx, client.session, req.Callsign, nil, strip.Version)

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

func handleUpdateOrder(ctx context.Context, client *Client, message Message) error {
	var event frontend.UpdateOrderEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}

	s := client.hub.server
	stripRepo := s.GetStripRepository()
	
	bay, err := stripRepo.GetBay(ctx, client.session, event.Callsign)
	if err != nil {
		return err
	}

	if bay == "" {
		return errors.New("cannot update order of a strip which is not in a bay")
	}

	return client.hub.stripService.MoveStripBetween(ctx, client.session, event.Callsign, event.Before, bay)
}

func handleSendMessage(ctx context.Context, client *Client, message Message) error {
	var event frontend.SendMessageEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}

	outgoingEvent := frontend.BroadcastEvent{
		Message: event.Message,
		From:    client.position,
	}

	if event.To == nil {
		client.hub.Broadcast(client.session, outgoingEvent)
	} else {
		client.hub.SendToPosition(client.session, *event.To, outgoingEvent)
	}
	return nil
}

func handleCdmReady(ctx context.Context, client *Client, message Message) error {
	var event frontend.CdmReadyEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}

	cdmService := client.hub.server.GetCdmService()
	return cdmService.RequestBetterTobt(ctx, client.session, event.Callsign)
}

func handleReleasePoint(ctx context.Context, client *Client, message Message) error {
	var event frontend.ReleasePointEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}

	stripRepo := client.hub.server.GetStripRepository()
	affected, err := stripRepo.UpdateReleasePoint(ctx, client.session, event.Callsign, &event.ReleasePoint)

	if err != nil {
		return err
	}
	if affected != 1 {
		return errors.New("failed to update release point")
	}

	client.hub.Broadcast(client.session, event)

	return nil
}

func handleIssuePdcClearance(ctx context.Context, client *Client, message Message) error {
	var req frontend.IssuePdcClearanceRequest
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}

	// Get PDC service from server
	pdcService := client.hub.server.GetPdcService()
	if pdcService == nil {
		return errors.New("PDC service not available")
	}

	// Issue clearance
	return pdcService.IssueClearance(ctx, req.Callsign, req.Remarks, client.GetCid(), client.session)
}

func handlePdcManualStateChange(ctx context.Context, client *Client, message Message) error {
	var req frontend.PdcManualStateChangeRequest
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}

	// Get PDC service
	pdcService := client.hub.server.GetPdcService()
	if pdcService == nil {
		return errors.New("PDC service not available")
	}

	// Manually update PDC state
	return pdcService.ManualStateChange(ctx, req.Callsign, client.session, req.State)
}

func handleRevertToVoice(ctx context.Context, client *Client, message Message) error {
	var req frontend.RevertToVoiceRequest
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}

	pdcService := client.hub.server.GetPdcService()
	if pdcService == nil {
		return errors.New("PDC service not available")
	}

	return pdcService.RevertToVoice(ctx, req.Callsign, client.session, client.GetCid())
}
