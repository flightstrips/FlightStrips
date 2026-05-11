package frontend

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/services"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	frontendEvents "FlightStrips/pkg/events/frontend"
	pkgModels "FlightStrips/pkg/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stripUpdateValidationReevaluator struct {
	testutil.NoOpStripService
	reevaluateForStripFn  func(ctx context.Context, session int32, strip *models.Strip, activeDepartureRunways []string, publish bool, forceReactivate bool) error
	reevaluateDepartureFn func(ctx context.Context, session int32, callsign string, publish bool, forceReactivate bool) error
}

type recordingCdmService struct {
	triggerRecalculateFn             func(ctx context.Context, session int32, airport string)
	syncAirportLvoFromRunwayStatusFn func(ctx context.Context, airport string, runwayStatus map[string]string)
	handleEobtUpdateFn               func(ctx context.Context, session int32, callsign string, eobt string, sourcePosition string, sourceRole string) error
}

func (s *recordingCdmService) TriggerRecalculate(ctx context.Context, session int32, airport string) {
	if s.triggerRecalculateFn != nil {
		s.triggerRecalculateFn(ctx, session, airport)
	}
}

func (s *recordingCdmService) SyncAirportLvoFromRunwayStatus(ctx context.Context, airport string, runwayStatus map[string]string) {
	if s.syncAirportLvoFromRunwayStatusFn != nil {
		s.syncAirportLvoFromRunwayStatusFn(ctx, airport, runwayStatus)
	}
}

func (s *recordingCdmService) HandleReadyRequest(context.Context, int32, string, string, string) error {
	return nil
}

func (s *recordingCdmService) HandleEobtUpdate(ctx context.Context, session int32, callsign string, eobt string, sourcePosition string, sourceRole string) error {
	if s.handleEobtUpdateFn != nil {
		return s.handleEobtUpdateFn(ctx, session, callsign, eobt, sourcePosition, sourceRole)
	}
	return nil
}

func (s *recordingCdmService) HandleTobtUpdate(context.Context, int32, string, string, string, string) error {
	return nil
}

func (s *recordingCdmService) HandleClxTobtUpdate(context.Context, int32, string, string, string, string) error {
	return nil
}

func (s *recordingCdmService) HandleDeiceUpdate(context.Context, int32, string, string) error {
	return nil
}

func (s *recordingCdmService) HandleAsrtToggle(context.Context, int32, string, string) error {
	return nil
}

func (s *recordingCdmService) HandleTsacUpdate(context.Context, int32, string, string) error {
	return nil
}

func (s *recordingCdmService) HandleManualCtot(context.Context, int32, string, string) error {
	return nil
}

func (s *recordingCdmService) HandleCtotRemove(context.Context, int32, string) error {
	return nil
}

func (s *recordingCdmService) HandleApproveReqTobt(context.Context, int32, string, string, string) error {
	return nil
}

func (s *recordingCdmService) SyncAsatForGroundState(context.Context, int32, string, string) error {
	return nil
}

func (s *recordingCdmService) RequestBetterTobt(context.Context, int32, string) error {
	return nil
}

func (s *recordingCdmService) SetSessionCdmMaster(context.Context, int32, bool) error {
	return nil
}

func (s *stripUpdateValidationReevaluator) ReevaluatePdcInvalidValidationForStrip(ctx context.Context, session int32, strip *models.Strip, activeDepartureRunways []string, publish bool, forceReactivate bool) error {
	if s.reevaluateForStripFn == nil {
		return nil
	}
	return s.reevaluateForStripFn(ctx, session, strip, activeDepartureRunways, publish, forceReactivate)
}

func (s *stripUpdateValidationReevaluator) ReevaluateDepartureValidation(ctx context.Context, session int32, callsign string, publish bool, forceReactivate bool) error {
	if s.reevaluateDepartureFn == nil {
		return nil
	}
	return s.reevaluateDepartureFn(ctx, session, callsign, publish, forceReactivate)
}

