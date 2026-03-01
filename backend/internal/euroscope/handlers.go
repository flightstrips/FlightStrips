package euroscope

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/services"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	"FlightStrips/pkg/events/euroscope"
	"FlightStrips/pkg/models"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	gorilla "github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
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
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	controllerRepo := s.GetControllerRepository()
	controller, err := controllerRepo.GetByCallsign(ctx, session, event.Callsign)

	if errors.Is(err, pgx.ErrNoRows) {
		newController := &internalModels.Controller{
			Callsign: event.Callsign,
			Position: event.Position,
			Session:  session,
		}

		err = controllerRepo.Create(ctx, newController)
		if err != nil {
			return err
		}
		err = s.UpdateSectors(client.session)
		if err != nil {
			return err
		}
		return s.UpdateLayouts(session)
	}

	if controller.Position == event.Position || err != nil {
		return err
	}

	_, err = controllerRepo.SetPosition(ctx, session, event.Callsign, event.Position)
	if err != nil {
		return err
	}

	shouldUpdate := false
	if _, err := config.GetPositionBasedOnFrequency(event.Position); err == nil {
		shouldUpdate = true
	}

	if _, err := config.GetPositionBasedOnFrequency(controller.Position); err == nil {
		shouldUpdate = true
	}

	slog.Debug("Controller online with updated position", slog.String("callsign", event.Callsign), slog.String("position", event.Position), slog.Bool("shouldUpdate", shouldUpdate))

	if shouldUpdate {
		err = s.UpdateSectors(client.session)
		if err != nil {
			return err
		}
		return s.UpdateLayouts(session)
	}

	return nil
}

func handleControllerOffline(ctx context.Context, client *Client, message Message) error {
	var event euroscope.ControllerOfflineEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	controllerRepo := s.GetControllerRepository()
	controller, err := controllerRepo.GetByCallsign(ctx, session, event.Callsign)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	if errors.Is(err, pgx.ErrNoRows) {
		slog.Debug("Controller going offline does not exist in database", slog.String("callsign", event.Callsign))
		s.GetFrontendHub().SendControllerOffline(session, event.Callsign, "", "")
		return nil
	}

	err = controllerRepo.Delete(ctx, session, event.Callsign)

	s.GetFrontendHub().SendControllerOffline(session, event.Callsign, controller.Position, "")
	if err != nil {
		return err
	}

	if _, err := config.GetPositionBasedOnFrequency(controller.Position); err == nil {
		if err := s.UpdateSectors(client.session); err != nil {
			return err
		}
		return s.UpdateLayouts(client.session)
	}

	return nil
}

func handleAssignedSquawk(ctx context.Context, client *Client, message Message) error {
	var event euroscope.AssignedSquawkEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	stripRepo := s.GetStripRepository()
	count, err := stripRepo.UpdateAssignedSquawk(ctx, session, event.Callsign, &event.Squawk, nil)

	if err != nil {
		return err
	}

	if count != 1 {
		slog.Debug("Strip being updated does not exist in database", slog.String("callsign", event.Callsign), slog.String("event", "AssignedSquawk"))
	} else {
		s.GetFrontendHub().SendAssignedSquawkEvent(session, event.Callsign, event.Squawk)
	}

	return err
}

func handleSquawk(ctx context.Context, client *Client, message Message) error {
	var event euroscope.SquawkEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	stripRepo := s.GetStripRepository()
	count, err := stripRepo.UpdateSquawk(ctx, session, event.Callsign, &event.Squawk, nil)

	if err != nil {
		return err
	}

	if count != 1 {
		slog.Debug("Strip being updated does not exist in database", slog.String("callsign", event.Callsign), slog.String("event", "Squawk"))
	} else {
		s.GetFrontendHub().SendSquawkEvent(session, event.Callsign, event.Squawk)
	}

	return err
}

func handleRequestedAltitude(ctx context.Context, client *Client, message Message) error {
	var event euroscope.RequestedAltitudeEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	stripRepo := s.GetStripRepository()

	intAltitude := int32(event.Altitude)
	count, err := stripRepo.UpdateRequestedAltitude(ctx, session, event.Callsign, &intAltitude, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		slog.Debug("Strip being updated does not exist in database", slog.String("callsign", event.Callsign), slog.String("event", "RequestedAltitude"))
	} else {
		s.GetFrontendHub().SendRequestedAltitudeEvent(session, event.Callsign, event.Altitude)
	}
	return err
}

