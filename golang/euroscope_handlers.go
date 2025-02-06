package main

import (
	"errors"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"log"
)

func (s *Server) euroscopehandlerAuthentication(event EuroscopeAuthenticationEvent, authServerURL string) (token *jwt.Token, err error) {

	// TODO: Sort out Logging
	JWTToken := event.Token
	// TODO: Parameterize the server URL

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