func TestHandleStripUpdate_RunwayChangePersistsSelectedRunway(t *testing.T) {
	ctx := context.Background()
	const session = int32(7)
	const callsign = "SAS123"
	currentRunway := "22L"
	selectedRunway := "04R"

	var updatedRunway *string
	var markedField string

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, gotSession int32, gotCallsign string) (*models.Strip, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			return &models.Strip{
				Callsign: callsign,
				Session:  session,
				Runway:   &currentRunway,
			}, nil
		},
		UpdateRunwayFn: func(_ context.Context, gotSession int32, gotCallsign string, runway *string, version *int32) (int64, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			assert.Nil(t, version)
			updatedRunway = runway
			return 1, nil
		},
		AppendControllerModifiedFieldFn: func(_ context.Context, gotSession int32, gotCallsign string, field string) error {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			markedField = field
			return nil
		},
	}

	euroscopeHub := &testutil.MockEuroscopeHub{}
	server := &testutil.MockServer{
		StripRepoVal:    stripRepo,
		EuroscopeHubVal: euroscopeHub,
	}

	hub := &Hub{server: server, send: make(chan internalMessage, 1)}
	client := &Client{
		session:  session,
		hub:      hub,
		position: "EKCH_DEL",
	}
	client.SetUser(shared.NewAuthenticatedUser("123456", 0, nil))

	payload, err := json.Marshal(frontendEvents.UpdateStripDataEvent{
		Type:     frontendEvents.UpdateStripData,
		Callsign: callsign,
		Runway:   &selectedRunway,
	})
	require.NoError(t, err)

	err = handleStripUpdate(ctx, client, Message{
		Type:    frontendEvents.UpdateStripData,
		Message: payload,
	})
	require.NoError(t, err)

	require.NotNil(t, updatedRunway)
	assert.Equal(t, selectedRunway, *updatedRunway)
	assert.Equal(t, "runway", markedField)
}

func TestRoundedClxTobtAddsFifteenMinutesAndRoundsUpToFive(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		want string
	}{
		{
			name: "already on five minute boundary after offset",
			now:  time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
			want: "1015",
		},
		{
			name: "rounds up after offset",
			now:  time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC),
			want: "1020",
		},
		{
			name: "rolls over midnight",
			now:  time.Date(2026, 1, 1, 23, 48, 0, 0, time.UTC),
			want: "0005",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, roundedClxTobt(tt.now))
		})
	}
}

func TestHandleStripUpdate_EobtChangeTriggersCdmRecalculation(t *testing.T) {
	ctx := context.Background()
	const session = int32(7)
	const callsign = "SAS123"
	currentEobt := "1000"
	updatedEobt := "1015"
	tobt := "1020"
	tsat := "1030"
	ctot := "1040"

	var handledEobt string
	getByCallsignCalls := 0

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, gotSession int32, gotCallsign string) (*models.Strip, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			getByCallsignCalls++
			return &models.Strip{
				Callsign: callsign,
				Session:  session,
				Origin:   "EKCH",
				CdmData: &models.CdmData{
					Eobt: &currentEobt,
					Tobt: &tobt,
					Tsat: &tsat,
					Ctot: &ctot,
				},
			}, nil
		},
	}

	cdmService := &recordingCdmService{
		handleEobtUpdateFn: func(_ context.Context, gotSession int32, gotCallsign string, eobt string, sourcePosition string, sourceRole string) error {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			assert.Equal(t, "EKCH_DEL", sourcePosition)
			assert.Equal(t, "ATC", sourceRole)
			handledEobt = eobt
			return nil
		},
	}
	euroscopeHub := &testutil.MockEuroscopeHub{}

	server := &testutil.MockServer{
		StripRepoVal:    stripRepo,
		CdmServiceVal:   cdmService,
		EuroscopeHubVal: euroscopeHub,
	}
	hub := &Hub{
		server: server,
		send:   make(chan internalMessage, 2),
	}
	client := &Client{
		session:  session,
		hub:      hub,
		position: "EKCH_DEL",
	}
	client.SetUser(shared.NewAuthenticatedUser("123456", 0, nil))

	payload, err := json.Marshal(frontendEvents.UpdateStripDataEvent{
		Type:     frontendEvents.UpdateStripData,
		Callsign: callsign,
		Eobt:     &updatedEobt,
	})
	require.NoError(t, err)

	err = handleStripUpdate(ctx, client, Message{
		Type:    frontendEvents.UpdateStripData,
		Message: payload,
	})
	require.NoError(t, err)

	assert.Equal(t, updatedEobt, handledEobt)
	require.Len(t, euroscopeHub.Eobts, 1)
	assert.Equal(t, session, euroscopeHub.Eobts[0].Session)
	assert.Equal(t, "123456", euroscopeHub.Eobts[0].Cid)
	assert.Equal(t, callsign, euroscopeHub.Eobts[0].Callsign)
	assert.Equal(t, updatedEobt, euroscopeHub.Eobts[0].Eobt)
	assert.Equal(t, 2, getByCallsignCalls)
}

