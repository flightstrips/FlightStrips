package frontend

import (
	"context"
	"encoding/json"
	"testing"

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
