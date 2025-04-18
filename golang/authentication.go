package main

import (
	"errors"
	"log"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

func (s *Server) parseAuthenticationToken(eventToken string) (user *ClientUser, err error) {
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

	esUser := &ClientUser{cid: cid, rating: int(rating), token: token}
	return esUser, nil
}
