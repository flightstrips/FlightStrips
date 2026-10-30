package services

import (
	"context"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	ownerPosition    = "119.905"
	nonOwnerPosition = "121.630"
	testSession      = int32(42)
	testCallsign     = "SAS101"
)

func ptr[T any](v T) *T { return &v }

// newReleasePointFixture builds a minimal StripService wired to the given mock repo.
func newReleasePointFixture(stripRepo *testutil.MockStripRepository) (*StripService, *testutil.MockFrontendHub) {
	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)
	return svc, hub
}

// ── ApplyReleasePoint (maps to handleReleasePoint) ───────────────────────────

// TestHandleReleasePoint_NonOwner_ExistingValue_Rejected verifies that a non-owner
// must use the coordination proposal flow instead of changing the assigned point.
func TestHandleReleasePoint_NonOwner_ExistingValue_Rejected(t *testing.T) {
	ctx := context.Background()

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign:     testCallsign,
				Owner:        ptr(ownerPosition),
				ReleasePoint: ptr("K"),
			}, nil
		},
	}

	svc, hub := newReleasePointFixture(stripRepo)

	err := svc.ApplyReleasePoint(ctx, testSession, testCallsign, "L", nonOwnerPosition)
	require.ErrorContains(t, err, "cannot modify release point on unowned strip")
	assert.Empty(t, hub.StripUpdates)
}

func TestHandleReleasePoint_NonOwner_ExistingValueWithActiveValidation_Rejected(t *testing.T) {
	ctx := context.Background()

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign:     testCallsign,
				Owner:        ptr(ownerPosition),
				ReleasePoint: ptr("K"),
				ValidationStatus: &models.ValidationStatus{
					IssueType:      "RUNWAY TYPE",
					OwningPosition: ownerPosition,
					Active:         true,
					ActivationKey:  "validation-key",
				},
			}, nil
		},
	}

	svc, hub := newReleasePointFixture(stripRepo)

	err := svc.ApplyReleasePoint(ctx, testSession, testCallsign, "L", nonOwnerPosition)
	require.ErrorContains(t, err, "cannot modify release point on unowned strip")
	assert.Empty(t, hub.StripUpdates)
}

// TestHandleReleasePoint_NonOwner_NoExistingValue_Rejected verifies that a non-owner
// cannot set a release point on a strip that has none.
func TestHandleReleasePoint_NonOwner_NoExistingValue_Rejected(t *testing.T) {
	ctx := context.Background()

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign:     testCallsign,
				Owner:        ptr(ownerPosition),
				ReleasePoint: nil,
			}, nil
		},
		// UpdateReleasePointFn intentionally NOT set — panics if called
		// AppendUnexpectedChangeFieldFn intentionally NOT set — panics if called
	}

	svc, hub := newReleasePointFixture(stripRepo)

	err := svc.ApplyReleasePoint(ctx, testSession, testCallsign, "K", nonOwnerPosition)
	require.Error(t, err, "expected error when non-owner sets release point with no existing value")

	assert.Empty(t, hub.StripUpdates, "expected no broadcast on rejection")
}

// TestHandleReleasePoint_Owner_NoUnexpectedChange verifies that the strip owner can
// update the release point without triggering an unexpected change mark.
func TestHandleReleasePoint_Owner_NoUnexpectedChange(t *testing.T) {
	ctx := context.Background()

	var updatedReleasePoint string

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign:     testCallsign,
				Owner:        ptr(ownerPosition),
				ReleasePoint: ptr("K"),
			}, nil
		},
		UpdateReleasePointFn: func(_ context.Context, _ int32, _ string, rp *string) (int64, error) {
			updatedReleasePoint = *rp
			return 1, nil
		},
		// AppendUnexpectedChangeFieldFn intentionally NOT set — panics if called
	}

	svc, hub := newReleasePointFixture(stripRepo)

	err := svc.ApplyReleasePoint(ctx, testSession, testCallsign, "L", ownerPosition)
	require.NoError(t, err)

	assert.Equal(t, "L", updatedReleasePoint)
	assert.Empty(t, hub.StripUpdates, "owner update must not trigger extra strip broadcast")
}

