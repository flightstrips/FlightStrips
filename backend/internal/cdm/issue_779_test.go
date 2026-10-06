package cdm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogonTobtAlignsEobtAndPreservesConfirmedEstimate(t *testing.T) {
	now := time.Date(2026, 9, 26, 23, 50, 0, 0, time.UTC)
	action := &ActionService{}
	far := action.PrepareEuroscopeLogonSync(&models.CdmData{}, "0031", now)
	require.NotNil(t, far.Eobt)
	assert.Equal(t, "0020", *far.Eobt)
	assert.Equal(t, "0020", *far.Tobt)
	assert.True(t, far.TobtAutoAdjusted)
	result := Calculate(CalcInput{Tobt: *far.Tobt, Eobt: *far.Eobt, TaxiMin: 10}, nil, nil, now)
	assertClockResult(t, result, "002000", "003000")

	boundary := action.PrepareEuroscopeLogonSync(&models.CdmData{}, "0030", now)
	assert.Equal(t, "0030", *boundary.Tobt)
	assert.Equal(t, "0030", *boundary.Eobt)
	assert.False(t, boundary.TobtAutoAdjusted)

	confirmed := "0110"
	for _, source := range []string{models.TobtConfirmedByPilot, models.TobtConfirmedByATC} {
		t.Run(source, func(t *testing.T) {
			manual := action.PrepareEuroscopeLogonSync(&models.CdmData{
				Tobt: &confirmed, TobtConfirmedBy: &source, TobtManuallyConfirmed: true,
			}, "0031", now)
			assert.Equal(t, confirmed, *manual.Tobt)
			assert.Equal(t, "0031", *manual.Eobt)
		})
	}
}

func TestClearanceTobtAlignsEobtAfterPersistenceAndPreservesConfirmation(t *testing.T) {
	for _, source := range []string{"", models.TobtConfirmedByPilot, models.TobtConfirmedByATC} {
		t.Run("confirmation="+source, func(t *testing.T) {
			future := time.Now().UTC().Add(40 * time.Minute).Format("1504")
			data := &models.CdmData{Tobt: &future, Eobt: &future}
			if source != "" {
				data.TobtConfirmedBy = &source
				data.TobtManuallyConfirmed = true
			}
			persisted := false
			repo := &testutil.MockStripRepository{
				GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
					return &models.Strip{Callsign: "SAS779", Origin: "EKCH"}, nil
				},
				GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) {
					return data.Clone(), nil
				},
				SetCdmDataFn: func(_ context.Context, _ int32, _ string, updated *models.CdmData) (int64, error) {
					data = updated.Clone()
					persisted = true
					return 1, nil
				},
			}
			hub := &testutil.MockEuroscopeHub{
				GetMasterCallsignFn: func(int32) string {
					assert.True(t, persisted, "EOBT must be sent after persistence")
					return "EKCH_DEL"
				},
			}
			controllers := &testutil.MockControllerRepository{
				GetByCallsignFn: func(context.Context, int32, string) (*models.Controller, error) {
					return &models.Controller{Cid: stringPtr("12345")}, nil
				},
			}
			service := newTestCdmService(newTestClientWithAirportMasters(nil), repo, &testutil.MockSessionRepository{}, controllers)
			service.client.isValid = false
			setTestCdmEuroscope(service, hub)
			require.NoError(t, service.HandleClearanceTobt(context.Background(), 779, "SAS779"))
			if source != "" {
				assert.False(t, persisted)
				assert.Empty(t, hub.Eobts)
				assert.Equal(t, future, *data.Tobt)
				return
			}
			require.True(t, persisted)
			assert.Equal(t, *data.Tobt, *data.Eobt)
			assert.InDelta(t, 15, minutesBetween(time.Now().UTC().Format("1504"), *data.Tobt), 1)
			require.Len(t, hub.Eobts, 1)
			assert.Equal(t, *data.Eobt, hub.Eobts[0].Eobt)
		})
	}
}

