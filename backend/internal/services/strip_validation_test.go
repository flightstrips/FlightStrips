package services

import (
	"context"
	"fmt"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/testutil"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type validationStoreFake struct {
	getByCallsignFn               func(ctx context.Context, session int32, callsign string) (*models.Strip, error)
	setValidationStatusFn         func(ctx context.Context, session int32, callsign string, status *models.ValidationStatus) error
	acknowledgeValidationStatusFn func(ctx context.Context, session int32, callsign string, activationKey string) (int64, error)
	clearValidationStatusFn       func(ctx context.Context, session int32, callsign string) error
}

func (f *validationStoreFake) GetByCallsign(ctx context.Context, session int32, callsign string) (*models.Strip, error) {
	if f.getByCallsignFn == nil {
		panic("unexpected call to validationStoreFake.GetByCallsign")
	}
	return f.getByCallsignFn(ctx, session, callsign)
}

func (f *validationStoreFake) SetValidationStatus(ctx context.Context, session int32, callsign string, status *models.ValidationStatus) error {
	if f.setValidationStatusFn == nil {
		panic("unexpected call to validationStoreFake.SetValidationStatus")
	}
	return f.setValidationStatusFn(ctx, session, callsign, status)
}

func (f *validationStoreFake) CompareAndSetStandValidation(ctx context.Context, session int32, callsign string, update models.StandValidationUpdate) (bool, error) {
	if update.Next == nil {
		return true, f.ClearValidationStatus(ctx, session, callsign)
	}
	return true, f.SetValidationStatus(ctx, session, callsign, update.Next)
}

func (f *validationStoreFake) AcknowledgeValidationStatus(ctx context.Context, session int32, callsign string, activationKey string) (int64, error) {
	if f.acknowledgeValidationStatusFn == nil {
		panic("unexpected call to validationStoreFake.AcknowledgeValidationStatus")
	}
	return f.acknowledgeValidationStatusFn(ctx, session, callsign, activationKey)
}

func (f *validationStoreFake) ClearValidationStatus(ctx context.Context, session int32, callsign string) error {
	if f.clearValidationStatusFn == nil {
		panic("unexpected call to validationStoreFake.ClearValidationStatus")
	}
	return f.clearValidationStatusFn(ctx, session, callsign)
}

type readerOnlyStripFake struct{}

func (readerOnlyStripFake) GetByCallsign(ctx context.Context, session int32, callsign string) (*models.Strip, error) {
	return &models.Strip{Callsign: callsign}, nil
}

func TestAcknowledgeValidationStatus_AllowsPdcValidationForNonOwnerPosition(t *testing.T) {
	t.Parallel()

	var acknowledged bool
	repo := &validationStoreFake{
		getByCallsignFn: func(_ context.Context, _ int32, callsign string) (*models.Strip, error) {
			assert.Equal(t, "SAS123", callsign)
			return &models.Strip{
				Callsign: "SAS123",
				ValidationStatus: &models.ValidationStatus{
					IssueType:      pdcInvalidValidationIssueType,
					OwningPosition: "EKCH_DEL",
					Active:         true,
					ActivationKey:  "activation-key",
				},
			}, nil
		},
		acknowledgeValidationStatusFn: func(_ context.Context, _ int32, callsign string, activationKey string) (int64, error) {
			assert.Equal(t, "SAS123", callsign)
			assert.Equal(t, "activation-key", activationKey)
			acknowledged = true
			return 1, nil
		},
	}

	svc := newTestStripValidationService(repo, repo)
	svc.publisher = &testutil.MockFrontendHub{}

	require.NoError(t, svc.AcknowledgeValidationStatus(context.Background(), 1, "SAS123", "activation-key", "EKCH_GND"))
	assert.True(t, acknowledged)
}

func TestAcknowledgeValidationStatus_RejectsNonOwnerForNonPdcValidation(t *testing.T) {
	t.Parallel()

	repo := &validationStoreFake{
		getByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign: "SAS123",
				ValidationStatus: &models.ValidationStatus{
					IssueType:      "RUNWAY TYPE",
					OwningPosition: "EKCH_DEL",
					Active:         true,
					ActivationKey:  "activation-key",
				},
			}, nil
		},
	}

	svc := newTestStripValidationService(repo, repo)

	err := svc.AcknowledgeValidationStatus(context.Background(), 1, "SAS123", "activation-key", "EKCH_GND")
	require.Error(t, err)
	assert.EqualError(t, err, "acknowledge_validation_status: requesting position is not the owning position")
}

func TestNewStripValidationServiceRejectsMissingStore(t *testing.T) {
	t.Parallel()

	_, err := NewStripValidationService(StripValidationDependencies{
		Strips: readerOnlyStripFake{}, Publisher: testStripValidationPublisher{},
	})

	require.Error(t, err)
	assert.EqualError(t, err, "strip validation service requires validation status store")
}