func handleClearedAltitude(ctx context.Context, client *Client, message Message) error {
	var event euroscope.ClearedAltitudeEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	stripRepo := s.GetStripRepository()
	intAltitude := int32(event.Altitude)
	count, err := stripRepo.UpdateClearedAltitude(ctx, session, event.Callsign, &intAltitude, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		slog.Debug("Strip being updated does not exist in database", slog.String("callsign", event.Callsign), slog.String("event", "ClearedAltitude"))
	} else {
		s.GetFrontendHub().SendClearedAltitudeEvent(session, event.Callsign, event.Altitude)
	}
	return err
}

func handleCommunicationType(ctx context.Context, client *Client, message Message) error {
	var event euroscope.CommunicationTypeEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	stripRepo := s.GetStripRepository()

	count, err := stripRepo.UpdateCommunicationType(ctx, session, event.Callsign, &event.CommunicationType, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		slog.Debug("Strip being updated does not exist in database", slog.String("callsign", event.Callsign), slog.String("event", "CommunicationType"))
		return nil
	}
	s.GetFrontendHub().SendCommunicationTypeEvent(session, event.Callsign, event.CommunicationType)
	return nil
}

func handleGroundState(ctx context.Context, client *Client, message Message) error {
	var event euroscope.GroundStateEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	stripRepo := s.GetStripRepository()
	existingStrip, err := stripRepo.GetByCallsign(ctx, session, event.Callsign)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Debug("Strip being updated does not exist in database", slog.String("callsign", event.Callsign), slog.String("event", "GroundState"))
			return nil
		}
		return err
	}

	if existingStrip.State != nil && *existingStrip.State == event.GroundState {
		return nil
	}

	// Convert domain model to database model for shared helper function
	dbStrip := database.Strip{
		Origin:      existingStrip.Origin,
		Destination: existingStrip.Destination,
		Cleared:     existingStrip.Cleared,
	}
	bay := shared.GetDepartureBayFromGroundState(event.GroundState, dbStrip)

	_, err = stripRepo.UpdateGroundState(ctx, session, event.Callsign, &event.GroundState, bay, nil)

	if err != nil {
		return err
	}

	if existingStrip.Bay != bay {
		return client.hub.stripService.MoveToBay(context.Background(), client.session, event.Callsign, bay, true)
	}

	return nil
}

func handleClearedFlag(ctx context.Context, client *Client, message Message) error {
	var event euroscope.ClearedFlagEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	stripRepo := s.GetStripRepository()
	existingStrip, err := stripRepo.GetByCallsign(ctx, session, event.Callsign)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Debug("Strip being updated does not exist in database", slog.String("callsign", event.Callsign), slog.String("event", "FlightStripOnline"))
			return nil
		}
		return err
	}

	if existingStrip.Cleared == event.Cleared {
		return nil
	}

	bay := existingStrip.Bay
	if bay == shared.BAY_NOT_CLEARED || bay == shared.BAY_UNKNOWN {
		bay = shared.BAY_CLEARED
	}

	_, err = stripRepo.UpdateClearedFlag(ctx, session, event.Callsign, event.Cleared, bay, nil)
	if err != nil {
		return err
	}

	if event.Cleared {
		if err := client.hub.stripService.AutoAssumeForClearedStrip(ctx, session, event.Callsign, existingStrip.Version+1); err != nil {
			slog.Error("Failed to auto-assume cleared strip from EuroScope", slog.Any("error", err))
		}
	}

	if existingStrip.Bay != bay {
		return client.hub.stripService.MoveToBay(ctx, client.session, event.Callsign, bay, true)
	}

	return err
}

func handlePositionUpdate(ctx context.Context, client *Client, message Message) error {
	var event euroscope.AircraftPositionUpdateEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	stripRepo := s.GetStripRepository()
	existingStrip, err := stripRepo.GetByCallsign(ctx, session, event.Callsign)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Debug("Strip being updated does not exist in database", slog.String("callsign", event.Callsign), slog.String("event", "FlightStripOffline"))
			return nil
		}
		return err
	}

	// Convert domain model to database model for shared helper function
	dbStrip := database.Strip{
		Origin:      existingStrip.Origin,
		Destination: existingStrip.Destination,
		Cleared:     existingStrip.Cleared,
		Bay:         existingStrip.Bay,
	}
	bay := shared.GetDepartureBayFromPosition(event.Lat, event.Lon, event.Altitude, dbStrip)
	intAltitude := int32(event.Altitude)

	_, err = stripRepo.UpdateAircraftPosition(ctx, session, event.Callsign, &event.Lat, &event.Lon, &intAltitude, bay, nil)

	if err != nil {
		return err
	}

	if existingStrip.Bay != bay {
		return client.hub.stripService.MoveToBay(context.Background(), client.session, event.Callsign, bay, true)
	}

	return nil
}

