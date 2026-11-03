package cdm

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	es "FlightStrips/pkg/events/euroscope"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCDMCorrectionAndDeferredWorkRetainHumanAuthority(t *testing.T) {
	b := &testutil.MockCommandBinding{SessionID: 7, CIDValue: "OWN"}
	ctx, err := shared.WithHumanCommand(context.Background(), b)
	require.NoError(t, err)
	ctx = detachedContext(ctx)
	hub := &testutil.MockEuroscopeHub{GetMasterCallsignFn: func(int32) string { t.Fatal("human correction consulted master"); return "MASTER" }}
	c := &ActionService{service: &Service{euroscopeHub: hub}}
	c.pushCorrectedEobtToEuroscope(ctx, 7, "SAS123", "1234")
	require.Equal(t, []es.OutgoingMessage{es.EobtEvent{Callsign: "SAS123", Eobt: "1234"}}, b.Sent())
	b.Unavailable.Store(true)
	c.pushCorrectedEobtToEuroscope(ctx, 7, "SAS123", "1300")
	require.Len(t, b.Sent(), 1)
	require.Error(t, c.PushTobt(ctx, 7, "SAS123", "1300"))
	require.Error(t, c.sendPendingAobt(ctx, 7, "SAS123"))
	require.Error(t, c.sendPendingAtot(ctx, 7, "SAS123"))
}

func TestClxTobtSilentWritePersistsHumanAuthorityWithoutRecalculation(t *testing.T) {
	b := &testutil.MockCommandBinding{SessionID: 7, CIDValue: "OWN"}
	ctx, err := shared.WithHumanCommand(context.Background(), b)
	require.NoError(t, err)
	tobt, tsat, ttot := "1200", "1234", "1244"
	var stored atomic.Pointer[models.CdmData]
	stored.Store(&models.CdmData{Tobt: &tobt, Tsat: &tsat, Ttot: &ttot})
	strip := &models.Strip{ID: 42, Session: 7, Callsign: "SAS123", Origin: "EKCH"}
	repo := &testutil.MockStripRepository{
		GetByCallsignFn:         func(context.Context, int32, string) (*models.Strip, error) { return strip, nil },
		GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) { return stored.Load().Clone(), nil },
		SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
			stored.Store(data.Clone())
			b.Unavailable.Store(true)
			return 1, nil
		},
	}
	s := newTestCdmService(NewClient(WithAPIKey("test"), WithHTTPClient(newFailingHTTPClient())), repo, milestoneTestSessions(), &testutil.MockControllerRepository{})
	markSessionLive(s, 7)
	_, _, _, _, changed, recalculate, err := s.actionService.prepareTobtUpdate(ctx, 7, "SAS123", "1233", "GND", "ATC", time.Now())
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, recalculate)
	require.NoError(t, s.HandleClxTobtUpdate(ctx, 7, "SAS123", "1233", "GND", "ATC"))
	require.NotEmpty(t, stored.Load().ViffAuthority)
	_, found := s.deferredAuthorities.Load(stored.Load().ViffAuthority)
	require.True(t, found)
	s.masterViffSync.ensureMasterFlightExport(context.Background(), 7, "SAS123", stored.Load(), IFPSData{})
	require.Empty(t, s.masterViffSync.pushRetries.entries)
}

func TestCoalescedViffProposalsKeepTheirOwnAuthority(t *testing.T) {
	for _, automaticFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "automatic_then_human", false: "controller_A_then_B"}[automaticFirst], func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests++; _, _ = w.Write([]byte("true")) }))
			defer server.Close()
			s := newTestCdmService(NewClient(WithAPIKey("test"), WithBaseURL(server.URL)), &testutil.MockStripRepository{}, milestoneTestSessions(), &testutil.MockControllerRepository{})
			a := &testutil.MockCommandBinding{SessionID: 7, CIDValue: "A"}
			b := &testutil.MockCommandBinding{SessionID: 7, CIDValue: "B"}
			first := viffPushState{Suspend: true, session: 7, sourceSet: true}
			if !automaticFirst {
				first.source = a
			}
			next := viffPushState{Suspend: true, session: 7, sourceSet: true, source: b}
			tracker := &viffPushRetryTracker{}
			now := time.Now()
			attempt := tracker.begin(7, "SAS123", first, now)
			require.NotNil(t, attempt)
			require.Nil(t, tracker.begin(7, "SAS123", next, now))
			selected := tracker.prepare(attempt, now)
			require.NotNil(t, selected)
			require.True(t, selected.state.source.Same(b))
			workerCtx := context.Background()
			if !automaticFirst {
				var err error
				workerCtx, err = shared.WithHumanCommand(workerCtx, a)
				require.NoError(t, err)
			}
			b.Unavailable.Store(true)
			require.Error(t, s.masterViffSync.pushViffState(workerCtx, "SAS123", selected.state))
			require.Zero(t, requests)
			b.Unavailable.Store(false)
			a.Unavailable.Store(true)
			require.NoError(t, s.masterViffSync.pushViffState(workerCtx, "SAS123", selected.state))
			require.Equal(t, 1, requests)
		})
	}
}

