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
	euroscopeEvents "FlightStrips/pkg/events/euroscope"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogonTobtThresholdIncludesConfirmedEstimates(t *testing.T) {
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
			}, "0010", now)
			assert.Equal(t, "0020", *manual.Tobt)
			assert.Equal(t, "0020", *manual.Eobt)
			assert.True(t, manual.TobtAutoAdjusted)
			assert.False(t, manual.TobtManuallyConfirmed)
			assert.Nil(t, manual.TobtConfirmedBy)
			boundary := action.PrepareEuroscopeLogonSync(&models.CdmData{
				Tobt: stringPtr("0030"), TobtConfirmedBy: &source, TobtManuallyConfirmed: true,
			}, "0110", now)
			assert.Equal(t, "0030", *boundary.Tobt)
			assert.Equal(t, "0110", *boundary.Eobt)
			assert.True(t, boundary.TobtManuallyConfirmed)
		})
	}
}

func TestClearanceTobtAlignsEobtAfterPersistenceIncludingConfirmedEstimates(t *testing.T) {
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
			service := newTestCdmService(newTestClientWithAirportMasters(nil), withMilestoneAcknowledgements(repo), milestoneTestSessions(), controllers)
			service.client.isValid = false
			setTestCdmEuroscope(service, hub)
			require.NoError(t, service.HandleClearanceTobt(context.Background(), 779, "SAS779"))
			require.True(t, persisted)
			assert.False(t, data.TobtManuallyConfirmed)
			assert.Nil(t, data.TobtConfirmedBy)
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
	service := newTestCdmService(NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)), withMilestoneAcknowledgements(repo), milestoneTestSessions(), &testutil.MockControllerRepository{})
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
		GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
			return &models.Strip{Callsign: "SAS779", Origin: "EKCH"}, nil
		},
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
	service := newTestCdmService(NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)), withMilestoneAcknowledgements(repo), milestoneTestSessions(), &testutil.MockControllerRepository{})
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
	service := newTestCdmService(NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)), &testutil.MockStripRepository{}, milestoneTestSessions(), &testutil.MockControllerRepository{})
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
	service := newTestCdmService(NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)), withMilestoneAcknowledgements(repo), milestoneTestSessions(), &testutil.MockControllerRepository{})
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
	service := newTestCdmService(NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)), withMilestoneAcknowledgements(repo), milestoneTestSessions(), &testutil.MockControllerRepository{})
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
	got, changed = clearanceTobt(&models.CdmData{Tobt: &future, TobtConfirmedBy: &source, TobtManuallyConfirmed: true}, now)
	assert.True(t, changed)
	assert.Equal(t, "0005", got)
	_, changed = clearanceTobt(&models.CdmData{Tobt: &boundary, TobtConfirmedBy: &source, TobtManuallyConfirmed: true}, now)
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

