package euroscope

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events/euroscope"
	"FlightStrips/pkg/models"
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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

	db := database.New(s.GetDatabasePool())
	getParams := database.GetControllerParams{Callsign: event.Callsign, Session: session}
	controller, err := db.GetController(context.TODO(), getParams)

	if errors.Is(err, pgx.ErrNoRows) {
		params := database.InsertControllerParams{
			Callsign: event.Callsign,
			Position: event.Position,
			Session:  session,
		}

		err = db.InsertController(context.Background(), params)
		if err != nil {
			return err
		}
		err = s.UpdateSectors(client.session)
		if err != nil {
			return err
		}

		return nil
	}

	if controller.Position == event.Position || err != nil {
		return err
	}

	setParams := database.SetControllerPositionParams{Session: session, Callsign: event.Callsign, Position: event.Position}
	_, err = db.SetControllerPosition(context.TODO(), setParams)
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
	airport := client.airport

	db := database.New(s.GetDatabasePool())
	getParams := database.GetControllerParams{Session: session, Callsign: event.Callsign}
	controller, err := db.GetController(context.TODO(), getParams)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	if errors.Is(err, pgx.ErrNoRows) {
		log.Printf("Controller %s which was going offline does not exist in the database\n", event.Callsign)
		s.GetFrontendHub().SendControllerOffline(session, event.Callsign, "", "")
		return nil
	}

	params := database.RemoveControllerParams{Session: session, Callsign: event.Callsign}
	count, err := db.RemoveController(context.TODO(), params)

	s.GetFrontendHub().SendControllerOffline(session, event.Callsign, controller.Position, "")
	if err != nil {
		return err
	}

	if count != 1 {
		log.Printf("Controller %s at airport %s which was online did not exist in the database\n",
			event.Callsign, airport)
		return nil
	}

	if _, err := config.GetPositionBasedOnFrequency(controller.Position); err == nil {
		return s.UpdateSectors(client.session)
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

	db := database.New(s.GetDatabasePool())

	insertData := database.UpdateStripAssignedSquawkByIDParams{
		AssignedSquawk: pgtype.Text{Valid: true, String: event.Squawk},
		Callsign:       event.Callsign,
		Session:        session,
	}

	count, err := db.UpdateStripAssignedSquawkByID(context.TODO(), insertData)

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

	db := database.New(s.GetDatabasePool())

	insertData := database.UpdateStripSquawkByIDParams{
		Squawk:   pgtype.Text{Valid: true, String: event.Squawk},
		Callsign: event.Callsign,
		Session:  session,
	}

	count, err := db.UpdateStripSquawkByID(context.TODO(), insertData)

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

	db := database.New(s.GetDatabasePool())

	insertData := database.UpdateStripRequestedAltitudeByIDParams{
		RequestedAltitude: pgtype.Int4{Valid: true, Int32: int32(event.Altitude)},
		Callsign:          event.Callsign,
		Session:           session,
	}

	count, err := db.UpdateStripRequestedAltitudeByID(context.TODO(), insertData)
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

	db := database.New(s.GetDatabasePool())
	insertData := database.UpdateStripClearedAltitudeByIDParams{
		ClearedAltitude: pgtype.Int4{Valid: true, Int32: int32(event.Altitude)},
		Callsign:        event.Callsign,
		Session:         session,
	}

	count, err := db.UpdateStripClearedAltitudeByID(context.TODO(), insertData)
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

	db := database.New(s.GetDatabasePool())

	insertData := database.UpdateStripCommunicationTypeByIDParams{
		CommunicationType: pgtype.Text{Valid: true, String: event.CommunicationType},
		Callsign:          event.Callsign,
		Session:           session,
	}

	count, err := db.UpdateStripCommunicationTypeByID(context.TODO(), insertData)
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

	db := database.New(s.GetDatabasePool())
	existingStrip, err := db.GetStrip(context.TODO(), database.GetStripParams{Callsign: event.Callsign, Session: session})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Printf("Strip %v which is being updated does not exist in the database", event.Callsign)
			return nil
		}
		return err
	}

	if existingStrip.State.String == event.GroundState {
		return nil
	}

	bay := shared.GetDepartureBayFromGroundState(event.GroundState, existingStrip)

	insertData := database.UpdateStripGroundStateByIDParams{
		State:    pgtype.Text{Valid: true, String: event.GroundState},
		Bay:      pgtype.Text{Valid: true, String: bay},
		Callsign: event.Callsign,
		Session:  session,
	}

	_, err = db.UpdateStripGroundStateByID(context.TODO(), insertData)

	if err != nil {
		return err
	}

	if existingStrip.Bay.String != bay {
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

	db := database.New(s.GetDatabasePool())
	existingStrip, err := db.GetStrip(context.TODO(), database.GetStripParams{Callsign: event.Callsign, Session: session})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Printf("Strip %v which is being updated does not exist in the database", event.Callsign)
			return nil
		}
		return err
	}

	if existingStrip.Cleared.Valid && existingStrip.Cleared.Bool == event.Cleared {
		return nil
	}

	bay := existingStrip.Bay.String
	if bay == shared.BAY_NOT_CLEARED || bay == shared.BAY_UNKNOWN {
		bay = shared.BAY_CLEARED
	}

	insertData := database.UpdateStripClearedFlagByIDParams{
		Cleared:  pgtype.Bool{Valid: true, Bool: event.Cleared},
		Bay:      pgtype.Text{Valid: true, String: bay},
		Callsign: event.Callsign,
		Session:  session,
	}
	_, err = db.UpdateStripClearedFlagByID(context.TODO(), insertData)
	if err != nil {
		return err
	}

	if existingStrip.Bay.String != bay {
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

	db := database.New(s.GetDatabasePool())
	existingStrip, err := db.GetStrip(context.TODO(), database.GetStripParams{Callsign: event.Callsign, Session: session})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Printf("Strip %v which is being updated does not exist in the database", event.Callsign)
			return nil
		}
		return err
	}

	bay := shared.GetDepartureBayFromPosition(event.Lat, event.Lon, event.Altitude, existingStrip)

	insertData := database.UpdateStripAircraftPositionByIDParams{
		PositionLatitude:  pgtype.Float8{Valid: true, Float64: event.Lat},
		PositionLongitude: pgtype.Float8{Valid: true, Float64: event.Lon},
		PositionAltitude:  pgtype.Int4{Valid: true, Int32: int32(event.Altitude)},
		Bay:               pgtype.Text{Valid: true, String: bay},
		Callsign:          event.Callsign,
		Session:           session,
	}
	_, err = db.UpdateStripAircraftPositionByID(context.TODO(), insertData)

	if err != nil {
		return err
	}

	if existingStrip.Bay.String != bay {
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

	db := database.New(s.GetDatabasePool())
	insertData := database.UpdateStripHeadingByIDParams{
		Heading:  pgtype.Int4{Valid: true, Int32: int32(event.Heading)},
		Callsign: event.Callsign,
		Session:  session,
	}

	count, err := db.UpdateStripHeadingByID(context.TODO(), insertData)
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

	db := database.New(s.GetDatabasePool())
	err = db.RemoveStripByID(context.TODO(), database.RemoveStripByIDParams{Callsign: event.Callsign, Session: session})
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

	db := database.New(s.GetDatabasePool())
	insertData := database.UpdateStripStandByIDParams{
		Stand:    pgtype.Text{Valid: true, String: event.Stand},
		Callsign: event.Callsign,
		Session:  session,
	}

	count, err := db.UpdateStripStandByID(context.TODO(), insertData)
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

	db := database.New(s.GetDatabasePool())

	for _, controller := range event.Controllers {
		// Check if the controller exists
		_, err := db.GetController(context.TODO(), database.GetControllerParams{Callsign: controller.Callsign, Session: session})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		if errors.Is(err, pgx.ErrNoRows) {
			// Controller doesn't exist, so insert
			controllerParams := database.InsertControllerParams{
				Callsign:          controller.Callsign,
				Session:           session,
				Position:          controller.Position,
				Cid:               pgtype.Text{Valid: false},
				LastSeenEuroscope: pgtype.Timestamp{Valid: false},
				LastSeenFrontend:  pgtype.Timestamp{Valid: false},
			}
			err = db.InsertController(context.TODO(), controllerParams)
			if err != nil {
				return err
			}
			log.Printf("Inserted controller: %s", controller.Callsign)
		} else {
			// Controller exists, update it
			updateControllerParams := database.SetControllerPositionParams{
				Callsign: controller.Callsign,
				Session:  session,
				Position: controller.Position,
			}
			_, err = db.SetControllerPosition(context.TODO(), updateControllerParams)
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

	for _, strip := range event.Strips {
		err = client.hub.handleStripUpdateHelper(db, strip, session)
		if err != nil {
			return err
		}
	}

	return err
}

func (hub *Hub) handleStripUpdateHelper(db *database.Queries, strip euroscope.Strip, session int32) error {
	// Check if the strip exists
	server := hub.server
	existingStrip, err := db.GetStrip(context.TODO(), database.GetStripParams{Callsign: strip.Callsign, Session: session})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	var bay string

	if errors.Is(err, pgx.ErrNoRows) {
		// Strip doesn't exist, so insert

		bay = shared.GetDepartureBay(strip, nil)

		stripParams := database.InsertStripParams{ //keep this for insert
			Callsign:          strip.Callsign,
			Session:           session,
			Origin:            strip.Origin,
			Destination:       strip.Destination,
			Alternative:       pgtype.Text{Valid: true, String: strip.Alternate},
			Route:             pgtype.Text{Valid: true, String: strip.Route},
			Remarks:           pgtype.Text{Valid: true, String: strip.Remarks},
			Runway:            pgtype.Text{Valid: true, String: strip.Runway},
			Squawk:            pgtype.Text{Valid: true, String: strip.Squawk},
			AssignedSquawk:    pgtype.Text{Valid: true, String: strip.AssignedSquawk},
			Sid:               pgtype.Text{Valid: true, String: strip.Sid},
			Cleared:           pgtype.Bool{Valid: true, Bool: strip.Cleared},
			State:             pgtype.Text{Valid: true, String: strip.GroundState},
			ClearedAltitude:   pgtype.Int4{Valid: true, Int32: int32(strip.ClearedAltitude)},
			RequestedAltitude: pgtype.Int4{Valid: true, Int32: int32(strip.RequestedAltitude)},
			Heading:           pgtype.Int4{Valid: true, Int32: int32(strip.Heading)},
			AircraftType:      pgtype.Text{Valid: true, String: strip.AircraftType},
			AircraftCategory:  pgtype.Text{Valid: true, String: strip.AircraftCategory},
			PositionLatitude:  pgtype.Float8{Valid: true, Float64: strip.Position.Lat},
			PositionLongitude: pgtype.Float8{Valid: true, Float64: strip.Position.Lon},
			PositionAltitude:  pgtype.Int4{Valid: true, Int32: int32(strip.Position.Altitude)},
			Stand:             pgtype.Text{Valid: true, String: strip.Stand},
			Capabilities:      pgtype.Text{Valid: true, String: strip.Capabilities},
			CommunicationType: pgtype.Text{Valid: true, String: strip.CommunicationType},
			Tobt:              pgtype.Text{Valid: true, String: strip.Eobt},
			Bay:               pgtype.Text{Valid: true, String: bay},
			Eobt:              pgtype.Text{Valid: true, String: strip.Eobt},
		}
		err = db.InsertStrip(context.TODO(), stripParams)
		if err != nil {
			return err
		}
		log.Printf("Inserted strip: %s", strip.Callsign)
	} else {
		// Strip exists, update it
		// TODO we need to ensure the master is synced first otherwise this will overwrite the strip with potential wrong values
		bay = shared.GetDepartureBay(strip, &existingStrip)

		// Do not overwrite with an empty stand
		stand := existingStrip.Stand.String
		if strip.Stand != "" {
			stand = strip.Stand
		}

		updateStripParams := database.UpdateStripParams{ // create this
			Callsign:          strip.Callsign,
			Session:           session,
			Origin:            strip.Origin,
			Destination:       strip.Destination,
			Alternative:       pgtype.Text{Valid: true, String: strip.Alternate}, // Assuming these fields exist
			Route:             pgtype.Text{Valid: true, String: strip.Route},
			Remarks:           pgtype.Text{Valid: true, String: strip.Remarks},
			AssignedSquawk:    pgtype.Text{Valid: true, String: strip.AssignedSquawk},
			Squawk:            pgtype.Text{Valid: true, String: strip.Squawk},
			Sid:               pgtype.Text{Valid: true, String: strip.Sid},
			ClearedAltitude:   pgtype.Int4{Valid: true, Int32: int32(strip.ClearedAltitude)},
			Heading:           pgtype.Int4{Valid: true, Int32: int32(strip.Heading)},
			AircraftType:      pgtype.Text{Valid: true, String: strip.AircraftType},
			Runway:            pgtype.Text{Valid: true, String: strip.Runway},
			RequestedAltitude: pgtype.Int4{Valid: true, Int32: int32(strip.RequestedAltitude)},
			Capabilities:      pgtype.Text{Valid: true, String: strip.Capabilities},
			CommunicationType: pgtype.Text{Valid: true, String: strip.CommunicationType},
			AircraftCategory:  pgtype.Text{Valid: true, String: strip.AircraftCategory},
			Stand:             pgtype.Text{Valid: true, String: stand},
			Cleared:           pgtype.Bool{Valid: true, Bool: strip.Cleared},
			State:             pgtype.Text{Valid: true, String: strip.GroundState},
			PositionLatitude:  pgtype.Float8{Valid: true, Float64: strip.Position.Lat},
			PositionLongitude: pgtype.Float8{Valid: true, Float64: strip.Position.Lon},
			PositionAltitude:  pgtype.Int4{Valid: true, Int32: int32(strip.Position.Altitude)},
			Bay:               pgtype.Text{Valid: true, String: bay},
			Tobt:              pgtype.Text{Valid: true, String: strip.Eobt},
			Eobt:              pgtype.Text{Valid: true, String: strip.Eobt},
		}
		_, err = db.UpdateStrip(context.TODO(), updateStripParams)
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
	s := client.hub.server

	db := database.New(s.GetDatabasePool())

	err = client.hub.handleStripUpdateHelper(db, event.Strip, client.session)
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
		db := database.New(s.GetDatabasePool())

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

		params := database.UpdateActiveRunwaysParams{
			ID:            client.session,
			ActiveRunways: activeRunways,
		}

		err = db.UpdateActiveRunways(context.Background(), params)
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
