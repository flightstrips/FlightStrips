package services

import (
	"FlightStrips/internal/shared"
	"errors"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

type AuthenticationService struct {
	authSigningAlgorithm string
	authServerUrl        string
	serverKeyfunc        keyfunc.Keyfunc
}

func NewAuthenticationService(authSigningAlgorithm string, authServerUrl string) (*AuthenticationService, error) {
	k, err := keyfunc.NewDefault([]string{authServerUrl})
	if err != nil {
		return nil, err
	}

	return &AuthenticationService{authSigningAlgorithm, authServerUrl, k}, nil
}

func (a AuthenticationService) Validate(jwtToken string) (shared.AuthenticatedUser, error) {
	options := jwt.WithValidMethods([]string{a.authSigningAlgorithm})
	token, err := jwt.Parse(jwtToken, a.serverKeyfunc.Keyfunc, options)
	if err != nil {
		return shared.AuthenticatedUser{}, err
	}
	if !token.Valid {
		return shared.AuthenticatedUser{}, errors.New("invalid jwt")
	}

	claims := token.Claims.(jwt.MapClaims)

	cid, ok := claims["vatsim/cid"].(string)

	if !ok {
		return shared.AuthenticatedUser{}, errors.New("missing CID claim")
	}

	/*
		rating, ok := claims["vatsim/rating"].(int)

		if !ok {
			return shared.AuthenticatedUser{}, errors.New("missing Rating claim")
		}
	*/

	esUser := shared.NewAuthenticatedUser(cid, 0, token)
	return esUser, nil
}
