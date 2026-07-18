package standstatus

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/standdiagnostics"
	"FlightStrips/internal/vatsim"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type standStatusAuthStub struct {
	err error
}

func (s standStatusAuthStub) Validate(string) (shared.AuthenticatedUser, error) {
	return shared.NewAuthenticatedUser("1234567", 0, nil), s.err
}

type standStatusSessionStub struct {
	sessions []*models.Session
	err      error
}

func (s standStatusSessionStub) List(context.Context) ([]*models.Session, error) {
	return s.sessions, s.err
}

type standStatusAssignmentStub struct {
	assignments map[int32][]*models.StandAssignment
	blocks      map[int32][]*models.StandBlock
}

func (s standStatusAssignmentStub) ListAssignments(_ context.Context, session int32) ([]*models.StandAssignment, error) {
	return s.assignments[session], nil
}

func (s standStatusAssignmentStub) ListBlocks(_ context.Context, session int32) ([]*models.StandBlock, error) {
	return s.blocks[session], nil
}

type standStatusFeedStub struct {
	snapshot vatsim.Snapshot
}

func (s standStatusFeedStub) Snapshot() vatsim.Snapshot {
	return s.snapshot
}

func TestStandStatusRequiresAuthorization(t *testing.T) {
	t.Parallel()

	api := NewWebAPI(WebAPIConfig{Auth: standStatusAuthStub{}})
	recorder := httptest.NewRecorder()
	api.handleStatus(recorder, httptest.NewRequest(http.MethodGet, "/stand/status", nil))

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestStandStatusReturnsOperationalSnapshot(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	eta := now.Add(20 * time.Minute)
	expiry := now.Add(time.Hour)
	rule := "sas-arrivals"
	tier := int32(2)
	variant := "NON-SCHENGEN/A320"
	callsign := "SAS123"
	reason := "maintenance"
	expired := now.Add(-time.Minute)
	failures := standdiagnostics.NewAllocationFailureLog(10)
	failures.Record(standdiagnostics.AllocationFailure{
		OccurredAt: now.Add(-time.Minute), SessionID: 7, Airport: "EKCH", Callsign: "SAS999",
		Command: "AUTOMATIC_ALLOCATION", Outcome: "no_available_stand",
		Reason: "no compatible stand is available", Direction: "ARRIVAL", Stage: "CONFIRMED", Attempts: 1,
	})

	api := NewWebAPI(WebAPIConfig{
		Auth:     standStatusAuthStub{},
		Sessions: standStatusSessionStub{sessions: []*models.Session{{ID: 7, Name: "LIVE", Airport: "EKCH"}}},
		Assignments: standStatusAssignmentStub{
			assignments: map[int32][]*models.StandAssignment{
				7: {
					{
						ID: 11, SessionID: 7, Callsign: callsign, Stand: "A12", Direction: "ARRIVAL",
						Stage: "CONFIRMED", Source: "AUTO", RuleID: &rule, Tier: &tier,
						MatchedVariant: &variant, ETA: &eta, ExpiresAt: &expiry, Acknowledged: true,
						Version: 3, CreatedAt: now.Add(-time.Hour), UpdatedAt: now,
					},
				},
			},
			blocks: map[int32][]*models.StandBlock{
				7: {
					{
						ID: 21, SessionID: 7, Stand: "B2", BlockType: "OCCUPIED", Source: "CONTROLLER",
						Reason: &reason, Callsign: &callsign, Manual: true, Version: 2,
						CreatedAt: now.Add(-time.Minute), UpdatedAt: now,
					},
					{
						ID: 22, SessionID: 7, Stand: "B3", BlockType: "CLOSURE", Source: "CONTROLLER",
						ExpiresAt: &expired, Manual: true, Version: 1,
						CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Minute),
					},
				},
			},
		},
		Feed:        standStatusFeedStub{snapshot: vatsim.Snapshot{Timestamp: now.Add(-10 * time.Second)}},
		Enabled:     true,
		Ready:       true,
		StaleAfter:  time.Minute,
		Diagnostics: WebAPIDiagnostics{AircraftTypes: 812, Stands: 97, StandVariants: 121, AirlineRules: 18},
		Failures:    failures,
	})
	api.now = func() time.Time { return now }

	request := httptest.NewRequest(http.MethodGet, "/stand/status", nil)
	request.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()
	api.handleStatus(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var payload standStatusResponse
	require.NoError(t, json.NewDecoder(recorder.Body).Decode(&payload))
	require.NotContains(t, recorder.Body.String(), "snapshot_age_seconds")
	require.Equal(t, "ready", payload.System.Status)
	require.True(t, payload.System.Ready)
	require.Equal(t, 97, payload.Configuration.Stands)
	require.Equal(t, "ready", payload.Feed.Status)
	require.Len(t, payload.Failures, 1)
	require.Equal(t, "SAS999", payload.Failures[0].Callsign)
	require.Equal(t, "no_available_stand", payload.Failures[0].Outcome)
	require.Len(t, payload.Sessions, 1)
	require.Len(t, payload.Sessions[0].Assignments, 1)
	require.Equal(t, "SAS123", payload.Sessions[0].Assignments[0].Callsign)
	require.Equal(t, "sas-arrivals", *payload.Sessions[0].Assignments[0].RuleID)
	require.Len(t, payload.Sessions[0].Blocks, 1)
	require.Equal(t, int64(21), payload.Sessions[0].Blocks[0].ID)
	require.Equal(t, "maintenance", *payload.Sessions[0].Blocks[0].Reason)
}

func TestStandStatusShowsConfigurationAndFeedFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		config     WebAPIConfig
		wantStatus string
		wantReason string
	}{
		{
			name:       "invalid configuration",
			config:     WebAPIConfig{Auth: standStatusAuthStub{}, Enabled: true, Reason: "load stand capabilities: bad row"},
			wantStatus: "invalid_config",
			wantReason: "load stand capabilities: bad row",
		},
		{
			name: "failed feed",
			config: WebAPIConfig{
				Auth: standStatusAuthStub{}, Enabled: true, Ready: true, StaleAfter: time.Minute,
				Feed: standStatusFeedStub{snapshot: vatsim.Snapshot{
					Timestamp: time.Now().UTC(), LastRefreshError: errors.New("network down"),
				}},
			},
			wantStatus: "feed_failed",
			wantReason: "network down",
		},
		{
			name: "stale feed",
			config: WebAPIConfig{
				Auth: standStatusAuthStub{}, Enabled: true, Ready: true, StaleAfter: time.Minute,
				Feed: standStatusFeedStub{snapshot: vatsim.Snapshot{
					Timestamp: time.Now().UTC().Add(-2 * time.Minute),
				}},
			},
			wantStatus: "feed_stale",
			wantReason: "VATSIM snapshot is stale",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			api := NewWebAPI(test.config)
			request := httptest.NewRequest(http.MethodGet, "/stand/status", nil)
			request.Header.Set("Authorization", "Bearer token")
			recorder := httptest.NewRecorder()
			api.handleStatus(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code)
			var payload standStatusResponse
			require.NoError(t, json.NewDecoder(recorder.Body).Decode(&payload))
			require.Equal(t, test.wantStatus, payload.System.Status)
			require.Equal(t, test.wantReason, payload.System.Reason)
		})
	}
}

func TestStandStatusReturnsEmptyFailureListWhenNoFailuresExist(t *testing.T) {
	t.Parallel()

	api := NewWebAPI(WebAPIConfig{
		Auth:     standStatusAuthStub{},
		Failures: standdiagnostics.NewAllocationFailureLog(10),
	})
	request := httptest.NewRequest(http.MethodGet, "/stand/status", nil)
	request.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()

	api.handleStatus(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(recorder.Body).Decode(&payload))
	require.JSONEq(t, `[]`, string(payload["failures"]))
}
