package testtools

import (
	appconfig "FlightStrips/internal/config"
	"FlightStrips/internal/shared"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

type acceptingAuth struct{}

func (acceptingAuth) Validate(string) (shared.AuthenticatedUser, error) {
	return shared.AuthenticatedUser{}, nil
}

func TestWebAPIRequiresAuthenticationBeforeExposingStatus(t *testing.T) {
	mux := http.NewServeMux()
	NewWebAPI(&Service{}, acceptingAuth{}, appconfig.StandAssignmentReadiness{}).RegisterRoutes(mux)

	unauthorized := httptest.NewRecorder()
	mux.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/test/status", nil))
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)

	authorized := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/test/status", nil)
	request.Header.Set("Authorization", "Bearer local-token")
	mux.ServeHTTP(authorized, request)
	require.Equal(t, http.StatusServiceUnavailable, authorized.Code)
}
