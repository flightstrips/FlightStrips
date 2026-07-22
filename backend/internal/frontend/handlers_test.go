package frontend

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/services"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	frontendEvents "FlightStrips/pkg/events/frontend"
	pkgModels "FlightStrips/pkg/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stripUpdateValidationReevaluator struct {
	noOpStripService
	reevaluateForStripFn  func(ctx context.Context, session int32, strip *models.Strip, activeDepartureRunways []string, publish bool, forceReactivate bool) error
	reevaluateDepartureFn func(ctx context.Context, session int32, callsign string, publish bool, forceReactivate bool) error
}

type recordingCdmService struct {
	triggerRecalculateFn             func(ctx context.Context, session int32, airport string)
	syncAirportLvoFromRunwayStatusFn func(ctx context.Context, airport string, runwayStatus map[string]string)
	handleEobtUpdateFn               func(ctx context.Context, session int32, callsign string, eobt string, sourcePosition string, sourceRole string) error
}

type transferStripService struct {
	noOpStripService
	createCoordinationTransferFn func(ctx context.Context, session int32, callsign string, from string, to string) error
	updateMarkedFn               func(ctx context.Context, session int32, callsign string, marked bool) error
}

func (s *transferStripService) CreateCoordinationTransfer(ctx context.Context, session int32, callsign string, from string, to string) error {
	if s.createCoordinationTransferFn == nil {
		return nil
	}
	return s.createCoordinationTransferFn(ctx, session, callsign, from, to)
}

func (s *transferStripService) UpdateMarked(ctx context.Context, session int32, callsign string, marked bool) error {
	if s.updateMarkedFn == nil {
		return nil
	}
	return s.updateMarkedFn(ctx, session, callsign, marked)
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

func (*recordingCdmService) HandleReadyRequest(context.Context, int32, string, string, string) error {
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

func TestHandleSendPrivateMessage_SendsOnlyToMatchingEuroscopeClient(t *testing.T) {
	ctx := context.Background()
	const session = int32(7)
	const cid = "123456"
	const callsign = "EKDK_CTR"
	const messageText = "Hello from the frontend"

	euroscopeHub := &testutil.MockEuroscopeHub{}
	server := &testutil.MockServer{
		EuroscopeHubVal: euroscopeHub,
	}
	hub := &Hub{server: server, send: make(chan internalMessage, 1)}
	client := &Client{
		session: session,
		hub:     hub,
	}
	client.SetUser(shared.NewAuthenticatedUser(cid, 0, nil))

	payload, err := json.Marshal(frontendEvents.SendPrivateMessageEvent{
		Callsign: callsign,
		Message:  messageText,
	})
	require.NoError(t, err)

	err = handleSendPrivateMessage(ctx, client, Message{
		Type:    frontendEvents.SendPrivateMessage,
		Message: payload,
	})
	require.NoError(t, err)

	assert.Empty(t, euroscopeHub.Broadcasts)
	require.Len(t, euroscopeHub.SentMessages, 1)
	assert.Equal(t, session, euroscopeHub.SentMessages[0].Session)
	assert.Equal(t, cid, euroscopeHub.SentMessages[0].Cid)

	event, ok := euroscopeHub.SentMessages[0].Message.(euroscopeEvents.SendPrivateMessageEvent)
	require.True(t, ok)
	assert.Equal(t, callsign, event.Callsign)
	assert.Equal(t, messageText, event.Message)
}

func TestHandleCoordinationTransferRequest_ClearsMarkForMarkedStrip(t *testing.T) {
	ctx := context.Background()
	const session = int32(7)
	const callsign = "SAS123"
	const owner = "EKCH_DEL"
	const target = "EKCH_GND"

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, gotSession int32, gotCallsign string) (*models.Strip, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			return &models.Strip{
				Callsign: callsign,
				Session:  session,
				Owner:    ptr(owner),
				Bay:      shared.BAY_STAND,
				Marked:   true,
			}, nil
		},
	}

	server := &testutil.MockServer{StripRepoVal: stripRepo}

	createCalled := false
	updateMarkedCalled := false
	stripService := &transferStripService{
		createCoordinationTransferFn: func(_ context.Context, gotSession int32, gotCallsign string, from string, to string) error {
			createCalled = true
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			assert.Equal(t, owner, from)
			assert.Equal(t, target, to)
			return nil
		},
		updateMarkedFn: func(_ context.Context, gotSession int32, gotCallsign string, marked bool) error {
			updateMarkedCalled = true
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			assert.False(t, marked)
			return nil
		},
	}

	hub := &Hub{server: server, stripService: stripService}
	client := &Client{
		session:  session,
		hub:      hub,
		position: owner,
	}

	payload, err := json.Marshal(frontendEvents.CoordinationTransferRequestEvent{
		Type:     string(frontendEvents.CoordinationTransferRequestType),
		Callsign: callsign,
		To:       target,
	})
	require.NoError(t, err)

	err = handleCoordinationTransferRequest(ctx, client, Message{
		Type:    frontendEvents.CoordinationTransferRequestType,
		Message: payload,
	})
	require.NoError(t, err)

	assert.True(t, createCalled)
	assert.True(t, updateMarkedCalled)
}

