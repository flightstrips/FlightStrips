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

	if result.SingleOnPosition && positionName != "" && len(result.SectorChanges) > 0 {
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

	if result.ShouldScheduleTimer {
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

	if master, ok := client.hub.master[client.session]; ok && master == client {
		s := client.hub.server
		sessionRepo := s.GetSessionRepository()

		departure := make([]string, 0)
		arrival := make([]string, 0)

		for _, runway := range event.Runways {
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

		slog.Info("Runway change received",
			slog.Int("session", int(client.session)),
			slog.Any("departure", departure),
			slog.Any("arrival", arrival),
		)

		// Capture old active runways before overwriting.
		currentSession, err := sessionRepo.GetByID(ctx, client.session)
		if err != nil {
			return err
		}
		oldActiveRunways := currentSession.ActiveRunways

		if err = sessionRepo.UpdateActiveRunways(ctx, client.session, activeRunways); err != nil {
			return err
		}

		// Update runway on strips that had an auto-assigned runway matching the old
		// active runways. Strips with a manually-set runway (not matching old active)
		// are not touched.
		if err := client.hub.stripService.PropagateRunwayChange(ctx, client.session, currentSession.Airport, oldActiveRunways, activeRunways); err != nil {
			slog.Error("Failed to propagate runway change to strips", slog.Int("session", int(client.session)), slog.Any("error", err))
			// Non-fatal: continue — route recalculation is still attempted below.
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
		return nil
	}

	return nil
}
