package main

import (
	"errors"
	"time"

	"FlightStrips/internal/services"
	"FlightStrips/internal/shared"

	"github.com/golang-jwt/jwt/v5"
)

// Identity verification is an external boundary. Roles and observer restrictions
// still come from controllers created through the genuine EuroScope login.
type browserIdentity struct{}

func (browserIdentity) Validate(token string) (shared.AuthenticatedUser, error) {
	var cid string
	switch token {
	case services.TestToken:
		cid = "900001"
	case services.TestFrontendToken:
		cid = "900002"
	default:
		return shared.AuthenticatedUser{}, errors.New("unknown browser E2E identity")
	}
	return shared.NewAuthenticatedUser(cid, 0, &jwt.Token{Claims: jwt.MapClaims{"exp": float64(time.Now().Add(time.Hour).Unix())}}), nil
}
