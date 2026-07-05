package services

import (
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	euroscope "FlightStrips/pkg/events/euroscope"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type frontendMovePdcSpy struct {
	confirmCalls []struct {
		callsign string
		session  int32
	}
	err error
}

func (s *frontendMovePdcSpy) IssueClearance(_ context.Context, _ string, _ string, _ string, _ int32) error {
	return nil
}

func (s *frontendMovePdcSpy) ManualStateChange(_ context.Context, _ string, _ int32, _ string) error {
	return nil
}

func (s *frontendMovePdcSpy) ConfirmVoiceClearance(_ context.Context, callsign string, session int32) error {
	s.confirmCalls = append(s.confirmCalls, struct {
		callsign string
		session  int32
	}{callsign: callsign, session: session})
	return s.err
}

func (s *frontendMovePdcSpy) RevertToVoice(_ context.Context, _ string, _ int32, _ string) error {
	return nil
}

type frontendMoveFixture struct {
	svc                  *StripService
	repo                 *testutil.MockStripRepository
	hub                  *testutil.MockFrontendHub
	esHub                *testutil.MockEuroscopeHub
	server               *testutil.MockServer
	strip                *internalModels.Strip
	ctx                  context.Context
	updateClearedBays    []string
	updateClearedFlags   []bool
	updateGroundBays     []string
	updateGroundStates   []string
	updateBayTargets     []string
	setPreviousOwners    int
	setOwnerClearedCount int
	startReqUpdates      []bool
	routeUpdates         int
	pdcSpy               *frontendMovePdcSpy
}

func cloneStrip(strip *internalModels.Strip) *internalModels.Strip {
	if strip == nil {
		return nil
	}
	clone := *strip
	if strip.Sequence != nil {
		sequence := *strip.Sequence
		clone.Sequence = &sequence
	}
	if strip.State != nil {
		state := *strip.State
		clone.State = &state
	}
	if strip.Owner != nil {
		owner := *strip.Owner
		clone.Owner = &owner
	}
	if strip.ValidationStatus != nil {
		status := *strip.ValidationStatus
		clone.ValidationStatus = &status
	}
	clone.NextOwners = append([]string(nil), strip.NextOwners...)
	clone.PreviousOwners = append([]string(nil), strip.PreviousOwners...)
	return &clone
}

func newFrontendMoveFixture(strip *internalModels.Strip) *frontendMoveFixture {
	fixture := &frontendMoveFixture{
		strip:  strip,
		hub:    &testutil.MockFrontendHub{},
		esHub:  &testutil.MockEuroscopeHub{},
		pdcSpy: &frontendMovePdcSpy{},
	}

	fixture.repo = &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*internalModels.Strip, error) {
			return cloneStrip(fixture.strip), nil
		},
		UpdateClearedFlagFn: func(_ context.Context, _ int32, _ string, cleared bool, bay string, _ *int32) (int64, error) {
			fixture.updateClearedFlags = append(fixture.updateClearedFlags, cleared)
			fixture.updateClearedBays = append(fixture.updateClearedBays, bay)
			fixture.strip.Cleared = cleared
			return 1, nil
		},
		UpdateGroundStateFn: func(_ context.Context, _ int32, _ string, state *string, bay string, _ *int32) (int64, error) {
			fixture.updateGroundBays = append(fixture.updateGroundBays, bay)
			if state == nil {
				fixture.updateGroundStates = append(fixture.updateGroundStates, "")
				fixture.strip.State = nil
			} else {
				fixture.updateGroundStates = append(fixture.updateGroundStates, *state)
				value := *state
				fixture.strip.State = &value
			}
			return 1, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, bay string, sequence int32) (int64, error) {
			fixture.updateBayTargets = append(fixture.updateBayTargets, bay)
			fixture.strip.Bay = bay
			fixture.strip.Sequence = &sequence
			return 1, nil
		},
		GetMaxSequenceInBayFn: func(_ context.Context, _ int32, _ string) (int32, error) {
			return 0, nil
		},
		SetPreviousOwnersFn: func(_ context.Context, _ int32, _ string, previousOwners []string) error {
			fixture.setPreviousOwners++
			fixture.strip.PreviousOwners = previousOwners
			return nil
		},
		SetOwnerFn: func(_ context.Context, _ int32, _ string, owner *string, _ int32) (int64, error) {
			if owner == nil {
				fixture.setOwnerClearedCount++
			}
			fixture.strip.Owner = owner
			fixture.strip.Version++
			return 1, nil
		},
		UpdateStartReqFn: func(_ context.Context, _ int32, _ string, startReq bool, _ *int32) (int64, error) {
			fixture.startReqUpdates = append(fixture.startReqUpdates, startReq)
			fixture.strip.StartReq = startReq
			return 1, nil
		},
	}

	fixture.server = &testutil.MockServer{
		FrontendHubVal:  fixture.hub,
		EuroscopeHubVal: fixture.esHub,
		StripRepoVal:    fixture.repo,
		PdcServiceVal:   fixture.pdcSpy,
		UpdateRouteForStripCtxFn: func(_ context.Context, _ string, _ int32, _ bool) error {
			fixture.routeUpdates++
			return nil
		},
	}
	fixture.hub.SetServer(fixture.server)
	fixture.esHub.SetServer(fixture.server)

	fixture.svc = NewStripService(fixture.repo)
	fixture.svc.SetFrontendHub(fixture.hub)
	fixture.svc.SetEuroscopeHub(fixture.esHub)
	fixture.ctx = shared.WithSyncState(context.Background(), &shared.SyncState{
		ExistingStrips: map[string]*internalModels.Strip{
			strip.Callsign: strip,
		},
	})

	return fixture
}