func TestReconcileStandAssignmentValidationActivatesBlockedAssignment(t *testing.T) {
	owner := "EKCH_APP"
	var persisted *models.ValidationStatus
	repo := &validationStoreFake{
		getByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{Callsign: "SAS123", Owner: &owner}, nil
		},
		setValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
			persisted = status
			return nil
		},
	}
	svc := newTestStripValidationService(repo, repo)

	require.NoError(t, svc.ReconcileStandAssignmentValidation(context.Background(), 1, "SAS123", "ARRIVAL", []string{"A22"}, "", models.StandValidationAssignments{Target: models.StandAssignmentRevision{ID: 10, Version: 7}}))
	require.NotNil(t, persisted)
	require.Equal(t, models.ValidationIssueTypeStandAssignment, persisted.IssueType)
	require.Equal(t, "Assigned stand is blocked by A22.", persisted.Message)
	require.Equal(t, owner, persisted.OwningPosition)
	require.True(t, persisted.Active)
	require.Equal(t, "assign_stand", persisted.CustomAction.ActionKind)
	require.JSONEq(t, `{"assignment_id":10,"assignment_version":7}`, string(persisted.CustomAction.Payload))
}

func TestReconcileStandAssignmentValidationIgnoresRemovedStrip(t *testing.T) {
	repo := &validationStoreFake{
		getByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
	}
	svc := newTestStripValidationService(repo, repo)

	require.NoError(t, svc.ReconcileStandAssignmentValidation(context.Background(), 1, "SAS123", "ARRIVAL", nil, "", models.StandValidationAssignments{}))
}

func TestReconcileStandAssignmentValidationClearsResolvedSatIssueOnly(t *testing.T) {
	repo := &validationStoreFake{
		getByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{Callsign: "SAS123", ValidationStatus: &models.ValidationStatus{IssueType: models.ValidationIssueTypeStandAssignment}}, nil
		},
		clearValidationStatusFn: func(_ context.Context, _ int32, callsign string) error {
			require.Equal(t, "SAS123", callsign)
			return nil
		},
	}
	svc := newTestStripValidationService(repo, repo)

	require.NoError(t, svc.ReconcileStandAssignmentValidation(context.Background(), 1, "SAS123", "ARRIVAL", nil, "", models.StandValidationAssignments{}))
}

func TestReconcileStandAssignmentValidationSuppressesWrongStandWorkflow(t *testing.T) {
	cleared := false
	repo := &validationStoreFake{
		getByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{Callsign: "SAS123", ValidationStatus: &models.ValidationStatus{IssueType: models.ValidationIssueTypeStandAssignment}}, nil
		},
		clearValidationStatusFn: func(_ context.Context, _ int32, callsign string) error {
			assert.Equal(t, "SAS123", callsign)
			cleared = true
			return nil
		},
	}
	svc := newTestStripValidationService(repo, repo)

	require.NoError(t, svc.ReconcileStandAssignmentValidation(context.Background(), 1, "SAS123", "DEPARTURE", []string{"A2"}, wrongStandPendingPrefix+"A2", models.StandValidationAssignments{Target: models.StandAssignmentRevision{ID: 10, Version: 1}}))
	assert.True(t, cleared, "the wrong-stand workflow must not leave a controller validation behind")
}

func TestReconcileStandAssignmentValidationSuppressesObservedDepartureConflict(t *testing.T) {
	cleared := false
	repo := &validationStoreFake{
		getByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{Callsign: "SAS503", ValidationStatus: &models.ValidationStatus{IssueType: models.ValidationIssueTypeStandAssignment}}, nil
		},
		clearValidationStatusFn: func(_ context.Context, _ int32, callsign string) error {
			assert.Equal(t, "SAS503", callsign)
			cleared = true
			return nil
		},
	}
	svc := newTestStripValidationService(repo, repo)

	require.NoError(t, svc.ReconcileStandAssignmentValidation(
		context.Background(),
		1,
		"SAS503",
		"DEPARTURE",
		[]string{"D3"},
		observedDepartureConflictPrefix+" reserved by NSZ3525",
		models.StandValidationAssignments{Target: models.StandAssignmentRevision{ID: 10, Version: 1}},
	))
	assert.True(t, cleared, "the parked departure must not receive a validation for an inbound reservation conflict")
}

func TestReconcileStandAssignmentValidationSuppressesEveryDepartureConflict(t *testing.T) {
	setCalled := false
	repo := &validationStoreFake{
		getByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{Callsign: "SAS503"}, nil
		},
		setValidationStatusFn: func(_ context.Context, _ int32, _ string, _ *models.ValidationStatus) error {
			setCalled = true
			return nil
		},
	}
	svc := newTestStripValidationService(repo, repo)

	require.NoError(t, svc.ReconcileStandAssignmentValidation(context.Background(), 1, "SAS503", "DEPARTURE", []string{"D3"}, "generic conflict", models.StandValidationAssignments{Target: models.StandAssignmentRevision{ID: 10, Version: 1}}))
	assert.False(t, setCalled, "departure assignments must not produce stand validations")
}