func TestPeriodicSequenceRecoversExpiredTsatWithoutReadyRequest(t *testing.T) {
	for _, ctot := range []string{"", "0030"} {
		t.Run("ctot="+ctot, func(t *testing.T) {
			now := time.Date(2026, 10, 9, 23, 57, 0, 0, time.UTC)
			stored := &models.CdmData{Tobt: stringPtr("2350"), Tsat: stringPtr("235100"), Ttot: stringPtr("000100")}
			if ctot != "" {
				stored.Ctot = &ctot
				stored.CtotSource = stringPtr(models.CtotSourceATFCM)
			}
			repo := &testutil.MockStripRepository{
				ListByOriginFn: func(context.Context, int32, string) ([]*models.Strip, error) {
					return []*models.Strip{{Callsign: "SAS779", Origin: "EKCH", Runway: stringPtr("22R"), EuroscopeSeenAt: &now, CdmData: stored.Clone()}}, nil
				},
				SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
					stored = data.Clone()
					return 1, nil
				},
			}
			service := newTestSequenceService(repo, milestoneTestSessions(), NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil), nil, nil)
			service.now = func() time.Time { return now }
			exports := 0
			service.SetAfterPersist(func(context.Context, int32, string) { exports++ })
			require.NoError(t, service.RecalculateAirport(context.Background(), 779, "EKCH"))
			assert.Equal(t, "I", valueOrEmpty(stored.Phase))
			assert.Nil(t, stored.Tsat)
			assert.Equal(t, "235100", valueOrEmpty(stored.ExpiredTsat))
			now = now.Add(2 * time.Minute)
			require.NoError(t, service.RecalculateAirport(context.Background(), 779, "EKCH"))
			assert.Equal(t, "2350", valueOrEmpty(stored.Tobt), "exactly eight minutes past does not recover")
			now = now.Add(time.Minute)
			require.NoError(t, service.RecalculateAirport(context.Background(), 779, "EKCH"))
			assert.Equal(t, "0010", valueOrEmpty(stored.Tobt))
			assert.True(t, stored.TobtAutoAdjusted)
			assert.Nil(t, stored.Phase)
			assert.Nil(t, stored.ExpiredTsat)
			assert.Equal(t, "001000", valueOrEmpty(stored.ViffProposalTsat))
			assert.NotEmpty(t, valueOrEmpty(stored.Tsat))
			state, ok := buildViffPushState("SAS779", nil, stored)
			require.True(t, ok)
			assert.Equal(t, "001000", state.Params.Tobt)
			assert.Equal(t, "001000", state.Params.Tsat)
			assert.Equal(t, 2, exports, "recovery reaches the vIFF publishing hook")
			if ctot != "" {
				assert.Equal(t, ctot, valueOrEmpty(stored.Ctot))
				assert.NotEqual(t, valueOrEmpty(stored.Tsat), valueOrEmpty(stored.ViffProposalTsat))
			}
			require.NoError(t, service.RecalculateAirport(context.Background(), 779, "EKCH"))
			assert.Equal(t, 2, exports, "a recovered assignment is not exported repeatedly")
		})
	}
}

func TestPeriodicSequenceRecoversTsatBeforeInvalidation(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	strip := &models.Strip{CdmData: &models.CdmData{Tobt: stringPtr("1140"), Tsat: stringPtr("115100")}}
	recoverExpiredTsat(strip, now)
	assert.Equal(t, "1210", valueOrEmpty(strip.EffectiveTobt()))
	assert.True(t, strip.CdmData.NeedsLocalRecalculation())
	assert.Nil(t, strip.EffectiveTsat())
}

