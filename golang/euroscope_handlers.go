package main

import (
	"encoding/json"
	"errors"
	"log"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

func (s *Server) euroscopeeventhandlerAuthentication(msg []byte, authServerURL string) (token *jwt.Token, err error) {

	var event EuroscopeEvent
	err = json.Unmarshal(msg, &event)
	if err != nil {
		return token, err
	}
	if event.Type != EuroscopeAuthentication {
		return token, errors.New("invalid event type")
	}

	var authEvent EuroscopeAuthenticationEvent
	err = json.Unmarshal(msg, &authEvent)
	if err != nil {
		return token, err
	}

	return s.euroscopeeventhandlerAuthenticationTokenValidation(authEvent.Token, authServerURL)
}

func (s *Server) euroscopeeventhandlerAuthenticationTokenValidation(eventToken string, authServerURL string) (token *jwt.Token, err error) {
	// TODO: Sort out Logging
	JWTToken := eventToken

	k, err := keyfunc.NewDefault([]string{authServerURL})
	if err != nil {
		log.Fatalf("Failed to create a keyfunc.Keyfunc from the server's URL.\nError: %s", err)
	}
	token, err = jwt.Parse(JWTToken, k.Keyfunc)
	if err != nil {
		return token, err
	}
	if !token.Valid {
		return token, errors.New("invalid jwt")
	}

	return token, nil
}

func (s *Server) euroscopeeventhandlerLogin(msg []byte) (success bool, err error) {
	var event EuroscopeEvent
	err = json.Unmarshal(msg, &event)
	if err != nil {
		return false, err
	}

	if event.Type != EuroscopeLogin {
		return false, errors.New("invalid event type")
	}

	var loginEvent EuroscopeLoginEvent
	err = json.Unmarshal(msg, &loginEvent)
	if err != nil {
		return false, err
	}

	// TODO: Login Check Login Event and place into DB.

	return true, nil
}

func (s *Server) euroscopeeventhandlerControllerOnline(msg []byte) (success bool, err error) {
	var event EuroscopeEvent
	err = json.Unmarshal(msg, &event)
	if err != nil {
		return false, err
	}

	if event.Type != EuroscopeControllerOnline {
		return false, errors.New("invalid event type")
	}

	var positionOnlineEvent EuroscopeControllerOnlineEvent
	err = json.Unmarshal(msg, &positionOnlineEvent)
	if err != nil {
		return false, err
	}

	//TODO: Put into DB and shit

	return true, nil
}