func TestPeriodicViffReconciliationCannotAdoptUnresolvedHumanState(t *testing.T) {
	b := &testutil.MockCommandBinding{SessionID: 7, CIDValue: "OWN"}
	ctx, err := shared.WithHumanCommand(context.Background(), b)
	require.NoError(t, err)
	strip := &models.Strip{ID: 42, Session: 7, Callsign: "SAS123", Origin: "EKCH"}
	phase := "I"
	data := &models.CdmData{Phase: &phase}
	repo := &testutil.MockStripRepository{GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return strip, nil }}
	newService := func() *Service {
		s := newTestCdmService(NewClient(WithAPIKey("test"), WithHTTPClient(newFailingHTTPClient())), repo, milestoneTestSessions(), &testutil.MockControllerRepository{})
		markSessionLive(s, 7)
		return s
	}
	s := newService()
	require.NoError(t, s.captureViffAuthority(ctx, 7, strip, data))
	require.NotEmpty(t, data.ViffAuthority)
	b.Unavailable.Store(true)
	s.masterViffSync.ensureMasterFlightExport(context.Background(), 7, "SAS123", data, IFPSData{})
	require.Empty(t, s.masterViffSync.pushRetries.entries, "no automatic worker may adopt the pending human source")
	restarted := newService()
	restarted.masterViffSync.ensureMasterFlightExport(context.Background(), 7, "SAS123", data, IFPSData{})
	require.Empty(t, restarted.masterViffSync.pushRetries.entries)
}

func TestPollingCannotAdoptDeferredHumanCDMWork(t *testing.T) {
	for _, kind := range []string{"AOBT", "ATOT", "READY"} {
		t.Run(kind, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests++; _, _ = w.Write([]byte("true")) }))
			defer server.Close()
			value := "1234"
			strip := &models.Strip{ID: 42, Session: 7, Callsign: "SAS123", Origin: "EKCH"}
			data := &models.CdmData{Aobt: &value, AobtViffPending: true, Atot: &value, AtotViffPending: true, Asrt: &value, ReadySyncPending: true}
			repo := &testutil.MockStripRepository{GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return strip, nil }, GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) { return data.Clone(), nil }}
			newService := func() *Service {
				s := newTestCdmService(NewClient(WithAPIKey("test"), WithBaseURL(server.URL)), repo, milestoneTestSessions(), &testutil.MockControllerRepository{})
				markSessionLive(s, 7)
				return s
			}
			s := newService()
			original := &testutil.MockCommandBinding{SessionID: 7, CIDValue: "OWN"}
			ctx, err := shared.WithHumanCommand(context.Background(), original)
			require.NoError(t, err)
			token, err := s.actionService.rememberDeferredAuthority(ctx, 7, strip, kind, value, "")
			require.NoError(t, err)
			require.NotEmpty(t, token)
			switch kind {
			case "AOBT":
				data.AobtViffAuthority = token
			case "ATOT":
				data.AtotViffAuthority = token
			case "READY":
				data.ReadyViffAuthority = token
			}
			// Model persisted source metadata and an initiating connection lost
			// before the polling coordinator retries the existing pending record.
			encoded, err := json.Marshal(data)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(encoded, &data))
			original.Unavailable.Store(true)
			retry := func(service *Service, retryCtx context.Context) error {
				switch kind {
				case "AOBT":
					return service.actionService.sendPendingAobt(retryCtx, 7, "SAS123")
				case "ATOT":
					return service.actionService.sendPendingAtot(retryCtx, 7, "SAS123")
				default:
					return service.actionService.completeReadyViffSync(retryCtx, 7, strip, data, false)
				}
			}
			require.Error(t, retry(s, context.Background()))
			replacement := &testutil.MockCommandBinding{SessionID: 7, CIDValue: "OWN"}
			replacementCtx, err := shared.WithHumanCommand(context.Background(), replacement)
			require.NoError(t, err)
			require.Error(t, retry(s, replacementCtx), "a new connection with the same CID cannot adopt pending work")
			require.Error(t, retry(newService(), context.Background()), "missing process state after restart must fail closed")
			require.Zero(t, requests, "rejected retries must not write to the provider")
		})
	}
}