func TestMoveFrontendStrip_ToClearedBayUsesSingleBayWrite(t *testing.T) {
	fixture := newFrontendMoveFixture(&internalModels.Strip{
		Callsign:    "SAS123",
		Bay:         shared.BAY_NOT_CLEARED,
		Origin:      "EKCH",
		Destination: "ESSA",
	})

	err := fixture.svc.MoveFrontendStrip(fixture.ctx, 1, "SAS123", shared.BAY_CLEARED, "1234567", "EKCH", "EKCH_DEL")
	require.NoError(t, err)

	assert.Equal(t, []string{shared.BAY_NOT_CLEARED}, fixture.updateClearedBays)
	assert.Equal(t, []bool{true}, fixture.updateClearedFlags)
	assert.Equal(t, []string{shared.BAY_CLEARED}, fixture.updateBayTargets)
	assert.True(t, fixture.strip.Cleared)
	assert.Equal(t, shared.BAY_CLEARED, fixture.strip.Bay)
	require.Len(t, fixture.esHub.ClearedFlags, 1)
	assert.True(t, fixture.esHub.ClearedFlags[0].Flag)
}

func TestMoveFrontendStrip_ToNotClearedBayClearsOwnerStateWithoutExtraBayWrite(t *testing.T) {
	fixture := newFrontendMoveFixture(&internalModels.Strip{
		Callsign:       "SAS124",
		Bay:            shared.BAY_CLEARED,
		Origin:         "EKCH",
		Destination:    "ESSA",
		Cleared:        true,
		PreviousOwners: []string{"EKCH_DEL"},
	})

	err := fixture.svc.MoveFrontendStrip(fixture.ctx, 1, "SAS124", shared.BAY_NOT_CLEARED, "1234567", "EKCH", "EKCH_DEL")
	require.NoError(t, err)

	assert.Equal(t, []string{shared.BAY_CLEARED}, fixture.updateClearedBays)
	assert.Equal(t, []bool{false}, fixture.updateClearedFlags)
	assert.Equal(t, 1, fixture.setPreviousOwners)
	assert.Equal(t, []string{shared.BAY_NOT_CLEARED}, fixture.updateBayTargets)
	assert.False(t, fixture.strip.Cleared)
	assert.Equal(t, shared.BAY_NOT_CLEARED, fixture.strip.Bay)
	assert.Equal(t, 1, fixture.routeUpdates)
}

func TestMoveFrontendStrip_ToGeneralBayUsesSingleBayWrite(t *testing.T) {
	fixture := newFrontendMoveFixture(&internalModels.Strip{
		Callsign:    "SAS125",
		Bay:         shared.BAY_STAND,
		Origin:      "EKCH",
		Destination: "ESSA",
		StartReq:    true,
	})

	err := fixture.svc.MoveFrontendStrip(fixture.ctx, 1, "SAS125", shared.BAY_PUSH, "1234567", "EKCH", "EKCH_DEL")
	require.NoError(t, err)

	assert.Equal(t, []string{shared.BAY_STAND}, fixture.updateGroundBays)
	assert.Equal(t, []string{euroscope.GroundStatePush}, fixture.updateGroundStates)
	assert.Equal(t, []bool{false}, fixture.startReqUpdates)
	assert.Equal(t, []string{shared.BAY_PUSH}, fixture.updateBayTargets)
	assert.Equal(t, shared.BAY_PUSH, fixture.strip.Bay)
	require.Len(t, fixture.esHub.GroundStates, 1)
	assert.Equal(t, euroscope.GroundStatePush, fixture.esHub.GroundStates[0].GroundState)
}

