package euroscope

import (
	"context"
	"testing"
	"time"

	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	esEvents "FlightStrips/pkg/events/euroscope"
	frontendEvents "FlightStrips/pkg/events/frontend"
	pkgModels "FlightStrips/pkg/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type syncRuntimeEvaluateCall struct {
	Session  int32
	CID      string
	Callsign string
	Current  pkgModels.ActiveRunways
	Master   pkgModels.ActiveRunways
	IsMaster bool
}

type syncRuntimeResyncCall struct {
	Session   int32
	MasterCID string
	Master    pkgModels.ActiveRunways
}

type syncRuntimeSendCall struct {
	Session int32
	CID     string
	Message esEvents.OutgoingMessage
}

type syncRuntimeScheduleOfflineCall struct {
	Session      int32
	Callsign     string
	PositionFreq string
	PositionName string
	Delay        time.Duration
}

type syncRuntimeScheduleDisconnectCall struct {
	Session  int32
	Callsign string
	Delay    time.Duration
}

type syncRuntimeSpy struct {
	evaluateResult          runwayClientEvaluation
	evaluateCalls           []syncRuntimeEvaluateCall
	resyncCalls             []syncRuntimeResyncCall
	sendCalls               []syncRuntimeSendCall
	scheduleOfflineCalls    []syncRuntimeScheduleOfflineCall
	scheduleDisconnectCalls []syncRuntimeScheduleDisconnectCall
	hasMaster               bool
	isMaster                bool
}

func (s *syncRuntimeSpy) CancelOfflineTimer(_ int32, _ string) {}

func (s *syncRuntimeSpy) CancelAircraftDisconnect(_ int32, _ string) {}

func (s *syncRuntimeSpy) EvaluateClientRunwayState(session int32, cid, callsign string, current, master pkgModels.ActiveRunways, isMaster bool) runwayClientEvaluation {
	s.evaluateCalls = append(s.evaluateCalls, syncRuntimeEvaluateCall{
		Session:  session,
		CID:      cid,
		Callsign: callsign,
		Current:  current,
		Master:   master,
		IsMaster: isMaster,
	})
	return s.evaluateResult
}

func (s *syncRuntimeSpy) ResyncSessionRunwayMismatchTargets(session int32, masterCID string, master pkgModels.ActiveRunways) {
	s.resyncCalls = append(s.resyncCalls, syncRuntimeResyncCall{
		Session:   session,
		MasterCID: masterCID,
		Master:    master,
	})
}

func (s *syncRuntimeSpy) CurrentMasterStatus(_ int32, _, _ string) (bool, bool) {
	return s.hasMaster, s.isMaster
}

func (s *syncRuntimeSpy) Send(session int32, cid string, message esEvents.OutgoingMessage) {
	s.sendCalls = append(s.sendCalls, syncRuntimeSendCall{
		Session: session,
		CID:     cid,
		Message: message,
	})
}

func (s *syncRuntimeSpy) ScheduleOfflineActions(session int32, callsign, positionFreq, positionName string, delay time.Duration) {
	s.scheduleOfflineCalls = append(s.scheduleOfflineCalls, syncRuntimeScheduleOfflineCall{
		Session:      session,
		Callsign:     callsign,
		PositionFreq: positionFreq,
		PositionName: positionName,
		Delay:        delay,
	})
}

func (s *syncRuntimeSpy) ScheduleAircraftDisconnect(session int32, callsign string, delay time.Duration) {
	s.scheduleDisconnectCalls = append(s.scheduleDisconnectCalls, syncRuntimeScheduleDisconnectCall{
		Session:  session,
		Callsign: callsign,
		Delay:    delay,
	})
}

type syncControllerServiceSpy struct {
	upsertFn func(ctx context.Context, session int32, callsign, position string) error
}

func (s *syncControllerServiceSpy) ControllerOnline(_ context.Context, _ int32, _, _, _ string) (shared.ControllerOnlineResult, error) {
	panic("unexpected call to syncControllerServiceSpy.ControllerOnline")
}

func (s *syncControllerServiceSpy) ControllerOnlineWithOptions(_ context.Context, _ int32, _, _, _ string, _ shared.ControllerOnlineOptions) (shared.ControllerOnlineResult, error) {
	panic("unexpected call to syncControllerServiceSpy.ControllerOnlineWithOptions")
}

func (s *syncControllerServiceSpy) ControllerOffline(_ context.Context, _ int32, _ string) (shared.ControllerOfflineResult, error) {
	panic("unexpected call to syncControllerServiceSpy.ControllerOffline")
}

