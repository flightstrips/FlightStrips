package frontend

import (
	"FlightStrips/internal/config"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	"FlightStrips/pkg/events/frontend"
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	gorilla "github.com/gorilla/websocket"
)

type Message = shared.Message[frontend.EventType]

type pdcInvalidValidationStripReevaluator interface {
	ReevaluatePdcInvalidValidationForStrip(ctx context.Context, session int32, strip *internalModels.Strip, activeDepartureRunways []string, publish bool, forceReactivate bool) error
}

type departureValidationStripReevaluator interface {
	ReevaluateDepartureValidation(ctx context.Context, session int32, callsign string, publish bool, forceReactivate bool) error
}

// validBays is the set of bay values that the frontend is permitted to move a strip to.
// Any move event carrying a bay outside this set is rejected.
var validBays = map[string]bool{
	shared.BAY_NOT_CLEARED: true,
	shared.BAY_CLEARED:     true,
	shared.BAY_PUSH:        true,
	shared.BAY_TAXI:        true,
	shared.BAY_TAXI_LWR:    true,
	shared.BAY_TAXI_TWR:    true,
	shared.BAY_DEPART:      true,
	shared.BAY_AIRBORNE:    true,
	shared.BAY_FINAL:       true,
	shared.BAY_RWY_ARR:     true,
	shared.BAY_TWY_ARR:     true,
	shared.BAY_STAND:       true,
	shared.BAY_HIDDEN:      true,
	shared.BAY_ARR_HIDDEN:  true,
	shared.BAY_CONTROLZONE: true,
}

