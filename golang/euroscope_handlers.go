package main

import (
	"FlightStrips/data"
	"context"
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"strconv"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Server) euroscopeeventhandlerAuthentication(msg []byte) (user *EuroscopeUser, err error) {
	var event EuroscopeAuthenticationEvent
	err = json.Unmarshal(msg, &event)
	if err != nil {
		return user, err
	}

	return s.euroscopeeventhandlerAuthenticationTokenValidation(event.Token)
}

func (s *Server) euroscopeeventhandlerConnectionClosed(client *EuroscopeClient) error {
	// Controller may still be online and may have just logged out or unloaded the plugin

	// Do not set last seen as we want to keep the data for some time before removing the session if this is the last
	// controller disconnecting

	return nil
}

func (s *Server) euroscopeeventhandlerAuthenticationTokenValidation(eventToken string) (user *EuroscopeUser, err error) {
	// TODO: Sort out Logging
	JWTToken := eventToken

	k, err := keyfunc.NewDefault([]string{s.AuthServerURL})
	if err != nil {
		log.Fatalf("Failed to create a keyfunc.Keyfunc from the server's URL.\nError: %s", err)
	}
	options := jwt.WithValidMethods([]string{s.AuthSigningAlgo})
	token, err := jwt.Parse(JWTToken, k.Keyfunc, options)
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("invalid jwt")
	}

	claims := token.Claims.(jwt.MapClaims)

	cid, ok := claims["vatsim/cid"].(string)

	if !ok {
		return nil, errors.New("missing CID claim")
	}

	rating, ok := claims["vatsim/rating"].(float64)

	if !ok {
		return nil, errors.New("missing Rating claim")
	}

	esUser := &EuroscopeUser{cid: cid, rating: int(rating), authToken: token}
	return esUser, nil
}

func (s *Server) euroscopeeventhandlerLogin(msg []byte) (event EuroscopeLoginEvent, sessionId int32, err error) {
	err = json.Unmarshal(msg, &event)
	if err != nil {
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

	db := data.New(s.DBPool)
	params := data.GetControllerParams{Callsign: event.Callsign, Session: session.Id}
	controller, err := db.GetController(context.TODO(), params)

	if errors.Is(err, pgx.ErrNoRows) {
		params := data.InsertControllerParams{
			Callsign: event.Callsign,
			Session:  session.Id,
			Position: event.Position,
			Airport:  event.Airport,
			//Cid: , // TODO,
			LastSeenEuroscope: pgtype.Timestamp{Valid: true, Time: time.Now().UTC()},
		}

		err = db.InsertController(context.Background(), params)

		return event, session.Id, err
	}

	if err != nil {
		return event, session.Id, err
	}

	if controller.Position != event.Position {
		params := data.SetControllerPositionParams{Session: session.Id, Callsign: event.Callsign, Position: event.Position}
		_, err = db.SetControllerPosition(context.TODO(), params)

		if err != nil {
			return event, session.Id, err
		}
	}

	setParams := data.SetControllerEuroscopeSeenParams{Session: session.Id, Callsign: event.Callsign, LastSeenEuroscope: pgtype.Timestamp{Time: time.Now().UTC(), Valid: true}}
	_, err = db.SetControllerEuroscopeSeen(context.TODO(), setParams)

	return event, session.Id, err
}

func (s *Server) euroscopeeventhandlerControllerOnline(msg []byte, session int32, airport string) error {
	var event EuroscopeControllerOnlineEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}

	db := data.New(s.DBPool)
	getParams := data.GetControllerParams{Callsign: event.Callsign, Session: session}
	controller, err := db.GetController(context.TODO(), getParams)

	if errors.Is(err, pgx.ErrNoRows) {
		// New controller insert
		params := data.InsertControllerParams{
			Callsign: event.Callsign,
			Position: event.Position,
			Airport:  airport,
			Session:  session,
		}

		err = db.InsertController(context.Background(), params)

		return err
	}

	if err != nil {
		return err
	}

	if controller.Position == event.Position {
		return nil
	}

	setParams := data.SetControllerPositionParams{Session: session, Callsign: event.Callsign, Position: event.Position}
	_, err = db.SetControllerPosition(context.TODO(), setParams)

	return err
}