func (s *syncControllerServiceSpy) UpsertController(ctx context.Context, session int32, callsign, position string) error {
	if s.upsertFn != nil {
		return s.upsertFn(ctx, session, callsign, position)
	}
	return nil
}

type syncStripServiceSpy struct {
	noOpStripService
	syncStripFn            func(ctx context.Context, session int32, cid string, strip interface{}, airport string) error
	autoAssumePositions    []string
	propagateRunwayCalls   []pkgModels.ActiveRunways
	moveToBayCalls         []string
	pdcValidationCallsigns []string
	squawkValidationCalls  int
	landingValidationCalls int
}

func (s *syncStripServiceSpy) SyncStrip(ctx context.Context, session int32, cid string, strip interface{}, airport string) error {
	if s.syncStripFn != nil {
		return s.syncStripFn(ctx, session, cid, strip, airport)
	}
	return nil
}

func (s *syncStripServiceSpy) AutoAssumeForControllerOnline(_ context.Context, _ int32, controllerPosition string) error {
	s.autoAssumePositions = append(s.autoAssumePositions, controllerPosition)
	return nil
}

func (s *syncStripServiceSpy) PropagateRunwayChange(_ context.Context, _ int32, _ string, oldRunways pkgModels.ActiveRunways, newRunways pkgModels.ActiveRunways) error {
	s.propagateRunwayCalls = append(s.propagateRunwayCalls, oldRunways, newRunways)
	return nil
}

func (s *syncStripServiceSpy) MoveToBay(_ context.Context, _ int32, callsign string, _ string, _ bool) error {
	s.moveToBayCalls = append(s.moveToBayCalls, callsign)
	return nil
}

func (s *syncStripServiceSpy) ReevaluatePdcRequestValidationsForStrip(_ context.Context, _ int32, strip *internalModels.Strip, _ []string, _ bool, _ bool) error {
	if strip != nil {
		s.pdcValidationCallsigns = append(s.pdcValidationCallsigns, strip.Callsign)
	}
	return nil
}

func (s *syncStripServiceSpy) ReevaluateSquawkValidationsForSession(_ context.Context, _ int32, _ bool) error {
	s.squawkValidationCalls++
	return nil
}

func (s *syncStripServiceSpy) ReevaluateLandingClearanceValidationsForSession(_ context.Context, _ int32, _ bool, _ bool) error {
	s.landingValidationCalls++
	return nil
}

