package euroscope

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/metrics"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	"FlightStrips/pkg/events/euroscope"
	"context"
	"log/slog"
	"regexp"
	"strings"
	"time"

	gorilla "github.com/gorilla/websocket"
)

type Message = shared.Message[euroscope.EventType]

func handleLoginEvent(ctx context.Context, client *Client, message Message) error {
	previousPosition := client.position
	previousCallsign := client.callsign
	previousAirport := client.airport

	event, _, err := client.hub.handleLogin(message.Message, client.user)
	if err != nil {
		return err
	}

	client.position = event.Position
	client.callsign = event.Callsign
	client.observer = event.Observer
	client.localIP = event.LocalIP
	client.hub.setObserverCid(client.GetCid(), event.Observer)
	client.hub.setClientLocalIP(client.session, client.GetCid(), event.LocalIP)
	if master, ok := client.hub.master[client.session]; ok && master == client && previousCallsign != client.callsign {
		client.hub.setMasterClient(client)
	}

	if !event.Observer {
		client.hub.markPendingOnlineOrchestration(client.session, client.callsign)
		if layoutErr := client.hub.server.UpdateLayouts(client.session); layoutErr != nil {
			slog.ErrorContext(ctx, "Failed to update layouts after ES re-login", slog.String("cid", client.GetCid()), slog.Any("error", layoutErr))
		}
	} else if previousPosition != client.position || previousCallsign != client.callsign || previousAirport != client.airport {
		client.hub.server.GetFrontendHub().CidOnline(client.session, client.GetCid())
	}

	return nil
}

var hhmmPattern = regexp.MustCompile(`^(?:[01]\d|2[0-3])[0-5]\d$`)

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

func handleControllerOnline(ctx context.Context, client *Client, message Message) error {
	var event euroscope.ControllerOnlineEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	session := client.session

	// Resolve the position name for the timer key.
	positionName := ""
	if posConfig, configErr := config.GetPositionBasedOnFrequency(event.Position); configErr == nil {
		positionName = posConfig.Name
	}

	// Cancel any pending offline timer for this position.
	if positionName != "" {
		client.hub.cancelOfflineTimer(session, positionName)
	}

	result, err := client.hub.controllerService.ControllerOnlineWithOptions(
		ctx,
		session,
		event.Callsign,
		event.Position,
		positionName,
		shared.ControllerOnlineOptions{
			ForceOrchestration: client.hub.consumePendingOnlineOrchestration(session, event.Callsign),
		},
	)
	if err != nil {
		return err
	}

	slog.DebugContext(ctx, "Controller online result",
		slog.String("callsign", event.Callsign),
		slog.String("position", event.Position),
		slog.String("positionName", positionName),
		slog.Bool("notifyOnline", result.NotifyOnline),
		slog.Bool("singleOnPosition", result.SingleOnPosition),
		slog.Int("sectorChanges", len(result.SectorChanges)))

	if result.NotifyOnline {
		client.hub.server.GetFrontendHub().SendControllerOnline(session, event.Callsign, event.Position, "", nil)
	}

	if result.SingleOnPosition && positionName != "" {
		slog.InfoContext(ctx, "Scheduling online broadcast",
			slog.String("position", positionName),
			slog.String("callsign", event.Callsign),
			slog.Int("session", int(session)))
		client.hub.scheduleOnlineBroadcast(session, positionName, result.SectorChanges)
	}

	return nil
}

func handleControllerOffline(ctx context.Context, client *Client, message Message) error {
	var event euroscope.ControllerOfflineEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	session := client.session

	result, err := client.hub.controllerService.ControllerOffline(ctx, session, event.Callsign)
	if err != nil {
		return err
	}

	slog.DebugContext(ctx, "Controller offline result",
		slog.String("callsign", event.Callsign),
		slog.Bool("shouldScheduleTimer", result.ShouldScheduleTimer),
		slog.String("positionName", result.PositionName),
		slog.Int("session", int(session)))

	if result.ShouldScheduleTimer {
		slog.InfoContext(ctx, "Scheduling offline grace period timer",
			slog.String("callsign", event.Callsign),
			slog.String("position", result.PositionName),
			slog.Int("session", int(session)))
		client.hub.scheduleOfflineActions(session, event.Callsign, result.PositionFrequency, result.PositionName, controllerOfflineGracePeriod)
	}

	return nil
}

