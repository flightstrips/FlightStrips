package main

import (
	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"log"
)

func (s *Server) euroscopehandlerAuthentication(event EuroscopeAuthenticationEvent) (token *jwt.Token, err error) {

	// TODO: Sort out Logging
	JWTToken := event.Token
	// TODO: Parameterize the server URL
	serverURL := "https://dev-xd0uf4sd1v27r8tg.eu.auth0.com/.well-known/jwks.json"
	
	k, err := keyfunc.NewDefault([]string{serverURL}) // Context is used to end the refresh goroutine.
	if err != nil {
		log.Fatalf("Failed to create a keyfunc.Keyfunc from the server's URL.\nError: %s", err)
	}
	token, err = jwt.Parse(JWTToken, k.Keyfunc)
	if err != nil {
		log.Fatalf("Failed to parse the JWT.\nError: %s", err)
	}
	if !token.Valid {
		log.Fatalf("The JWT is invalid.")
	}

	return token, nil
}