func TestEuroscopeSyncServiceApplySync_MasterRunwaysRecalculateSession(t *testing.T) {
	session := int32(1)
	sessionModel := &internalModels.Session{
		ID:      session,
		Name:    "LIVE",
		Airport: "EKCH",
		ActiveRunways: pkgModels.ActiveRunways{
			DepartureRunways: []string{"04L"},
			ArrivalRunways:   []string{"22R"},
			RunwayStatus:     map[string]string{"04L": "OPEN"},
		},
	}

	frontendHub := &testutil.MockFrontendHub{}
	runtime := &syncRuntimeSpy{hasMaster: true, isMaster: true}
	stripService := &syncStripServiceSpy{}
	updateActiveRunwaysCalls := 0
	recalculateCalls := 0

	server := &testutil.MockServer{
		FrontendHubVal: frontendHub,
		ControllerRepoVal: &testutil.MockControllerRepository{
			ListFn: func(_ context.Context, gotSession int32) ([]*internalModels.Controller, error) {
				assert.Equal(t, session, gotSession)
				return nil, nil
			},
		},
		StripRepoVal: &testutil.MockStripRepository{
			ListFn: func(_ context.Context, gotSession int32) ([]*internalModels.Strip, error) {
				assert.Equal(t, session, gotSession)
				return nil, nil
			},
		},
		SessionRepoVal: &testutil.MockSessionRepository{
			GetByIDFn: func(_ context.Context, gotSession int32) (*internalModels.Session, error) {
				assert.Equal(t, session, gotSession)
				return sessionModel, nil
			},
			UpdateActiveRunwaysFn: func(_ context.Context, gotSession int32, activeRunways pkgModels.ActiveRunways) error {
				assert.Equal(t, session, gotSession)
				updateActiveRunwaysCalls++
				sessionModel.ActiveRunways = activeRunways
				return nil
			},
		},
		RecalculateSessionContextFn: func(_ context.Context, gotSession int32, sendUpdate bool) ([]shared.SectorChange, error) {
			assert.Equal(t, session, gotSession)
			assert.True(t, sendUpdate)
			recalculateCalls++
			return nil, nil
		},
	}

	service := newEuroscopeSyncService(server, &syncControllerServiceSpy{}, stripService, runtime)
	result, err := service.ApplySync(context.Background(), EuroscopeSyncRequest{
		Session:     session,
		SessionName: "LIVE",
		Airport:     "EKCH",
		CID:         "cid-master",
		Callsign:    "EKCH_TWR",
		IsMaster:    true,
		HasMaster:   true,
		Event: esEvents.SyncEvent{
			Runways: []esEvents.SyncRunway{
				{Name: "22L", Departure: true},
				{Name: "04R", Arrival: true},
			},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, 1, updateActiveRunwaysCalls)
	assert.Equal(t, 1, recalculateCalls)
	require.Len(t, runtime.evaluateCalls, 1)
	assert.True(t, runtime.evaluateCalls[0].IsMaster)
	require.Len(t, runtime.resyncCalls, 1)
	assert.Equal(t, "cid-master", runtime.resyncCalls[0].MasterCID)
	assert.Len(t, stripService.propagateRunwayCalls, 2)
	assert.True(t, result.MarkSessionSynced)
	assert.Equal(t, "cid-master", result.WakeFrontendCID)
	assert.Equal(t, 4, result.Metrics.DBOperations)
}

func TestEuroscopeSyncServiceApplySync_SlaveRunwaysDoNotPersist(t *testing.T) {
	session := int32(1)
	sessionModel := &internalModels.Session{
		ID:      session,
		Name:    "LIVE",
		Airport: "EKCH",
		ActiveRunways: pkgModels.ActiveRunways{
			DepartureRunways: []string{"04L"},
			ArrivalRunways:   []string{"22R"},
		},
	}

	frontendHub := &testutil.MockFrontendHub{}
	runtime := &syncRuntimeSpy{
		hasMaster: true,
		isMaster:  false,
		evaluateResult: runwayClientEvaluation{
			CID:               "cid-slave",
			DepartureMismatch: true,
			ArrivalMismatch:   true,
			Changed:           true,
			Alert: &esEvents.RunwayMismatchAlertEvent{
				ExpectedDeparture: []string{"04L"},
				ExpectedArrival:   []string{"22R"},
				CurrentDeparture:  []string{"22L"},
				CurrentArrival:    []string{"04R"},
			},
		},
	}
	updateActiveRunwaysCalls := 0
	recalculateCalls := 0

	server := &testutil.MockServer{
		FrontendHubVal: frontendHub,
		ControllerRepoVal: &testutil.MockControllerRepository{
			ListFn: func(_ context.Context, _ int32) ([]*internalModels.Controller, error) { return nil, nil },
		},
		StripRepoVal: &testutil.MockStripRepository{
			ListFn: func(_ context.Context, _ int32) ([]*internalModels.Strip, error) { return nil, nil },
		},
		SessionRepoVal: &testutil.MockSessionRepository{
			GetByIDFn: func(_ context.Context, _ int32) (*internalModels.Session, error) { return sessionModel, nil },
			UpdateActiveRunwaysFn: func(_ context.Context, _ int32, _ pkgModels.ActiveRunways) error {
				updateActiveRunwaysCalls++
				return nil
			},
		},
		RecalculateSessionContextFn: func(_ context.Context, _ int32, _ bool) ([]shared.SectorChange, error) {
			recalculateCalls++
			return nil, nil
		},
	}

	service := newEuroscopeSyncService(server, &syncControllerServiceSpy{}, &syncStripServiceSpy{}, runtime)
	result, err := service.ApplySync(context.Background(), EuroscopeSyncRequest{
		Session:     session,
		SessionName: "LIVE",
		Airport:     "EKCH",
		CID:         "cid-slave",
		Callsign:    "EKCH_APP",
		IsMaster:    false,
		HasMaster:   true,
		Event: esEvents.SyncEvent{
			Runways: []esEvents.SyncRunway{
				{Name: "22L", Departure: true},
				{Name: "04R", Arrival: true},
			},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, 0, updateActiveRunwaysCalls)
	assert.Equal(t, 0, recalculateCalls)
	require.Len(t, runtime.evaluateCalls, 1)
	assert.False(t, runtime.evaluateCalls[0].IsMaster)
	require.Len(t, frontendHub.SentMessages, 1)
	_, ok := frontendHub.SentMessages[0].Message.(frontendEvents.RunwayConfigurationEvent)
	assert.True(t, ok)
	require.Len(t, runtime.sendCalls, 1)
	_, ok = runtime.sendCalls[0].Message.(esEvents.RunwayMismatchAlertEvent)
	assert.True(t, ok)
	assert.True(t, result.MarkSessionSynced)
	assert.Equal(t, "cid-slave", result.WakeFrontendCID)
}

func TestEuroscopeSyncServiceApplySync_ChangedControllersTriggerOrchestration(t *testing.T) {
	session := int32(1)
	sessionModel := &internalModels.Session{
		ID:      session,
		Name:    "LIVE",
		Airport: "EKCH",
	}

	updateSectorsCalls := 0
	updateLayoutsCalls := 0
	stripService := &syncStripServiceSpy{}

	server := &testutil.MockServer{
		FrontendHubVal: &testutil.MockFrontendHub{},
		ControllerRepoVal: &testutil.MockControllerRepository{
			ListFn: func(_ context.Context, _ int32) ([]*internalModels.Controller, error) {
				return []*internalModels.Controller{
					{Callsign: "EKCH_A_GND", Position: "121.900"},
				}, nil
			},
		},
		StripRepoVal: &testutil.MockStripRepository{
			ListFn: func(_ context.Context, _ int32) ([]*internalModels.Strip, error) { return nil, nil },
		},
		SessionRepoVal: &testutil.MockSessionRepository{
			GetByIDFn: func(_ context.Context, _ int32) (*internalModels.Session, error) { return sessionModel, nil },
		},
		UpdateSectorsFn: func(_ int32) ([]shared.SectorChange, error) {
			updateSectorsCalls++
			return nil, nil
		},
		UpdateLayoutsFn: func(_ int32) error {
			updateLayoutsCalls++
			return nil
		},
	}

	controllerService := &syncControllerServiceSpy{
		upsertFn: func(ctx context.Context, _ int32, callsign, position string) error {
			syncState := shared.GetSyncState(ctx)
			require.NotNil(t, syncState)
			syncState.ChangedControllers++
			syncState.ExistingControllers[callsign] = &internalModels.Controller{
				Callsign: callsign,
				Position: position,
			}
			return nil
		},
	}

	service := newEuroscopeSyncService(server, controllerService, stripService, &syncRuntimeSpy{})
	result, err := service.ApplySync(context.Background(), EuroscopeSyncRequest{
		Session:     session,
		SessionName: "LIVE",
		Airport:     "EKCH",
		CID:         "cid-master",
		Callsign:    "EKCH_TWR",
		Position:    "118.700",
		Event: esEvents.SyncEvent{
			Controllers: []struct {
				Position string `json:"position"`
				Callsign string `json:"callsign"`
			}{
				{Callsign: "EKCH_A_GND", Position: "121.800"},
			},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, 1, updateSectorsCalls)
	assert.Equal(t, 1, updateLayoutsCalls)
	assert.Equal(t, 1, result.Metrics.ChangedControllers)
	assert.ElementsMatch(t, []string{"121.800", "121.900", "118.700"}, stripService.autoAssumePositions)
}

func TestEuroscopeSyncServiceApplySync_FinalizesChangedStrips(t *testing.T) {
	session := int32(1)
	sequence := int32(10)
	existingStrip := &internalModels.Strip{
		Callsign: "SAS123",
		Bay:      "STAND",
		Sequence: &sequence,
	}
	sessionModel := &internalModels.Session{
		ID:      session,
		Name:    "LIVE",
		Airport: "EKCH",
		ActiveRunways: pkgModels.ActiveRunways{
			DepartureRunways: []string{"04L"},
		},
	}

	frontendHub := &testutil.MockFrontendHub{}
	stripService := &syncStripServiceSpy{
		syncStripFn: func(ctx context.Context, _ int32, _ string, strip interface{}, _ string) error {
			syncState := shared.GetSyncState(ctx)
			require.NotNil(t, syncState)
			esStrip, ok := strip.(esEvents.Strip)
			require.True(t, ok)
			syncState.ChangedStrips++
			syncState.MarkRouteRecalc(esStrip.Callsign)
			syncState.MarkBayUpdate(esStrip.Callsign, "PUSH")
			syncState.MarkPdcValidation(esStrip.Callsign)
			syncState.MarkStripUpdate(esStrip.Callsign)
			syncState.SquawkValidation = true
			syncState.LandingValidation = true
			return nil
		},
	}
	routeUpdates := []string{}

	server := &testutil.MockServer{
		FrontendHubVal: frontendHub,
		ControllerRepoVal: &testutil.MockControllerRepository{
			ListFn: func(_ context.Context, _ int32) ([]*internalModels.Controller, error) { return nil, nil },
		},
		StripRepoVal: &testutil.MockStripRepository{
			ListFn: func(_ context.Context, _ int32) ([]*internalModels.Strip, error) {
				return []*internalModels.Strip{existingStrip}, nil
			},
		},
		SessionRepoVal: &testutil.MockSessionRepository{
			GetByIDFn: func(_ context.Context, _ int32) (*internalModels.Session, error) { return sessionModel, nil },
		},
		UpdateRouteForStripCtxFn: func(_ context.Context, callsign string, _ int32, _ bool) error {
			routeUpdates = append(routeUpdates, callsign)
			return nil
		},
	}

	service := newEuroscopeSyncService(server, &syncControllerServiceSpy{}, stripService, &syncRuntimeSpy{})
	result, err := service.ApplySync(context.Background(), EuroscopeSyncRequest{
		Session:     session,
		SessionName: "LIVE",
		Airport:     "EKCH",
		CID:         "cid-master",
		Callsign:    "EKCH_TWR",
		Position:    "118.700",
		Event: esEvents.SyncEvent{
			Strips: []esEvents.Strip{
				{Callsign: "SAS123"},
			},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, 1, result.Metrics.ChangedStrips)
	assert.Equal(t, []string{"SAS123"}, routeUpdates)
	assert.Equal(t, []string{"SAS123"}, stripService.moveToBayCalls)
	assert.Equal(t, []string{"SAS123"}, stripService.pdcValidationCallsigns)
	assert.Equal(t, 1, stripService.squawkValidationCalls)
	assert.Equal(t, 1, stripService.landingValidationCalls)
	require.Len(t, frontendHub.StripUpdates, 1)
	assert.Equal(t, "SAS123", frontendHub.StripUpdates[0].Callsign)
	assert.Equal(t, []string{"118.700"}, stripService.autoAssumePositions)
}

func TestEuroscopeSyncServiceApplySync_DynamicMasterStatusOverridesStaleRequestSnapshot(t *testing.T) {
	session := int32(1)
	sessionModel := &internalModels.Session{
		ID:      session,
		Name:    "LIVE",
		Airport: "EKCH",
		ActiveRunways: pkgModels.ActiveRunways{
			DepartureRunways: []string{"04L"},
			ArrivalRunways:   []string{"22R"},
		},
	}

	updateActiveRunwaysCalls := 0
	recalculateCalls := 0
	runtime := &syncRuntimeSpy{
		hasMaster: true,
		isMaster:  false,
	}

	server := &testutil.MockServer{
		FrontendHubVal: &testutil.MockFrontendHub{},
		ControllerRepoVal: &testutil.MockControllerRepository{
			ListFn: func(_ context.Context, _ int32) ([]*internalModels.Controller, error) { return nil, nil },
		},
		StripRepoVal: &testutil.MockStripRepository{
			ListFn: func(_ context.Context, _ int32) ([]*internalModels.Strip, error) { return nil, nil },
		},
		SessionRepoVal: &testutil.MockSessionRepository{
			GetByIDFn: func(_ context.Context, _ int32) (*internalModels.Session, error) { return sessionModel, nil },
			UpdateActiveRunwaysFn: func(_ context.Context, _ int32, _ pkgModels.ActiveRunways) error {
				updateActiveRunwaysCalls++
				return nil
			},
		},
		RecalculateSessionContextFn: func(_ context.Context, _ int32, _ bool) ([]shared.SectorChange, error) {
			recalculateCalls++
			return nil, nil
		},
	}

	service := newEuroscopeSyncService(server, &syncControllerServiceSpy{}, &syncStripServiceSpy{}, runtime)
	_, err := service.ApplySync(context.Background(), EuroscopeSyncRequest{
		Session:     session,
		SessionName: "LIVE",
		Airport:     "EKCH",
		CID:         "cid-old-master",
		Callsign:    "EKCH_TWR",
		IsMaster:    true,
		HasMaster:   true,
		Event: esEvents.SyncEvent{
			Runways: []esEvents.SyncRunway{
				{Name: "22L", Departure: true},
			},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, 0, updateActiveRunwaysCalls)
	assert.Equal(t, 0, recalculateCalls)
}
