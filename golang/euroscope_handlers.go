package main

import (
	"FlightStrips/data"
	"context"
	"encoding/json"
	"errors"
	"log"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
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

	esUser := &EuroscopeUser { cid: cid, rating: int(rating), authToken: token }
	return esUser, nil
}

func (s *Server) euroscopeeventhandlerLogin(msg []byte) (success bool, err error) {
	var event EuroscopeLoginEvent
	err = json.Unmarshal(msg, &event)
	if err != nil {
		return false, err
	}

	// TODO: Login Check Login Event and place into DB.

	return true, nil
}

func (s *Server) euroscopeeventhandlerControllerOnline(msg []byte) (success bool, err error) {
	var event EuroscopeControllerOnlineEvent
	err = json.Unmarshal(msg, &event)
	if err != nil {
		return false, err
	}

	db := data.New(s.DBPool)
	data := data.InsertControllerParams{
		Cid:      "1111",
		Airport:  pgtype.Text{String: "EKCH", Valid: true},
		Position: pgtype.Text{String: event.Position, Valid: true},
	}

	db.InsertController(context.Background(), data)

	//TODO: Put into DB and shit

	return true, nil
}