func TestExpiredTsatRecoverySyncsFlightPlanEobtAfterPersistence(t *testing.T) {
	for _, notify := range []bool{false, true} {
		for _, failPersistence := range []bool{false, true} {
			t.Run(fmt.Sprintf("notify=%t/fail_persistence=%t", notify, failPersistence), func(t *testing.T) {
				ctx := context.Background()
				now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
				stored := &models.CdmData{
					Tobt: stringPtr("1140"), Eobt: stringPtr("1230"), Tsat: stringPtr("115100"), Ttot: stringPtr("120100"),
					TobtConfirmedBy: stringPtr(models.TobtConfirmedByPilot), TobtManuallyConfirmed: true,
				}
				hub := &testutil.MockEuroscopeHub{}
				corrections := func() []string {
					var values []string
					for _, message := range hub.Broadcasts {
						if event, ok := message.(euroscopeEvents.EobtEvent); ok {
							assert.Equal(t, "SAS779", event.Callsign)
							values = append(values, event.Eobt)
						}
					}
					return values
				}
				repo := &testutil.MockStripRepository{
					ListByOriginFn: func(context.Context, int32, string) ([]*models.Strip, error) {
						return []*models.Strip{{Callsign: "SAS779", Origin: "EKCH", Runway: stringPtr("22R"), EuroscopeSeenAt: &now, CdmData: stored.Clone()}}, nil
					},
					SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
						assert.Empty(t, corrections(), "flight-plan EOBT must not be sent before persistence succeeds")
						if failPersistence {
							return 0, fmt.Errorf("persistence failed")
						}
						stored = data.Clone()
						return 1, nil
					},
				}
				sequence := newTestSequenceService(repo, milestoneTestSessions(), NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil), nil, hub)
				sequence.now = func() time.Time { return now }
				recalculate := func() error {
					if notify {
						return sequence.RecalculateAirport(ctx, 779, "EKCH")
					}
					return sequence.RecalculateAirportSilently(ctx, 779, "EKCH")
				}
				err := recalculate()
				if failPersistence {
					require.ErrorContains(t, err, "persistence failed")
					assert.Empty(t, corrections())
					assert.Equal(t, "1230", valueOrEmpty(stored.Eobt))
					return
				}
				require.NoError(t, err)
				require.Equal(t, []string{"1210"}, corrections())
				assert.Equal(t, "1210", valueOrEmpty(stored.Tobt))
				assert.Equal(t, "1210", valueOrEmpty(stored.Eobt))
				// EuroScope updates the actual flight plan through EobtEvent;
				// the next strip sync now echoes the recovered EOBT.
				action := &ActionService{service: newTestCdmService(newTestClientWithAirportMasters(nil), repo, milestoneTestSessions(), &testutil.MockControllerRepository{})}
				updated, eobt, clamped := action.PrepareEuroscopeEobtSync(779, stored, corrections()[0], now)
				assert.False(t, clamped)
				assert.Equal(t, "1210", eobt)
				assert.Equal(t, "1210", valueOrEmpty(updated.Tobt))
				stored = updated
				require.NoError(t, recalculate())
				assert.Equal(t, []string{"1210"}, corrections(), "unchanged recovery must not resend the flight-plan command")
			})
		}
	}
}

