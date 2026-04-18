package services

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	pkgModels "FlightStrips/pkg/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupLandingClearanceValidationConfig(t *testing.T) {
	t.Helper()
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_A_TWR", Frequency: "118.105", Section: "TWR"},
		{Name: "EKCH_D_TWR", Frequency: "119.355", Section: "TWR"},
		{Name: "EKCH_C_TWR", Frequency: "118.580", Section: "TWR"},
	}))
	t.Cleanup(config.SetSectorsForTest([]config.Sector{
		{Name: "TE", Active: []string{"22L", "22R"}, Owner: []string{"EKCH_A_TWR", "EKCH_D_TWR"}},
		{Name: "TW", Active: []string{"22L", "22R"}, Owner: []string{"EKCH_D_TWR", "EKCH_A_TWR"}},
	}))
}

func buildLandingClearanceValidationSvc(t *testing.T, repo *testutil.MockStripRepository) *StripService {
	t.Helper()

	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
			assert.Equal(t, int32(1), id)
			return &models.Session{
				ID: 1,
				ActiveRunways: pkgModels.ActiveRunways{
					ArrivalRunways: []string{"22L", "22R"},
				},
			}, nil
		},
	}
	server := &testutil.MockServer{SessionRepoVal: sessionRepo}
	hub := &testutil.MockFrontendHub{}
	hub.SetServer(server)

	svc := NewStripService(repo)
	svc.SetFrontendHub(hub)
	return svc
}

func TestReevaluateLandingClearanceValidationsForSession_ActivatesForApplicableArrivalOwner(t *testing.T) {
	setupLandingClearanceValidationConfig(t)

	owner := "EKCH_A_TWR"
	finalSeq := int32(2000)
	twySeq := int32(1000)
	var persisted *models.ValidationStatus

	repo := &testutil.MockStripRepository{
		ListFn: func(context.Context, int32) ([]*models.Strip, error) {
			return []*models.Strip{
				{Callsign: "SAS123", Owner: &owner, Bay: shared.BAY_FINAL, Sequence: &finalSeq},
				{Callsign: "DLH456", Bay: shared.BAY_TWY_ARR, Sequence: &twySeq},
			}, nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, callsign string, status *models.ValidationStatus) error {
			assert.Equal(t, "SAS123", callsign)
			persisted = status
			return nil
		},
	}

	svc := buildLandingClearanceValidationSvc(t, repo)
	require.NoError(t, svc.ReevaluateLandingClearanceValidationsForSession(context.Background(), 1, false, true))

	require.NotNil(t, persisted)
	assert.Equal(t, landingClearanceValidationIssueType, persisted.IssueType)
	assert.Equal(t, landingClearanceValidationMessage, persisted.Message)
	assert.Equal(t, owner, persisted.OwningPosition)
	require.NotNil(t, persisted.CustomAction)
	assert.Equal(t, landingClearanceValidationActionKind, persisted.CustomAction.ActionKind)
	assert.Equal(t, landingClearanceValidationActionLabel, persisted.CustomAction.Label)
	assert.True(t, persisted.Active)
	assert.NotEmpty(t, persisted.ActivationKey)
}

func TestReevaluateLandingClearanceValidationsForSession_RequiresActivationEdge(t *testing.T) {
	setupLandingClearanceValidationConfig(t)

	owner := "EKCH_A_TWR"
	finalSeq := int32(2000)
	twySeq := int32(1000)
	setCalled := false

	repo := &testutil.MockStripRepository{
		ListFn: func(context.Context, int32) ([]*models.Strip, error) {
			return []*models.Strip{
				{Callsign: "SAS123", Owner: &owner, Bay: shared.BAY_FINAL, Sequence: &finalSeq},
				{Callsign: "DLH456", Bay: shared.BAY_TWY_ARR, Sequence: &twySeq},
			}, nil
		},
		SetValidationStatusFn: func(context.Context, int32, string, *models.ValidationStatus) error {
			setCalled = true
			return nil
		},
	}

	svc := buildLandingClearanceValidationSvc(t, repo)
	require.NoError(t, svc.ReevaluateLandingClearanceValidationsForSession(context.Background(), 1, false, false))
	assert.False(t, setCalled, "validation must not activate without a new TWY_ARR timer edge")
}

func TestReevaluateLandingClearanceValidationsForSession_ClearsResolvedValidation(t *testing.T) {
	setupLandingClearanceValidationConfig(t)

	owner := "EKCH_A_TWR"
	finalSeq := int32(2000)
	cleared := false

	repo := &testutil.MockStripRepository{
		ListFn: func(context.Context, int32) ([]*models.Strip, error) {
			return []*models.Strip{
				{
					Callsign:      "SAS123",
					Owner:         &owner,
					Bay:           shared.BAY_FINAL,
					Sequence:      &finalSeq,
					RunwayCleared: true,
					ValidationStatus: &models.ValidationStatus{
						IssueType:      landingClearanceValidationIssueType,
						Message:        landingClearanceValidationMessage,
						OwningPosition: owner,
						Active:         true,
						ActivationKey:  "edge-1",
						CustomAction:   landingClearanceValidationAction(),
					},
				},
			}, nil
		},
		ClearValidationStatusFn: func(_ context.Context, _ int32, callsign string) error {
			assert.Equal(t, "SAS123", callsign)
			cleared = true
			return nil
		},
	}

	svc := buildLandingClearanceValidationSvc(t, repo)
	require.NoError(t, svc.ReevaluateLandingClearanceValidationsForSession(context.Background(), 1, false, false))
	assert.True(t, cleared)
}

func TestMoveToBay_SchedulesLandingClearanceValidationOnlyOnTwyArrTransition(t *testing.T) {
	current := &models.Strip{Callsign: "SAS123", Bay: shared.BAY_RWY_ARR}
	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
			clone := *current
			return &clone, nil
		},
		GetMaxSequenceInBayFn: func(context.Context, int32, string) (int32, error) {
			return 0, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, bay string, _ int32) (int64, error) {
			current.Bay = bay
			return 1, nil
		},
		ListFn: func(context.Context, int32) ([]*models.Strip, error) {
			return []*models.Strip{}, nil
		},
	}

	var delays []time.Duration
	originalAfterFunc := landingClearanceValidationAfterFunc
	landingClearanceValidationAfterFunc = func(delay time.Duration, fn func()) {
		delays = append(delays, delay)
	}
	t.Cleanup(func() {
		landingClearanceValidationAfterFunc = originalAfterFunc
	})

	svc := NewStripService(repo)
	require.NoError(t, svc.MoveToBay(context.Background(), 1, "SAS123", shared.BAY_TWY_ARR, false))
	require.NoError(t, svc.MoveToBay(context.Background(), 1, "SAS123", shared.BAY_TWY_ARR, false))

	require.Len(t, delays, 1)
	assert.Equal(t, landingClearanceValidationDelay, delays[0])
}
