package euroscope

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	"FlightStrips/pkg/events/euroscope"
	"FlightStrips/pkg/models"
	"context"
	"log/slog"

	gorilla "github.com/gorilla/websocket"
)

type Message = shared.Message[euroscope.EventType]

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

	result, err := client.hub.controllerService.ControllerOnline(ctx, session, event.Callsign, event.Position, positionName)
	if err != nil {
		return err
	}

	slog.Debug("Controller online result",
		slog.String("callsign", event.Callsign),
		slog.String("position", event.Position),
		slog.String("positionName", positionName),
		slog.Bool("notifyOnline", result.NotifyOnline),
		slog.Bool("singleOnPosition", result.SingleOnPosition),
		slog.Int("sectorChanges", len(result.SectorChanges)))

	if result.NotifyOnline {
		client.hub.server.GetFrontendHub().SendControllerOnline(session, event.Callsign, event.Position, "")
	}

	if result.SingleOnPosition && positionName != "" {
		slog.Info("Scheduling online broadcast",
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

	slog.Debug("Controller offline result",
		slog.String("callsign", event.Callsign),
		slog.Bool("shouldScheduleTimer", result.ShouldScheduleTimer),
		slog.String("positionName", result.PositionName),
		slog.Int("session", int(session)))

	if result.ShouldScheduleTimer {
		slog.Info("Scheduling offline grace period timer",
			slog.String("callsign", event.Callsign),
			slog.String("position", result.PositionName),
			slog.Int("session", int(session)))
		client.hub.scheduleOfflineActions(session, event.Callsign, result.PositionFrequency, result.PositionName)
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
	return client.hub.stripService.DeleteStrip(ctx, client.session, event.Callsign)
}

func handleStand(ctx context.Context, client *Client, message Message) error {
	var event euroscope.StandEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.stripService.UpdateStand(ctx, client.session, event.Callsign, event.Stand)
}

func handleCdmLocalData(ctx context.Context, client *Client, message Message) error {
	var event euroscope.CdmLocalDataEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}

	if event.SourcePosition == "" {
		event.SourcePosition = client.callsign
	}

	if event.SourceRole == "" {
		if master, ok := client.hub.master[client.session]; ok && master == client {
			event.SourceRole = "master"
		} else {
			event.SourceRole = "slave"
		}
	}

	return client.hub.server.GetCdmService().HandleLocalObservation(ctx, client.session, event)
}

func handlePositionUpdate(ctx context.Context, client *Client, message Message) error {
	var event euroscope.AircraftPositionUpdateEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
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
	return client.hub.stripService.HandleCoordinationReceived(ctx, client.session, event.Callsign, event.ControllerCallsign)
}

func handleSync(ctx context.Context, client *Client, message Message) error {
	var event euroscope.SyncEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}

	s := client.hub.server
	session := client.session

	slog.Debug("Received sync event", slog.Int("session", int(session)), slog.String("client", client.callsign))

	for _, controller := range event.Controllers {
		if err := client.hub.controllerService.UpsertController(ctx, session, controller.Callsign, controller.Position); err != nil {
			return err
		}
	}

	if len(event.Runways) > 0 {
		if err := applyOrValidateRunways(ctx, client, event.Runways); err != nil {
			return err
		}
	}

	if _, err := s.UpdateSectors(session); err != nil {
		return err
	}
	if err := s.UpdateLayouts(session); err != nil {
		return err
	}

	for _, strip := range event.Strips {
		if err := client.hub.stripService.SyncStrip(ctx, session, strip, client.airport); err != nil {
			return err
		}
	}

	// Auto-assume for all controllers visible in the sync, plus the local (master) controller.
	// The local controller is not included in event.Controllers (which only lists remote controllers),
	// but its strips may be waiting for assumption.
	positionsToAssume := make(map[string]bool)
	for _, controller := range event.Controllers {
		positionsToAssume[controller.Position] = true
	}
	if client.position != "" {
		positionsToAssume[client.position] = true
	}
	for position := range positionsToAssume {
		if err := client.hub.stripService.AutoAssumeForControllerOnline(ctx, session, position); err != nil {
			slog.Error("AutoAssumeForControllerOnline failed during sync",
				slog.String("position", position), slog.Any("error", err))
		}
	}

	client.hub.server.GetFrontendHub().CidOnline(session, client.user.GetCid())

	if len(event.Sids) > 0 {
		sessionRepo := s.GetSessionRepository()
		availSids := models.AvailableSids(event.Sids)
		if err := sessionRepo.UpdateSessionSids(ctx, session, availSids); err != nil {
			slog.Error("Failed to persist available SIDs", slog.Any("error", err))
			// non-fatal — do not return
		}
		s.GetFrontendHub().SendAvailableSids(session, availSids)
	}

	return nil
}