func TestHandleStripUpdate_OwnerCanUpdateRemarksAndAircraftInfo(t *testing.T) {
	ctx := context.Background()
	const session = int32(7)
	const callsign = "SAS123"
	const owner = "EKCH_DEL"
	currentRemarks := "REG/OYABC PBN/A1"
	currentAircraftInfo := "B738/M-SDE2FGHIWY/LB1"
	updatedRemarks := "REG/OYABC PBN/A1B1C1D1S1S2"
	updatedAircraftInfo := "B738/M-SDE2FGHIWYR/LB1"

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, gotSession int32, gotCallsign string) (*models.Strip, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			return &models.Strip{
				Callsign:     callsign,
				Session:      session,
				Owner:        ptr(owner),
				Remarks:      &currentRemarks,
				AircraftType: &currentAircraftInfo,
			}, nil
		},
	}

	euroscopeHub := &testutil.MockEuroscopeHub{}
	server := &testutil.MockServer{
		StripRepoVal:    stripRepo,
		EuroscopeHubVal: euroscopeHub,
	}

	hub := &Hub{server: server, send: make(chan internalMessage, 1)}
	client := &Client{
		session:  session,
		hub:      hub,
		position: owner,
	}
	client.SetUser(shared.NewAuthenticatedUser("123456", 0, nil))

	payload, err := json.Marshal(frontendEvents.UpdateStripDataEvent{
		Type:     frontendEvents.UpdateStripData,
		Callsign: callsign,
		Remarks:  &updatedRemarks,
		Aircraft: &updatedAircraftInfo,
	})
	require.NoError(t, err)

	err = handleStripUpdate(ctx, client, Message{
		Type:    frontendEvents.UpdateStripData,
		Message: payload,
	})
	require.NoError(t, err)

	assert.Empty(t, euroscopeHub.RemarksUpdates)
	assert.Empty(t, euroscopeHub.AircraftInfoUpdates)
	require.Len(t, euroscopeHub.AircraftInfoRemarks, 1)
	assert.Equal(t, updatedRemarks, euroscopeHub.AircraftInfoRemarks[0].Remarks)
	assert.Equal(t, updatedAircraftInfo, euroscopeHub.AircraftInfoRemarks[0].AircraftType)
	assert.Equal(t, []string{"aircraft_info_remarks"}, euroscopeHub.FlightPlanUpdateOrder)
}