func TestHandleCoordinationTransferRequest_UsesNextOwnerWhenTargetIsOmitted(t *testing.T) {
	ctx := context.Background()
	const session = int32(7)
	const callsign = "SAS126"
	const owner = "EKCH_DEL"
	const target = "EKCH_A_GND"

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{Callsign: callsign, Session: session, Owner: ptr(owner), NextOwners: []string{target}}, nil
		},
	}
	server := &testutil.MockServer{StripRepoVal: stripRepo}
	stripService := &transferStripService{
		createCoordinationTransferFn: func(_ context.Context, _ int32, _ string, from string, to string) error {
			assert.Equal(t, owner, from)
			assert.Equal(t, target, to)
			return nil
		},
	}
	hub := &Hub{server: server, stripService: stripService}
	client := &Client{session: session, hub: hub, position: owner}

	payload, err := json.Marshal(frontendEvents.CoordinationTransferRequestEvent{
		Type: string(frontendEvents.CoordinationTransferRequestType), Callsign: callsign,
	})
	require.NoError(t, err)

	err = handleCoordinationTransferRequest(ctx, client, Message{
		Type: frontendEvents.CoordinationTransferRequestType, Message: payload,
	})
	require.NoError(t, err)
}

func TestHandleCoordinationTransferRequest_RejectsOmittedTargetWithoutNextOwner(t *testing.T) {
	ctx := context.Background()
	const owner = "EKCH_DEL"

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{Owner: ptr(owner)}, nil
		},
	}
	server := &testutil.MockServer{StripRepoVal: stripRepo}
	hub := &Hub{server: server, stripService: &transferStripService{}}
	client := &Client{session: 7, hub: hub, position: owner}

	payload, err := json.Marshal(frontendEvents.CoordinationTransferRequestEvent{
		Type: string(frontendEvents.CoordinationTransferRequestType), Callsign: "SAS127",
	})
	require.NoError(t, err)

	err = handleCoordinationTransferRequest(ctx, client, Message{
		Type: frontendEvents.CoordinationTransferRequestType, Message: payload,
	})
	require.EqualError(t, err, "cannot transfer strip without a next controller")
}

func TestHandleCoordinationTransferRequest_DoesNotClearMarkForUnmarkedStrip(t *testing.T) {
	ctx := context.Background()
	const session = int32(7)
	const callsign = "SAS124"
	const owner = "EKCH_DEL"
	const target = "EKCH_GND"

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, gotSession int32, gotCallsign string) (*models.Strip, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			return &models.Strip{
				Callsign: callsign,
				Session:  session,
				Owner:    ptr(owner),
				Bay:      shared.BAY_STAND,
				Marked:   false,
			}, nil
		},
	}

	server := &testutil.MockServer{StripRepoVal: stripRepo}

	createCalled := false
	updateMarkedCalled := false
	stripService := &transferStripService{
		createCoordinationTransferFn: func(_ context.Context, gotSession int32, gotCallsign string, from string, to string) error {
			createCalled = true
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			assert.Equal(t, owner, from)
			assert.Equal(t, target, to)
			return nil
		},
		updateMarkedFn: func(_ context.Context, _ int32, _ string, _ bool) error {
			updateMarkedCalled = true
			return nil
		},
	}

	hub := &Hub{server: server, stripService: stripService}
	client := &Client{
		session:  session,
		hub:      hub,
		position: owner,
	}

	payload, err := json.Marshal(frontendEvents.CoordinationTransferRequestEvent{
		Type:     string(frontendEvents.CoordinationTransferRequestType),
		Callsign: callsign,
		To:       target,
	})
	require.NoError(t, err)

	err = handleCoordinationTransferRequest(ctx, client, Message{
		Type:    frontendEvents.CoordinationTransferRequestType,
		Message: payload,
	})
	require.NoError(t, err)

	assert.True(t, createCalled)
	assert.False(t, updateMarkedCalled)
}

