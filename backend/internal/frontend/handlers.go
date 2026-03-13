package frontend

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	"FlightStrips/pkg/events/frontend"
	"context"
	"errors"
	"log/slog"

	gorilla "github.com/gorilla/websocket"
)

type Message = shared.Message[frontend.EventType]

// validBays is the set of bay values that the frontend is permitted to move a strip to.
// Any move event carrying a bay outside this set is rejected.
var validBays = map[string]bool{
	shared.BAY_NOT_CLEARED: true,
	shared.BAY_CLEARED:     true,
	shared.BAY_PUSH:        true,
	shared.BAY_TAXI:        true,
	shared.BAY_TAXI_TWR:    true,
	shared.BAY_DEPART:      true,
	shared.BAY_AIRBORNE:    true,
	shared.BAY_FINAL:       true,
	shared.BAY_STAND:       true,
	shared.BAY_HIDDEN:      true,
	shared.BAY_ARR_HIDDEN:  true,
}

func handleTokenEvent(ctx context.Context, client *Client, message Message) error {
	var event events.AuthenticationEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}

	user, err := client.hub.authenticationService.Validate(event.Token)
	if err != nil {
		slog.Info("Token re-validation failed, disconnecting client", slog.String("cid", client.GetCid()), slog.Any("error", err))
		_ = client.GetConnection().WriteMessage(gorilla.CloseMessage,
			gorilla.FormatCloseMessage(gorilla.CloseNormalClosure, "token invalid"))
		client.GetConnection().Close()
		return err
	}

	client.SetUser(user)
	return nil
}

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

	if !validBays[move.Bay] {
		slog.Warn("handleMove: rejecting move event with invalid bay",
			slog.String("callsign", move.Callsign),
			slog.String("bay", move.Bay),
			slog.String("cid", client.GetCid()),
		)
		return errors.New("invalid bay value: " + move.Bay)
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
		isCleared := move.Bay == shared.BAY_CLEARED
		if err := client.hub.stripService.UpdateClearedFlagForMove(ctx, client.session, move.Callsign, isCleared, move.Bay, client.GetCid()); err != nil {
			return err
		}
	} else {
		if err := client.hub.stripService.UpdateGroundStateForMove(ctx, client.session, move.Callsign, move.Bay, client.GetCid(), client.airport); err != nil {
			return err
		}
	}

	if err := client.hub.stripService.MoveToBay(ctx, client.session, move.Callsign, move.Bay, true); err != nil {
		return err
	}

	if move.Bay == shared.BAY_AIRBORNE {
		return client.hub.stripService.AutoTransferAirborneStrip(ctx, client.session, move.Callsign)
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

	if event.Runway != nil && strip.Runway != event.Runway {
		s.GetEuroscopeHub().SendRunway(client.session, client.GetCid(), event.Callsign, *event.Runway)
	}

	if event.Eobt != nil && strip.Eobt != event.Eobt {
		slog.Warn("EOBT updates are currently not supported and will be ignored", slog.String("callsign", event.Callsign))
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

	strip, err := stripRepo.GetByCallsign(ctx, client.session, req.Callsign)
	if err != nil {
		return err
	}

	if strip.Owner == nil || *strip.Owner != position {
		return errors.New("cannot transfer strip which is not assumed")
	}

	return client.hub.stripService.CreateCoordinationTransfer(ctx, client.session, req.Callsign, position, req.To)
}

func handleCoordinationAssumeRequest(ctx context.Context, client *Client, message Message) error {
	var req frontend.CoordinationAssumeRequestEvent
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	return client.hub.stripService.AssumeStripCoordination(ctx, client.session, req.Callsign, client.position)
}

func handleCoordinationRejectRequest(ctx context.Context, client *Client, message Message) error {
	var req frontend.CoordinationRejectRequestEvent
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	return client.hub.stripService.RejectCoordination(ctx, client.session, req.Callsign, client.position)
}

func handleCoordinationFreeRequest(ctx context.Context, client *Client, message Message) error {
	var req frontend.CoordinationFreeRequestEvent
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	return client.hub.stripService.FreeStrip(ctx, client.session, req.Callsign, client.position)
}

func handleCoordinationCancelTransferRequest(ctx context.Context, client *Client, message Message) error {
	var req frontend.CoordinationCancelTransferRequestEvent
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	return client.hub.stripService.CancelCoordinationTransfer(ctx, client.session, req.Callsign, client.position)
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

	return client.hub.stripService.MoveStripBetween(ctx, client.session, event.Callsign, event.InsertAfter, bay)
}

func handleSendMessage(ctx context.Context, client *Client, message Message) error {
	var req frontend.SendMessageEvent
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}

	recipients := req.Recipients
	if recipients == nil {
		recipients = []string{}
	}

	msg := frontend.MessageReceivedEvent{
		ID:          client.hub.NextMessageID(),
		Sender:      client.position,
		Text:        req.Text,
		IsBroadcast: len(recipients) == 0,
		Recipients:  recipients,
	}

	client.hub.storeMessage(client.session, msg)
	client.hub.dispatchMessage(client.session, msg, client.user.GetCid())
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
	return client.hub.stripService.UpdateReleasePoint(ctx, client.session, event.Callsign, event.ReleasePoint)
}