func TestHandleStripUpdate_NonOwnerCannotUpdateRemarksOrAircraftInfo(t *testing.T) {
	ctx := context.Background()
	const session = int32(7)
	const callsign = "SAS123"
	owner := "EKCH_TWR"
	updatedRemarks := "PBN/A1"
	updatedAircraftInfo := "B738/M-SR"

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign: callsign,
				Session:  session,
				Owner:    &owner,
			}, nil
		},
	}

	server := &testutil.MockServer{
		StripRepoVal:    stripRepo,
		EuroscopeHubVal: &testutil.MockEuroscopeHub{},
	}
	hub := &Hub{server: server}
	client := &Client{
		session:  session,
		hub:      hub,
		position: "EKCH_DEL",
	}
	client.SetUser(shared.NewAuthenticatedUser("123456", 0, nil))

	payload, err := json.Marshal(frontendEvents.UpdateStripDataEvent{
		Type:     frontendEvents.UpdateStripData,
		Callsign: callsign,
		Remarks:  &updatedRemarks,
		Aircraft: &updatedAircraftInfo,
	})
	require.NoError(t, err)

	err = handleStripUpdate(ctx, client, Message{
		Type:    frontendEvents.UpdateStripData,
		Message: payload,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-owner")
}

func TestHandleUpdateRunwayStatus_SynchronizesAirportLvo(t *testing.T) {
	ctx := context.Background()
	const sessionID = int32(12)

	session := &models.Session{
		ID:      sessionID,
		Airport: "EKCH",
		ActiveRunways: pkgModels.ActiveRunways{
			DepartureRunways: []string{"04L"},
			ArrivalRunways:   []string{"22L"},
			RunwayStatus:     map[string]string{"04L/22L": "OPEN"},
		},
	}

	var persisted pkgModels.ActiveRunways
	var syncedAirport string
	var syncedStatus map[string]string
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
			assert.Equal(t, sessionID, id)
			return session, nil
		},
		UpdateActiveRunwaysFn: func(_ context.Context, id int32, activeRunways pkgModels.ActiveRunways) error {
			assert.Equal(t, sessionID, id)
			persisted = activeRunways
			return nil
		},
	}
	cdmService := &recordingCdmService{
		syncAirportLvoFromRunwayStatusFn: func(_ context.Context, airport string, runwayStatus map[string]string) {
			syncedAirport = airport
			syncedStatus = map[string]string{}
			for pair, status := range runwayStatus {
				syncedStatus[pair] = status
			}
		},
	}
	server := &testutil.MockServer{
		CdmServiceVal:  cdmService,
		SessionRepoVal: sessionRepo,
	}
	hub := &Hub{server: server, send: make(chan internalMessage, 1)}
	client := &Client{session: sessionID, hub: hub}

	payload, err := json.Marshal(frontendEvents.UpdateRunwayStatusAction{
		Pair:   "04L/22L",
		Status: "LOW_VIS",
	})
	require.NoError(t, err)

	err = handleUpdateRunwayStatus(ctx, client, Message{
		Type:    frontendEvents.UpdateRunwayStatus,
		Message: payload,
	})
	require.NoError(t, err)
	assert.Equal(t, "LOW_VIS", persisted.RunwayStatus["04L/22L"])
	assert.Equal(t, "EKCH", syncedAirport)
	assert.Equal(t, "LOW_VIS", syncedStatus["04L/22L"])
}

func TestHandleStripUpdate_RunwayChangeReevaluatesDepartureValidation(t *testing.T) {
	ctx := context.Background()
	const session = int32(9)
	const callsign = "SAS123"
	currentRunway := "22L"
	selectedRunway := "04R"

	var reevaluatedCallsign string
	var reevaluatedPublish bool
	var reevaluatedForce bool

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign: callsign,
				Session:  session,
				Runway:   &currentRunway,
			}, nil
		},
		UpdateRunwayFn: func(_ context.Context, _ int32, _ string, _ *string, _ *int32) (int64, error) {
			return 1, nil
		},
		AppendControllerModifiedFieldFn: func(_ context.Context, _ int32, _ string, _ string) error {
			return nil
		},
	}

	server := &testutil.MockServer{
		StripRepoVal:    stripRepo,
		EuroscopeHubVal: &testutil.MockEuroscopeHub{},
	}

	hub := &Hub{
		server: server,
		stripService: &stripUpdateValidationReevaluator{
			reevaluateDepartureFn: func(_ context.Context, gotSession int32, gotCallsign string, publish bool, forceReactivate bool) error {
				assert.Equal(t, session, gotSession)
				reevaluatedCallsign = gotCallsign
				reevaluatedPublish = publish
				reevaluatedForce = forceReactivate
				return nil
			},
		},
	}
	client := &Client{
		session:  session,
		hub:      hub,
		position: "EKCH_A_GND",
	}
	client.SetUser(shared.NewAuthenticatedUser("123456", 0, nil))

	payload, err := json.Marshal(frontendEvents.UpdateStripDataEvent{
		Type:     frontendEvents.UpdateStripData,
		Callsign: callsign,
		Runway:   &selectedRunway,
	})
	require.NoError(t, err)

	err = handleStripUpdate(ctx, client, Message{
		Type:    frontendEvents.UpdateStripData,
		Message: payload,
	})
	require.NoError(t, err)
	assert.Equal(t, callsign, reevaluatedCallsign)
	assert.True(t, reevaluatedPublish)
	assert.False(t, reevaluatedForce)
}