func TestCtotUpdatesRefreshStartedDepartureTiming(t *testing.T) {
	for _, stage := range []string{"STUP", "PUSH", "TAXI"} {
		for _, update := range []struct{ source, requestedTobt string }{
			{models.CtotSourceATFCM, ""},
			{models.CtotSourceATFCM, "1205"},
			{models.CtotSourceManual, ""},
		} {
			source := update.source
			for _, nextCtot := range []string{"1230", "1215", ""} {
				t.Run(stage+"/"+source+"/ctot="+nextCtot+"/requested_tobt="+update.requestedTobt, func(t *testing.T) {
					ctx := context.Background()
					now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
					stored := &models.CdmData{
						Tobt: stringPtr("1150"), Tsat: stringPtr("115500"), Ttot: stringPtr("120500"),
						Ctot: stringPtr("1225"), CtotSource: &source, Asat: stringPtr("1155"),
					}
					if stage != "STUP" {
						stored.Aobt = stringPtr("1158")
					}
					flight := func() *models.Strip {
						return &models.Strip{Callsign: "SAS779", Origin: "EKCH", State: &stage, Runway: stringPtr("22R"), EuroscopeSeenAt: &now, CdmData: stored.Clone()}
					}
					writes := 0
					repo := &testutil.MockStripRepository{
						ListByOriginFn:          func(context.Context, int32, string) ([]*models.Strip, error) { return []*models.Strip{flight()}, nil },
						GetByCallsignFn:         func(context.Context, int32, string) (*models.Strip, error) { return flight(), nil },
						GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) { return stored.Clone(), nil },
						SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
							stored = data.Clone()
							writes++
							return 1, nil
						},
					}
					sequence := newTestSequenceService(repo, milestoneTestSessions(), NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil), nil, nil)
					sequence.now = func() time.Time { return now }
					publications := 0
					sequence.SetAfterPersist(func(context.Context, int32, string) { publications++ })
					service := newTestCdmService(newTestClientWithAirportMasters(nil), repo, milestoneTestSessions(), &testutil.MockControllerRepository{})
					if source == models.CtotSourceATFCM {
						row := IFPSData{CTOT: nextCtot, CDMData: CDMData{ReqTOBT: update.requestedTobt}}
						_, recalculate, err := service.masterViffSync.mergeMasterViffFlight(ctx, 779, "SAS779", stored.Clone(), row, nextCtot, source)
						require.NoError(t, err)
						require.True(t, recalculate)
					} else if nextCtot == "" {
						require.NoError(t, service.HandleCtotRemove(ctx, 779, "SAS779"))
					} else {
						require.NoError(t, service.HandleManualCtot(ctx, 779, "SAS779", nextCtot))
					}
					require.True(t, stored.CtotRecalculate, "CTOT changes must bypass the startup freeze")
					require.NoError(t, sequence.RecalculateAirport(ctx, 779, "EKCH"))
					assert.Equal(t, nextCtot, valueOrEmpty(stored.Ctot))
					if nextCtot != "" {
						assert.Equal(t, nextCtot+"00", valueOrEmpty(stored.Ttot))
						assert.Equal(t, subtractMinutes(nextCtot+"00", 10), valueOrEmpty(stored.Tsat))
					} else {
						if update.requestedTobt != "" {
							assert.Equal(t, "121500", valueOrEmpty(stored.Ttot))
						} else {
							assert.Equal(t, "120000", valueOrEmpty(stored.Ttot))
						}
					}
					assert.Nil(t, stored.Phase, "an expired TSAT must not invalidate a started departure")
					if update.requestedTobt != "" {
						assert.Equal(t, update.requestedTobt, valueOrEmpty(stored.Tobt))
					} else {
						assert.Equal(t, "1150", valueOrEmpty(stored.Tobt))
					}
					assert.Equal(t, "1155", valueOrEmpty(stored.Asat))
					if stage != "STUP" {
						assert.Equal(t, "1158", valueOrEmpty(stored.Aobt))
					}
					assert.False(t, stored.CtotRecalculate)
					assert.False(t, stored.NeedsLocalRecalculation())
					assert.Equal(t, 1, publications)
					writesBefore := writes
					stored.MarkLocalRecalculationPending()
					require.NoError(t, sequence.RecalculateAirport(ctx, 779, "EKCH"))
					assert.Equal(t, writesBefore, writes, "unrelated updates still respect the startup freeze")
				})
			}
		}
	}
}

