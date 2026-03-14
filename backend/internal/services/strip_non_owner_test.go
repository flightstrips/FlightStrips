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

// TestHandleReleasePoint_NonOwner_ExistingValue_MarksUnexpected verifies that when
// a non-owner overwrites an existing release point the field is marked as an
// unexpected change and the update is applied.
func TestHandleReleasePoint_NonOwner_ExistingValue_MarksUnexpected(t *testing.T) {
	ctx := context.Background()

	var appendedField string
	var updatedReleasePoint string

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign:     testCallsign,
				Owner:        ptr(ownerPosition),
				ReleasePoint: ptr("K"),
			}, nil
		},
		AppendUnexpectedChangeFieldFn: func(_ context.Context, _ int32, _ string, field string) error {
			appendedField = field
			return nil
		},
		UpdateReleasePointFn: func(_ context.Context, _ int32, _ string, rp *string) (int64, error) {
			updatedReleasePoint = *rp
			return 1, nil
		},
	}

	svc, hub := newReleasePointFixture(stripRepo)

	err := svc.ApplyReleasePoint(ctx, testSession, testCallsign, "L", nonOwnerPosition)
	require.NoError(t, err)

	assert.Equal(t, "release_point", appendedField, "expected release_point to be marked as unexpected change")
	assert.Equal(t, "L", updatedReleasePoint, "expected release point to be updated to L")
	require.Len(t, hub.StripUpdates, 1, "expected strip broadcast after unexpected change")
	assert.Equal(t, testCallsign, hub.StripUpdates[0].Callsign)
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

// TestHandleStripUpdate_NonOwner_TwyOverwrite_MarksUnexpected verifies the same
// non-owner overwrite behaviour for the TWY/clearance-limit case (TWY and HP both
// share the release_point field).
func TestHandleStripUpdate_NonOwner_TwyOverwrite_MarksUnexpected(t *testing.T) {
	ctx := context.Background()

	var appendedField string

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign:     testCallsign,
				Owner:        ptr(ownerPosition),
				ReleasePoint: ptr("TWY-B"), // existing clearance limit
			}, nil
		},
		AppendUnexpectedChangeFieldFn: func(_ context.Context, _ int32, _ string, field string) error {
			appendedField = field
			return nil
		},
		UpdateReleasePointFn: func(_ context.Context, _ int32, _ string, _ *string) (int64, error) {
			return 1, nil
		},
	}

	svc, hub := newReleasePointFixture(stripRepo)

	err := svc.ApplyReleasePoint(ctx, testSession, testCallsign, "TWY-C", nonOwnerPosition)
	require.NoError(t, err)

	assert.Equal(t, "release_point", appendedField)
	require.Len(t, hub.StripUpdates, 1)
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