func TestHandleStripUpdate_SidChangeReevaluatesPdcInvalidValidationUsingSelectedSid(t *testing.T) {
	ctx := context.Background()
	const session = int32(8)
	const callsign = "SAS123"
	currentSid := "MIKRO"
	selectedSid := "BETUD"
	owner := "EKCH_DEL"

	var markedField string
	var reevaluatedSid *string
	var reevaluatedRunways []string
	var reevaluatedPublish bool
	var reevaluatedForce bool

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, gotSession int32, gotCallsign string) (*models.Strip, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			return &models.Strip{
				Callsign: callsign,
				Session:  session,
				Owner:    &owner,
				Sid:      &currentSid,
				PdcState: "REQUESTED_WITH_FAULTS",
			}, nil
		},
		AppendControllerModifiedFieldFn: func(_ context.Context, gotSession int32, gotCallsign string, field string) error {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			markedField = field
			return nil
		},
	}

	euroscopeHub := &testutil.MockEuroscopeHub{}
	server := &testutil.MockServer{
		StripRepoVal:    stripRepo,
		EuroscopeHubVal: euroscopeHub,
		SessionRepoVal: &testutil.MockSessionRepository{
			GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
				assert.Equal(t, session, id)
				return &models.Session{
					ID: id,
					ActiveRunways: pkgModels.ActiveRunways{
						DepartureRunways: []string{"22R"},
					},
				}, nil
			},
		},
	}

	stripService := &stripUpdateValidationReevaluator{
		reevaluateForStripFn: func(_ context.Context, gotSession int32, strip *models.Strip, activeDepartureRunways []string, publish bool, forceReactivate bool) error {
			assert.Equal(t, session, gotSession)
			reevaluatedSid = strip.Sid
			reevaluatedRunways = activeDepartureRunways
			reevaluatedPublish = publish
			reevaluatedForce = forceReactivate
			return nil
		},
	}
	hub := &Hub{server: server, stripService: stripService}
	client := &Client{
		session:  session,
		hub:      hub,
		position: owner,
	}
	client.SetUser(shared.NewAuthenticatedUser("123456", 0, nil))

	payload, err := json.Marshal(frontendEvents.UpdateStripDataEvent{
		Type:     frontendEvents.UpdateStripData,
		Callsign: callsign,
		Sid:      &selectedSid,
	})
	require.NoError(t, err)

	err = handleStripUpdate(ctx, client, Message{
		Type:    frontendEvents.UpdateStripData,
		Message: payload,
	})
	require.NoError(t, err)

	assert.Equal(t, "sid", markedField)
	require.NotNil(t, reevaluatedSid)
	assert.Equal(t, selectedSid, *reevaluatedSid)
	assert.Equal(t, []string{"22R"}, reevaluatedRunways)
	assert.True(t, reevaluatedPublish)
	assert.False(t, reevaluatedForce)
}