func (s *Server) euroscopeeventhandlerControllerOffline(msg []byte, session int32, airport string) error {
	var event EuroscopeControllerOfflineEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}

	db := data.New(s.DBPool)

	params := data.RemoveControllerParams{Session: session, Callsign: event.Callsign}
	count, err := db.RemoveController(context.TODO(), params)

	if err != nil {
		return err
	}

	if count != 1 {
		log.Printf("Controller %s at airport %s which was online did not exist in the database\n",
			event.Callsign, airport)
	}

	return err
}

func (s *Server) euroscopeeventhandlerAssignedSquawk(msg []byte, session int32) error {
	var event EuroscopeAssignedSquawkEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}

	db := data.New(s.DBPool)

	insertData := data.UpdateStripAssignedSquawkByIDParams{
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
	}

	return err
}

func (s *Server) euroscopeeventhandlerSquawk(msg []byte, session int32) error {
	var event EuroscopeSquawkEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}

	db := data.New(s.DBPool)

	insertData := data.UpdateStripSquawkByIDParams{
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
	}

	return err
}

func (s *Server) euroscopeeventhandlerRequestedAltitude(msg []byte, session int32) error {
	var event EuroscopeRequestedAltitudeEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}

	db := data.New(s.DBPool)

	insertData := data.UpdateStripRequestedAltitudeByIDParams{
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
	}
	return err
}

func (s *Server) euroscopeeventhandlerClearedAltitude(msg []byte, session int32) error {
	var event EuroscopeClearedAltitudeEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}

	db := data.New(s.DBPool)
	insertData := data.UpdateStripClearedAltitudeByIDParams{
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
	}
	return err
}

func (s *Server) euroscopeeventhandlerCommunicationType(msg []byte, session int32) error {
	var event EuroscopeCommunicationTypeEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}

	db := data.New(s.DBPool)

	insertData := data.UpdateStripCommunicationTypeByIDParams{
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
	}
	return err
}

func (s *Server) euroscopeeventhandlerGroundState(msg []byte, session int32) error {
	var event EuroscopeGroundStateEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}

	db := data.New(s.DBPool)
	insertData := data.UpdateStripGroundStateByIDParams{
		State:    pgtype.Text{Valid: true, String: event.GroundState},
		Callsign: event.Callsign,
		Session:  session,
	}

	count, err := db.UpdateStripGroundStateByID(context.TODO(), insertData)
	if err != nil {
		return err
	}
	if count != 1 {
		log.Printf("Strip %v which is being updated does not exist in the database", event.Callsign)
	}
	return err
}

func (s *Server) euroscopeeventhandlerClearedFlag(msg []byte, session int32) error {
	var event EuroscopeClearedFlagEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}

	db := data.New(s.DBPool)
	insertData := data.UpdateStripClearedFlagByIDParams{
		Cleared:  pgtype.Bool{Valid: true, Bool: event.Cleared},
		Callsign: event.Callsign,
		Session:  session,
	}
	count, err := db.UpdateStripClearedFlagByID(context.TODO(), insertData)
	if err != nil {
		return err
	}
	if count != 1 {
		log.Printf("Strip %v which is being updated does not exist in the database", event.Callsign)
	}
	return err
}

func (s *Server) euroscopeeventhandlerPositionUpdate(msg []byte, session int32) error {
	var event EuroscopeAircraftPositionUpdateEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}

	db := data.New(s.DBPool)
	insertData := data.UpdateStripAircraftPositionByIDParams{
		PositionLatitude:  pgtype.Float8{Valid: true, Float64: event.Lat},
		PositionLongitude: pgtype.Float8{Valid: true, Float64: event.Lon},
		PositionAltitude:  pgtype.Int4{Valid: true, Int32: int32(event.Altitude)},
		Callsign:          event.Callsign,
		Session:           session,
	}

	count, err := db.UpdateStripAircraftPositionByID(context.TODO(), insertData)
	if err != nil {
		return err
	}
	if count != 1 {
		log.Printf("Strip %v which is being updated does not exist in the database", event.Callsign)
	}
	return err
}

