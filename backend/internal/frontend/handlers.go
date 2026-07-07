package frontend

import (
	"FlightStrips/internal/config"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	euroscope "FlightStrips/pkg/events/euroscope"
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
	ReevaluatePdcInvalidValidation(ctx context.Context, session int32, callsign string, publish bool, forceReactivate bool) error
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
	if version := strings.TrimSpace(event.Version); version != "" {
		client.version = version
	}
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

	return client.hub.stripService.MoveFrontendStrip(ctx, client.session, move.Callsign, move.Bay, client.GetCid(), client.airport, client.position)
}

func handleStripUpdate(ctx context.Context, client *Client, message Message) error {
	var event frontend.UpdateStripDataEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}

	service := client.hub.getStripUpdateService()
	if service == nil {
		return errors.New("strip update service not available")
	}

	return service.UpdateStrip(ctx, FrontendStripUpdateRequest{
		Session:  client.session,
		Cid:      client.GetCid(),
		Position: client.position,
		Event:    event,
	})
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringPtrsEqual(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func int32PtrsEqual(left, right *int32) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
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

	if err := client.hub.stripService.CreateCoordinationTransfer(ctx, client.session, req.Callsign, position, req.To); err != nil {
		return err
	}

	if strip.Marked {
		if err := client.hub.stripService.UpdateMarked(ctx, client.session, req.Callsign, false); err != nil {
			slog.WarnContext(ctx, "handleCoordinationTransferRequest: transfer succeeded but failed to clear mark",
				slog.String("callsign", req.Callsign),
				slog.Int("session", int(client.session)),
				slog.Any("error", err),
			)
		}
	}

	return nil
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

	ts, err := tacticalRepo.GetByID(ctx, req.ID, client.session)
	if err != nil {
		return err
	}

	if ts.Type != internalModels.TacticalStripTypeMemaid && ts.Type != internalModels.TacticalStripTypeCrossing {
		return errors.New("confirm is only valid for MEMAID and CROSSING strips")
	}
	if ts.ProducedBy == client.position {
		return errors.New("producer cannot confirm their own MEMAID strip")
	}

	ts, err = tacticalRepo.Confirm(ctx, req.ID, client.session, client.position)
	if err != nil {
		return err
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

	ts, err := tacticalRepo.GetByID(ctx, req.ID, client.session)
	if err != nil {
		return err
	}

	if ts.Type != internalModels.TacticalStripTypeStart && ts.Type != internalModels.TacticalStripTypeLand {
		return errors.New("start timer is only valid for START and LAND strips")
	}

	ts, err = tacticalRepo.StartTimer(ctx, req.ID, client.session)
	if err != nil {
		return err
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
	if client.hub.validationService == nil {
		return errors.New("acknowledge_validation_status: validation service is not configured")
	}
	return client.hub.validationService.AcknowledgeValidationStatus(ctx, client.session, req.Callsign, req.ActivationKey, client.position)
}

func handleSendPrivateMessage(ctx context.Context, client *Client, message Message) error {
	var req frontend.SendPrivateMessageEvent
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	event := euroscope.SendPrivateMessageEvent{
		Callsign: req.Callsign,
		Message:  req.Message,
	}
	euroscopeHub := client.hub.server.GetEuroscopeHub()
	euroscopeHub.Send(client.session, client.GetCid(), event)
	return nil
}