func TestHandleReleasePoint_OwnerMarksControllerModified(t *testing.T) {
	ctx := context.Background()
	const session = int32(9)
	const callsign = "SAS456"
	ownerPosition := "118.105"
	currentReleasePoint := "K1"
	nextReleasePoint := "K2"

	var updatedReleasePoint *string
	var markedField string

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, gotSession int32, gotCallsign string) (*models.Strip, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			return &models.Strip{
				Callsign:     callsign,
				Session:      session,
				Owner:        &ownerPosition,
				ReleasePoint: &currentReleasePoint,
			}, nil
		},
		UpdateReleasePointFn: func(_ context.Context, gotSession int32, gotCallsign string, releasePoint *string) (int64, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			updatedReleasePoint = releasePoint
			return 1, nil
		},
		AppendControllerModifiedFieldFn: func(_ context.Context, gotSession int32, gotCallsign string, field string) error {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			markedField = field
			return nil
		},
	}

	server := &testutil.MockServer{StripRepoVal: stripRepo}
	frontendHub := &testutil.MockFrontendHub{}
	stripService := services.NewStripService(stripRepo)
	stripService.SetFrontendHub(frontendHub)

	hub := &Hub{server: server, stripService: stripService}
	client := &Client{
		session:  session,
		hub:      hub,
		position: ownerPosition,
	}

	payload, err := json.Marshal(frontendEvents.ReleasePointEvent{
		Callsign:     callsign,
		ReleasePoint: nextReleasePoint,
	})
	require.NoError(t, err)

	err = handleReleasePoint(ctx, client, Message{
		Type:    frontendEvents.ReleasePoint,
		Message: payload,
	})
	require.NoError(t, err)

	require.NotNil(t, updatedReleasePoint)
	assert.Equal(t, nextReleasePoint, *updatedReleasePoint)
	assert.Equal(t, "release_point", markedField)
}

func TestHandleReleasePoint_NonOwnerSkipsControllerModified(t *testing.T) {
	ctx := context.Background()
	const session = int32(10)
	const callsign = "SAS789"
	ownerPosition := "118.105"
	nonOwnerPosition := "121.630"
	currentReleasePoint := "K1"
	nextReleasePoint := "K2"

	var updatedReleasePoint *string
	var unexpectedField string

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, gotSession int32, gotCallsign string) (*models.Strip, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			return &models.Strip{
				Callsign:     callsign,
				Session:      session,
				Owner:        &ownerPosition,
				ReleasePoint: &currentReleasePoint,
			}, nil
		},
		UpdateReleasePointFn: func(_ context.Context, gotSession int32, gotCallsign string, releasePoint *string) (int64, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			updatedReleasePoint = releasePoint
			return 1, nil
		},
		AppendUnexpectedChangeFieldFn: func(_ context.Context, gotSession int32, gotCallsign string, field string) error {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			unexpectedField = field
			return nil
		},
	}

	server := &testutil.MockServer{StripRepoVal: stripRepo}
	frontendHub := &testutil.MockFrontendHub{}
	stripService := services.NewStripService(stripRepo)
	stripService.SetFrontendHub(frontendHub)

	hub := &Hub{server: server, stripService: stripService}
	client := &Client{
		session:  session,
		hub:      hub,
		position: nonOwnerPosition,
	}

	payload, err := json.Marshal(frontendEvents.ReleasePointEvent{
		Callsign:     callsign,
		ReleasePoint: nextReleasePoint,
	})
	require.NoError(t, err)

	err = handleReleasePoint(ctx, client, Message{
		Type:    frontendEvents.ReleasePoint,
		Message: payload,
	})
	require.NoError(t, err)

	require.NotNil(t, updatedReleasePoint)
	assert.Equal(t, nextReleasePoint, *updatedReleasePoint)
	assert.Equal(t, "release_point", unexpectedField)
}