func (s *Server) euroscopeeventhandlerSetHeading(msg []byte, session int32) error {
	var event EuroscopeHeadingEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}

	db := data.New(s.DBPool)
	insertData := data.UpdateStripHeadingByIDParams{
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
	}
	return err
}

func (s *Server) euroscopeeventhandlerAircraftDisconnected(msg []byte, session int32) error {
	var event EuroscopeAircraftDisconnectEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}

	db := data.New(s.DBPool)
	err = db.RemoveStripByID(context.TODO(), data.RemoveStripByIDParams{Callsign: event.Callsign, Session: session})
	return err
}

func (s *Server) euroscopeeventhandlerStand(msg []byte, session int32) error {
	var event EuroscopeStandEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}

	db := data.New(s.DBPool)
	insertData := data.UpdateStripStandByIDParams{
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
	}
	return err
}

func (s *Server) euroscopeeventhandlerSync(msg []byte, session int32, airport string) error {
	var event EuroscopeSyncEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}

	db := data.New(s.DBPool)

	for _, controller := range event.Controllers {
		// Check if the controller exists
		_, err := db.GetController(context.TODO(), data.GetControllerParams{Callsign: controller.Callsign, Session: session})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		if errors.Is(err, pgx.ErrNoRows) {
			// Controller doesn't exist, so insert
			controllerParams := data.InsertControllerParams{
				Callsign:          controller.Callsign,
				Session:           session,
				Airport:           airport,
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
			updateControllerParams := data.SetControllerPositionParams{
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
		err = handleStripUpdate(db, strip, session)
		if err != nil {
			return err
		}
	}

	return nil
}

func handleStripUpdate(db *data.Queries, strip EuroscopeStrip, session int32) error {
	// Check if the strip exists
	_, err := db.GetStrip(context.TODO(), data.GetStripParams{Callsign: strip.Callsign, Session: session})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	stripParams := data.InsertStripParams{ //keep this for insert
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
		Tobt:              pgtype.Text{Valid: false}, // These fields are not in the provided event.
	}

	if errors.Is(err, pgx.ErrNoRows) {
		// Strip doesn't exist, so insert
		err = db.InsertStrip(context.TODO(), stripParams)
		if err != nil {
			return err
		}
		log.Printf("Inserted strip: %s", strip.Callsign)
	} else {
		// Strip exists, update it
		// TODO we need to ensure the master is synced first otherwise this will overwrite the strip with potential wrong values
		updateStripParams := data.UpdateStripParams{ // create this
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
			Stand:             pgtype.Text{Valid: true, String: strip.Stand},
			Cleared:           pgtype.Bool{Valid: true, Bool: strip.Cleared},
			State:             pgtype.Text{Valid: true, String: strip.GroundState},
			PositionLatitude:  pgtype.Float8{Valid: true, Float64: strip.Position.Lat},
			PositionLongitude: pgtype.Float8{Valid: true, Float64: strip.Position.Lon},
			PositionAltitude:  pgtype.Int4{Valid: true, Int32: int32(strip.Position.Altitude)},
		}
		_, err = db.UpdateStrip(context.TODO(), updateStripParams)
		if err != nil {
			return err
		}
		log.Printf("Updated strip: %s", strip.Callsign)
	}

	return nil
}

func (s *Server) euroscopeeventhandlerStripUpdate(msg []byte, session int32) error {
	var event EuroscopeStripUpdateEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}

	db := data.New(s.DBPool)

	err = handleStripUpdate(db, event.EuroscopeStrip, session)
	return err
}