func TestHandleCoordinationTransferRequest_MarkClearFailureDoesNotRejectTransfer(t *testing.T) {
	ctx := context.Background()
	const session = int32(7)
	const callsign = "SAS125"
	const owner = "EKCH_DEL"
	const target = "EKCH_GND"

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, gotSession int32, gotCallsign string) (*models.Strip, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			return &models.Strip{
				Callsign: callsign,
				Session:  session,
				Owner:    ptr(owner),
				Bay:      shared.BAY_STAND,
				Marked:   true,
			}, nil
		},
	}

	server := &testutil.MockServer{StripRepoVal: stripRepo}

	createCalled := false
	updateMarkedCalled := false
	stripService := &transferStripService{
		createCoordinationTransferFn: func(_ context.Context, gotSession int32, gotCallsign string, from string, to string) error {
			createCalled = true
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			assert.Equal(t, owner, from)
			assert.Equal(t, target, to)
			return nil
		},
		updateMarkedFn: func(_ context.Context, gotSession int32, gotCallsign string, marked bool) error {
			updateMarkedCalled = true
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			assert.False(t, marked)
			return errors.New("mark update failed")
		},
	}

	hub := &Hub{server: server, stripService: stripService}
	client := &Client{
		session:  session,
		hub:      hub,
		position: owner,
	}

	payload, err := json.Marshal(frontendEvents.CoordinationTransferRequestEvent{
		Type:     string(frontendEvents.CoordinationTransferRequestType),
		Callsign: callsign,
		To:       target,
	})
	require.NoError(t, err)

	err = handleCoordinationTransferRequest(ctx, client, Message{
		Type:    frontendEvents.CoordinationTransferRequestType,
		Message: payload,
	})
	require.NoError(t, err)

	assert.True(t, createCalled)
	assert.True(t, updateMarkedCalled)
}

type recordingStripUpdateUseCase struct {
	req FrontendStripUpdateRequest
	err error
}

func (r *recordingStripUpdateUseCase) UpdateStrip(_ context.Context, req FrontendStripUpdateRequest) error {
	r.req = req
	return r.err
}

func TestHandleStripUpdate_DecodesAndDelegatesToUseCase(t *testing.T) {
	ctx := context.Background()
	const session = int32(7)
	const cid = "123456"
	const callsign = "SAS123"
	const position = "EKCH_DEL"
	route := "NEXIL Z20"

	useCase := &recordingStripUpdateUseCase{}
	hub := &Hub{stripUpdateService: useCase}
	client := &Client{
		session:  session,
		hub:      hub,
		position: position,
	}
	client.SetUser(shared.NewAuthenticatedUser(cid, 0, nil))

	payload, err := json.Marshal(frontendEvents.UpdateStripDataEvent{
		Type:     frontendEvents.UpdateStripData,
		Callsign: callsign,
		Route:    &route,
	})
	require.NoError(t, err)

	err = handleStripUpdate(ctx, client, Message{
		Type:    frontendEvents.UpdateStripData,
		Message: payload,
	})
	require.NoError(t, err)
	assert.Equal(t, session, useCase.req.Session)
	assert.Equal(t, cid, useCase.req.Cid)
	assert.Equal(t, position, useCase.req.Position)
	assert.Equal(t, callsign, useCase.req.Event.Callsign)
	require.NotNil(t, useCase.req.Event.Route)
	assert.Equal(t, route, *useCase.req.Event.Route)
}