func handleMarked(ctx context.Context, client *Client, message Message) error {
	var event frontend.MarkedEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.UpdateMarked(ctx, client.session, event.Callsign, event.Marked)
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

func handleCreateTacticalStrip(ctx context.Context, client *Client, message Message) error {
	var req frontend.CreateTacticalStripAction
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}

	validTypes := map[string]bool{"MEMAID": true, "CROSSING": true, "START": true, "LAND": true}
	if !validTypes[req.StripType] {
		return errors.New("invalid tactical strip type: " + req.StripType)
	}
	if req.Bay == "" {
		return errors.New("bay is required")
	}
	if req.StripType == "MEMAID" && req.Label == "" {
		return errors.New("label is required for MEMAID strips")
	}
	if req.StripType == "START" || req.StripType == "LAND" {
		if req.Label == "" {
			return errors.New("runway label is required for START and LAND strips")
		}
		validRunways := config.GetRunways()
		isValid := false
		for _, rwy := range validRunways {
			if rwy == req.Label {
				isValid = true
				break
			}
		}
		if !isValid {
			return errors.New("invalid runway: " + req.Label)
		}
	}

	tacticalRepo := client.hub.server.GetTacticalStripRepository()
	if tacticalRepo == nil {
		return errors.New("tactical strip repository not available")
	}

	maxSeq, err := tacticalRepo.GetMaxSequenceInBayUnified(ctx, client.session, req.Bay)
	if err != nil {
		return err
	}

	sequence := maxSeq + 1000 // InitialOrderSpacing

	var aircraft *string
	if req.Aircraft != "" {
		a := req.Aircraft
		aircraft = &a
	}

	ts, err := tacticalRepo.Create(ctx, client.session, req.StripType, req.Bay, req.Label, aircraft, client.position, sequence)
	if err != nil {
		return err
	}

	client.hub.SendTacticalStripCreated(client.session, MapTacticalStripToPayload(ts))
	return nil
}

func handleDeleteTacticalStrip(ctx context.Context, client *Client, message Message) error {
	var req frontend.DeleteTacticalStripAction
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}

	tacticalRepo := client.hub.server.GetTacticalStripRepository()
	if tacticalRepo == nil {
		return errors.New("tactical strip repository not available")
	}

	// Need the bay for the deleted event
	// We load it first, then delete
	strips, err := tacticalRepo.ListBySession(ctx, client.session)
	if err != nil {
		return err
	}
	bay := ""
	for _, s := range strips {
		if s.ID == req.ID {
			bay = s.Bay
			break
		}
	}

	if err := tacticalRepo.Delete(ctx, req.ID, client.session); err != nil {
		return err
	}

	client.hub.SendTacticalStripDeleted(client.session, req.ID, bay)
	return nil
}

func handleConfirmTacticalStrip(ctx context.Context, client *Client, message Message) error {
	var req frontend.ConfirmTacticalStripAction
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}

	tacticalRepo := client.hub.server.GetTacticalStripRepository()
	if tacticalRepo == nil {
		return errors.New("tactical strip repository not available")
	}

	ts, err := tacticalRepo.Confirm(ctx, req.ID, client.session, client.position)
	if err != nil {
		return err
	}

	if ts.Type != "MEMAID" && ts.Type != "CROSSING" {
		return errors.New("confirm is only valid for MEMAID and CROSSING strips")
	}
	if ts.ProducedBy == client.position {
		return errors.New("producer cannot confirm their own MEMAID strip")
	}

	client.hub.SendTacticalStripUpdated(client.session, MapTacticalStripToPayload(ts))
	return nil
}

func handleStartTacticalTimer(ctx context.Context, client *Client, message Message) error {
	var req frontend.StartTacticalTimerAction
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}

	tacticalRepo := client.hub.server.GetTacticalStripRepository()
	if tacticalRepo == nil {
		return errors.New("tactical strip repository not available")
	}

	ts, err := tacticalRepo.StartTimer(ctx, req.ID, client.session)
	if err != nil {
		return err
	}

	if ts.Type != "START" && ts.Type != "LAND" {
		return errors.New("start timer is only valid for START and LAND strips")
	}

	client.hub.SendTacticalStripUpdated(client.session, MapTacticalStripToPayload(ts))
	return nil
}

func handleMoveTacticalStrip(ctx context.Context, client *Client, message Message) error {
	var req frontend.MoveTacticalStripAction
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}

	tacticalRepo := client.hub.server.GetTacticalStripRepository()
	if tacticalRepo == nil {
		return errors.New("tactical strip repository not available")
	}

	seq, err := tacticalRepo.GetSequenceByID(ctx, req.ID, client.session)
	if err != nil {
		return err
	}
	_ = seq

	// We need the bay — find it from the session list
	strips, err := tacticalRepo.ListBySession(ctx, client.session)
	if err != nil {
		return err
	}
	bay := ""
	for _, s := range strips {
		if s.ID == req.ID {
			bay = s.Bay
			break
		}
	}
	if bay == "" {
		return errors.New("tactical strip not found")
	}

	return client.hub.stripService.MoveTacticalStripBetween(ctx, client.session, req.ID, req.InsertAfter, bay)
}