func TestPeriodicSequenceRecoversExpiredTsatAfterCtotChange(t *testing.T) {
	for _, initialCtot := range []string{"", "0025"} {
		t.Run("initial_ctot="+initialCtot, func(t *testing.T) {
			ctx := context.Background()
			now := time.Date(2026, 10, 9, 23, 57, 0, 0, time.UTC)
			stored := &models.CdmData{Tobt: stringPtr("2350"), Tsat: stringPtr("235100"), Ttot: stringPtr("000100")}
			if initialCtot != "" {
				stored.Ctot = &initialCtot
				stored.CtotSource = stringPtr(models.CtotSourceATFCM)
			}
			flight := func() *models.Strip {
				return &models.Strip{Callsign: "SAS779", Origin: "EKCH", Runway: stringPtr("22R"), EuroscopeSeenAt: &now, CdmData: stored.Clone()}
			}
			repo := &testutil.MockStripRepository{
				ListByOriginFn:          func(context.Context, int32, string) ([]*models.Strip, error) { return []*models.Strip{flight()}, nil },
				GetByCallsignFn:         func(context.Context, int32, string) (*models.Strip, error) { return flight(), nil },
				GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) { return stored.Clone(), nil },
				SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
					stored = data.Clone()
					return 1, nil
				},
			}
			sequence := newTestSequenceService(repo, milestoneTestSessions(), NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil), nil, nil)
			sequence.now = func() time.Time { return now }
			var proposals []SetCdmDataParams
			sequence.SetAfterPersist(func(context.Context, int32, string) {
				if state, ok := buildViffPushState("SAS779", flight(), stored); ok && !state.Suspend {
					proposals = append(proposals, state.Params)
				}
			})
			require.NoError(t, sequence.RecalculateAirport(ctx, 779, "EKCH"))
			require.Equal(t, "235100", valueOrEmpty(stored.ExpiredTsat))
			require.Nil(t, stored.Tsat)

			// vIFF changes the CTOT seven minutes after the old TSAT.
			now = now.Add(time.Minute)
			service := newTestCdmService(newTestClientWithAirportMasters(nil), repo, milestoneTestSessions(), &testutil.MockControllerRepository{})
			_, recalculate, err := service.masterViffSync.mergeMasterViffFlight(ctx, 779, "SAS779", stored.Clone(), IFPSData{CTOT: "0030"}, "0030", models.CtotSourceATFCM)
			require.NoError(t, err)
			require.True(t, recalculate)
			require.NoError(t, sequence.RecalculateAirport(ctx, 779, "EKCH"))
			require.Nil(t, stored.Tsat, "stale TOBT cannot produce an assignment yet")
			require.Equal(t, "235100", valueOrEmpty(stored.ExpiredTsat), "CTOT-only recalculation must retain the recovery reference")
			require.Equal(t, models.CdmInvalidReasonStaleTsat, valueOrEmpty(stored.Calculation.InvalidReason))
			assert.Empty(t, proposals)

			now = now.Add(time.Minute)
			require.NoError(t, sequence.RecalculateAirport(ctx, 779, "EKCH"))
			assert.Equal(t, "2350", valueOrEmpty(stored.Tobt), "exactly eight minutes past does not recover")
			now = now.Add(time.Minute)
			require.NoError(t, sequence.RecalculateAirport(ctx, 779, "EKCH"))
			assert.Equal(t, "0010", valueOrEmpty(stored.Tobt))
			assert.Equal(t, "0030", valueOrEmpty(stored.Ctot))
			assert.Nil(t, stored.ExpiredTsat)
			require.Len(t, proposals, 1, "recovery publishes a fresh vIFF proposal")
			assert.Equal(t, "001000", proposals[0].Tobt)
			assert.Equal(t, "001000", proposals[0].Tsat)
			assert.NotEqual(t, valueOrEmpty(stored.Tsat), proposals[0].Tsat, "the exported proposal excludes the received CTOT")
		})
	}
}

