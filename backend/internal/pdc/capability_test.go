package pdc

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func pdcCapabilityAirports(t *testing.T) *config.AirportRegistry {
	t.Helper()
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "../../.."))
	data, err := os.ReadFile(filepath.Join(root, ".github/specs/multi-airport-support/examples/zzzz.airport.yaml"))
	require.NoError(t, err)
	selected := []string{"backend/config/airports/ekch.airport.yaml"}
	// The regional labels below deliberately use fictional paper-handler inputs;
	// they are disabled service fixtures and cannot activate in production.
	for _, airport := range []string{"EKBI", "EKYT"} {
		path := filepath.Join(t.TempDir(), airport+".yaml")
		require.NoError(t, os.WriteFile(path, []byte(strings.ReplaceAll(string(data), "ZZZZ", airport)), 0600))
		selected = append(selected, path)
	}
	catalog, err := config.NewAirportCatalog(config.EKCHDescriptor(), config.HandlerDescriptor{ID: "example-paper/v1", View: "example-paper", Layouts: []string{"example-paper"}, DecodeSettings: config.TypedAirportSettings[map[string]any], Validate: func(*config.AirportDocument) error { return nil }})
	require.NoError(t, err)
	registry, err := config.LoadAirportFiles(catalog, selected, config.AirportLoadOptions{RepositoryRoot: root, DeploymentServices: map[string]bool{"pdc": true}})
	require.NoError(t, err)
	return registry
}

type countedPDCClient struct {
	polls []string
	sends atomic.Int32
	fail  string
}

type capabilityAuth struct{}

func (capabilityAuth) Validate(string) (shared.AuthenticatedUser, error) {
	return shared.NewAuthenticatedUser("123", 0, nil), nil
}

func (c *countedPDCClient) Poll(_ context.Context, airport string) ([]Message, error) {
	c.polls = append(c.polls, airport)
	if airport == c.fail {
		return nil, errors.New("synthetic mailbox failure")
	}
	return nil, nil
}
func (c *countedPDCClient) SendCPDLC(context.Context, string, string, string) error {
	c.sends.Add(1)
	return nil
}
func (c *countedPDCClient) SendTelex(context.Context, string, string, string) error {
	c.sends.Add(1)
	return nil
}

func TestDisabledRegionalPDCRejectsDirectCallsAndNeverPollsMailboxes(t *testing.T) {
	registry := pdcCapabilityAirports(t)
	sessions := []*models.Session{{ID: 1, Name: "LIVE", Airport: "EKCH"}, {ID: 2, Name: "LIVE", Airport: "EKBI"}, {ID: 3, Name: "LIVE", Airport: "EKYT"}}
	deps := validPdcServiceDependencies()
	deps.Airports = registry
	client := &countedPDCClient{}
	deps.Client = client
	deps.Sessions = &testutil.MockSessionRepository{GetByNamesFn: func(context.Context, string) ([]*models.Session, error) { return sessions, nil }, ListFn: func(context.Context) ([]*models.Session, error) { return sessions, nil }, GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
		for _, session := range sessions {
			if session.ID == id {
				return session, nil
			}
		}
		return nil, sql.ErrNoRows
	}}
	deps.Strips = &testutil.MockStripRepository{ListFn: func(_ context.Context, id int32) ([]*models.Strip, error) {
		require.EqualValues(t, 1, id, "disabled airports must not restore timeout work")
		return nil, nil
	}, GetByCallsignFn: func(_ context.Context, id int32, _ string) (*models.Strip, error) {
		require.EqualValues(t, 1, id, "disabled airports must not be searched")
		return nil, sql.ErrNoRows
	}}
	service, err := NewPDCService(deps)
	require.NoError(t, err)
	require.NoError(t, service.pollAndProcess(t.Context()))
	require.Equal(t, []string{"EKCH"}, client.polls)
	_, err = service.restorePendingTimeouts(t.Context())
	require.NoError(t, err)
	for _, session := range sessions[1:] {
		info := sessionInformation{id: session.ID, name: session.Name, airport: session.Airport, callsign: session.Airport}
		for _, call := range []func() error{
			func() error { return service.IssueClearance(t.Context(), "SAS123", "", "123", session.ID) },
			func() error { return service.RevertToVoice(t.Context(), "SAS123", session.ID, "123") },
			func() error { return service.ConfirmVoiceClearance(t.Context(), "SAS123", session.ID) },
			func() error { return service.ProcessPDCRequest(t.Context(), &IncomingMessage{}, info) },
			func() error { return service.HandleWilco(t.Context(), &IncomingMessage{}, info) },
			func() error { return service.HandleUnable(t.Context(), "SAS123", info) },
			func() error {
				return service.PersistPdcRequestOutcome(t.Context(), session.ID, "SAS123", time.Now(), PdcRequestOutcome{})
			},
			func() error {
				_, err := service.BuildClearanceOptions(t.Context(), info, nil, "", false, nil)
				return err
			},
		} {
			require.ErrorIs(t, call(), ErrCapabilityDisabled)
		}
		service.StartClearanceTimeout(t.Context(), "EKCH", "SAS123", 1, info, "123", false)
	}
	require.Empty(t, service.timeouts)
	require.Zero(t, client.sends.Load())
	_, err = service.FindWebStripByCallsign(t.Context(), "SASREG")
	require.ErrorIs(t, err, ErrWebStripNotFound)
	mux := http.NewServeMux()
	NewWebAPI(capabilityAuth{}, service, nil, false).RegisterRoutes(mux)
	request := httptest.NewRequest("POST", "/pdc/request", strings.NewReader(`{"callsign":"SASREG","aircraft_type":"A320","atis":"C"}`))
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	require.Equal(t, http.StatusNotFound, response.Code)
	require.Zero(t, client.sends.Load())
	definition, _ := registry.Lookup("EKCH")
	require.Equal(t, definition.DomainConfig().PDCValidation, service.validationForAirport("EKCH"))
	forged := sessionInformation{id: 2, name: "LIVE", airport: "EKCH", callsign: "EKCH"}
	require.Error(t, service.pollAndProcessForSession(t.Context(), forged))
}

func TestSharedPDCPollContinuesAfterMailboxFailure(t *testing.T) {
	deps := validPdcServiceDependencies()
	client := &countedPDCClient{fail: "AAAA"}
	deps.Client = client
	deps.Sessions = &testutil.MockSessionRepository{GetByNamesFn: func(context.Context, string) ([]*models.Session, error) {
		return []*models.Session{{ID: 1, Name: "LIVE", Airport: "AAAA"}, {ID: 2, Name: "LIVE", Airport: "BBBB"}}, nil
	}}
	service, err := NewPDCService(deps)
	require.NoError(t, err)
	require.ErrorContains(t, service.pollAndProcess(t.Context()), "synthetic mailbox failure")
	require.ElementsMatch(t, []string{"AAAA", "BBBB"}, client.polls)
}
