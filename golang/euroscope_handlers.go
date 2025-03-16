package main

import (
	"FlightStrips/data"
	"context"
	"encoding/json"
	"errors"
	"log"
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

	db := data.New(s.DBPool)

	params := data.SetControllerEuroscopeSeenParams{
		Callsign:  client.callsign,
		Session: client.session,
	}

	count, err := db.SetControllerEuroscopeSeen(context.Background(), params)

	if err != nil {
		return err
	}

	if count != 1 {
		log.Printf("Disconnected client did not exist in the database. Callsign: %s", client.callsign)
	}

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
		return nil, errors.New("Missing CID claim")
	}

	rating, ok := claims["vatsim/rating"].(float64)

	if !ok {
		return nil, errors.New("Missing Rating claim")
	}

	esUser := &EuroscopeUser{cid: cid, rating: int(rating), authToken: token}
	return esUser, nil
}

func (s *Server) euroscopeeventhandlerLogin(msg []byte) (event EuroscopeLoginEvent, sessionId int32, err error) {
	err = json.Unmarshal(msg, &event)
	if err != nil {
		return
	}

	const sessionName = "LIVE"

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

	if err == pgx.ErrNoRows {
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

	if err == pgx.ErrNoRows {
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

func (s *Server) euroscopeeventhandlerAssignedSquawk(msg []byte, session int32, airport string) error {
	var event EuroscopeAssignedSquawkEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}

	db := data.New(s.DBPool)

	data := data.UpdateStripAssignedSquawkByIDParams{
		AssignedSquawk: pgtype.Text{Valid: true, String: event.Squawk},
		Callsign:       event.Callsign,
		Session:        session,
	}

	count, err := db.UpdateStripAssignedSquawkByID(context.TODO(), data)

	if err != nil {
		return err
	}

	if count != 1 {
		log.Printf("Strip %v which is being updated does not exist in the database", event.Callsign)
	}

	return err
}

func (s *Server) euroscopeeventhandlerSquawk(msg []byte, session int32, airport string) error {
	var event EuroscopeSquawkEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		return err
	}

	db := data.New(s.DBPool)

	data := data.UpdateStripSquawkByIDParams{
		Squawk:   pgtype.Text{Valid: true, String: event.Squawk},
		Callsign: event.Callsign,
		Session:  session,
	}

	count, err := db.UpdateStripSquawkByID(context.TODO(), data)

	if err != nil {
		return err
	}

	if count != 1 {
		log.Printf("Strip %v which is being updated does not exist in the database", event.Callsign)
	}

	return err
}
