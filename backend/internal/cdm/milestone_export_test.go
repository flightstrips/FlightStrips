package cdm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"github.com/stretchr/testify/require"
)

func milestoneTestSessions() *testutil.MockSessionRepository {
	return &testutil.MockSessionRepository{GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
		return &models.Session{ID: id, Name: "LIVE", Airport: "EKCH"}, nil
	}}
}

func withMilestoneAcknowledgements(repo *testutil.MockStripRepository) *testutil.MockStripRepository {
	repo.AcknowledgeCdmMilestoneFn = func(ctx context.Context, session int32, callsign, kind, value string) (int64, error) {
		data, err := repo.GetCdmDataForCallsign(ctx, session, callsign)
		if err != nil {
			return 0, err
		}
		current, pending := milestoneValue(data, kind == "AOBT")
		if !pending || current != value {
			return 0, nil
		}
		updated := data.Clone()
		if kind == "AOBT" {
			updated.AobtViffPending = false
		} else {
			updated.AtotViffPending = false
		}
		return repo.SetCdmData(ctx, session, callsign, updated)
	}
	return repo
}

func TestAobtRetriesAfterPersistenceAndKeepsLatestCdmFields(t *testing.T) {
	const session = int32(779)
	aobt := "1810"
	data := &models.CdmData{Aobt: &aobt, AobtViffPending: true}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/ifps/dpi", r.URL.Path)
		require.Equal(t, "AOBT/1810", r.URL.Query().Get("value"))
		requests++
		if requests == 1 {
			_, _ = w.Write([]byte("false"))
			return
		}
		// Another action updates the data while the network request is in flight.
		tobt := "1820"
		data.Tobt = &tobt
		_, _ = w.Write([]byte("true"))
	}))
	defer server.Close()
	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
			return &models.Strip{Origin: "EKCH", Callsign: "SAS779"}, nil
		},
		GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) { return data.Clone(), nil },
		SetCdmDataFn: func(_ context.Context, _ int32, _ string, next *models.CdmData) (int64, error) {
			data = next.Clone()
			return 1, nil
		},
	}
	s := newTestCdmService(NewClient(WithAPIKey("test"), WithBaseURL(server.URL)), withMilestoneAcknowledgements(repo), milestoneTestSessions(), &testutil.MockControllerRepository{})
	markSessionLive(s, session)
	require.ErrorContains(t, s.actionService.sendPendingAobt(context.Background(), session, "SAS779"), "did not confirm")
	require.True(t, data.AobtViffPending)
	// Round-trip persisted JSON to model a later sync after process restart.
	persisted, err := json.Marshal(data)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(persisted, &data))
	s = newTestCdmService(NewClient(WithAPIKey("test"), WithBaseURL(server.URL)), withMilestoneAcknowledgements(repo), milestoneTestSessions(), &testutil.MockControllerRepository{})
	markSessionLive(s, session)
	require.NoError(t, s.actionService.sendPendingAobt(context.Background(), session, "SAS779"))
	require.False(t, data.AobtViffPending)
	require.Equal(t, "1820", *data.Tobt)
	require.NoError(t, s.actionService.sendPendingAobt(context.Background(), session, "SAS779"))
	require.Equal(t, 2, requests)
}

func TestArrivalCannotRecordDepartureMilestones(t *testing.T) {
	for _, action := range []string{"push", "transfer", "takeoff"} {
		t.Run(action, func(t *testing.T) {
			data := &models.CdmData{}
			repo := &testutil.MockStripRepository{
				GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
					return &models.Strip{Callsign: "QTR8258", Origin: "OTHH", Destination: "EKCH"}, nil
				},
				GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) { return data.Clone(), nil },
				SetCdmDataFn: func(_ context.Context, _ int32, _ string, next *models.CdmData) (int64, error) {
					data = next.Clone()
					return 1, nil
				},
			}
			s := newTestCdmService(newTestClientWithAirportMasters(nil), repo, milestoneTestSessions(), &testutil.MockControllerRepository{})
			markSessionLive(s, 779)
			var err error
			switch action {
			case "push":
				err = s.SyncAsatForGroundState(context.Background(), 779, "QTR8258", "PUSH")
			case "transfer":
				err = s.RecordAobtForTransfer(context.Background(), 779, "QTR8258")
			case "takeoff":
				err = s.RecordTakeoffClearanceAtot(context.Background(), 779, "QTR8258")
			}
			require.NoError(t, err)
			require.Nil(t, data.Aobt)
			require.Nil(t, data.Atot)
			require.False(t, data.AobtViffPending)
			require.False(t, data.AtotViffPending)
		})
	}
}

func TestPushbackVerificationPreservesRemoteErrors(t *testing.T) {
	for _, tc := range []struct {
		name, writeResponse, readResponse, want string
		readStatus                              int
	}{
		{"rejected export", "false", "", "export pushback proposal: vIFF did not confirm", 200},
		{"read unavailable", "true", "down", "503", 503},
		{"read malformed", "true", "{", "verify pushback proposal:", 200},
		{"different TOBT", "true", `{"callsign":"SAS779","tobt":"2000"}`, "did not confirm the proposed TOBT", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/ifps/callsign" {
					w.WriteHeader(tc.readStatus)
					_, _ = w.Write([]byte(tc.readResponse))
					return
				}
				_, _ = w.Write([]byte(tc.writeResponse))
			}))
			defer server.Close()
			s := newTestCdmService(NewClient(WithAPIKey("test"), WithBaseURL(server.URL)), &testutil.MockStripRepository{}, milestoneTestSessions(), &testutil.MockControllerRepository{})
			markSessionLive(s, 779)
			tobt, tsat := "1800", "180500"
			got, _, verified, err := s.actionService.verifyPushbackProposal(context.Background(), 779, "SAS779", &models.Strip{Origin: "EKCH"}, &models.CdmData{Tobt: &tobt, Tsat: &tsat}, tobt)
			require.False(t, verified)
			require.Equal(t, tsat, got)
			require.ErrorContains(t, err, tc.want)
			require.ErrorIs(t, err, ErrPushbackVerification)
		})
	}
}