func TestProposeReleasePoint_StoresSuggestionWithoutChangingAssignment(t *testing.T) {
	var savedCallsign, savedOwner, savedPoint, savedBy string
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{Callsign: testCallsign, Owner: ptr(ownerPosition), ReleasePoint: ptr("K")}, nil
		},
		SetReleasePointProposalFn: func(_ context.Context, _ int32, callsign, owner, releasePoint, proposedBy string) (int64, error) {
			savedCallsign, savedOwner, savedPoint, savedBy = callsign, owner, releasePoint, proposedBy
			return 1, nil
		},
	}
	svc, hub := newReleasePointFixture(stripRepo)

	require.NoError(t, svc.ProposeReleasePoint(context.Background(), testSession, testCallsign, "L", nonOwnerPosition))
	assert.Equal(t, testCallsign, savedCallsign)
	assert.Equal(t, ownerPosition, savedOwner)
	assert.Equal(t, "L", savedPoint)
	assert.Equal(t, nonOwnerPosition, savedBy)
	require.Len(t, hub.StripUpdates, 1)
	assert.Equal(t, testCallsign, hub.StripUpdates[0].Callsign)
}

func TestApplyReleasePoint_OwnerResolvesProposalAndSetsManualPoint(t *testing.T) {
	var resolvedOwner, resolvedPoint string
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign:             testCallsign,
				Owner:                ptr(ownerPosition),
				ReleasePoint:         ptr("K"),
				ReleasePointProposal: ptr("L"),
			}, nil
		},
		ResolveReleasePointProposalFn: func(_ context.Context, _ int32, _ string, owner, releasePoint string) (int64, error) {
			resolvedOwner, resolvedPoint = owner, releasePoint
			return 1, nil
		},
	}
	svc, hub := newReleasePointFixture(stripRepo)

	require.NoError(t, svc.ApplyReleasePoint(context.Background(), testSession, testCallsign, "M", ownerPosition))
	assert.Equal(t, ownerPosition, resolvedOwner)
	assert.Equal(t, "M", resolvedPoint)
	require.Len(t, hub.StripUpdates, 1)
	assert.Equal(t, testCallsign, hub.StripUpdates[0].Callsign)
}

// TestHandleStripUpdate_NonOwner_TwyOverwrite_Rejected verifies non-owners cannot
// directly overwrite the shared TWY/HP release-point field.
func TestHandleStripUpdate_NonOwner_TwyOverwrite_Rejected(t *testing.T) {
	ctx := context.Background()

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign:     testCallsign,
				Owner:        ptr(ownerPosition),
				ReleasePoint: ptr("TWY-B"), // existing clearance limit
			}, nil
		},
	}

	svc, hub := newReleasePointFixture(stripRepo)

	err := svc.ApplyReleasePoint(ctx, testSession, testCallsign, "TWY-C", nonOwnerPosition)
	require.ErrorContains(t, err, "cannot modify release point on unowned strip")
	assert.Empty(t, hub.StripUpdates)
}

// TestHandleStripUpdate_NonOwner_SidChange_Rejected verifies that a non-owner
// attempting to set a release point on a strip without an existing one is rejected,
// mirroring the general rule that non-owners cannot modify unowned-strip fields.
// (SID and other EuroScope-forwarded fields are rejected at the handler level; this
// test covers the service-layer ownership guard via the release_point path.)
func TestHandleStripUpdate_NonOwner_SidChange_Rejected(t *testing.T) {
	ctx := context.Background()

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign:     testCallsign,
				Owner:        ptr(ownerPosition),
				ReleasePoint: nil, // no existing release point
			}, nil
		},
		// UpdateReleasePointFn intentionally NOT set
	}

	svc, hub := newReleasePointFixture(stripRepo)

	// Attempting any write by a non-owner on a fresh strip is rejected.
	err := svc.ApplyReleasePoint(ctx, testSession, testCallsign, "K", nonOwnerPosition)
	require.Error(t, err)
	assert.Empty(t, hub.StripUpdates)
}
