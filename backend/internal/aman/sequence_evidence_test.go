package aman

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSequenceDemandPredictionTiming(t *testing.T) {
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	f := AMANFlight{Slot: &Slot{Time: at.Add(time.Hour)}}
	require.Nil(t, SequenceDemandArrivalAt(f))
	f.Prediction = &Prediction{Publishable: true, RawTETA: at, OperationalTETA: at.Add(time.Minute)}
	require.Equal(t, at, *SequenceDemandArrivalAt(f))
	f.Prediction.RawTETA = time.Time{}
	require.Equal(t, at.Add(time.Minute), *SequenceDemandArrivalAt(f))
	f.Prediction.Publishable = false
	require.Nil(t, SequenceDemandArrivalAt(f))
}

func TestConfirmedHoldingRequiresCurrentEvidence(t *testing.T) {
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	id := "MONAK"
	f := AMANFlight{SelectedHolding: &id, Prediction: &Prediction{HoldingFixETA: &at, GeneratedAt: at}}
	require.Nil(t, ConfirmedActiveHoldingSince(f), "assignment and approach do not activate spacing")
	f.HoldingStack = &HoldingStackState{HoldingID: id, FirstObservedAt: at}
	require.Nil(t, ConfirmedActiveHoldingSince(f), "unconfirmed footprint does not activate spacing")
	f.HoldingStack.Confirmed = true
	require.Equal(t, at, *ConfirmedActiveHoldingSince(f))
	f.HoldingClearanceCanceledAt = &at
	require.Nil(t, ConfirmedActiveHoldingSince(f), "cancellation supersedes retained evidence")
	newer := at.Add(time.Second)
	f.Prediction.InputObservedAt = newer
	require.NotNil(t, ConfirmedActiveHoldingSince(f), "new physical evidence can establish holding again")
	f.Prediction.HoldingFixETA = nil
	require.Nil(t, ConfirmedActiveHoldingSince(f), "departed hold does not activate spacing")
	f.Prediction.HoldingFixETA = &at
	other := "TUDLO"
	f.SelectedHolding = &other
	require.Nil(t, ConfirmedActiveHoldingSince(f), "old stack cannot activate a new selected hold")
}
