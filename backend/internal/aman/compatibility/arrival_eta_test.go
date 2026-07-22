package compatibility

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/models"
	"github.com/stretchr/testify/require"
)

func TestProjectArrivalETAHonoursOwnershipAndAvailability(t *testing.T) {
	now := time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
	prediction := &aman.Prediction{OperationalTETA: now.Add(20 * time.Minute), GeneratedAt: now, Publishable: true}

	for _, mode := range []aman.RolloutMode{aman.ModeDisabled, aman.ModeShadow} {
		require.Nil(t, ProjectArrivalETA(mode, prediction, now, time.Minute))
	}
	for _, mode := range []aman.RolloutMode{aman.ModeReadOnly, aman.ModeAuthoritative} {
		eta := ProjectArrivalETA(mode, prediction, now, time.Minute)
		require.NotNil(t, eta)
		require.Equal(t, prediction.OperationalTETA, eta.Time)
		require.Equal(t, ArrivalETASource, eta.Source)
		require.Equal(t, prediction.GeneratedAt, eta.CalculatedAt)
	}

	prediction.Publishable = false
	require.Nil(t, ProjectArrivalETA(aman.ModeReadOnly, prediction, now, time.Minute))
	prediction.Publishable = true
	require.Nil(t, ProjectArrivalETA(aman.ModeReadOnly, prediction, now.Add(2*time.Minute), time.Minute))
}

func TestWriterOwnsOperationalModesAndClearsUnavailableOutput(t *testing.T) {
	now := time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
	store := &recordingArrivalETAStore{}
	writer := NewWriter(aman.ModeReadOnly, time.Minute, store)
	prediction := &aman.Prediction{OperationalTETA: now.Add(20 * time.Minute), GeneratedAt: now, Publishable: true}

	changed, err := writer.Apply(context.Background(), 7, "SAS123", prediction, now)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 1, store.updated)
	require.Equal(t, ArrivalETASource, store.eta.Source)

	prediction.Publishable = false
	changed, err = writer.Apply(context.Background(), 7, "SAS123", prediction, now)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 1, store.cleared)

	shadow := NewWriter(aman.ModeShadow, time.Minute, store)
	changed, err = shadow.Apply(context.Background(), 7, "SAS123", prediction, now)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, 1, store.cleared)
}

type recordingArrivalETAStore struct {
	eta     models.ArrivalETA
	updated int
	cleared int
}

func (s *recordingArrivalETAStore) UpdateArrivalETA(_ context.Context, _ int32, _ string, eta models.ArrivalETA) (int64, error) {
	s.eta = eta
	s.updated++
	return 1, nil
}

func (s *recordingArrivalETAStore) ClearArrivalETA(context.Context, int32, string) (int64, error) {
	s.cleared++
	return 1, nil
}