func handleAssignedSquawk(ctx context.Context, client *Client, message Message) error {
	var event euroscope.AssignedSquawkEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.UpdateAssignedSquawk(ctx, client.session, event.Callsign, event.Squawk)
}

func handleSquawk(ctx context.Context, client *Client, message Message) error {
	var event euroscope.SquawkEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.UpdateSquawk(ctx, client.session, event.Callsign, event.Squawk)
}

func handleRequestedAltitude(ctx context.Context, client *Client, message Message) error {
	var event euroscope.RequestedAltitudeEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.UpdateRequestedAltitude(ctx, client.session, event.Callsign, event.Altitude)
}

func handleClearedAltitude(ctx context.Context, client *Client, message Message) error {
	var event euroscope.ClearedAltitudeEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.UpdateClearedAltitude(ctx, client.session, event.Callsign, event.Altitude)
}

func handleCommunicationType(ctx context.Context, client *Client, message Message) error {
	var event euroscope.CommunicationTypeEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.UpdateCommunicationType(ctx, client.session, event.Callsign, event.CommunicationType)
}

func handleGroundState(ctx context.Context, client *Client, message Message) error {
	var event euroscope.GroundStateEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.UpdateGroundState(ctx, client.session, event.Callsign, event.GroundState, client.airport)
}

func handleClearedFlag(ctx context.Context, client *Client, message Message) error {
	var event euroscope.ClearedFlagEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.UpdateClearedFlag(ctx, client.session, event.Callsign, event.Cleared)
}

func handleSetHeading(ctx context.Context, client *Client, message Message) error {
	var event euroscope.HeadingEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.UpdateHeading(ctx, client.session, event.Callsign, event.Heading)
}

func handleAircraftDisconnected(ctx context.Context, client *Client, message Message) error {
	var event euroscope.AircraftDisconnectEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	client.hub.scheduleAircraftDisconnect(client.session, event.Callsign, offlineGracePeriod)
	return nil
}

func handleStand(ctx context.Context, client *Client, message Message) error {
	var event euroscope.StandEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.UpdateStand(ctx, client.session, event.Callsign, event.Stand)
}

func handleCdmTobtUpdate(ctx context.Context, client *Client, message Message) error {
	var event euroscope.CdmTobtUpdateEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	if !hhmmPattern.MatchString(event.Tobt) {
		return nil
	}
	return client.hub.server.GetCdmService().HandleTobtUpdate(ctx, client.session, event.Callsign, event.Tobt, client.callsign, clientRole(client))
}

func handleCdmDeiceUpdate(ctx context.Context, client *Client, message Message) error {
	var event euroscope.CdmDeiceUpdateEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	switch event.DeiceType {
	case "", "L", "M", "H", "J":
	default:
		return nil
	}
	return client.hub.server.GetCdmService().HandleDeiceUpdate(ctx, client.session, event.Callsign, event.DeiceType)
}

func handleCdmAsrtToggle(ctx context.Context, client *Client, message Message) error {
	var event euroscope.CdmAsrtToggleEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.server.GetCdmService().HandleAsrtToggle(ctx, client.session, event.Callsign, event.Asrt)
}

func handleCdmTsacUpdate(ctx context.Context, client *Client, message Message) error {
	var event euroscope.CdmTsacUpdateEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.server.GetCdmService().HandleTsacUpdate(ctx, client.session, event.Callsign, event.Tsac)
}

func handleCdmManualCtot(ctx context.Context, client *Client, message Message) error {
	var event euroscope.CdmManualCtotEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	if !hhmmPattern.MatchString(event.Ctot) {
		return nil
	}
	return client.hub.server.GetCdmService().HandleManualCtot(ctx, client.session, event.Callsign, event.Ctot)
}

func handleCdmCtotRemove(ctx context.Context, client *Client, message Message) error {
	var event euroscope.CdmCtotRemoveEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.server.GetCdmService().HandleCtotRemove(ctx, client.session, event.Callsign)
}