func (hub *Hub) handleStripUpdateHelper(ctx context.Context, strip euroscope.Strip, session int32, airport string) error {
	return hub.stripService.SyncStrip(ctx, session, strip, airport)
}

func handleStripUpdateEvent(ctx context.Context, client *Client, message Message) error {
	var event euroscope.StripUpdateEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}
	return client.hub.handleStripUpdateHelper(ctx, event.Strip, client.session, client.airport)
}

func handleRunways(ctx context.Context, client *Client, message Message) error {
	var event euroscope.RunwayEvent
	if err := message.JsonUnmarshal(&event); err != nil {
		return err
	}

	slog.Debug("Received runway configuration change", slog.Int("session", int(client.session)), slog.Any("event", event))

	return applyOrValidateRunways(ctx, client, event.Runways)
}

// applyOrValidateRunways applies the runway configuration when the client is master,
// or compares it against the session's current runways and logs a warning if they differ
// (conflict detection for slave clients).
func applyOrValidateRunways(ctx context.Context, client *Client, runways []euroscope.SyncRunway) error {
	s := client.hub.server
	sessionRepo := s.GetSessionRepository()

	departure := make([]string, 0)
	arrival := make([]string, 0)
	for _, runway := range runways {
		if runway.Arrival {
			arrival = append(arrival, runway.Name)
		}
		if runway.Departure {
			departure = append(departure, runway.Name)
		}
	}

	activeRunways := models.ActiveRunways{
		DepartureRunways: departure,
		ArrivalRunways:   arrival,
	}

	isMaster := false
	if master, ok := client.hub.master[client.session]; ok && master == client {
		isMaster = true
	}

	if !isMaster {
		currentSession, err := sessionRepo.GetByID(ctx, client.session)
		if err != nil {
			return err
		}
		masterDep := currentSession.ActiveRunways.DepartureRunways
		masterArr := currentSession.ActiveRunways.ArrivalRunways
		if !slicesEqual(masterDep, departure) || !slicesEqual(masterArr, arrival) {
			slog.Warn("Slave ES client has different runway configuration than master",
				slog.Int("session", int(client.session)),
				slog.String("client", client.callsign),
				slog.Any("slave_departure", departure),
				slog.Any("slave_arrival", arrival),
				slog.Any("master_departure", masterDep),
				slog.Any("master_arrival", masterArr),
			)
		}
		return nil
	}

	slog.Info("Runway change received",
		slog.Int("session", int(client.session)),
		slog.Any("departure", departure),
		slog.Any("arrival", arrival),
	)

	currentSession, err := sessionRepo.GetByID(ctx, client.session)
	if err != nil {
		return err
	}
	oldActiveRunways := currentSession.ActiveRunways

	if err = sessionRepo.UpdateActiveRunways(ctx, client.session, activeRunways); err != nil {
		return err
	}

	if err := client.hub.stripService.PropagateRunwayChange(ctx, client.session, currentSession.Airport, oldActiveRunways, activeRunways); err != nil {
		slog.Error("Failed to propagate runway change to strips", slog.Int("session", int(client.session)), slog.Any("error", err))
	}

	s.GetFrontendHub().SendRunwayConfiguration(client.session, departure, arrival)

	if _, err = s.UpdateSectors(client.session); err != nil {
		slog.Error("UpdateSectors failed after runway change", slog.Int("session", int(client.session)), slog.Any("error", err))
		return err
	}
	slog.Debug("UpdateSectors completed", slog.Int("session", int(client.session)))

	if err = s.UpdateRoutesForSession(client.session, true); err != nil {
		slog.Error("UpdateRoutesForSession failed after runway change", slog.Int("session", int(client.session)), slog.Any("error", err))
		return err
	}
	slog.Debug("UpdateRoutesForSession completed", slog.Int("session", int(client.session)))

	// Recalculate and broadcast per-controller layouts after runway change.
	// Do not return on failure — a layout error must not block the runway change.
	if err = s.UpdateLayouts(client.session); err != nil {
		slog.Error("Failed to update layouts after runway change",
			slog.Int("session", int(client.session)),
			slog.Any("error", err))
	}

	return nil
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