func TestTakeoffClearanceAtotIsSentOnceAfterPersistence(t *testing.T) {
	requests := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/ifps/dpi", r.URL.Path)
		assert.Equal(t, "SAS779", r.URL.Query().Get("callsign"))
		requests <- r.URL.Query().Get("value")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	data := &models.CdmData{}
	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
			return &models.Strip{Callsign: "SAS779", Origin: "EKCH"}, nil
		},
		GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) {
			return data.Clone(), nil
		},
		SetCdmDataFn: func(_ context.Context, _ int32, _ string, updated *models.CdmData) (int64, error) {
			data = updated.Clone()
			return 1, nil
		},
	}
	service := newTestCdmService(NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)), repo, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	setTestCdmFrontend(service, &testutil.MockFrontendHub{})
	markSessionLive(service, 779)
	require.NoError(t, service.RecordTakeoffClearanceAtot(context.Background(), 779, "SAS779"))
	require.NoError(t, service.RecordTakeoffClearanceAtot(context.Background(), 779, "SAS779"))
	select {
	case value := <-requests:
		assert.Equal(t, "ATOT/"+*data.Atot, value)
	case <-time.After(time.Second):
		t.Fatal("ATOT was not sent")
	}
	select {
	case value := <-requests:
		t.Fatalf("duplicate ATOT: %s", value)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestPendingAtotRetriesOnCdmSync(t *testing.T) {
	atot, eobt := "1215", time.Now().UTC().Format("1504")
	data := &models.CdmData{Atot: &atot, AtotViffPending: true, Eobt: &eobt}
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ifps/depAirport":
			_, _ = w.Write([]byte("[]"))
		case "/ifps/dpi":
			assert.Equal(t, "ATOT/1215", r.URL.Query().Get("value"))
			attempts++
			if attempts == 1 {
				http.Error(w, "temporary failure", http.StatusBadGateway)
				return
			}
			_, _ = w.Write([]byte("true"))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	repo := &testutil.MockStripRepository{
		GetCdmDataFn: func(context.Context, int32) ([]*models.CdmDataRow, error) {
			return []*models.CdmDataRow{{Callsign: "SAS779", Data: data.Clone()}}, nil
		},
		GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) {
			return data.Clone(), nil
		},
		SetCdmDataFn: func(_ context.Context, _ int32, _ string, updated *models.CdmData) (int64, error) {
			data = updated.Clone()
			return 1, nil
		},
	}
	service := newTestCdmService(NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)), repo, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	markSessionLive(service, 779)
	require.ErrorContains(t, service.actionService.sendPendingAtot(context.Background(), 779, "SAS779"), "502")
	assert.True(t, data.AtotViffPending)
	require.NoError(t, service.syncCdmData(context.Background(), &models.Session{ID: 779, Name: "LIVE", Airport: "EKCH"}))
	assert.False(t, data.AtotViffPending)
	assert.Equal(t, 2, attempts)
	require.NoError(t, service.syncCdmData(context.Background(), &models.Session{ID: 779, Name: "LIVE", Airport: "EKCH"}))
	assert.Equal(t, 2, attempts)
}

func TestPushbackCtotReadIgnoresViffTsat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ifps/callsign", r.URL.Path)
		_, _ = fmt.Fprint(w, `{"callsign":"SAS779","ctot":"1240","cdmData":{"tsat":"130000"}}`)
	}))
	defer server.Close()
	service := newTestCdmService(NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)), &testutil.MockStripRepository{}, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	markSessionLive(service, 779)
	ctot, err := service.ReadPushbackCtot(context.Background(), 779, "SAS779")
	require.NoError(t, err)
	assert.Equal(t, "1240", ctot)
}