func TestHandleReleasePoint_NonOwnerWithActiveValidationSkipsControllerModified(t *testing.T) {
	ctx := context.Background()
	const session = int32(10)
	const callsign = "SAS789"
	ownerPosition := "118.105"
	nonOwnerPosition := "121.630"
	currentReleasePoint := "K1"
	nextReleasePoint := "K2"

	var updatedReleasePoint *string
	var unexpectedField string

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, gotSession int32, gotCallsign string) (*models.Strip, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			return &models.Strip{
				Callsign:     callsign,
				Session:      session,
				Owner:        &ownerPosition,
				ReleasePoint: &currentReleasePoint,
				ValidationStatus: &models.ValidationStatus{
					IssueType:      "RUNWAY TYPE",
					OwningPosition: ownerPosition,
					Active:         true,
					ActivationKey:  "validation-key",
				},
			}, nil
		},
		UpdateReleasePointFn: func(_ context.Context, gotSession int32, gotCallsign string, releasePoint *string) (int64, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			updatedReleasePoint = releasePoint
			return 1, nil
		},
		AppendUnexpectedChangeFieldFn: func(_ context.Context, gotSession int32, gotCallsign string, field string) error {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			unexpectedField = field
			return nil
		},
	}

	server := &testutil.MockServer{StripRepoVal: stripRepo}
	frontendHub := &testutil.MockFrontendHub{}
	stripService := services.NewStripService(stripRepo)
	stripService.SetFrontendHub(frontendHub)

	hub := &Hub{server: server, stripService: stripService}
	client := &Client{
		session:  session,
		hub:      hub,
		position: nonOwnerPosition,
	}

	payload, err := json.Marshal(frontendEvents.ReleasePointEvent{
		Callsign:     callsign,
		ReleasePoint: nextReleasePoint,
	})
	require.NoError(t, err)

	err = handleReleasePoint(ctx, client, Message{
		Type:    frontendEvents.ReleasePoint,
		Message: payload,
	})
	require.NoError(t, err)

	require.NotNil(t, updatedReleasePoint)
	assert.Equal(t, nextReleasePoint, *updatedReleasePoint)
	assert.Equal(t, "release_point", unexpectedField)
}

type standUpdateStripService struct {
	testutil.NoOpStripService
	updateStandFn func(ctx context.Context, session int32, callsign string, stand string) error
}

func (s *standUpdateStripService) UpdateStand(ctx context.Context, session int32, callsign string, stand string) error {
	if s.updateStandFn == nil {
		return nil
	}
	return s.updateStandFn(ctx, session, callsign, stand)
}

func TestHandleStripUpdate_StandChangeTriggersUpdateStand(t *testing.T) {
	ctx := context.Background()
	const session = int32(11)
	const callsign = "SAS123"
	const owner = "EKCH_A_GND"
	currentStand := ""
	selectedStand := "B12"

	var updateStandCallsign string
	var updateStandValue string
	var markedField string

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign: callsign,
				Session:  session,
				Owner:    ptr(owner),
				Stand:    &currentStand,
			}, nil
		},
		AppendControllerModifiedFieldFn: func(_ context.Context, _ int32, _ string, field string) error {
			markedField = field
			return nil
		},
	}

	server := &testutil.MockServer{
		StripRepoVal:    stripRepo,
		EuroscopeHubVal: &testutil.MockEuroscopeHub{},
	}

	stripService := &standUpdateStripService{
		updateStandFn: func(_ context.Context, _ int32, cs string, stand string) error {
			updateStandCallsign = cs
			updateStandValue = stand
			return nil
		},
	}

	hub := &Hub{server: server, stripService: stripService}
	client := &Client{
		session:  session,
		hub:      hub,
		position: owner,
	}
	client.SetUser(shared.NewAuthenticatedUser("123456", 0, nil))

	payload, err := json.Marshal(frontendEvents.UpdateStripDataEvent{
		Type:     frontendEvents.UpdateStripData,
		Callsign: callsign,
		Stand:    &selectedStand,
	})
	require.NoError(t, err)

	err = handleStripUpdate(ctx, client, Message{
		Type:    frontendEvents.UpdateStripData,
		Message: payload,
	})
	require.NoError(t, err)

	assert.Equal(t, "stand", markedField)
	assert.Equal(t, callsign, updateStandCallsign)
	assert.Equal(t, selectedStand, updateStandValue)
}

func ptr(s string) *string { return &s }
