package frontend

import (
	"context"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	frontendEvents "FlightStrips/pkg/events/frontend"
	pkgModels "FlightStrips/pkg/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingStripUpdatePublisher struct {
	session  int32
	callsign string
	calls    int
}

func (p *recordingStripUpdatePublisher) SendStripUpdate(session int32, callsign string) {
	p.session = session
	p.callsign = callsign
	p.calls++
}

type recordingStripUpdateEuroscopeSender struct {
	runways          []string
	clearedAltitudes []int32
	headings         []int32
}

func (s *recordingStripUpdateEuroscopeSender) SendRoute(_ int32, _ string, _ string, _ string) {}
func (s *recordingStripUpdateEuroscopeSender) SendAircraftInfoAndRemarks(_ int32, _ string, _ string, _ string, _ string) {
}
func (s *recordingStripUpdateEuroscopeSender) SendAircraftInfo(_ int32, _ string, _ string, _ string) {
}
func (s *recordingStripUpdateEuroscopeSender) SendRemarks(_ int32, _ string, _ string, _ string) {}
func (s *recordingStripUpdateEuroscopeSender) SendSid(_ int32, _ string, _ string, _ string)     {}
func (s *recordingStripUpdateEuroscopeSender) SendStand(_ int32, _ string, _ string, _ string)   {}
func (s *recordingStripUpdateEuroscopeSender) SendRunway(_ int32, _ string, _ string, runway string) {
	s.runways = append(s.runways, runway)
}
func (s *recordingStripUpdateEuroscopeSender) SendEobt(_ int32, _ string, _ string, _ string) {}
func (s *recordingStripUpdateEuroscopeSender) SendClearedAltitude(_ int32, _ string, _ string, altitude int32) {
	s.clearedAltitudes = append(s.clearedAltitudes, altitude)
}
func (s *recordingStripUpdateEuroscopeSender) SendHeading(_ int32, _ string, _ string, heading int32) {
	s.headings = append(s.headings, heading)
}