func handleCdmApproveReqTobt(ctx context.Context, client *Client, message Message) error {
	var event euroscope.CdmApproveReqTobtEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.server.GetCdmService().HandleApproveReqTobt(ctx, client.session, event.Callsign, client.callsign, clientRole(client))
}

func handlePositionUpdate(ctx context.Context, client *Client, message Message) error {
	var event euroscope.AircraftPositionUpdateEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	client.hub.cancelAircraftDisconnect(client.session, event.Callsign)
	return client.hub.stripService.UpdateAircraftPosition(ctx, client.session, event.Callsign, event.Lat, event.Lon, int32(event.Altitude), client.airport)
}

func handleTrackingControllerChanged(ctx context.Context, client *Client, message Message) error {
	var event euroscope.TrackingControllerChangedEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.HandleTrackingControllerChanged(ctx, client.session, event.Callsign, event.TrackingController)
}

func handleCoordinationReceived(ctx context.Context, client *Client, message Message) error {
	var event euroscope.CoordinationReceivedEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.HandleCoordinationReceived(
		ctx,
		client.session,
		event.Callsign,
		event.SourceControllerCallsign,
		event.ControllerCallsign,
	)
}

func handleSync(ctx context.Context, client *Client, message Message) error {
	startedAt := time.Now()

	var event euroscope.SyncEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}

	service := newEuroscopeSyncServiceForClient(client)
	result, err := service.ApplySync(ctx, newEuroscopeSyncRequest(client, event))
	if err != nil {
		return err
	}

	if result.MarkSessionSynced {
		client.hub.markSessionSynced(client.session)
	}
	if result.WakeFrontendCID != "" {
		client.hub.server.GetFrontendHub().CidOnline(client.session, result.WakeFrontendCID)
	}

	metrics.RecordEuroscopeSync(
		ctx,
		result.Metrics.SessionName,
		result.Metrics.Airport,
		client.version,
		result.Metrics.StripCount,
		result.Metrics.ControllerCount,
		result.Metrics.ChangedStrips,
		result.Metrics.ChangedControllers,
		result.Metrics.DBOperations,
		time.Since(startedAt),
	)

	return nil
}

func handleStripUpdateEvent(ctx context.Context, client *Client, message Message) error {
	var event euroscope.StripUpdateEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	client.hub.cancelAircraftDisconnect(client.session, event.Callsign)
	return client.hub.stripService.SyncStrip(ctx, client.session, client.GetCid(), event.Strip, client.airport)
}

func handleRunways(ctx context.Context, client *Client, message Message) error {
	var event euroscope.RunwayEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}

	slog.DebugContext(ctx, "Received runway configuration change", slog.Int("session", int(client.session)), slog.Any("event", event))

	_, err := applyOrValidateRunways(ctx, client, event.Runways)
	return err
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func clientRole(client *Client) string {
	if master, ok := client.hub.master[client.session]; ok && master == client {
		return "master"
	}
	return "slave"
}

func handleCdmMasterToggle(ctx context.Context, client *Client, message Message) error {
	var event euroscope.CdmMasterToggleEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.server.GetCdmService().SetSessionCdmMaster(ctx, client.session, event.Master)
}

func handleIssuePdcClearance(ctx context.Context, client *Client, message Message) error {
	var event euroscope.IssuePdcClearanceEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	pdcService := client.hub.server.GetPdcService()
	if pdcService == nil {
		return nil
	}
	return pdcService.IssueClearance(ctx, event.Callsign, event.Remarks, client.GetCid(), client.session)
}

func handlePdcRevertToVoice(ctx context.Context, client *Client, message Message) error {
	var event euroscope.PdcRevertToVoiceEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	pdcService := client.hub.server.GetPdcService()
	if pdcService == nil {
		return nil
	}
	return pdcService.RevertToVoice(ctx, event.Callsign, client.session, client.GetCid())
}

func handleSendPrivateMessage(ctx context.Context, client *Client, message Message) error {
	var event euroscope.SendPrivateMessageEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	client.hub.Broadcast(client.session, event)
	return nil
}
