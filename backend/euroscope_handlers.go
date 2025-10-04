package main

import (
	"FlightStrips/database"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Server) euroscopeeventhandlerLogin(msg []byte, user *ClientUser) (event EuroscopeLoginEvent, sessionId int32, err error) {
	err = json.Unmarshal(msg, &event)
	if err != nil {
		return
	}

	if event.Type != EuroscopeLogin {
		err = errors.New("invalid initial event type, expected login")
		return
	}

	sessionName := event.Connection
	if sessionName == "PLAYBACK" {
		sessionName = sessionName + "_" + strconv.Itoa(rand.Int())
	}

	session, err := s.GetOrCreateSession(event.Airport, sessionName)

	if err != nil {
		return
	}

	// Since the login is sent on first logon and when a position is changed we need to check if the controller is
	// already in the database. It may also already be in the database if the master have synced it before a new
	// controller connects to FlightStrips

	db := database.New(s.DBPool)
	params := database.GetControllerParams{Callsign: event.Callsign, Session: session.Id}
	controller, err := db.GetController(context.TODO(), params)

	if errors.Is(err, pgx.ErrNoRows) {
		params := database.InsertControllerParams{
			Callsign:          event.Callsign,
			Session:           session.Id,
			Position:          event.Position,
			Cid:               pgtype.Text{Valid: true, String: user.cid},
			LastSeenEuroscope: pgtype.Timestamp{Valid: true, Time: time.Now().UTC()},
		}

		err = db.InsertController(context.Background(), params)

		if err != nil {
			return event, session.Id, err
		}

		s.FrontendHub.CidOnline(session.Id, user.cid)

		return event, session.Id, nil
	} else if err != nil {
		return event, session.Id, err
	} else {
		// Set CID
		params := database.SetControllerCidParams{Session: session.Id, Cid: pgtype.Text{Valid: true, String: user.cid}, Callsign: event.Callsign}
		db.SetControllerCid(context.Background(), params)
	}

	if controller.Position != event.Position {
		params := database.SetControllerPositionParams{Session: session.Id, Callsign: event.Callsign, Position: event.Position}
		_, err = db.SetControllerPosition(context.TODO(), params)

		if err != nil {
			return event, session.Id, err
		}
	}

	return event, session.Id, err
}

func euroscopeeventhandlerControllerOnline(client *EuroscopeClient, msg []byte) error {
	var event EuroscopeControllerOnlineEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}
	s := client.server
	session := client.session

	db := database.New(s.DBPool)
	getParams := database.GetControllerParams{Callsign: event.Callsign, Session: session}
	controller, err := db.GetController(context.TODO(), getParams)

	if errors.Is(err, pgx.ErrNoRows) {
		params := database.InsertControllerParams{
			Callsign: event.Callsign,
			Position: event.Position,
			Session:  session,
		}

		err = db.InsertController(context.Background(), params)
		if err == nil {
			s.FrontendHub.SendControllerOnline(session, event.Callsign, event.Position)
		}
		return err
	}

	if err != nil {
		return err
	}

	if controller.Position != event.Position {
		setParams := database.SetControllerPositionParams{Session: session, Callsign: event.Callsign, Position: event.Position}
		_, err = db.SetControllerPosition(context.TODO(), setParams)
		if err == nil {
			s.FrontendHub.SendControllerOnline(session, event.Callsign, event.Position)
		}
		return err
	}

	return nil
}

func euroscopeeventhandlerControllerOffline(client *EuroscopeClient, msg []byte) error {
	var event EuroscopeControllerOfflineEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}
	s := client.server
	session := client.session
	airport := client.airport

	db := database.New(s.DBPool)
	getParams := database.GetControllerParams{Session: session, Callsign: event.Callsign}
	controller, err := db.GetController(context.TODO(), getParams)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	if errors.Is(err, pgx.ErrNoRows) {
		log.Printf("Controller %s which was going offline does not exist in the database\n", event.Callsign)
		s.FrontendHub.SendControllerOffline(session, event.Callsign, "")
		return nil
	}

	params := database.RemoveControllerParams{Session: session, Callsign: event.Callsign}
	count, err := db.RemoveController(context.TODO(), params)

	s.FrontendHub.SendControllerOffline(session, event.Callsign, controller.Position)
	if err != nil {
		return err
	}

	if count != 1 {
		log.Printf("Controller %s at airport %s which was online did not exist in the database\n",
			event.Callsign, airport)
	}

	return nil
}

func euroscopeeventhandlerAssignedSquawk(client *EuroscopeClient, msg []byte) error {
	var event EuroscopeAssignedSquawkEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}
	s := client.server
	session := client.session

	db := database.New(s.DBPool)

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
		s.FrontendHub.SendAssignedSquawkEvent(session, event.Callsign, event.Squawk)
	}

	return err
}

func euroscopeeventhandlerSquawk(client *EuroscopeClient, msg []byte) error {
	var event EuroscopeSquawkEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}
	s := client.server
	session := client.session

	db := database.New(s.DBPool)

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
		s.FrontendHub.SendSquawkEvent(session, event.Callsign, event.Squawk)
	}

	return err
}

func euroscopeeventhandlerRequestedAltitude(client *EuroscopeClient, msg []byte) error {
	var event EuroscopeRequestedAltitudeEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}
	s := client.server
	session := client.session

	db := database.New(s.DBPool)

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
		s.FrontendHub.SendRequestedAltitudeEvent(session, event.Callsign, event.Altitude)
	}
	return err
}

