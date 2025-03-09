package main

import (
	"FlightStrips/data"
	"context"
	"encoding/json"
	"errors"
	"log"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
)

func (s *Server) euroscopeeventhandlerAuthentication(msg []byte) (user *EuroscopeUser, err error) {
	var event EuroscopeAuthenticationEvent
	err = json.Unmarshal(msg, &event)
	if err != nil {
		return user, err
	}

	return s.euroscopeeventhandlerAuthenticationTokenValidation(event.Token)
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

func (s *Server) euroscopeeventhandlerLogin(msg []byte) (event EuroscopeLoginEvent, err error) {
	err = json.Unmarshal(msg, &event)
	if err != nil {
		return
	}

	// Since the login is sent on first logon and when a position is changed we need to check if the controller is
	// already in the database. It may also already be in the database if the master have synced it before a new
	// controller connects to FlightStrips

	db := data.New(s.DBPool)
	controller, err := db.GetController(context.TODO(), event.Callsign)

	if err == pgx.ErrNoRows {
		data := data.InsertControllerParams{
			Callsign:  event.Callsign,
			Position:  event.Position,
			Airport:   event.Airport,
			Connected: true,
			Master:    false,
		}

		err = db.InsertController(context.Background(), data)

		return event, err
	}

	if err != nil {
		return event, err
	}

	data := data.UpdateControllerParams{Callsign: event.Callsign, Connected: true, Master: controller.Master, Position: event.Position}

	err = db.UpdateController(context.TODO(), data)

	return event, err
}

func (s *Server) euroscopeeventhandlerControllerOnline(msg []byte, airport string) (success bool, err error) {
	var event EuroscopeControllerOnlineEvent
	err = json.Unmarshal(msg, &event)
	if err != nil {
		return false, err
	}

	db := data.New(s.DBPool)
	controller, err := db.GetController(context.TODO(), event.Callsign)

	if err == pgx.ErrNoRows {
		// New controller insert
		data := data.InsertControllerParams{
			Callsign:  event.Callsign,
			Position:  event.Position,
			Airport:   airport,
			Connected: false,
			Master:    false,
		}

		err = db.InsertController(context.Background(), data)

		if err != nil {
			return false, err
		}

		return true, nil
	}

	if err != nil {
		return false, err
	}

	if controller.Position == event.Position {
		return true, nil
	}

	data := data.UpdateControllerParams{Callsign: event.Callsign, Connected: controller.Connected, Master: controller.Master, Position: event.Position}

	err = db.UpdateController(context.TODO(), data)

	if err != nil {
		return false, err
	}

	return true, nil
}
