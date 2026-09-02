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

type standStatusStripStub struct {
	strips map[int32][]*models.Strip
}

func (s standStatusStripStub) List(_ context.Context, session int32) ([]*models.Strip, error) {
	return s.strips[session], nil
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
	expiredAssignment := now.Add(-time.Minute)
	rule := "sas-arrivals"
	tier := int32(2)
	variant := "NON-SCHENGEN/A320"
	callsign := "SAS123"
	reason := "maintenance"
	expired := now.Add(-time.Minute)
	failures := standdiagnostics.NewAllocationFailureLog(10)
	failures.Record(standdiagnostics.AllocationFailure{
		OccurredAt: now.Add(-time.Minute), SessionID: 7, Airport: "EKCH", Callsign: "SAS999",
		Command: "AUTOMATIC_ALLOCATION", Outcome: "no_available_stand", Severity: standdiagnostics.SeverityError,
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
					{
						ID: 12, SessionID: 7, Callsign: "SAS124", Stand: "A13", Direction: "ARRIVAL",
						Stage: "CONFIRMED", Source: "AUTO", ExpiresAt: &expiredAssignment,
						Version: 1, CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Minute),
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
	require.Equal(t, standdiagnostics.SeverityError, payload.Failures[0].Severity)
	require.Equal(t, 1, payload.Failures[0].Occurrences)
	require.Equal(t, now.Add(-time.Minute), payload.Failures[0].FirstOccurredAt)
	require.Len(t, payload.Sessions, 1)
	require.Len(t, payload.Sessions[0].Assignments, 2)
	require.Equal(t, "SAS123", payload.Sessions[0].Assignments[0].Callsign)
	require.Equal(t, "sas-arrivals", *payload.Sessions[0].Assignments[0].RuleID)
	require.Len(t, payload.Sessions[0].Blocks, 2)
	require.Equal(t, int64(21), payload.Sessions[0].Blocks[0].ID)
	require.Equal(t, "maintenance", *payload.Sessions[0].Blocks[0].Reason)
}

func TestStandStatusCoalescesRepeatedUnchangedFailures(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 3, 17, 30, 0, 0, time.UTC)
	failures := standdiagnostics.NewAllocationFailureLog(10)
	for _, occurredAt := range []time.Time{now.Add(-2 * time.Minute), now.Add(-time.Minute)} {
		failures.Record(standdiagnostics.AllocationFailure{
			OccurredAt: occurredAt, SessionID: 7, Airport: "EKCH", Callsign: "NSZ3631",
			Command: "AUTOMATIC_ALLOCATION", Outcome: "no_compatible_stand",
			Severity: standdiagnostics.SeverityWarning, Stage: "ESTIMATED",
			Reason: "no stand is compatible with the flight", AircraftType: "B73M", Attempts: 1,
		})
	}
	// A stage escalation is a separate operational error and must not be folded
	// into the earlier ESTIMATED warning.
	failures.Record(standdiagnostics.AllocationFailure{
		OccurredAt: now, SessionID: 7, Airport: "EKCH", Callsign: "NSZ3631",
		Command: "AUTOMATIC_ALLOCATION", Outcome: "no_compatible_stand",
		Severity: standdiagnostics.SeverityError, Stage: "ASSIGNED",
		Reason: "no stand is compatible with the flight", AircraftType: "B73M", Attempts: 1,
	})

	api := NewWebAPI(WebAPIConfig{Auth: standStatusAuthStub{}, Failures: failures})
	api.now = func() time.Time { return now }
	request := httptest.NewRequest(http.MethodGet, "/stand/status", nil)
	request.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()
	api.handleStatus(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var payload standStatusResponse
	require.NoError(t, json.NewDecoder(recorder.Body).Decode(&payload))
	require.Len(t, payload.Failures, 2)
	require.Equal(t, standdiagnostics.SeverityError, payload.Failures[0].Severity)
	require.Equal(t, 1, payload.Failures[0].Occurrences)
	require.Equal(t, standdiagnostics.SeverityWarning, payload.Failures[1].Severity)
	require.Equal(t, 2, payload.Failures[1].Occurrences)
	require.Equal(t, now.Add(-2*time.Minute), payload.Failures[1].FirstOccurredAt)
	require.Equal(t, now.Add(-time.Minute), payload.Failures[1].OccurredAt)
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

func TestStandStatusOnlyReturnsFailuresFromTheLastTwoHours(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 10, 18, 0, 0, 0, time.UTC)
	failures := standdiagnostics.NewAllocationFailureLog(10)
	failures.Record(standdiagnostics.AllocationFailure{Callsign: "OLD", OccurredAt: now.Add(-2*time.Hour - time.Second)})
	failures.Record(standdiagnostics.AllocationFailure{Callsign: "BOUNDARY", OccurredAt: now.Add(-2 * time.Hour)})
	failures.Record(standdiagnostics.AllocationFailure{Callsign: "RECENT", OccurredAt: now.Add(-time.Hour)})
	api := NewWebAPI(WebAPIConfig{Auth: standStatusAuthStub{}, Failures: failures})
	api.now = func() time.Time { return now }
	request := httptest.NewRequest(http.MethodGet, "/stand/status", nil)
	request.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()

	api.handleStatus(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload standStatusResponse
	require.NoError(t, json.NewDecoder(recorder.Body).Decode(&payload))
	require.Len(t, payload.Failures, 2)
	require.Equal(t, "RECENT", payload.Failures[0].Callsign)
	require.Equal(t, "BOUNDARY", payload.Failures[1].Callsign)
}

func TestStandStatusIncludesDepartureTiming(t *testing.T) {
	now := time.Date(2026, time.July, 24, 18, 0, 0, 0, time.UTC)
	tobt, tsat := "1810", "1820"
	projectedRelease := time.Date(2026, time.July, 24, 18, 24, 0, 0, time.UTC)
	response := mapStandStatusSession(
		&models.Session{ID: 7, Name: "LIVE", Airport: "EKCH"},
		[]*models.StandAssignment{{ID: 11, Callsign: "SAS123", Stand: "A12", Direction: "DEPARTURE", ProjectedReleaseAt: &projectedRelease}},
		nil,
		[]*models.Strip{{Callsign: "SAS123", CdmData: &models.CdmData{Tobt: &tobt, Tsat: &tsat}}},
		now,
	)

	require.Len(t, response.Assignments, 1)
	require.Equal(t, "1810", *response.Assignments[0].DepartureTOBT)
	require.Equal(t, "1820", *response.Assignments[0].DepartureTSAT)
	require.Equal(t, projectedRelease, *response.Assignments[0].PlannedReleaseAt, "debug timing must match the persisted allocator projection")
}