func TestHandleStripUpdate_UsesHubWiringForServiceDependencies(t *testing.T) {
	ctx := context.Background()
	const session = int32(7)
	const callsign = "SAS123"
	const cid = "123456"
	const owner = "EKCH_DEL"
	currentSid := "MIKRO"
	selectedSid := "BETUD"
	currentEobt := "1000"
	updatedEobt := "1015"

	getByCallsignCalls := 0
	var handledEobt string
	var reevaluatedSid *string
	var reevaluatedRunways []string

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, gotSession int32, gotCallsign string) (*models.Strip, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			getByCallsignCalls++
			return &models.Strip{
				Callsign: callsign,
				Session:  session,
				Owner:    ptr(owner),
				Sid:      &currentSid,
				PdcState: "REQUESTED_WITH_FAULTS",
				CdmData: &models.CdmData{
					Eobt: &currentEobt,
				},
			}, nil
		},
		AppendControllerModifiedFieldFn: func(_ context.Context, gotSession int32, gotCallsign string, field string) error {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			assert.Contains(t, []string{"sid"}, field)
			return nil
		},
	}

	cdmService := &recordingCdmService{
		handleEobtUpdateFn: func(_ context.Context, gotSession int32, gotCallsign string, eobt string, sourcePosition string, sourceRole string) error {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			assert.Equal(t, owner, sourcePosition)
			assert.Equal(t, "ATC", sourceRole)
			handledEobt = eobt
			return nil
		},
	}

	server := &testutil.MockServer{
		StripRepoVal: stripRepo,
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
		CdmServiceVal:   cdmService,
		EuroscopeHubVal: &testutil.MockEuroscopeHub{},
	}

	hub := &Hub{
		server: server,
		stripService: &stripUpdateValidationReevaluator{
			reevaluateForStripFn: func(_ context.Context, gotSession int32, strip *models.Strip, activeDepartureRunways []string, publish bool, forceReactivate bool) error {
				assert.Equal(t, session, gotSession)
				assert.True(t, publish)
				assert.False(t, forceReactivate)
				reevaluatedSid = strip.Sid
				reevaluatedRunways = activeDepartureRunways
				return nil
			},
		},
		send:         make(chan internalMessage, 1),
		clxOverrides: make(map[int32]map[string]bool),
	}
	client := &Client{
		session:  session,
		hub:      hub,
		position: owner,
	}
	client.SetUser(shared.NewAuthenticatedUser(cid, 0, nil))

	payload, err := json.Marshal(frontendEvents.UpdateStripDataEvent{
		Type:     frontendEvents.UpdateStripData,
		Callsign: callsign,
		Sid:      &selectedSid,
		Eobt:     &updatedEobt,
	})
	require.NoError(t, err)

	err = handleStripUpdate(ctx, client, Message{
		Type:    frontendEvents.UpdateStripData,
		Message: payload,
	})
	require.NoError(t, err)

	assert.Equal(t, updatedEobt, handledEobt)
	require.NotNil(t, reevaluatedSid)
	assert.Equal(t, selectedSid, *reevaluatedSid)
	assert.Equal(t, []string{"22R"}, reevaluatedRunways)
	assert.Equal(t, 2, getByCallsignCalls)

	select {
	case msg := <-hub.send:
		assert.Equal(t, session, msg.session)
		update, ok := msg.message.(frontendEvents.StripUpdateEvent)
		require.True(t, ok)
		assert.Equal(t, callsign, update.Callsign)
	case <-time.After(time.Second):
		t.Fatal("expected strip update broadcast from hub publisher")
	}
}

func TestHandleStripUpdate_SameRunwayValueSkipsSideEffects(t *testing.T) {
	ctx := context.Background()
	const session = int32(7)
	const callsign = "SAS123"
	currentRunway := "22L"
	selectedRunway := "22L"

	var updateRunwayCalls int
	var appendControllerModifiedCalls int
	var reevaluatePdcCalls int
	var reevaluateDepartureCalls int

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
		UpdateRunwayFn: func(_ context.Context, _ int32, _ string, _ *string, _ *int32) (int64, error) {
			updateRunwayCalls++
			return 1, nil
		},
		AppendControllerModifiedFieldFn: func(_ context.Context, _ int32, _ string, _ string) error {
			appendControllerModifiedCalls++
			return nil
		},
	}

	euroscopeHub := &testutil.MockEuroscopeHub{}
	server := &testutil.MockServer{
		StripRepoVal:    stripRepo,
		EuroscopeHubVal: euroscopeHub,
	}

	hub := &Hub{
		server: server,
		stripService: &stripUpdateValidationReevaluator{
			reevaluateForStripFn: func(_ context.Context, _ int32, _ *models.Strip, _ []string, _ bool, _ bool) error {
				reevaluatePdcCalls++
				return nil
			},
			reevaluateDepartureFn: func(_ context.Context, _ int32, _ string, _ bool, _ bool) error {
				reevaluateDepartureCalls++
				return nil
			},
		},
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
		Runway:   &selectedRunway,
	})
	require.NoError(t, err)

	err = handleStripUpdate(ctx, client, Message{
		Type:    frontendEvents.UpdateStripData,
		Message: payload,
	})
	require.NoError(t, err)

	assert.Empty(t, euroscopeHub.Runways)
	assert.Zero(t, updateRunwayCalls)
	assert.Zero(t, appendControllerModifiedCalls)
	assert.Zero(t, reevaluatePdcCalls)
	assert.Zero(t, reevaluateDepartureCalls)
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
	noOpStripService
	updateStandFn func(ctx context.Context, session int32, callsign string, stand string) error
}

func (s *standUpdateStripService) UpdateStand(ctx context.Context, session int32, callsign string, stand string) error {
	if s.updateStandFn == nil {
		return nil
	}
	return s.updateStandFn(ctx, session, callsign, stand)
}

func ptr(s string) *string { return &s }