func TestViffMissingCtotPreservesManualCtotStartupFreeze(t *testing.T) {
	for _, stage := range []string{"STUP", "PUSH", "TAXI"} {
		t.Run(stage, func(t *testing.T) {
			ctx := context.Background()
			now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
			stored := &models.CdmData{
				Tobt: stringPtr("1230"), Tsat: stringPtr("121500"), Ttot: stringPtr("122500"),
				Ctot: stringPtr("1225"), CtotSource: stringPtr(models.CtotSourceManual), Asat: stringPtr("1155"),
			}
			if stage != "STUP" {
				stored.Aobt = stringPtr("1158")
			}
			// An unrelated pending TOBT update must not unfreeze an already
			// started flight just because vIFF does not echo its manual CTOT.
			stored.MarkLocalRecalculationPending()
			flight := func() *models.Strip {
				return &models.Strip{Callsign: "SAS779", Origin: "EKCH", State: &stage, Runway: stringPtr("22R"), EuroscopeSeenAt: &now, CdmData: stored.Clone()}
			}
			writes := 0
			repo := &testutil.MockStripRepository{
				ListByOriginFn:          func(context.Context, int32, string) ([]*models.Strip, error) { return []*models.Strip{flight()}, nil },
				GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) { return stored.Clone(), nil },
				SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
					stored = data.Clone()
					writes++
					return 1, nil
				},
			}
			sequence := newTestSequenceService(repo, milestoneTestSessions(), NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil), nil, nil)
			sequence.now = func() time.Time { return now }
			service := newTestCdmService(newTestClientWithAirportMasters(nil), repo, milestoneTestSessions(), &testutil.MockControllerRepository{})
			for poll := 0; poll < 2; poll++ {
				row := IFPSData{MostPenalizingAirspace: "EGTT"}
				ctot, source := effectiveIfpsCtotAndSource(row, stored)
				_, recalculate, err := service.masterViffSync.mergeMasterViffFlight(ctx, 779, "SAS779", stored.Clone(), row, ctot, source)
				require.NoError(t, err)
				assert.False(t, recalculate, "a missing network CTOT does not change the manual restriction")
				assert.False(t, stored.CtotRecalculate)
				writesBefore := writes
				require.NoError(t, sequence.RecalculateAirport(ctx, 779, "EKCH"))
				assert.Equal(t, writesBefore, writes, "periodic sequencing must retain the startup freeze")
				assert.Equal(t, "1225", valueOrEmpty(stored.Ctot))
				assert.Equal(t, models.CtotSourceManual, valueOrEmpty(stored.CtotSource))
				assert.Equal(t, "121500", valueOrEmpty(stored.Tsat))
				assert.Equal(t, "122500", valueOrEmpty(stored.Ttot))
			}
		})
	}
}

func TestExpiredTsatRecoveryPreservesStartedFlightsAndPendingEdits(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	for _, state := range []string{"STUP", "PUSH", "TAXI", "pending edit", "ASAT", "AOBT"} {
		t.Run(state, func(t *testing.T) {
			data := &models.CdmData{Tobt: stringPtr("1140"), Tsat: stringPtr("115000")}
			strip := &models.Strip{State: &state, CdmData: data}
			switch state {
			case "pending edit":
				data.MarkLocalRecalculationPending()
			case "ASAT":
				data.Asat = stringPtr("1155")
			case "AOBT":
				data.Aobt = stringPtr("1155")
			}
			recoverExpiredTsat(strip, now)
			assert.Equal(t, "1140", valueOrEmpty(strip.EffectiveTobt()))
		})
	}
}

func TestTobtUpdatesSupersedeRetainedExpiredTsat(t *testing.T) {
	for _, automatic := range []bool{false, true} {
		t.Run(fmt.Sprintf("automatic=%t", automatic), func(t *testing.T) {
			data := &models.CdmData{
				Tobt: stringPtr("1140"), ExpiredTsat: stringPtr("115100"), Phase: stringPtr("I"),
				Calculation: &models.CdmCalculation{InvalidReason: stringPtr(models.CdmInvalidReasonStaleTsat)},
			}
			if automatic {
				applyAutoSyncedTobtUpdate(data, "1145")
			} else {
				applyConfirmedTobtUpdate(data, "1145", "EKCH_DEL", "ATC")
				applyTobtRecalculationPolicy(data, "1145")
			}
			assert.Nil(t, data.ExpiredTsat)
			data.ClearLocalRecalculationPending()
			strip := &models.Strip{CdmData: data}
			recoverExpiredTsat(strip, time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC))
			assert.Equal(t, "1145", valueOrEmpty(strip.EffectiveTobt()), "the old TSAT must not override a replacement TOBT")
		})
	}
}

func TestReadyRequestUsesRetainedExpiredTsat(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	data := &models.CdmData{
		Tobt: stringPtr("1140"), ExpiredTsat: stringPtr("115100"), Phase: stringPtr("I"),
		Calculation: &models.CdmCalculation{InvalidReason: stringPtr(models.CdmInvalidReasonStaleTsat)},
	}
	tobt, changed := readyRequestTobt(data, now)
	assert.True(t, changed)
	assert.Equal(t, "1210", tobt)
}