func TestFrontendStripUpdateService_RunwayChangePersistsSelectedRunway(t *testing.T) {
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

	service := NewFrontendStripUpdateService(
		stripRepo,
		nil,
		&testutil.MockEuroscopeHub{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	err := service.UpdateStrip(ctx, FrontendStripUpdateRequest{
		Session:  session,
		Cid:      "123456",
		Position: "EKCH_DEL",
		Event: frontendEvents.UpdateStripDataEvent{
			Callsign: callsign,
			Runway:   &selectedRunway,
		},
	})
	require.NoError(t, err)

	require.NotNil(t, updatedRunway)
	assert.Equal(t, selectedRunway, *updatedRunway)
	assert.Equal(t, "runway", markedField)
}

func TestFrontendStripUpdateService_EobtChangeTriggersCdmRecalculation(t *testing.T) {
	ctx := context.Background()
	const session = int32(7)
	const callsign = "SAS123"
	currentEobt := "1000"
	updatedEobt := "1015"
	tobt := "1020"
	tsat := "1030"
	ctot := "1040"

	var handledEobt string

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, gotSession int32, gotCallsign string) (*models.Strip, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
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
	publisher := &recordingStripUpdatePublisher{}

	service := NewFrontendStripUpdateService(
		stripRepo,
		nil,
		euroscopeHub,
		cdmService,
		nil,
		nil,
		nil,
		publisher,
	)

	err := service.UpdateStrip(ctx, FrontendStripUpdateRequest{
		Session:  session,
		Cid:      "123456",
		Position: "EKCH_DEL",
		Event: frontendEvents.UpdateStripDataEvent{
			Callsign: callsign,
			Eobt:     &updatedEobt,
		},
	})
	require.NoError(t, err)

	assert.Equal(t, updatedEobt, handledEobt)
	require.Len(t, euroscopeHub.Eobts, 1)
	assert.Equal(t, session, euroscopeHub.Eobts[0].Session)
	assert.Equal(t, "123456", euroscopeHub.Eobts[0].Cid)
	assert.Equal(t, callsign, euroscopeHub.Eobts[0].Callsign)
	assert.Equal(t, updatedEobt, euroscopeHub.Eobts[0].Eobt)
	assert.Equal(t, 1, publisher.calls)
	assert.Equal(t, session, publisher.session)
	assert.Equal(t, callsign, publisher.callsign)
}

func TestFrontendStripUpdateService_OwnerCanUpdateRemarksAndAircraftInfo(t *testing.T) {
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
	service := NewFrontendStripUpdateService(
		stripRepo,
		nil,
		euroscopeHub,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	err := service.UpdateStrip(ctx, FrontendStripUpdateRequest{
		Session:  session,
		Cid:      "123456",
		Position: owner,
		Event: frontendEvents.UpdateStripDataEvent{
			Callsign: callsign,
			Remarks:  &updatedRemarks,
			Aircraft: &updatedAircraftInfo,
		},
	})
	require.NoError(t, err)

	assert.Empty(t, euroscopeHub.RemarksUpdates)
	assert.Empty(t, euroscopeHub.AircraftInfoUpdates)
	require.Len(t, euroscopeHub.AircraftInfoRemarks, 1)
	assert.Equal(t, updatedRemarks, euroscopeHub.AircraftInfoRemarks[0].Remarks)
	assert.Equal(t, updatedAircraftInfo, euroscopeHub.AircraftInfoRemarks[0].AircraftType)
	assert.Equal(t, []string{"aircraft_info_remarks"}, euroscopeHub.FlightPlanUpdateOrder)
}

func TestFrontendStripUpdateService_NonOwnerCannotUpdateRemarksOrAircraftInfo(t *testing.T) {
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

	service := NewFrontendStripUpdateService(
		stripRepo,
		nil,
		&testutil.MockEuroscopeHub{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	err := service.UpdateStrip(ctx, FrontendStripUpdateRequest{
		Session:  session,
		Cid:      "123456",
		Position: "EKCH_DEL",
		Event: frontendEvents.UpdateStripDataEvent{
			Callsign: callsign,
			Remarks:  &updatedRemarks,
			Aircraft: &updatedAircraftInfo,
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-owner")
}

func TestFrontendStripUpdateService_RunwayChangeReevaluatesDepartureValidation(t *testing.T) {
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

	service := NewFrontendStripUpdateService(
		stripRepo,
		nil,
		&testutil.MockEuroscopeHub{},
		nil,
		nil,
		nil,
		&stripUpdateValidationReevaluator{
			reevaluateDepartureFn: func(_ context.Context, gotSession int32, gotCallsign string, publish bool, forceReactivate bool) error {
				assert.Equal(t, session, gotSession)
				reevaluatedCallsign = gotCallsign
				reevaluatedPublish = publish
				reevaluatedForce = forceReactivate
				return nil
			},
		},
		nil,
	)

	err := service.UpdateStrip(ctx, FrontendStripUpdateRequest{
		Session:  session,
		Cid:      "123456",
		Position: "EKCH_A_GND",
		Event: frontendEvents.UpdateStripDataEvent{
			Callsign: callsign,
			Runway:   &selectedRunway,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, callsign, reevaluatedCallsign)
	assert.True(t, reevaluatedPublish)
	assert.False(t, reevaluatedForce)
}

func TestFrontendStripUpdateService_SidChangeReevaluatesPdcInvalidValidationUsingSelectedSid(t *testing.T) {
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

	service := NewFrontendStripUpdateService(
		stripRepo,
		&testutil.MockSessionRepository{
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
		&testutil.MockEuroscopeHub{},
		nil,
		nil,
		&stripUpdateValidationReevaluator{
			reevaluateForStripFn: func(_ context.Context, gotSession int32, strip *models.Strip, activeDepartureRunways []string, publish bool, forceReactivate bool) error {
				assert.Equal(t, session, gotSession)
				reevaluatedSid = strip.Sid
				reevaluatedRunways = activeDepartureRunways
				reevaluatedPublish = publish
				reevaluatedForce = forceReactivate
				return nil
			},
		},
		nil,
		nil,
	)

	err := service.UpdateStrip(ctx, FrontendStripUpdateRequest{
		Session:  session,
		Cid:      "123456",
		Position: owner,
		Event: frontendEvents.UpdateStripDataEvent{
			Callsign: callsign,
			Sid:      &selectedSid,
		},
	})
	require.NoError(t, err)

	assert.Equal(t, "sid", markedField)
	require.NotNil(t, reevaluatedSid)
	assert.Equal(t, selectedSid, *reevaluatedSid)
	assert.Equal(t, []string{"22R"}, reevaluatedRunways)
	assert.True(t, reevaluatedPublish)
	assert.False(t, reevaluatedForce)
}

func TestFrontendStripUpdateService_StandChangeTriggersUpdateStand(t *testing.T) {
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

	stripService := &standUpdateStripService{
		updateStandFn: func(_ context.Context, _ int32, cs string, stand string) error {
			updateStandCallsign = cs
			updateStandValue = stand
			return nil
		},
	}

	service := NewFrontendStripUpdateService(
		stripRepo,
		nil,
		&testutil.MockEuroscopeHub{},
		nil,
		stripService,
		nil,
		nil,
		nil,
	)

	err := service.UpdateStrip(ctx, FrontendStripUpdateRequest{
		Session:  session,
		Cid:      "123456",
		Position: owner,
		Event: frontendEvents.UpdateStripDataEvent{
			Callsign: callsign,
			Stand:    &selectedStand,
		},
	})
	require.NoError(t, err)

	assert.Equal(t, "stand", markedField)
	assert.Equal(t, callsign, updateStandCallsign)
	assert.Equal(t, selectedStand, updateStandValue)
}

func TestFrontendStripUpdateService_ValueMatchedRunwayAltitudeAndHeadingAreNoOps(t *testing.T) {
	ctx := context.Background()
	const session = int32(5)
	const callsign = "SAS123"
	runway := "22L"
	altitude := int32(5000)
	heading := int32(220)

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign:        callsign,
				Session:         session,
				Runway:          ptr(runway),
				ClearedAltitude: &altitude,
				Heading:         &heading,
			}, nil
		},
	}
	sender := &recordingStripUpdateEuroscopeSender{}

	sameRunway := "22L"
	sameAltitude := int32(5000)
	sameHeading := int32(220)

	service := NewFrontendStripUpdateService(
		stripRepo,
		nil,
		sender,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	err := service.UpdateStrip(ctx, FrontendStripUpdateRequest{
		Session:  session,
		Cid:      "123456",
		Position: "EKCH_DEL",
		Event: frontendEvents.UpdateStripDataEvent{
			Callsign: callsign,
			Runway:   &sameRunway,
			Altitude: &sameAltitude,
			Heading:  &sameHeading,
		},
	})
	require.NoError(t, err)
	assert.Empty(t, sender.runways)
	assert.Empty(t, sender.clearedAltitudes)
	assert.Empty(t, sender.headings)
}

func TestFrontendStripUpdateService_NilStoredAltitudeAndHeadingStillForwardExplicitZero(t *testing.T) {
	ctx := context.Background()
	const session = int32(6)
	const callsign = "SAS321"
	zero := int32(0)

	var markedFields []string

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign: callsign,
				Session:  session,
			}, nil
		},
		AppendControllerModifiedFieldFn: func(_ context.Context, _ int32, _ string, fieldName string) error {
			markedFields = append(markedFields, fieldName)
			return nil
		},
	}
	sender := &recordingStripUpdateEuroscopeSender{}

	service := NewFrontendStripUpdateService(
		stripRepo,
		nil,
		sender,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	err := service.UpdateStrip(ctx, FrontendStripUpdateRequest{
		Session:  session,
		Cid:      "123456",
		Position: "EKCH_DEL",
		Event: frontendEvents.UpdateStripDataEvent{
			Callsign: callsign,
			Altitude: &zero,
			Heading:  &zero,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, []int32{0}, sender.clearedAltitudes)
	assert.Equal(t, []int32{0}, sender.headings)
	assert.Equal(t, []string{"cleared_altitude", "heading"}, markedFields)
}