func TestNewStripValidationServiceRejectsMissingReader(t *testing.T) {
	t.Parallel()

	_, err := NewStripValidationService(StripValidationDependencies{})

	require.Error(t, err)
	assert.EqualError(t, err, "strip validation service requires strip reader")
}

func TestStandValidationRejectsStalePublications(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	for _, scenario := range []string{"removed pin", "replaced pin", "higher priority validation", "acknowledgement during refresh"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := t.Context()
			session := testdata.SeedTestSessionNamedWithSectors(t, queries, t.Name(), nil)
			const callsign = "SAS910"
			seedTestArrivalStrip(t, queries, session, callsign)
			strips := postgres.NewStripRepository(pool)
			assignments := postgres.NewStandAssignmentRepository(pool)
			validation := newTestStripValidationService(strips, strips)
			observed := &standWarningReadHook{StripRepository: strips}
			delayed := newTestStripValidationService(observed, strips)

			pin := &models.StandAssignment{
				SessionID: session, Callsign: callsign, Stand: "A1", Direction: "ARRIVAL",
				Stage: StageEstimated, Manual: true, Source: "MANUAL",
			}
			require.NoError(t, assignments.CreateAssignment(ctx, pin))
			update := func(service *StripValidationService, revision models.StandAssignmentRevision, reason string) {
				t.Helper()
				require.NoError(t, service.ReconcileStandAssignmentValidation(ctx, session, callsign, "ARRIVAL", nil, reason, models.StandValidationAssignments{Target: revision}))
			}
			remove := func() {
				t.Helper()
				deleted, err := assignments.DeleteAssignment(ctx, session, pin.ID, pin.Version)
				require.NoError(t, err)
				require.EqualValues(t, 1, deleted)
				update(validation, models.StandAssignmentRevision{}, "")
			}
			load := func() *models.ValidationStatus {
				t.Helper()
				strip, err := strips.GetByCallsign(ctx, session, callsign)
				require.NoError(t, err)
				return strip.ValidationStatus
			}
			const reason = "Pinned stand A1 is occupied"

			switch scenario {
			case "removed pin":
				observed.afterRead = func() {
					update(validation, pin.Revision(), reason)
					remove()
				}
				update(delayed, pin.Revision(), reason)
				require.Nil(t, load())
			case "replaced pin":
				update(validation, pin.Revision(), reason)
				var replacementKey string
				observed.afterRead = func() {
					remove()
					replacement := *pin
					replacement.Stand = "A2"
					require.NoError(t, assignments.CreateAssignment(ctx, &replacement))
					update(validation, replacement.Revision(), "Pinned stand A2 is occupied")
					replacementKey = load().ActivationKey
				}
				update(delayed, models.StandAssignmentRevision{}, "")
				current := load()
				require.NotNil(t, current)
				assert.Equal(t, "Pinned stand A2 is occupied", current.Message)
				assert.Equal(t, replacementKey, current.ActivationKey)
			case "higher priority validation":
				observed.afterRead = func() {
					require.NoError(t, validation.SetValidationStatus(ctx, session, callsign, &models.ValidationStatus{
						IssueType: models.ValidationIssueTypeReadyTransferTsat, Message: "Request start", Active: true,
					}))
				}
				update(delayed, pin.Revision(), reason)
				current := load()
				require.NotNil(t, current)
				assert.Equal(t, models.ValidationIssueTypeReadyTransferTsat, current.IssueType)
			case "acknowledgement during refresh":
				update(validation, pin.Revision(), reason)
				warning := load()
				require.NotNil(t, warning)
				rows, err := assignments.UpdateAssignment(ctx, pin)
				require.NoError(t, err)
				require.EqualValues(t, 1, rows)
				pin.Version++
				observed.afterRead = func() {
					require.NoError(t, validation.AcknowledgeValidationStatus(ctx, session, callsign, warning.ActivationKey, warning.OwningPosition))
				}
				update(delayed, pin.Revision(), reason)
				current := load()
				require.NotNil(t, current)
				require.NotNil(t, current.CustomAction)
				assert.False(t, current.Active)
				assert.Equal(t, warning.ActivationKey, current.ActivationKey)
				require.JSONEq(t, fmt.Sprintf(`{"assignment_id":%d,"assignment_version":%d}`, pin.ID, pin.Version), string(current.CustomAction.Payload))
			}
		})
	}
}

type standWarningReadHook struct {
	repository.StripRepository
	afterRead func()
}

func (r *standWarningReadHook) GetByCallsign(ctx context.Context, session int32, callsign string) (*models.Strip, error) {
	strip, err := r.StripRepository.GetByCallsign(ctx, session, callsign)
	if r.afterRead != nil {
		r.afterRead()
		r.afterRead = nil
	}
	return strip, err
}
