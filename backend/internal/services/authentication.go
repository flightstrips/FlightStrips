package services

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/shared"
	"errors"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

const TestToken = "__TEST_TOKEN__"
const TestFrontendToken = "__TEST_FRONTEND_TOKEN__"

type AuthenticationService struct {
	authSigningAlgorithm string
	authServerUrl        string
	requiredAudience     string
	serverKeyfunc        keyfunc.Keyfunc
}

func NewAuthenticationService(authSigningAlgorithm string, authServerUrl string, requiredAudience string) (*AuthenticationService, error) {
	k, err := keyfunc.NewDefault([]string{authServerUrl})
	if err != nil {
		return nil, err
	}

	return &AuthenticationService{authSigningAlgorithm, authServerUrl, requiredAudience, k}, nil
}

// NewTestAuthenticationService creates an authentication service for testing (no real validation)
func NewTestAuthenticationService() *AuthenticationService {
	return &AuthenticationService{
		authSigningAlgorithm: "test",
		authServerUrl:        "test",
		requiredAudience:     "test",
		serverKeyfunc:        nil,
	}
}

func (a AuthenticationService) Validate(jwtToken string) (shared.AuthenticatedUser, error) {
	// Bypass authentication in test mode
	if config.IsTestMode() {
		// Return different CIDs for different test tokens
		if jwtToken == TestFrontendToken {
			return shared.NewAuthenticatedUser("TEST_FRONTEND_CID", 0, nil), nil
		}
		// Default test user for EuroScope replay
		return shared.NewAuthenticatedUser("TEST_CID", 0, nil), nil
	}

	options := jwt.WithValidMethods([]string{a.authSigningAlgorithm})
	token, err := jwt.Parse(jwtToken, a.serverKeyfunc.Keyfunc, options, jwt.WithAudience(a.requiredAudience))
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

	ratingFloat, ok := claims["vatsim/rating"].(float64)

	if !ok {
		return shared.AuthenticatedUser{}, errors.New("missing Rating claim")
	}

	esUser := shared.NewAuthenticatedUser(cid, int(ratingFloat), token)
	return esUser, nil
}