func handleSetHeading(ctx context.Context, client *Client, message Message) error {
	var event euroscope.HeadingEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	stripRepo := s.GetStripRepository()
	count, err := stripRepo.UpdateHeading(ctx, session, event.Callsign, &event.Heading, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		slog.Debug("Strip being updated does not exist in database", slog.String("callsign", event.Callsign), slog.String("event", "SetHeading"))
		return nil
	}
	s.GetFrontendHub().SendSetHeadingEvent(session, event.Callsign, event.Heading)
	return nil
}

func handleAircraftDisconnected(ctx context.Context, client *Client, message Message) error {
	var event euroscope.AircraftDisconnectEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	stripRepo := s.GetStripRepository()
	err = stripRepo.Delete(ctx, session, event.Callsign)
	s.GetFrontendHub().SendAircraftDisconnect(session, event.Callsign)
	return err
}

func handleStand(ctx context.Context, client *Client, message Message) error {
	var event euroscope.StandEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	stripRepo := s.GetStripRepository()
	count, err := stripRepo.UpdateStand(ctx, session, event.Callsign, &event.Stand, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		slog.Debug("Strip being updated does not exist in database", slog.String("callsign", event.Callsign), slog.String("event", "Stand"))
		return nil
	}

	s.GetFrontendHub().SendStandEvent(session, event.Callsign, event.Stand)
	return nil
}

func handleSync(ctx context.Context, client *Client, message Message) error {
	var event euroscope.SyncEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}

	s := client.hub.server
	session := client.session

	controllerRepo := s.GetControllerRepository()

	for _, controller := range event.Controllers {
		// Check if the controller exists
		_, err := controllerRepo.GetByCallsign(ctx, session, controller.Callsign)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return errors.Join(errors.New("something went wrong with fetching controller"), err)
		}

		if errors.Is(err, pgx.ErrNoRows) {
			// Controller doesn't exist, so insert
			newController := &internalModels.Controller{
				Callsign: controller.Callsign,
				Session:  session,
				Position: controller.Position,
				Cid:      nil,
			}
			err = controllerRepo.Create(ctx, newController)
			if err != nil {
				return fmt.Errorf("error inserting controller: %w", err)
			}
			slog.Debug("Inserted controller", slog.String("callsign", controller.Callsign))
		} else {
			// Controller exists, update it
			_, err = controllerRepo.SetPosition(ctx, session, controller.Callsign, controller.Position)
			if err != nil {
				return fmt.Errorf("error updating controller position: %w", err)
			}
			slog.Debug("Updated controller", slog.String("callsign", controller.Callsign))
		}
	}

	err = s.UpdateSectors(client.session)
	if err != nil {
		return err
	}
	err = s.UpdateLayouts(client.session)
	if err != nil {
		return err
	}

	for _, strip := range event.Strips {
		err = client.hub.handleStripUpdateHelper(ctx, strip, session)
		if err != nil {
			return err
		}
	}

	return err
}