func TestTransferAobtIsNotReplacedAtPushback(t *testing.T) {
	requests := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL.Query().Get("value")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	data := &models.CdmData{}
	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
			return &models.Strip{Callsign: "SAS779", Origin: "EKCH"}, nil
		},
		GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) { return data.Clone(), nil },
		SetCdmDataFn: func(_ context.Context, _ int32, _ string, updated *models.CdmData) (int64, error) {
			data = updated.Clone()
			return 1, nil
		},
	}
	service := newTestCdmService(NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)), repo, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	setTestCdmFrontend(service, &testutil.MockFrontendHub{})
	markSessionLive(service, 779)
	require.NoError(t, service.RecordAobtForTransfer(context.Background(), 779, "SAS779"))
	first := *data.Aobt
	require.NoError(t, service.SyncAsatForGroundState(context.Background(), 779, "SAS779", "PUSH"))
	assert.Equal(t, first, *data.Aobt)
	select {
	case value := <-requests:
		assert.Equal(t, "AOBT/"+first, value)
	case <-time.After(time.Second):
		t.Fatal("AOBT was not sent")
	}
	select {
	case value := <-requests:
		t.Fatalf("duplicate AOBT: %s", value)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestStartupDoesNotRecordAobtBeforePushback(t *testing.T) {
	requests := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL.Query().Get("value")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	data := &models.CdmData{}
	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
			return &models.Strip{Callsign: "SAS779", Origin: "EKCH"}, nil
		},
		GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) { return data.Clone(), nil },
		SetCdmDataFn: func(_ context.Context, _ int32, _ string, updated *models.CdmData) (int64, error) {
			data = updated.Clone()
			return 1, nil
		},
	}
	service := newTestCdmService(NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)), repo, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	setTestCdmFrontend(service, &testutil.MockFrontendHub{})
	markSessionLive(service, 779)
	require.NoError(t, service.SyncAsatForGroundState(context.Background(), 779, "SAS779", "STUP"))
	require.NotNil(t, data.Asat)
	assert.Nil(t, data.Aobt)
	require.NoError(t, service.SyncAsatForGroundState(context.Background(), 779, "SAS779", "PUSH"))
	require.NotNil(t, data.Aobt)
	require.NoError(t, service.SyncAsatForGroundState(context.Background(), 779, "SAS779", "PUSH"))
	select {
	case value := <-requests:
		assert.Equal(t, "AOBT/"+*data.Aobt, value)
	case <-time.After(time.Second):
		t.Fatal("AOBT was not sent on pushback")
	}
	select {
	case value := <-requests:
		t.Fatalf("duplicate AOBT: %s", value)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestReadyRequestTimingWithAndWithoutCtot(t *testing.T) {
	now := time.Date(2026, 9, 26, 23, 55, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, tsat, ctot, want string
		change                 bool
	}{
		{"expired with ctot", "2346", "0020", "0005", true},
		{"exactly eight past", "2347", "0020", "2340", false},
		{"five past with ctot", "2350", "0020", "2340", false},
		{"near future with ctot", "0004", "0020", "2340", false},
		{"near future without ctot", "0004", "", "2355", true},
		{"ten ahead with ctot", "0005", "0020", "2355", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := "2340"
			data := &models.CdmData{Tobt: &current, Tsat: &tc.tsat}
			if tc.ctot != "" {
				data.Ctot = &tc.ctot
			}
			got, change := readyRequestTobt(data, now)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.change, change)
		})
	}
}

func TestClearanceTobtThresholdAndConfirmation(t *testing.T) {
	now := time.Date(2026, 9, 26, 23, 50, 0, 0, time.UTC)
	future := "0021"
	got, changed := clearanceTobt(&models.CdmData{Tobt: &future}, now)
	assert.True(t, changed)
	assert.Equal(t, "0005", got)

	boundary := "0020"
	_, changed = clearanceTobt(&models.CdmData{Tobt: &boundary}, now)
	assert.False(t, changed)

	source := models.TobtConfirmedByPilot
	_, changed = clearanceTobt(&models.CdmData{Tobt: &future, TobtConfirmedBy: &source, TobtManuallyConfirmed: true}, now)
	assert.False(t, changed)
}