func handleTokenEvent(ctx context.Context, client *Client, message Message) error {
	var event events.AuthenticationEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}

	user, err := client.hub.authenticationService.Validate(event.Token)
	if err != nil {
		slog.InfoContext(ctx, "Token re-validation failed, disconnecting client", slog.String("cid", client.GetCid()), slog.Any("error", err))
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
		slog.WarnContext(ctx, "handleMove: rejecting move event with invalid bay",
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

	if strip.IsValidationLocked() {
		return errors.New("strip is locked by an active validation")
	}

	if strip.Origin == client.airport && strip.Destination != client.airport && shared.IsArrivalBay(move.Bay) {
		return errors.New("departure strips cannot be moved to arrival bays")
	}

	// Ownership enforcement: reject the move if the strip is owned by someone else,
	// unless the target bay is an arrival bay (FINAL/RWY_ARR/TWY_ARR/STAND) or the client
	// holds an active coordination transfer for this strip.
	if strip.Owner != nil && *strip.Owner != "" && *strip.Owner != client.position {
		if !shared.IsArrivalBay(move.Bay) {
			coordRepo := client.hub.server.GetCoordinationRepository()
			coord, coordErr := coordRepo.GetByStripCallsign(ctx, client.session, move.Callsign)
			if coordErr != nil || coord == nil || coord.ToPosition != client.position {
				return errors.New("not authorized: strip is owned by another controller")
			}
		}
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

	isOwner := strip.Owner == nil || *strip.Owner == "" || *strip.Owner == client.position
	if !isOwner {
		// Non-owners may not modify EuroScope-forwarded fields (SID, route, stand, runway,
		// altitude, heading, remarks, aircraft info).
		if event.Sid != nil || event.Route != nil || event.Stand != nil || event.Runway != nil || event.Altitude != nil || event.Heading != nil || event.Remarks != nil || event.Aircraft != nil {
			return errors.New("non-owner cannot modify strip fields")
		}
		return nil
	}

	if event.Route != nil && stringPtrValue(strip.Route) != *event.Route {
		s.GetEuroscopeHub().SendRoute(client.session, client.GetCid(), event.Callsign, *event.Route)
	}

	aircraftChanged := event.Aircraft != nil && stringPtrValue(strip.AircraftType) != *event.Aircraft
	remarksChanged := event.Remarks != nil && stringPtrValue(strip.Remarks) != *event.Remarks
	if aircraftChanged && remarksChanged {
		s.GetEuroscopeHub().SendAircraftInfoAndRemarks(client.session, client.GetCid(), event.Callsign, *event.Aircraft, *event.Remarks)
	} else if aircraftChanged {
		s.GetEuroscopeHub().SendAircraftInfo(client.session, client.GetCid(), event.Callsign, *event.Aircraft)
	} else if remarksChanged {
		s.GetEuroscopeHub().SendRemarks(client.session, client.GetCid(), event.Callsign, *event.Remarks)
	}

	if event.Sid != nil && stringPtrValue(strip.Sid) != *event.Sid {
		s.GetEuroscopeHub().SendSid(client.session, client.GetCid(), event.Callsign, *event.Sid)
		if err := stripRepo.AppendControllerModifiedField(ctx, client.session, event.Callsign, "sid"); err != nil {
			return err
		}
		if reevaluator, ok := client.hub.stripService.(pdcInvalidValidationStripReevaluator); ok {
			sessionData, err := s.GetSessionRepository().GetByID(ctx, client.session)
			if err != nil {
				return err
			}
			updatedStrip := *strip
			updatedStrip.Sid = event.Sid
			if err := reevaluator.ReevaluatePdcInvalidValidationForStrip(ctx, client.session, &updatedStrip, sessionData.ActiveRunways.DepartureRunways, true, false); err != nil {
				return err
			}
		}
	}

	if event.Stand != nil && stringPtrValue(strip.Stand) != *event.Stand {
		s.GetEuroscopeHub().SendStand(client.session, client.GetCid(), event.Callsign, *event.Stand)
		if err := stripRepo.AppendControllerModifiedField(ctx, client.session, event.Callsign, "stand"); err != nil {
			return err
		}
		if client.hub.stripService != nil {
			if err := client.hub.stripService.UpdateStand(ctx, client.session, event.Callsign, *event.Stand); err != nil {
				return err
			}
		}
	}

	if event.Runway != nil && strip.Runway != event.Runway {
		s.GetEuroscopeHub().SendRunway(client.session, client.GetCid(), event.Callsign, *event.Runway)
		if _, err := stripRepo.UpdateRunway(ctx, client.session, event.Callsign, event.Runway, nil); err != nil {
			return err
		}
		if err := stripRepo.AppendControllerModifiedField(ctx, client.session, event.Callsign, "runway"); err != nil {
			return err
		}
		if client.hub.stripService != nil {
			if err := client.hub.stripService.ReevaluatePdcInvalidValidation(ctx, client.session, event.Callsign, true, false); err != nil {
				return err
			}
			if reevaluator, ok := client.hub.stripService.(departureValidationStripReevaluator); ok {
				if err := reevaluator.ReevaluateDepartureValidation(ctx, client.session, event.Callsign, true, false); err != nil {
					return err
				}
			}
		}
	}

	if event.Eobt != nil && stringPtrValue(strip.EffectiveEobt()) != strings.TrimSpace(*event.Eobt) {
		eobt := strings.TrimSpace(*event.Eobt)
		if !isValidFrontendClockValue(eobt) {
			return errors.New("invalid eobt: expected HHMM")
		}
		s.GetEuroscopeHub().SendEobt(client.session, client.GetCid(), event.Callsign, eobt)
		cdmService := client.hub.server.GetCdmService()
		if cdmService == nil {
			return errors.New("CDM service not available")
		}
		if err := cdmService.HandleEobtUpdate(ctx, client.session, event.Callsign, eobt, client.position, "ATC"); err != nil {
			return err
		}
		client.hub.SendStripUpdate(client.session, event.Callsign)
	}

	if event.Altitude != nil && strip.ClearedAltitude != event.Altitude {
		s.GetEuroscopeHub().SendClearedAltitude(client.session, client.GetCid(), event.Callsign, *event.Altitude)
		if err := stripRepo.AppendControllerModifiedField(ctx, client.session, event.Callsign, "cleared_altitude"); err != nil {
			return err
		}
	}

	if event.Heading != nil && strip.Heading != event.Heading {
		s.GetEuroscopeHub().SendHeading(client.session, client.GetCid(), event.Callsign, *event.Heading)
		if err := stripRepo.AppendControllerModifiedField(ctx, client.session, event.Callsign, "heading"); err != nil {
			return err
		}
	}

	return nil
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
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

	if strip.IsValidationLocked() {
		return errors.New("strip is locked by an active validation")
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
	strip, err := client.hub.server.GetStripRepository().GetByCallsign(ctx, client.session, req.Callsign)
	if err != nil {
		return err
	}
	if strip.IsValidationLocked() {
		return errors.New("strip is locked by an active validation")
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
	strip, err := client.hub.server.GetStripRepository().GetByCallsign(ctx, client.session, req.Callsign)
	if err != nil {
		return err
	}
	if strip.IsValidationLocked() {
		return errors.New("strip is locked by an active validation")
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

func handleCoordinationForceAssumeRequest(ctx context.Context, client *Client, message Message) error {
	var req frontend.CoordinationForceAssumeRequestEvent
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	return client.hub.stripService.ForceAssumeStrip(ctx, client.session, req.Callsign, client.position)
}

func handleCoordinationTagRequest(ctx context.Context, client *Client, message Message) error {
	var req frontend.CoordinationTagRequestEvent
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	strip, err := client.hub.server.GetStripRepository().GetByCallsign(ctx, client.session, req.Callsign)
	if err != nil {
		return err
	}
	if strip.IsValidationLocked() {
		return errors.New("strip is locked by an active validation")
	}
	return client.hub.stripService.CreateTagRequest(ctx, client.session, req.Callsign, client.position)
}

func handleCoordinationAcceptTagRequest(ctx context.Context, client *Client, message Message) error {
	var req frontend.CoordinationAcceptTagRequestEvent
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	strip, err := client.hub.server.GetStripRepository().GetByCallsign(ctx, client.session, req.Callsign)
	if err != nil {
		return err
	}
	if strip.IsValidationLocked() {
		return errors.New("strip is locked by an active validation")
	}
	return client.hub.stripService.AcceptTagRequest(ctx, client.session, req.Callsign, client.position)
}

func handleUpdateOrder(ctx context.Context, client *Client, message Message) error {
	var event frontend.UpdateOrderEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}

	s := client.hub.server
	stripRepo := s.GetStripRepository()

	strip, err := stripRepo.GetByCallsign(ctx, client.session, event.Callsign)
	if err != nil {
		return err
	}

	if strip.IsValidationLocked() {
		return errors.New("strip is locked by an active validation")
	}

	if strip.Bay == "" {
		return errors.New("cannot update order of a strip which is not in a bay")
	}

	return client.hub.stripService.MoveStripBetween(ctx, client.session, event.Callsign, event.InsertAfter, strip.Bay)
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
	return cdmService.HandleReadyRequest(ctx, client.session, event.Callsign, client.position, "ATC")
}

func handleClxOverrideValidation(ctx context.Context, client *Client, message Message) error {
	var event frontend.ClxOverrideValidationAction
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	if event.Callsign == "" || event.OverrideKey == "" {
		return errors.New("callsign and override_key are required")
	}

	client.hub.setClxOverride(client.session, event.OverrideKey)
	client.hub.SendStripUpdate(client.session, event.Callsign)
	return nil
}

func handleClxUpdateTobt(ctx context.Context, client *Client, message Message) error {
	var event frontend.ClxUpdateTobtAction
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	if event.Callsign == "" {
		return errors.New("callsign is required")
	}

	cdmService := client.hub.server.GetCdmService()
	if cdmService == nil {
		return errors.New("CDM service not available")
	}

	tobt := roundedClxTobt(time.Now().UTC())
	if err := cdmService.HandleClxTobtUpdate(ctx, client.session, event.Callsign, tobt, client.position, "ATC"); err != nil {
		return err
	}
	client.hub.SendStripUpdate(client.session, event.Callsign)
	return nil
}

func roundedClxTobt(now time.Time) string {
	target := now.Add(15 * time.Minute)
	if remainder := target.Minute() % 5; remainder != 0 {
		target = target.Add(time.Duration(5-remainder) * time.Minute)
	}
	return time.Date(target.Year(), target.Month(), target.Day(), target.Hour(), target.Minute(), 0, 0, time.UTC).Format("1504")
}

func isValidFrontendClockValue(value string) bool {
	if len(value) != 4 {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	hour := int(value[0]-'0')*10 + int(value[1]-'0')
	minute := int(value[2]-'0')*10 + int(value[3]-'0')
	return hour <= 23 && minute <= 59
}

func handleReleasePoint(ctx context.Context, client *Client, message Message) error {
	var event frontend.ReleasePointEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	strip, err := client.hub.server.GetStripRepository().GetByCallsign(ctx, client.session, event.Callsign)
	if err != nil {
		return err
	}
	if err := client.hub.stripService.ApplyReleasePoint(ctx, client.session, event.Callsign, event.ReleasePoint, client.position); err != nil {
		return err
	}
	isOwner := strip.Owner == nil || *strip.Owner == "" || *strip.Owner == client.position
	if !isOwner {
		return nil
	}
	return client.hub.server.GetStripRepository().AppendControllerModifiedField(ctx, client.session, event.Callsign, "release_point")
}

func handleAcknowledgeUnexpectedChange(ctx context.Context, client *Client, message Message) error {
	var event frontend.AcknowledgeUnexpectedChangeEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}

	s := client.hub.server
	stripRepo := s.GetStripRepository()
	if err := stripRepo.RemoveUnexpectedChangeField(ctx, client.session, event.Callsign, event.FieldName); err != nil {
		return err
	}
	if err := stripRepo.AppendControllerModifiedField(ctx, client.session, event.Callsign, event.FieldName); err != nil {
		return err
	}
	client.hub.SendStripUpdate(client.session, event.Callsign)
	return nil
}

func handleStartReq(ctx context.Context, client *Client, message Message) error {
	var event frontend.StartReqEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.UpdateStartReq(ctx, client.session, event.Callsign, event.StartReq)
}

func handleMarked(ctx context.Context, client *Client, message Message) error {
	var event frontend.MarkedEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.UpdateMarked(ctx, client.session, event.Callsign, event.Marked)
}

func handleRunwayClearance(ctx context.Context, client *Client, message Message) error {
	var event frontend.RunwayClearanceEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.RunwayClearance(ctx, client.session, event.Callsign, client.GetCid(), client.airport)
}

func handleRunwayConfirmation(ctx context.Context, client *Client, message Message) error {
	var event frontend.RunwayConfirmationEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.RunwayConfirmation(ctx, client.session, event.Callsign)
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
	if !validBays[req.Bay] {
		return errors.New("invalid bay: " + req.Bay)
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

	bay := req.Bay
	if bay != "" {
		if !validBays[bay] {
			slog.WarnContext(ctx, "handleMoveTacticalStrip: rejecting move event with invalid bay",
				slog.Int64("id", req.ID),
				slog.String("bay", bay))
			return errors.New("invalid bay: " + bay)
		}
	} else {
		strips, err := tacticalRepo.ListBySession(ctx, client.session)
		if err != nil {
			return err
		}
		for _, s := range strips {
			if s.ID == req.ID {
				bay = s.Bay
				break
			}
		}
		if bay == "" {
			return errors.New("tactical strip not found")
		}
	}

	return client.hub.stripService.MoveTacticalStripBetween(ctx, client.session, req.ID, req.InsertAfter, bay)
}

func handleMissedApproach(ctx context.Context, client *Client, message Message) error {
	var req frontend.MissedApproachRequestEvent
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	if err := client.hub.stripService.MissedApproach(ctx, client.session, req.Callsign, client.position); err != nil {
		return err
	}

	client.hub.SendGoAround(client.session, req.Callsign)
	return nil
}

func handleCreateManualFPL(ctx context.Context, client *Client, message Message) error {
	var req frontend.CreateManualFPLAction
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	return client.hub.stripService.CreateManualFPL(ctx, client.session, req, client.GetCid(), client.airport)
}

func handleCreateVFRFPL(ctx context.Context, client *Client, message Message) error {
	var req frontend.CreateVFRFPLAction
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	return client.hub.stripService.CreateVFRFPL(ctx, client.session, req, client.GetCid())
}

func handleUpdateRunwayStatus(ctx context.Context, client *Client, message Message) error {
	var event frontend.UpdateRunwayStatusAction
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}

	validStatuses := map[string]bool{"OPEN": true, "LOW_VIS": true, "CLOSED": true}
	if !validStatuses[event.Status] {
		return errors.New("invalid runway status")
	}

	sessionRepo := client.hub.server.GetSessionRepository()
	session, err := sessionRepo.GetByID(ctx, client.session)
	if err != nil {
		return err
	}

	if session.ActiveRunways.RunwayStatus == nil {
		session.ActiveRunways.RunwayStatus = make(map[string]string)
	}
	session.ActiveRunways.RunwayStatus[event.Pair] = event.Status

	if err = sessionRepo.UpdateActiveRunways(ctx, client.session, session.ActiveRunways); err != nil {
		return err
	}

	client.hub.server.GetCdmService().SyncAirportLvoFromRunwayStatus(ctx, session.Airport, session.ActiveRunways.RunwayStatus)

	client.hub.SendRunwayConfiguration(
		client.session,
		session.ActiveRunways.DepartureRunways,
		session.ActiveRunways.ArrivalRunways,
		session.ActiveRunways.RunwayStatus,
	)
	return nil
}

func handleAcknowledgeValidationStatus(ctx context.Context, client *Client, message Message) error {
	var req frontend.AcknowledgeValidationStatusAction
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	return client.hub.stripService.AcknowledgeValidationStatus(ctx, client.session, req.Callsign, req.ActivationKey, client.position)
}