func euroscopeeventhandlerClearedAltitude(client *EuroscopeClient, msg []byte) error {
	var event EuroscopeClearedAltitudeEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}
	s := client.server
	session := client.session

	db := database.New(s.DBPool)
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
		s.FrontendHub.SendClearedAltitudeEvent(session, event.Callsign, event.Altitude)
	}
	return err
}

func euroscopeeventhandlerCommunicationType(client *EuroscopeClient, msg []byte) error {
	var event EuroscopeCommunicationTypeEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}
	s := client.server
	session := client.session

	db := database.New(s.DBPool)

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
	s.FrontendHub.SendCommunicationTypeEvent(session, event.Callsign, event.CommunicationType)
	return nil
}

func euroscopeeventhandlerGroundState(client *EuroscopeClient, msg []byte) error {
	var event EuroscopeGroundStateEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}
	s := client.server
	session := client.session

	db := database.New(s.DBPool)
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

	bay := GetDepartureBayFromGroundState(event.GroundState, existingStrip)

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
		s.FrontendHub.SendBayEvent(session, event.Callsign, bay)
	}

	return nil
}

func euroscopeeventhandlerClearedFlag(client *EuroscopeClient, msg []byte) error {
	var event EuroscopeClearedFlagEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}
	s := client.server
	session := client.session

	db := database.New(s.DBPool)
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
	if bay == BAY_NOT_CLEARED || bay == BAY_UNKNOWN {
		bay = BAY_CLEARED
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
		s.FrontendHub.SendBayEvent(session, event.Callsign, bay)
	}

	return err
}

func euroscopeeventhandlerPositionUpdate(client *EuroscopeClient, msg []byte) error {
	var event EuroscopeAircraftPositionUpdateEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}
	s := client.server
	session := client.session

	db := database.New(s.DBPool)
	existingStrip, err := db.GetStrip(context.TODO(), database.GetStripParams{Callsign: event.Callsign, Session: session})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Printf("Strip %v which is being updated does not exist in the database", event.Callsign)
			return nil
		}
		return err
	}

	bay := GetDepartureBayFromPosition(event.Lat, event.Lon, event.Altitude, existingStrip)

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
		s.FrontendHub.SendBayEvent(session, event.Callsign, bay)
	}

	return nil
}

func euroscopeeventhandlerSetHeading(client *EuroscopeClient, msg []byte) error {
	var event EuroscopeHeadingEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}
	s := client.server
	session := client.session

	db := database.New(s.DBPool)
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
	s.FrontendHub.SendSetHeadingEvent(session, event.Callsign, event.Heading)
	return nil
}

func euroscopeeventhandlerAircraftDisconnected(client *EuroscopeClient, msg []byte) error {
	var event EuroscopeAircraftDisconnectEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}
	s := client.server
	session := client.session

	db := database.New(s.DBPool)
	err = db.RemoveStripByID(context.TODO(), database.RemoveStripByIDParams{Callsign: event.Callsign, Session: session})
	s.FrontendHub.SendAircraftDisconnect(session, event.Callsign)
	return err
}

func euroscopeeventhandlerStand(client *EuroscopeClient, msg []byte) error {
	var event EuroscopeStandEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}
	s := client.server
	session := client.session

	db := database.New(s.DBPool)
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

	s.FrontendHub.SendStandEvent(session, event.Callsign, event.Stand)
	return nil
}

func euroscopeeventhandlerSync(client *EuroscopeClient, msg []byte) error {
	var event EuroscopeSyncEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}

	s := client.server
	session := client.session

	db := database.New(s.DBPool)

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

	for _, strip := range event.Strips {
		err = handleStripUpdate(s, db, strip, session)
		if err != nil {
			return err
		}
	}

	err = client.server.UpdateSectors(client.session)
	return err
}

func handleStripUpdate(server *Server, db *database.Queries, strip EuroscopeStrip, session int32) error {
	// Check if the strip exists
	existingStrip, err := db.GetStrip(context.TODO(), database.GetStripParams{Callsign: strip.Callsign, Session: session})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	if errors.Is(err, pgx.ErrNoRows) {
		// Strip doesn't exist, so insert

		bay := GetDepartureBay(strip, nil)

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
		bay := GetDepartureBay(strip, &existingStrip)

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

	server.FrontendHub.SendStripUpdate(session, strip.Callsign)

	return nil
}

func euroscopeeventhandlerStripUpdate(client *EuroscopeClient, msg []byte) error {
	var event EuroscopeStripUpdateEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}
	s := client.server

	db := database.New(s.DBPool)

	err = handleStripUpdate(s, db, event.EuroscopeStrip, client.session)
	return err
}

func euroscopeeventhandlerRunways(client *EuroscopeClient, msg []byte) error {
	var event EuroscopeRunwayEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}

	if client.server.EuroscopeHub.master == client {
		db := database.New(client.server.DBPool)

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

		activeRunways := ActiveRunways{
			DepartureRunways: departure,
			ArrivalRunways:   arrival,
		}

		data, err := json.Marshal(activeRunways)

		fmt.Printf("Setting active runways to %v for session %v\n", activeRunways, client.session)

		if err != nil {
			return err
		}

		params := database.UpdateActiveRunwaysParams{
			ID:            client.session,
			ActiveRunways: pgtype.Text{Valid: true, String: string(data)},
		}

		err = db.UpdateActiveRunways(context.Background(), params)
		if err != nil {
			return err
		}

		err = client.server.UpdateSectors(client.session)
		return err
	}

	return nil
}