func TestViffExportUsesProposalAndReceivedCtotReason(t *testing.T) {
	tobt, proposalTsat, proposalTtot := "1200", "120500", "121500"
	effectiveTsat, effectiveTtot, ctot := "123000", "124000", "1240"
	source := models.CtotSourceATFCM
	reason := "REG779"
	data := &models.CdmData{
		Tobt: &tobt, Tsat: &effectiveTsat, Ttot: &effectiveTtot,
		ViffProposalTsat: &proposalTsat, ViffProposalTtot: &proposalTtot,
		Ctot: &ctot, CtotSource: &source, EcfmpID: &reason,
	}
	state, ok := buildViffPushState("SAS779", nil, data)
	require.True(t, ok)
	assert.Equal(t, proposalTsat, state.Params.Tsat)
	assert.Equal(t, proposalTtot, state.Params.Ttot)
	assert.Equal(t, ctot, state.Params.Ctot)
	assert.Equal(t, reason, state.Params.Reason)
	assert.False(t, masterFlightNeedsExport(data, IFPSData{
		TOBT: "1200", CTOT: ctot,
		CDMData: CDMData{TSAT: "1205", TTOT: "1215", Reason: reason},
	}))
}

func TestViffExportUsesPluginDepartureInfoAndAsrtFormat(t *testing.T) {
	tobt, tsat, ttot, asrt := "1200", "120500", "121500", "120300"
	strip := &models.Strip{Runway: testStringPtr("22R"), Sid: testStringPtr("MIKLA1A")}
	state, ok := buildViffPushState("SAS779", strip, &models.CdmData{
		Tobt: &tobt, ViffProposalTsat: &tsat, ViffProposalTtot: &ttot, Asrt: &asrt,
	})
	require.True(t, ok)
	assert.Equal(t, "22R/MIKLA1A", state.Params.DepInfo)
	assert.Equal(t, "1203", state.Params.Asrt)
}

func TestViffExportIncludesManualCtotReason(t *testing.T) {
	tobt, tsat, ttot, ctot, reason := "1200", "120500", "121500", "1220", "REG779"
	source := models.CtotSourceManual
	state, ok := buildViffPushState("SAS779", nil, &models.CdmData{
		Tobt: &tobt, ViffProposalTsat: &tsat, ViffProposalTtot: &ttot,
		Ctot: &ctot, CtotSource: &source, EcfmpID: &reason,
	})
	require.True(t, ok)
	assert.Equal(t, ctot, state.Params.Ctot)
	assert.Equal(t, reason, state.Params.Reason)
}

func TestViffExportLeavesUnassignedCtotAndReasonEmpty(t *testing.T) {
	tobt, tsat, ttot := "1200", "120500", "121500"
	data := &models.CdmData{Tobt: &tobt, ViffProposalTsat: &tsat, ViffProposalTtot: &ttot}
	state, ok := buildViffPushState("SAS779", nil, data)
	require.True(t, ok)
	assert.Empty(t, state.Params.Ctot)
	assert.Empty(t, state.Params.Reason)
	assert.False(t, masterFlightNeedsExport(data, IFPSData{
		TOBT: tobt, CDMData: CDMData{TSAT: tsat, TTOT: ttot},
	}))

	reason := "LOCAL_REG"
	data.EcfmpID = &reason
	state, ok = buildViffPushState("SAS779", nil, data)
	require.True(t, ok)
	assert.Empty(t, state.Params.Ctot)
	assert.Equal(t, reason, state.Params.Reason)
	assert.True(t, masterFlightNeedsExport(data, IFPSData{
		TOBT: tobt, CDMData: CDMData{TSAT: tsat, TTOT: ttot, Reason: "VIFF_REG"},
	}))
}