func TestMoveFrontendStrip_DepartureToArrivalBayRejected(t *testing.T) {
	fixture := newFrontendMoveFixture(&internalModels.Strip{
		Callsign:    "SAS126",
		Bay:         shared.BAY_DEPART,
		Origin:      "EKCH",
		Destination: "ESSA",
	})

	err := fixture.svc.MoveFrontendStrip(fixture.ctx, 1, "SAS126", shared.BAY_FINAL, "1234567", "EKCH", "EKCH_TWR")
	require.ErrorContains(t, err, "departure strips cannot be moved to arrival bays")
	assert.Empty(t, fixture.updateBayTargets)
}

func TestMoveFrontendStrip_ArrivalToDepartureBayRejected(t *testing.T) {
	fixture := newFrontendMoveFixture(&internalModels.Strip{
		Callsign:    "SAS127",
		Bay:         shared.BAY_TWY_ARR,
		Origin:      "ESSA",
		Destination: "EKCH",
	})

	err := fixture.svc.MoveFrontendStrip(fixture.ctx, 1, "SAS127", shared.BAY_TAXI_LWR, "1234567", "EKCH", "EKCH_TWR")
	require.ErrorContains(t, err, "arrival strips cannot be moved to departure bays")
	assert.Empty(t, fixture.updateBayTargets)
}

func TestMoveFrontendStrip_ValidationLockRejected(t *testing.T) {
	fixture := newFrontendMoveFixture(&internalModels.Strip{
		Callsign: "SAS128",
		Bay:      shared.BAY_TAXI,
		ValidationStatus: &internalModels.ValidationStatus{
			Active: true,
		},
	})

	err := fixture.svc.MoveFrontendStrip(fixture.ctx, 1, "SAS128", shared.BAY_PUSH, "1234567", "EKCH", "EKCH_GND")
	require.ErrorContains(t, err, "strip is locked by an active validation")
	assert.Empty(t, fixture.updateBayTargets)
}

func TestMoveFrontendStrip_CoordinationTransferExceptionAllowsMove(t *testing.T) {
	owner := "EKCH_D_GND"
	fixture := newFrontendMoveFixture(&internalModels.Strip{
		Callsign: "SAS129",
		Bay:      shared.BAY_TAXI,
		Owner:    &owner,
	})
	fixture.svc.SetCoordinationRepo(&testutil.MockCoordinationRepository{
		GetByStripCallsignFn: func(_ context.Context, _ int32, _ string) (*internalModels.Coordination, error) {
			return &internalModels.Coordination{ToPosition: "EKCH_A_GND"}, nil
		},
	})

	err := fixture.svc.MoveFrontendStrip(fixture.ctx, 1, "SAS129", shared.BAY_TAXI_TWR, "1234567", "EKCH", "EKCH_A_GND")
	require.NoError(t, err)
	assert.Equal(t, []string{shared.BAY_TAXI_TWR}, fixture.updateBayTargets)
}

func TestMoveFrontendStrip_ToClearedWithActivePdcConfirmsVoiceClearance(t *testing.T) {
	fixture := newFrontendMoveFixture(&internalModels.Strip{
		Callsign:    "SAS130",
		Bay:         shared.BAY_NOT_CLEARED,
		Origin:      "EKCH",
		Destination: "ESSA",
		PdcState:    "REQUESTED",
	})

	err := fixture.svc.MoveFrontendStrip(fixture.ctx, 1, "SAS130", shared.BAY_CLEARED, "1234567", "EKCH", "EKCH_DEL")
	require.NoError(t, err)
	require.Len(t, fixture.pdcSpy.confirmCalls, 1)
	assert.Equal(t, "SAS130", fixture.pdcSpy.confirmCalls[0].callsign)
	assert.Equal(t, int32(1), fixture.pdcSpy.confirmCalls[0].session)
}

func TestMoveFrontendStrip_RevertsClearedMoveWhenVoiceConfirmationFails(t *testing.T) {
	fixture := newFrontendMoveFixture(&internalModels.Strip{
		Callsign:    "SAS131",
		Bay:         shared.BAY_NOT_CLEARED,
		Origin:      "EKCH",
		Destination: "ESSA",
		PdcState:    "REQUESTED",
	})
	fixture.pdcSpy.err = assert.AnError

	err := fixture.svc.MoveFrontendStrip(fixture.ctx, 1, "SAS131", shared.BAY_CLEARED, "1234567", "EKCH", "EKCH_DEL")
	require.ErrorIs(t, err, assert.AnError)
	assert.Equal(t, []bool{true, false}, fixture.updateClearedFlags)
	assert.Equal(t, []string{shared.BAY_NOT_CLEARED, shared.BAY_NOT_CLEARED}, fixture.updateClearedBays)
	assert.Equal(t, []string{shared.BAY_CLEARED, shared.BAY_NOT_CLEARED}, fixture.updateBayTargets)
	assert.False(t, fixture.strip.Cleared)
	assert.Equal(t, shared.BAY_NOT_CLEARED, fixture.strip.Bay)
}
