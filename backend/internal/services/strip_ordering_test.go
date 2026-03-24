package services

import (
	"context"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr32(v int32) *int32 { return &v }

func TestMidpoint_BasicCase(t *testing.T) {
	svc := NewStripService(&testutil.MockStripRepository{})
	order, needsRecalc := svc.calculateOrderBetween(100, ptr32(200))
	assert.Equal(t, int32(150), order)
	assert.False(t, needsRecalc)
}

func TestMidpoint_AdjacentValues(t *testing.T) {
	svc := NewStripService(&testutil.MockStripRepository{})
	// gap = 101 - 100 = 1, which is <= MinOrderGap (5)
	order, needsRecalc := svc.calculateOrderBetween(100, ptr32(101))
	assert.Equal(t, int32(0), order)
	assert.True(t, needsRecalc)
}

func TestMidpoint_GapExactlyMinOrderGap(t *testing.T) {
	svc := NewStripService(&testutil.MockStripRepository{})
	// gap = 5 which equals MinOrderGap — should also trigger recalculation
	order, needsRecalc := svc.calculateOrderBetween(100, ptr32(105))
	assert.Equal(t, int32(0), order)
	assert.True(t, needsRecalc)
}

func TestMidpoint_InsertAtEnd(t *testing.T) {
	svc := NewStripService(&testutil.MockStripRepository{})
	// nextOrder == nil => append after prevOrder
	order, needsRecalc := svc.calculateOrderBetween(1000, nil)
	assert.Equal(t, int32(1000+InitialOrderSpacing), order)
	assert.False(t, needsRecalc)
}

func TestRecalculateSequence_AssignsGaps(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const bay = "NOT_CLEARED"

	seq1 := int32(100)
	seq2 := int32(200)
	seq3 := int32(300)

	var capturedCallsigns []string
	var capturedSeqs []int32

	stripRepo := &testutil.MockStripRepository{
		RecalculateSequencesFn: func(_ context.Context, s int32, b string, spacing int32) error {
			assert.Equal(t, session, s)
			assert.Equal(t, bay, b)
			assert.Equal(t, int32(InitialOrderSpacing), spacing)
			return nil
		},
		ListSequencesFn: func(_ context.Context, s int32, b string) ([]*models.StripSequence, error) {
			return []*models.StripSequence{
				{Callsign: "AAA001", Sequence: &seq1},
				{Callsign: "AAA002", Sequence: &seq2},
				{Callsign: "AAA003", Sequence: &seq3},
			}, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := svc.recalculateFlightStripsOnly(ctx, session, bay)
	require.NoError(t, err)

	// recalculateFlightStripsOnly calls sendBulkSequenceUpdate which sends one bulk event
	require.Len(t, hub.BulkBayEvents, 1)
	bulkEvent := hub.BulkBayEvents[0]
	assert.Equal(t, bay, bulkEvent.Bay)
	require.Len(t, bulkEvent.Strips, 3)

	// The sequences from ListSequences are returned as-is (RecalculateSequences is the DB op)
	for _, entry := range bulkEvent.Strips {
		capturedCallsigns = append(capturedCallsigns, entry.Callsign)
		capturedSeqs = append(capturedSeqs, entry.Sequence)
	}

	assert.Equal(t, []string{"AAA001", "AAA002", "AAA003"}, capturedCallsigns)
	assert.Equal(t, []int32{100, 200, 300}, capturedSeqs)
}

func TestNeedsRecalculation_SmallGap(t *testing.T) {
	svc := NewStripService(&testutil.MockStripRepository{})
	assert.True(t, svc.needsRecalculation(100, 104))
}

func TestNeedsRecalculation_LargeGap(t *testing.T) {
	svc := NewStripService(&testutil.MockStripRepository{})
	assert.False(t, svc.needsRecalculation(100, 200))
}

func TestMoveToBay_CalculatesCorrectSequence(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "EZY123"
	const bay = shared.BAY_NOT_CLEARED

	var capturedBay string
	var capturedSeq int32

	stripRepo := &testutil.MockStripRepository{
		GetMaxSequenceInBayFn: func(_ context.Context, _ int32, _ string) (int32, error) {
			return int32(1000), nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, b string, seq int32) (int64, error) {
			capturedBay = b
			capturedSeq = seq
			return 1, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := svc.MoveToBay(ctx, session, callsign, bay, true)
	require.NoError(t, err)
	assert.Equal(t, bay, capturedBay)
	assert.Equal(t, int32(1000+InitialOrderSpacing), capturedSeq)
	require.Len(t, hub.BayEvents, 1)
	assert.Equal(t, callsign, hub.BayEvents[0].Callsign)
}
