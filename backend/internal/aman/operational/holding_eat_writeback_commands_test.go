package operational

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestHoldingEATWritebackSettingIsAuthorizedRevisionedAndRetained(t *testing.T) {
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	repository := &memoryRepository{has: true, state: aman.AirportState{Airport: "EKCH", Revision: 7, GeneratedAt: now, PolicyVersion: "settings-v1", Mode: aman.ModeAuthoritative}}
	publisher := &recordingPublisher{}
	actions := runwayGapActions(t, repository, publisher, now.Add(time.Second))
	auth := closureAuth(now)
	command := aman.SetHoldingEATWritebackCommand{Metadata: aman.CommandMetadata{CommandID: "disable-eat", ExpectedRevision: 7}, Enabled: false}
	unauthorized := auth
	unauthorized.Role = "EKCH_APP"
	_, err := actions.SetHoldingEATWriteback(context.Background(), unauthorized, command)
	requireDomainErrorClass(t, err, aman.ErrorUnauthorized)
	require.Nil(t, repository.state.HoldingEATWritebackEnabled)
	result, err := actions.SetHoldingEATWriteback(context.Background(), auth, command)
	require.NoError(t, err)
	require.True(t, result.Changed)
	require.Equal(t, aman.SequenceRevision(8), result.CurrentRevision)
	require.NotNil(t, repository.state.HoldingEATWritebackEnabled)
	require.False(t, *repository.state.HoldingEATWritebackEnabled)
	restarted := runwayGapActions(t, repository, publisher, now.Add(2*time.Second))
	retry, err := restarted.SetHoldingEATWriteback(context.Background(), auth, command)
	require.NoError(t, err)
	require.True(t, retry.Duplicate)
	command.Metadata.CommandID = "stale-eat"
	_, err = restarted.SetHoldingEATWriteback(context.Background(), auth, command)
	requireDomainErrorClass(t, err, aman.ErrorRevisionConflict)
	command.Metadata = aman.CommandMetadata{CommandID: "enable-eat", ExpectedRevision: 8}
	command.Enabled = true
	_, err = restarted.SetHoldingEATWriteback(context.Background(), auth, command)
	require.NoError(t, err)
	require.True(t, *repository.state.HoldingEATWritebackEnabled)
}

func TestHoldingEATWritebackToggleRetainsQueueOffersAtNewRevision(t *testing.T) {
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	teta := now.Add(10 * time.Minute)
	state := aman.AirportState{Airport: "EKCH", Revision: 7, GeneratedAt: now, PolicyVersion: "settings-v1", Mode: aman.ModeAuthoritative,
		RunwayGroups: []aman.RunwayGroupPolicy{{ID: "ARRIVAL-22", ActiveRatePerHour: 30, RateEffectiveAt: &now}},
		Flights: []aman.AMANFlight{
			gapCommandFlight("LEAD", "ARRIVAL-22", teta, 1, aman.StateUnstable, aman.FreezeNone),
			gapCommandFlight("TARGET", "ARRIVAL-22", teta, 2, aman.StateUnstable, aman.FreezeNone),
		}}
	for i := range state.Flights {
		state.Flights[i].UpdatedAt = now
		state.Flights[i].Slot = &aman.Slot{Time: teta.Add(time.Duration(i) * 4 * time.Minute), RunwayGroupID: "ARRIVAL-22", Sequence: i + 1, Revision: 7, Reason: "rate_wtc"}
	}
	service := &Service{}
	state, err := sequence.ProjectQueueOffers(state, service.sequenceInput(state), sequence.QueueOfferConfig{Validity: queueOfferValidity}, now)
	require.NoError(t, err)
	require.NotEmpty(t, state.Flights[1].QueueOffers, "fixture must have actual queue offers before toggling")
	repository := &memoryRepository{has: true, state: state}
	publisher := &recordingPublisher{}
	actions := runwayGapActions(t, repository, publisher, now.Add(time.Second))
	for i, enabled := range []bool{false, true} {
		_, err = actions.SetHoldingEATWriteback(context.Background(), closureAuth(now), aman.SetHoldingEATWritebackCommand{
			Metadata: aman.CommandMetadata{CommandID: []string{"disable-with-offers", "enable-with-offers"}[i], ExpectedRevision: repository.state.Revision}, Enabled: enabled,
		})
		require.NoError(t, err)
		currentOffers := repository.state.Flights[1].QueueOffers
		require.NotEmpty(t, currentOffers, "toggle must retain queue offers without waiting for reconciliation")
		for _, offer := range currentOffers {
			require.Equal(t, repository.state.Revision, offer.AirportRevision)
		}
		require.Equal(t, state.Flights[1].Prediction, repository.state.Flights[1].Prediction)
		require.Equal(t, state.Flights[1].Slot.Time, repository.state.Flights[1].Slot.Time)
	}
}
