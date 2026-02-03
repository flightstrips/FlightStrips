package euroscope

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events/euroscope"
	"FlightStrips/pkg/models"
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"
)

type Message = shared.Message[euroscope.EventType]

func handleControllerOnline(client *Client, message Message) error {
	var event euroscope.ControllerOnlineEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	controllerRepo := s.GetControllerRepository()
	controller, err := controllerRepo.GetByCallsign(context.TODO(), session, event.Callsign)

	if errors.Is(err, pgx.ErrNoRows) {
		newController := &internalModels.Controller{
			Callsign: event.Callsign,
			Position: event.Position,
			Session:  session,
		}

		err = controllerRepo.Create(context.Background(), newController)
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

	_, err = controllerRepo.SetPosition(context.TODO(), session, event.Callsign, event.Position)
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

	if shouldUpdate {
		err = s.UpdateSectors(client.session)
		if err != nil {
			return err
		}
		return s.UpdateLayouts(session)
	}

	return nil
}

func handleControllerOffline(client *Client, message Message) error {
	var event euroscope.ControllerOfflineEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	controllerRepo := s.GetControllerRepository()
	controller, err := controllerRepo.GetByCallsign(context.TODO(), session, event.Callsign)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	if errors.Is(err, pgx.ErrNoRows) {
		log.Printf("Controller %s which was going offline does not exist in the database\n", event.Callsign)
		s.GetFrontendHub().SendControllerOffline(session, event.Callsign, "", "")
		return nil
	}

	err = controllerRepo.Delete(context.TODO(), session, event.Callsign)

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

func handleAssignedSquawk(client *Client, message Message) error {
	var event euroscope.AssignedSquawkEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	stripRepo := s.GetStripRepository()
	count, err := stripRepo.UpdateAssignedSquawk(context.TODO(), session, event.Callsign, &event.Squawk, nil)

	if err != nil {
		return err
	}

	if count != 1 {
		log.Printf("Strip %v which is being updated does not exist in the database", event.Callsign)
	} else {
		s.GetFrontendHub().SendAssignedSquawkEvent(session, event.Callsign, event.Squawk)
	}

	return err
}

func handleSquawk(client *Client, message Message) error {
	var event euroscope.SquawkEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	stripRepo := s.GetStripRepository()
	count, err := stripRepo.UpdateSquawk(context.TODO(), session, event.Callsign, &event.Squawk, nil)

	if err != nil {
		return err
	}

	if count != 1 {
		log.Printf("Strip %v which is being updated does not exist in the database", event.Callsign)
	} else {
		s.GetFrontendHub().SendSquawkEvent(session, event.Callsign, event.Squawk)
	}

	return err
}

func handleRequestedAltitude(client *Client, message Message) error {
	var event euroscope.RequestedAltitudeEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	stripRepo := s.GetStripRepository()

	intAltitude := int32(event.Altitude)
	count, err := stripRepo.UpdateRequestedAltitude(context.TODO(), session, event.Callsign, &intAltitude, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		log.Printf("Strip %v which is being updated does not exist in the database", event.Callsign)
	} else {
		s.GetFrontendHub().SendRequestedAltitudeEvent(session, event.Callsign, event.Altitude)
	}
	return err
}

func handleClearedAltitude(client *Client, message Message) error {
	var event euroscope.ClearedAltitudeEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	stripRepo := s.GetStripRepository()
	intAltitude := int32(event.Altitude)
	count, err := stripRepo.UpdateClearedAltitude(context.TODO(), session, event.Callsign, &intAltitude, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		log.Printf("Strip %v which is being updated does not exist in the database", event.Callsign)
	} else {
		s.GetFrontendHub().SendClearedAltitudeEvent(session, event.Callsign, event.Altitude)
	}
	return err
}

func handleCommunicationType(client *Client, message Message) error {
	var event euroscope.CommunicationTypeEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	stripRepo := s.GetStripRepository()

	count, err := stripRepo.UpdateCommunicationType(context.TODO(), session, event.Callsign, &event.CommunicationType, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		log.Printf("Strip %v which is being updated does not exist in the database", event.Callsign)
		return nil
	}
	s.GetFrontendHub().SendCommunicationTypeEvent(session, event.Callsign, event.CommunicationType)
	return nil
}

func handleGroundState(client *Client, message Message) error {
	var event euroscope.GroundStateEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	stripRepo := s.GetStripRepository()
	existingStrip, err := stripRepo.GetByCallsign(context.TODO(), session, event.Callsign)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Printf("Strip %v which is being updated does not exist in the database", event.Callsign)
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

	_, err = stripRepo.UpdateGroundState(context.TODO(), session, event.Callsign, &event.GroundState, bay, nil)

	if err != nil {
		return err
	}

	if existingStrip.Bay != bay {
		return client.hub.stripService.MoveToBay(context.Background(), client.session, event.Callsign, bay, true)
	}

	return nil
}

func handleClearedFlag(client *Client, message Message) error {
	var event euroscope.ClearedFlagEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	stripRepo := s.GetStripRepository()
	existingStrip, err := stripRepo.GetByCallsign(context.TODO(), session, event.Callsign)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Printf("Strip %v which is being updated does not exist in the database", event.Callsign)
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

	_, err = stripRepo.UpdateClearedFlag(context.TODO(), session, event.Callsign, event.Cleared, bay, nil)
	if err != nil {
		return err
	}

	if existingStrip.Bay != bay {
		return client.hub.stripService.MoveToBay(context.Background(), client.session, event.Callsign, bay, true)
	}

	return err
}

func handlePositionUpdate(client *Client, message Message) error {
	var event euroscope.AircraftPositionUpdateEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	stripRepo := s.GetStripRepository()
	existingStrip, err := stripRepo.GetByCallsign(context.TODO(), session, event.Callsign)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Printf("Strip %v which is being updated does not exist in the database", event.Callsign)
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

	_, err = stripRepo.UpdateAircraftPosition(context.TODO(), session, event.Callsign, &event.Lat, &event.Lon, &intAltitude, bay, nil)
	_, err = stripRepo.UpdateAircraftPosition(context.TODO(), session, event.Callsign, &event.Lat, &event.Lon, &intAltitude, bay, nil)

	if err != nil {
		return err
	}

	if existingStrip.Bay != bay {
		return client.hub.stripService.MoveToBay(context.Background(), client.session, event.Callsign, bay, true)
	}

	return nil
}

func handleSetHeading(client *Client, message Message) error {
	var event euroscope.HeadingEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	stripRepo := s.GetStripRepository()
	count, err := stripRepo.UpdateHeading(context.TODO(), session, event.Callsign, &event.Heading, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		log.Printf("Strip %v which is being updated does not exist in the database", event.Callsign)
		return nil
	}
	s.GetFrontendHub().SendSetHeadingEvent(session, event.Callsign, event.Heading)
	return nil
}

func handleAircraftDisconnected(client *Client, message Message) error {
	var event euroscope.AircraftDisconnectEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	stripRepo := s.GetStripRepository()
	err = stripRepo.Delete(context.TODO(), session, event.Callsign)
	s.GetFrontendHub().SendAircraftDisconnect(session, event.Callsign)
	return err
}

func handleStand(client *Client, message Message) error {
	var event euroscope.StandEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}
	s := client.hub.server
	session := client.session

	stripRepo := s.GetStripRepository()
	count, err := stripRepo.UpdateStand(context.TODO(), session, event.Callsign, &event.Stand, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		log.Printf("Strip %v which is being updated does not exist in the database", event.Callsign)
		return nil
	}

	s.GetFrontendHub().SendStandEvent(session, event.Callsign, event.Stand)
	return nil
}

func handleSync(client *Client, message Message) error {
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
		_, err := controllerRepo.GetByCallsign(context.TODO(), session, controller.Callsign)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		if errors.Is(err, pgx.ErrNoRows) {
			// Controller doesn't exist, so insert
			newController := &internalModels.Controller{
				Callsign: controller.Callsign,
				Session:  session,
				Position: controller.Position,
				Cid:      nil,
			}
			err = controllerRepo.Create(context.TODO(), newController)
			if err != nil {
				return err
			}
			log.Printf("Inserted controller: %s", controller.Callsign)
		} else {
			// Controller exists, update it
			_, err = controllerRepo.SetPosition(context.TODO(), session, controller.Callsign, controller.Position)
			if err != nil {
				return err
			}
			log.Printf("Updated controller: %s", controller.Callsign)
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
		err = client.hub.handleStripUpdateHelper(strip, session)
		if err != nil {
			return err
		}
	}

	return err
}

func (hub *Hub) handleStripUpdateHelper(strip euroscope.Strip, session int32) error {
	server := hub.server
	stripRepo := server.GetStripRepository()
	
	existingStrip, err := stripRepo.GetByCallsign(context.TODO(), session, strip.Callsign)
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
		err = stripRepo.Create(context.TODO(), newStrip)
		if err != nil {
			return err
		}
		log.Printf("Inserted strip: %s", strip.Callsign)
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
		}
		_, err = stripRepo.Update(context.TODO(), updateStrip)
		if err != nil {
			return err
		}
		log.Printf("Updated strip: %s", strip.Callsign)
	}

	err = server.UpdateRouteForStrip(strip.Callsign, session, false)
	if err != nil {
		fmt.Printf("Error updating route for strip %s: %v\n", strip.Callsign, err)
	}

	err = hub.stripService.MoveToBay(context.Background(), session, strip.Callsign, bay, false)
	if err != nil {
		fmt.Printf("Error moving bay to strip %s: %v\n", strip.Callsign, err)
	}

	server.GetFrontendHub().SendStripUpdate(session, strip.Callsign)

	return nil
}

func handleStripUpdateEvent(client *Client, message Message) error {
	var event euroscope.StripUpdateEvent
	err := message.JsonUnmarshal(&event)
	if err != nil {
		return err
	}

	err = client.hub.handleStripUpdateHelper(event.Strip, client.session)
	return err
}

func handleRunways(client *Client, message Message) error {
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

		fmt.Printf("Setting active runways to %v for session %v\n", activeRunways, client.session)

		err = sessionRepo.UpdateActiveRunways(context.Background(), client.session, activeRunways)
		if err != nil {
			return err
		}

		err = s.UpdateSectors(client.session)
		if err != nil {
			return err
		}

		err = s.UpdateRoutesForSession(client.session, true)
		return err
	}

	return nil
}