func (hub *Hub) handleStripUpdateHelper(ctx context.Context, strip euroscope.Strip, session int32) error {
	server := hub.server
	stripRepo := server.GetStripRepository()

	existingStrip, err := stripRepo.GetByCallsign(ctx, session, strip.Callsign)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	var bay string

	if errors.Is(err, pgx.ErrNoRows) {
		// Strip doesn't exist, so insert
		bay = shared.GetDepartureBay(strip, nil)

		newStrip := &internalModels.Strip{
			Callsign:          strip.Callsign,
			Session:           session,
			Origin:            strip.Origin,
			Destination:       strip.Destination,
			Alternative:       &strip.Alternate,
			Route:             &strip.Route,
			Remarks:           &strip.Remarks,
			Runway:            &strip.Runway,
			Squawk:            &strip.Squawk,
			AssignedSquawk:    &strip.AssignedSquawk,
			Sid:               &strip.Sid,
			Cleared:           strip.Cleared,
			State:             &strip.GroundState,
			ClearedAltitude:   &strip.ClearedAltitude,
			RequestedAltitude: &strip.RequestedAltitude,
			Heading:           &strip.Heading,
			AircraftType:      &strip.AircraftType,
			AircraftCategory:  &strip.AircraftCategory,
			PositionLatitude:  &strip.Position.Lat,
			PositionLongitude: &strip.Position.Lon,
			PositionAltitude:  &strip.Position.Altitude,
			Stand:             &strip.Stand,
			Capabilities:      &strip.Capabilities,
			CommunicationType: &strip.CommunicationType,
			Tobt:              &strip.Eobt,
			Bay:               bay,
			Eobt:              &strip.Eobt,
		}
		reg := services.ParseRegistration(strip.Callsign, strip.Remarks)
		newStrip.Registration = &reg
		err = stripRepo.Create(ctx, newStrip)
		if err != nil {
			return err
		}
		slog.Debug("Inserted strip", slog.String("callsign", strip.Callsign))
	} else {
		// Strip exists, update it
		// Convert to database model for shared helper
		dbExistingStrip := database.Strip{
			Origin:      existingStrip.Origin,
			Destination: existingStrip.Destination,
			Cleared:     existingStrip.Cleared,
			Bay:         existingStrip.Bay,
			Stand:       existingStrip.Stand,
		}
		bay = shared.GetDepartureBay(strip, &dbExistingStrip)

		// Do not overwrite with an empty stand
		stand := existingStrip.Stand
		if strip.Stand != "" {
			stand = &strip.Stand
		}

		updateStrip := &internalModels.Strip{
			Callsign:          strip.Callsign,
			Session:           session,
			Origin:            strip.Origin,
			Destination:       strip.Destination,
			Alternative:       &strip.Alternate,
			Route:             &strip.Route,
			Remarks:           &strip.Remarks,
			AssignedSquawk:    &strip.AssignedSquawk,
			Squawk:            &strip.Squawk,
			Sid:               &strip.Sid,
			ClearedAltitude:   &strip.ClearedAltitude,
			Heading:           &strip.Heading,
			AircraftType:      &strip.AircraftType,
			Runway:            &strip.Runway,
			RequestedAltitude: &strip.RequestedAltitude,
			Capabilities:      &strip.Capabilities,
			CommunicationType: &strip.CommunicationType,
			AircraftCategory:  &strip.AircraftCategory,
			Stand:             stand,
			Cleared:           strip.Cleared,
			State:             &strip.GroundState,
			PositionLatitude:  &strip.Position.Lat,
			PositionLongitude: &strip.Position.Lon,
			PositionAltitude:  &strip.Position.Altitude,
			Bay:               bay,
			Tobt:              existingStrip.Tobt,
			Eobt:              existingStrip.Eobt,
			Registration:      existingStrip.Registration,
		}
		_, err = stripRepo.Update(ctx, updateStrip)
		if err != nil {
			return err
		}
		slog.Debug("Updated strip", slog.String("callsign", strip.Callsign))

		// If registration is NULL (backfill) or remarks now contain a REG/ token, update registration in DB.
		if existingStrip.Registration == nil || remarksContainsReg(strip.Remarks) {
			newReg := services.ParseRegistration(strip.Callsign, strip.Remarks)
			if err := stripRepo.UpdateRegistration(ctx, session, strip.Callsign, newReg); err != nil {
				slog.Error("Failed to update registration from remarks", slog.Any("error", err))
			}
		}
	}

	err = server.UpdateRouteForStrip(strip.Callsign, session, false)
	if err != nil {
		slog.Error("Error updating route for strip", slog.String("callsign", strip.Callsign), slog.Any("error", err))
	}

	err = hub.stripService.MoveToBay(ctx, session, strip.Callsign, bay, false)
	if err != nil {
		slog.Error("Error moving bay for strip", slog.String("callsign", strip.Callsign), slog.Any("error", err))
	}

	server.GetFrontendHub().SendStripUpdate(session, strip.Callsign)

	return nil
}

var remarksRegRe = regexp.MustCompile(`\bREG/([A-Z0-9-]+)`)

func remarksContainsReg(remarks string) bool {
	return remarksRegRe.MatchString(strings.ToUpper(remarks))
}

func handleStripUpdateEvent(ctx context.Context, client *Client, message Message) error {
	var event euroscope.StripUpdateEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}

	err = client.hub.handleStripUpdateHelper(ctx, event.Strip, client.session)
	return err
}

func handleRunways(ctx context.Context, client *Client, message Message) error {
	var event euroscope.RunwayEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}

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

		err = sessionRepo.UpdateActiveRunways(ctx, client.session, activeRunways)
		if err != nil {
			return err
		}

		s.GetFrontendHub().SendRunwayConfiguration(client.session, departure, arrival)

		err = s.UpdateSectors(client.session)
		if err != nil {
			slog.Error("UpdateSectors failed after runway change", slog.Int("session", int(client.session)), slog.Any("error", err))
			return err
		}
		slog.Debug("UpdateSectors completed", slog.Int("session", int(client.session)))

		err = s.UpdateRoutesForSession(client.session, true)
		if err != nil {
			slog.Error("UpdateRoutesForSession failed after runway change", slog.Int("session", int(client.session)), slog.Any("error", err))
			return err
		}
		slog.Debug("UpdateRoutesForSession completed", slog.Int("session", int(client.session)))
		return nil
	}

	return nil
}
