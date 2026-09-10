package sequence_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"github.com/stretchr/testify/require"
)

func TestQueueOffersRespectToleranceAndAssignDeterministicPositions(t *testing.T) {
	start := testTime()
	policy := queuePolicy("A", start, 60)
	policy.EarlyTolerance = 90 * time.Second
	input := sequence.Input{Revision: 7, Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{
		queueFlight("LEAD", "A", start, "M", 1, start),
		queueFlight("OCCUPIED", "A", start.Add(time.Minute), "M", 2, start.Add(time.Minute)),
		queueFlight("FIRST", "A", start.Add(2*time.Minute), "M", 3, start.Add(2*time.Minute)),
		queueFlight("SECOND", "A", start.Add(2*time.Minute), "M", 4, start.Add(3*time.Minute)),
	}}
	bindQueueRevision(&input)

	offers, err := sequence.CalculateQueueOffers(input, sequence.QueueOfferConfig{Validity: 10 * time.Minute}, start.Add(-time.Minute))
	require.NoError(t, err)
	candidateTwo := make([]aman.QueueOffer, 0, 2)
	for _, offer := range offers {
		if offer.CandidateSlot.Sequence == 2 {
			candidateTwo = append(candidateTwo, offer)
		}
		if offer.FlightID == "FIRST" {
			require.NotEqual(t, 1, offer.CandidateSlot.Sequence, "slot outside operational TETA tolerance must not be offered")
		}
	}
	require.Equal(t, []aman.QueueOffer{
		queueOffer("FIRST", "A", 2, start.Add(time.Minute), 1, 7, start.Add(time.Minute)),
	}, candidateTwo)
}

func TestQueueOffersValidateBothWTCBoundaries(t *testing.T) {
	start := testTime()
	policy := sequence.Policy{
		RunwayGroupID: "A", Rates: []sequence.RatePoint{{EffectiveAt: start, ArrivalsPerHour: 120}}, EarlyTolerance: 5 * time.Minute,
		SeparationRules: []sequence.SeparationRule{
			{Leading: "H", Trailing: "H", Minimum: 2 * time.Minute},
			{Leading: "H", Trailing: "M", Minimum: 30 * time.Second},
			{Leading: "M", Trailing: "H", Minimum: 30 * time.Second},
			{Leading: "M", Trailing: "M", Minimum: 30 * time.Second},
		}, UnknownSeparation: 2 * time.Minute,
	}
	input := sequence.Input{Revision: 3, Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{
		queueFlight("LEADING-H", "A", start, "H", 1, start),
		queueFlight("CANDIDATE-M", "A", start.Add(30*time.Second), "M", 2, start.Add(30*time.Second)),
		queueFlight("TRAILING-H", "A", start.Add(time.Minute), "H", 3, start.Add(time.Minute)),
		queueFlight("TARGET-H", "A", start.Add(30*time.Second), "H", 4, start.Add(3*time.Minute)),
	}}
	bindQueueRevision(&input)

	offers, err := sequence.CalculateQueueOffers(input, sequence.QueueOfferConfig{Validity: time.Minute}, start.Add(-time.Minute))
	require.NoError(t, err)
	for _, offer := range offers {
		require.NotEqual(t, 2, offer.CandidateSlot.Sequence, "candidate must fail leading and trailing H/H spacing")
	}
}

func TestQueueOffersDoNotCrossFreezeManualOrRunwayGroups(t *testing.T) {
	start := testTime()
	frozenAt := start.Add(-time.Minute)
	frozenTETA := start.Add(time.Minute)
	manualOrder := 1
	superstable := queueFlight("SUPER", "A", start.Add(time.Minute), "M", 2, start.Add(time.Minute))
	superstable.FreezeReason = aman.FreezeSuperstable
	superstable.FrozenAt = &frozenAt
	superstable.FrozenOperationalTETA = &frozenTETA
	superstable.CapturedSlot = superstable.CurrentSlot
	manual := queueFlight("MANUAL", "A", start.Add(2*time.Minute), "M", 3, start.Add(2*time.Minute))
	manual.FreezeReason = aman.FreezeManual
	manual.FrozenAt = &frozenAt
	manual.FrozenOperationalTETA = &frozenTETA
	manual.CapturedSlot = manual.CurrentSlot
	ordered := queueFlight("ORDERED", "A", start.Add(3*time.Minute), "M", 4, start.Add(3*time.Minute))
	ordered.ManualOrder = &manualOrder
	input := sequence.Input{Revision: 5, Policies: []sequence.Policy{queuePolicy("B", start, 60), queuePolicy("A", start, 60)}, Flights: []sequence.Flight{
		queueFlight("A-LEAD", "A", start, "M", 1, start), superstable, manual, ordered,
		queueFlight("A-TARGET", "A", start, "M", 5, start.Add(4*time.Minute)),
		queueFlight("B-LEAD", "B", start, "M", 1, start),
		queueFlight("B-TARGET", "B", start, "M", 2, start.Add(time.Minute)),
	}}
	bindQueueRevision(&input)

	offers, err := sequence.CalculateQueueOffers(input, sequence.QueueOfferConfig{Validity: 10 * time.Minute}, start.Add(-time.Minute))
	require.NoError(t, err)
	require.NotEmpty(t, offers)
	for _, offer := range offers {
		require.NotContains(t, []aman.FlightID{"SUPER", "MANUAL", "ORDERED"}, offer.FlightID)
		if offer.FlightID == "A-TARGET" {
			t.Fatalf("target received protected/manual candidate offer: %+v", offer)
		}
		if offer.FlightID == "B-TARGET" {
			require.Equal(t, aman.RunwayGroupID("B"), offer.CandidateSlot.RunwayGroupID)
		}
	}
}

func TestQueueOffersRejectProtectedCandidateSlot(t *testing.T) {
	start := testTime()
	frozenAt := start.Add(-time.Minute)
	frozenTETA := start
	manualOrder := 1
	tests := []struct {
		name      string
		configure func(*sequence.Flight)
	}{
		{name: "manual order", configure: func(candidate *sequence.Flight) { candidate.ManualOrder = &manualOrder }},
		{name: "manual freeze", configure: func(candidate *sequence.Flight) {
			candidate.FreezeReason = aman.FreezeManual
			candidate.FrozenAt = &frozenAt
			candidate.FrozenOperationalTETA = &frozenTETA
			candidate.CapturedSlot = candidate.CurrentSlot
		}},
		{name: "Superstable freeze", configure: func(candidate *sequence.Flight) {
			candidate.FreezeReason = aman.FreezeSuperstable
			candidate.FrozenAt = &frozenAt
			candidate.FrozenOperationalTETA = &frozenTETA
			candidate.CapturedSlot = candidate.CurrentSlot
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := queueFlight("PROTECTED", "A", start, "M", 1, start)
			test.configure(&candidate)
			input := sequence.Input{Revision: 6, Policies: []sequence.Policy{queuePolicy("A", start, 60)}, Flights: []sequence.Flight{
				candidate,
				queueFlight("TARGET", "A", start, "M", 2, start.Add(time.Minute)),
			}}
			bindQueueRevision(&input)

			offers, err := sequence.CalculateQueueOffers(input, sequence.QueueOfferConfig{Validity: time.Minute}, start.Add(-time.Minute))
			require.NoError(t, err)
			require.Empty(t, offers)
		})
	}
}

func TestQueueOffersExpireAndRateChangesRecompute(t *testing.T) {
	start := testTime()
	input := sequence.Input{Revision: 8, Policies: []sequence.Policy{queuePolicy("A", start, 60)}, Flights: []sequence.Flight{
		queueFlight("LEAD", "A", start, "M", 1, start),
		queueFlight("TARGET", "A", start, "M", 2, start.Add(time.Minute)),
	}}
	bindQueueRevision(&input)

	active, err := sequence.CalculateQueueOffers(input, sequence.QueueOfferConfig{Validity: 30 * time.Second}, start.Add(-time.Minute))
	require.NoError(t, err)
	require.Len(t, active, 1)
	require.Equal(t, start.Add(-30*time.Second), active[0].ExpiresAt)

	expired, err := sequence.CalculateQueueOffers(input, sequence.QueueOfferConfig{Validity: 30 * time.Second}, start)
	require.NoError(t, err)
	require.Empty(t, expired)

	rateInput := sequence.Input{Revision: 9, Policies: []sequence.Policy{queuePolicy("A", start, 60)}, Flights: []sequence.Flight{
		queueFlight("RATE-LEAD", "A", start, "M", 1, start),
		queueFlight("RATE-MIDDLE", "A", start, "M", 2, start.Add(time.Minute)),
		queueFlight("RATE-TARGET", "A", start, "M", 3, start.Add(2*time.Minute)),
	}}
	bindQueueRevision(&rateInput)
	beforeRateChange, err := sequence.CalculateQueueOffers(rateInput, sequence.QueueOfferConfig{Validity: time.Minute}, start.Add(-time.Minute))
	require.NoError(t, err)
	require.True(t, slices.ContainsFunc(beforeRateChange, func(offer aman.QueueOffer) bool { return offer.FlightID == "RATE-TARGET" }))
	rateInput.Policies = []sequence.Policy{queuePolicy("A", start, 30)}
	recomputed, err := sequence.CalculateQueueOffers(rateInput, sequence.QueueOfferConfig{Validity: time.Minute}, start.Add(-time.Minute))
	require.NoError(t, err)
	require.False(t, slices.ContainsFunc(recomputed, func(offer aman.QueueOffer) bool { return offer.FlightID == "RATE-TARGET" }), "the lower rate increases both adjacent base intervals")
}

func TestQueueOffersRejectStaleRevisionAndReplayDeterministically(t *testing.T) {
	start := testTime()
	input := sequence.Input{Revision: 11, Policies: []sequence.Policy{queuePolicy("B", start, 60), queuePolicy("A", start, 60)}, Flights: []sequence.Flight{
		queueFlight("B1", "B", start, "M", 1, start), queueFlight("B2", "B", start, "M", 2, start.Add(time.Minute)),
		queueFlight("A1", "A", start, "M", 1, start), queueFlight("A2", "A", start, "M", 2, start.Add(time.Minute)),
	}}
	bindQueueRevision(&input)
	config := sequence.QueueOfferConfig{Validity: time.Minute}
	at := start.Add(-time.Minute)

	first, err := sequence.CalculateQueueOffers(input, config, at)
	require.NoError(t, err)
	encodedInput, err := json.Marshal(input)
	require.NoError(t, err)
	var restored sequence.Input
	require.NoError(t, json.Unmarshal(encodedInput, &restored))
	slices.Reverse(restored.Policies)
	slices.Reverse(restored.Flights)
	second, err := sequence.CalculateQueueOffers(restored, config, at)
	require.NoError(t, err)
	firstJSON, err := json.Marshal(first)
	require.NoError(t, err)
	secondJSON, err := json.Marshal(second)
	require.NoError(t, err)
	require.Equal(t, string(firstJSON), string(secondJSON))

	restored.Flights[0].CurrentSlot.Revision--
	_, err = sequence.CalculateQueueOffers(restored, config, at)
	require.ErrorContains(t, err, "stale")
}

func TestQueueOfferProjectionPersistsWithOneAirportRevision(t *testing.T) {
	start := testTime()
	input := sequence.Input{Revision: 12, Policies: []sequence.Policy{queuePolicy("A", start, 60)}, Flights: []sequence.Flight{
		queueFlight("LEAD", "A", start, "M", 1, start),
		queueFlight("TARGET", "A", start, "M", 2, start.Add(time.Minute)),
	}}
	bindQueueRevision(&input)
	generatedAt := start.Add(-time.Minute)
	state := queueState(input, generatedAt)

	projected, err := sequence.ProjectQueueOffers(state, input, sequence.QueueOfferConfig{Validity: 30 * time.Second}, generatedAt)
	require.NoError(t, err)
	require.Len(t, projected.Flights[1].QueueOffers, 1)
	require.Equal(t, projected.Revision, projected.Flights[1].QueueOffers[0].AirportRevision)
	require.Equal(t, projected.Revision, projected.Flights[1].QueueOffers[0].CandidateSlot.Revision)

	encoded, err := json.Marshal(projected)
	require.NoError(t, err)
	var restored aman.AirportState
	require.NoError(t, json.Unmarshal(encoded, &restored))
	require.NoError(t, restored.Validate())
	require.Equal(t, projected, restored)

	input.Revision--
	_, err = sequence.ProjectQueueOffers(state, input, sequence.QueueOfferConfig{Validity: time.Minute}, generatedAt)
	var domainError *aman.DomainError
	require.ErrorAs(t, err, &domainError)
	require.Equal(t, aman.ErrorRevisionConflict, domainError.Class)
}

func TestCoordinatorRecomputesQueueOffersInChangedSlotRevision(t *testing.T) {
	start := testTime()
	input := sequence.Input{Revision: 12, Policies: []sequence.Policy{queuePolicy("A", start, 60)}, Flights: []sequence.Flight{
		queueFlight("LEAD", "A", start, "M", 1, start),
		queueFlight("TARGET", "A", start, "M", 2, start.Add(time.Minute)),
	}}
	bindQueueRevision(&input)
	initial, err := sequence.ProjectQueueOffers(queueState(input, start.Add(-2*time.Minute)), input, sequence.QueueOfferConfig{Validity: time.Minute}, start.Add(-2*time.Minute))
	require.NoError(t, err)
	require.Len(t, initial.Flights[1].QueueOffers, 1)

	repository := newCoordinatorRepository(initial)
	publisher := &recordingPublisher{repository: repository}
	coordinator, err := sequence.NewCoordinator(sequence.CoordinatorDependencies{
		States: repository, Outcomes: repository, Committer: repository, Publisher: publisher,
		Now: func() time.Time { return start.Add(-30 * time.Second) },
	})
	require.NoError(t, err)
	metadata := aman.CommandMetadata{CommandID: "rate-recompute", ExpectedRevision: input.Revision}
	result, err := coordinator.ExecuteCommand(context.Background(), "EKCH", metadata, func(state aman.AirportState) (sequence.CommandChange, error) {
		state.Flights[1].Slot.Time = start.Add(2 * time.Minute)
		updatedInput := input
		updatedInput.Flights = slices.Clone(input.Flights)
		for index := range updatedInput.Flights {
			updatedInput.Flights[index].CurrentSlot = cloneQueueSlot(input.Flights[index].CurrentSlot)
		}
		updatedInput.Flights[1].CurrentSlot.Time = start.Add(2 * time.Minute)
		change := commandChange(state, true)
		change.QueueOffers = &sequence.QueueOfferCalculation{Input: updatedInput, Config: sequence.QueueOfferConfig{Validity: time.Minute}}
		return change, nil
	})
	require.NoError(t, err)
	require.Equal(t, aman.SequenceRevision(13), result.State.Revision)
	require.Len(t, result.State.Flights[1].QueueOffers, 1)
	offer := result.State.Flights[1].QueueOffers[0]
	require.Equal(t, result.State.Revision, offer.AirportRevision)
	require.Equal(t, result.State.Revision, offer.CandidateSlot.Revision)
	require.Equal(t, result.State.Revision, result.State.Flights[1].Slot.Revision)
	require.Equal(t, result.State, repository.current())
	require.Equal(t, 1, publisher.calls())
}

func TestVacantSlotAutomaticallyPromotesFirstEligibleQueuedFlight(t *testing.T) {
	start := testTime()
	target := queueFlight("SECOND", "A", start.Add(time.Minute), "M", 3, start.Add(2*time.Minute))
	target.ProtectCurrentSlot = true
	input := sequence.Input{Revision: 20, Policies: []sequence.Policy{queuePolicy("A", start, 60)}, Flights: []sequence.Flight{
		queueFlight("LEAD", "A", start, "M", 1, start),
		target,
	}}
	bindQueueRevision(&input)
	candidate := aman.Slot{Time: start.Add(time.Minute), RunwayGroupID: "A", Sequence: 2, Revision: input.Revision, Reason: "rate_wtc"}
	offers := []aman.QueueOffer{
		{FlightID: "NO-LONGER-ELIGIBLE", RunwayGroupID: "A", CandidateSlot: candidate, QueuePosition: 1, ExpiresAt: candidate.Time, AirportRevision: input.Revision, Reason: aman.QueueOfferEarlierOccupiedSlot},
		{FlightID: "SECOND", RunwayGroupID: "A", CandidateSlot: candidate, QueuePosition: 2, ExpiresAt: candidate.Time, AirportRevision: input.Revision, Reason: aman.QueueOfferEarlierOccupiedSlot},
	}

	result, promotions, err := sequence.GenerateWithVacancyPromotions(input, offers, start)
	require.NoError(t, err)
	promoted := candidate
	promoted.Reason = string(sequence.ReasonQueuePromotion)
	require.Equal(t, []sequence.VacancyPromotion{{FlightID: "SECOND", From: *input.Flights[1].CurrentSlot, To: promoted}}, promotions)
	entry := candidateEntry(result, "SECOND")
	require.Equal(t, candidate.Time, entry.Time)
	require.Equal(t, sequence.ReasonQueuePromotion, entry.Reason)
	require.Equal(t, 2, entry.Sequence)
}

func TestVacancyCreatedByBaselineResequencePromotesQueuedStableFlight(t *testing.T) {
	start := testTime()
	lead := queueFlight("LEAD", "A", start, "M", 1, start)
	lead.ProtectCurrentSlot = true
	occupant := queueFlight("MOVES-LATER", "A", start.Add(3*time.Minute), "M", 2, start.Add(time.Minute))
	occupant.State = aman.StateUnstable
	target := queueFlight("TARGET", "A", start.Add(time.Minute), "M", 3, start.Add(2*time.Minute))
	target.ProtectCurrentSlot = true
	input := sequence.Input{Revision: 24, Policies: []sequence.Policy{queuePolicy("A", start, 60)}, Flights: []sequence.Flight{lead, occupant, target}}
	bindQueueRevision(&input)
	candidate := aman.Slot{Time: start.Add(time.Minute), RunwayGroupID: "A", Sequence: 2, Revision: input.Revision, Reason: "rate_wtc"}
	offer := aman.QueueOffer{FlightID: target.ID, RunwayGroupID: "A", CandidateSlot: candidate, QueuePosition: 1, ExpiresAt: candidate.Time, AirportRevision: input.Revision, Reason: aman.QueueOfferEarlierOccupiedSlot}

	result, promotions, err := sequence.GenerateWithVacancyPromotions(input, []aman.QueueOffer{offer}, start)
	require.NoError(t, err)
	require.Len(t, promotions, 1)
	require.Equal(t, target.ID, promotions[0].FlightID)
	require.Equal(t, candidate.Time, candidateEntry(result, target.ID).Time)
	require.Equal(t, start.Add(3*time.Minute), candidateEntry(result, occupant.ID).Time)
}

func TestVacancyPromotionRejectsOfferOutsideCurrentRateGrid(t *testing.T) {
	start := testTime()
	lead := queueFlight("LEAD", "A", start, "M", 1, start)
	lead.ProtectCurrentSlot = true
	target := queueFlight("TARGET", "A", start.Add(time.Minute), "M", 3, start.Add(3*time.Minute))
	target.ProtectCurrentSlot = true
	input := sequence.Input{Revision: 25, Policies: []sequence.Policy{queuePolicy("A", start, 40)}, Flights: []sequence.Flight{lead, target}}
	bindQueueRevision(&input)
	candidate := aman.Slot{Time: start.Add(time.Minute), RunwayGroupID: "A", Sequence: 2, Revision: input.Revision, Reason: "rate_wtc"}
	offer := aman.QueueOffer{FlightID: target.ID, RunwayGroupID: "A", CandidateSlot: candidate, QueuePosition: 1, ExpiresAt: candidate.Time, AirportRevision: input.Revision, Reason: aman.QueueOfferEarlierOccupiedSlot}

	result, promotions, err := sequence.GenerateWithVacancyPromotions(input, []aman.QueueOffer{offer}, start)
	require.NoError(t, err)
	require.Empty(t, promotions)
	require.Equal(t, target.CurrentSlot.Time, candidateEntry(result, target.ID).Time)
}

func TestVacancyPromotionReportsFinalRenumberedSlot(t *testing.T) {
	start := testTime()
	target := queueFlight("TARGET", "A", start.Add(time.Minute), "M", 3, start.Add(2*time.Minute))
	target.ProtectCurrentSlot = true
	input := sequence.Input{Revision: 26, Policies: []sequence.Policy{queuePolicy("A", start, 60)}, Flights: []sequence.Flight{target}}
	bindQueueRevision(&input)
	candidate := aman.Slot{Time: start.Add(time.Minute), RunwayGroupID: "A", Sequence: 2, Revision: input.Revision, Reason: "rate_wtc"}
	offer := aman.QueueOffer{FlightID: target.ID, RunwayGroupID: "A", CandidateSlot: candidate, QueuePosition: 1, ExpiresAt: candidate.Time, AirportRevision: input.Revision, Reason: aman.QueueOfferEarlierOccupiedSlot}

	result, promotions, err := sequence.GenerateWithVacancyPromotions(input, []aman.QueueOffer{offer}, start)
	require.NoError(t, err)
	require.Len(t, promotions, 1)
	entry := candidateEntry(result, target.ID)
	require.Equal(t, 1, entry.Sequence)
	require.Equal(t, entry.Sequence, promotions[0].To.Sequence)
	require.Equal(t, string(entry.Reason), promotions[0].To.Reason)
}

func TestVacancyPromotionPreservesStableOrderAndProtectedBoundaries(t *testing.T) {
	start := testTime()
	frozenAt := start.Add(-time.Minute)
	frozenTETA := start.Add(2 * time.Minute)
	lead := queueFlight("LEAD", "A", start, "M", 1, start)
	protected := queueFlight("PROTECTED", "A", frozenTETA, "M", 3, frozenTETA)
	protected.FreezeReason = aman.FreezeManual
	protected.FrozenAt = &frozenAt
	protected.FrozenOperationalTETA = &frozenTETA
	protected.CapturedSlot = protected.CurrentSlot
	target := queueFlight("TARGET", "A", start.Add(time.Minute), "M", 4, start.Add(3*time.Minute))
	target.ProtectCurrentSlot = true
	input := sequence.Input{Revision: 21, Policies: []sequence.Policy{queuePolicy("A", start, 60)}, Flights: []sequence.Flight{lead, protected, target}}
	bindQueueRevision(&input)
	candidate := aman.Slot{Time: start.Add(time.Minute), RunwayGroupID: "A", Sequence: 2, Revision: input.Revision, Reason: "rate_wtc"}
	offer := aman.QueueOffer{FlightID: "TARGET", RunwayGroupID: "A", CandidateSlot: candidate, QueuePosition: 1, ExpiresAt: candidate.Time, AirportRevision: input.Revision, Reason: aman.QueueOfferEarlierOccupiedSlot}

	result, promotions, err := sequence.GenerateWithVacancyPromotions(input, []aman.QueueOffer{offer}, start)
	require.NoError(t, err)
	require.Empty(t, promotions)
	require.Equal(t, target.CurrentSlot.Time, candidateEntry(result, "TARGET").Time)
	require.Equal(t, protected.CurrentSlot.Time, candidateEntry(result, "PROTECTED").Time)

	// Stable relative order is independently protected even without a freeze.
	protected.FreezeReason = aman.FreezeNone
	protected.FrozenAt, protected.FrozenOperationalTETA, protected.CapturedSlot = nil, nil, nil
	protected.ProtectCurrentSlot = true
	input.Flights = []sequence.Flight{lead, protected, target}
	bindQueueRevision(&input)
	result, promotions, err = sequence.GenerateWithVacancyPromotions(input, []aman.QueueOffer{offer}, start)
	require.NoError(t, err)
	require.Empty(t, promotions)
	require.Equal(t, target.CurrentSlot.Time, candidateEntry(result, "TARGET").Time)
}

func TestVacancyPromotionRejectsStaleCrossRunwayTooEarlyAndFrozenOffers(t *testing.T) {
	start := testTime()
	policyA, policyB := queuePolicy("A", start, 60), queuePolicy("B", start, 60)
	target := queueFlight("TARGET", "A", start.Add(2*time.Minute), "M", 3, start.Add(3*time.Minute))
	target.ProtectCurrentSlot = true
	input := sequence.Input{Revision: 22, Policies: []sequence.Policy{policyA, policyB}, Flights: []sequence.Flight{
		queueFlight("LEAD", "A", start, "M", 1, start), target,
	}}
	bindQueueRevision(&input)
	base := aman.QueueOffer{
		FlightID: "TARGET", RunwayGroupID: "A",
		CandidateSlot: aman.Slot{Time: start.Add(time.Minute), RunwayGroupID: "A", Sequence: 2, Revision: input.Revision, Reason: "rate_wtc"},
		QueuePosition: 1, ExpiresAt: start.Add(time.Minute), AirportRevision: input.Revision, Reason: aman.QueueOfferEarlierOccupiedSlot,
	}
	tests := []struct {
		name   string
		mutate func(*aman.QueueOffer, *sequence.Input)
	}{
		{name: "stale revision", mutate: func(offer *aman.QueueOffer, _ *sequence.Input) { offer.AirportRevision-- }},
		{name: "cross runway", mutate: func(offer *aman.QueueOffer, _ *sequence.Input) {
			offer.RunwayGroupID, offer.CandidateSlot.RunwayGroupID = "B", "B"
		}},
		{name: "too early", mutate: func(_ *aman.QueueOffer, input *sequence.Input) { input.Policies[0].EarlyTolerance = 30 * time.Second }},
		{name: "manual freeze", mutate: func(_ *aman.QueueOffer, input *sequence.Input) {
			at, teta := start.Add(-time.Minute), input.Flights[1].OperationalTETA
			input.Flights[1].FreezeReason, input.Flights[1].FrozenAt, input.Flights[1].FrozenOperationalTETA = aman.FreezeManual, &at, &teta
			input.Flights[1].CapturedSlot = input.Flights[1].CurrentSlot
		}},
		{name: "Superstable freeze", mutate: func(_ *aman.QueueOffer, input *sequence.Input) {
			at, teta := start.Add(-time.Minute), input.Flights[1].OperationalTETA
			input.Flights[1].FreezeReason, input.Flights[1].FrozenAt, input.Flights[1].FrozenOperationalTETA = aman.FreezeSuperstable, &at, &teta
			input.Flights[1].CapturedSlot = input.Flights[1].CurrentSlot
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidateInput := input
			candidateInput.Policies = slices.Clone(input.Policies)
			candidateInput.Flights = slices.Clone(input.Flights)
			offer := base
			test.mutate(&offer, &candidateInput)
			result, promotions, err := sequence.GenerateWithVacancyPromotions(candidateInput, []aman.QueueOffer{offer}, start)
			require.NoError(t, err)
			require.Empty(t, promotions)
			require.Equal(t, target.CurrentSlot.Time, candidateEntry(result, "TARGET").Time)
		})
	}
}

func TestVacancyPromotionIsDeterministicAcrossReplay(t *testing.T) {
	start := testTime()
	target := queueFlight("TARGET", "A", start.Add(time.Minute), "M", 3, start.Add(2*time.Minute))
	target.ProtectCurrentSlot = true
	input := sequence.Input{Revision: 23, Policies: []sequence.Policy{queuePolicy("B", start, 60), queuePolicy("A", start, 60)}, Flights: []sequence.Flight{
		queueFlight("B-LEAD", "B", start, "M", 1, start), queueFlight("A-LEAD", "A", start, "M", 1, start), target,
	}}
	bindQueueRevision(&input)
	offer := aman.QueueOffer{
		FlightID: "TARGET", RunwayGroupID: "A", CandidateSlot: aman.Slot{Time: start.Add(time.Minute), RunwayGroupID: "A", Sequence: 2, Revision: input.Revision, Reason: "rate_wtc"},
		QueuePosition: 1, ExpiresAt: start.Add(time.Minute), AirportRevision: input.Revision, Reason: aman.QueueOfferEarlierOccupiedSlot,
	}
	first, firstPromotions, err := sequence.GenerateWithVacancyPromotions(input, []aman.QueueOffer{offer}, start)
	require.NoError(t, err)
	slices.Reverse(input.Policies)
	slices.Reverse(input.Flights)
	second, secondPromotions, err := sequence.GenerateWithVacancyPromotions(input, []aman.QueueOffer{offer}, start)
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Equal(t, firstPromotions, secondPromotions)
}

func candidateEntry(result sequence.Result, id aman.FlightID) sequence.CandidateEntry {
	for _, entry := range result.Entries {
		if entry.FlightID == id {
			return entry
		}
	}
	return sequence.CandidateEntry{}
}

func queuePolicy(group aman.RunwayGroupID, start time.Time, rate uint32) sequence.Policy {
	return sequence.Policy{
		RunwayGroupID: group, Rates: []sequence.RatePoint{{EffectiveAt: start, ArrivalsPerHour: rate}}, EarlyTolerance: 10 * time.Minute,
		SeparationRules: []sequence.SeparationRule{{Leading: "M", Trailing: "M", Minimum: 0}}, UnknownSeparation: time.Minute,
	}
}

func queueFlight(id aman.FlightID, group aman.RunwayGroupID, teta time.Time, category sequence.WakeCategory, number int, slotTime time.Time) sequence.Flight {
	return sequence.Flight{
		ID: id, RunwayGroupID: group, State: aman.StateStable, OperationalTETA: teta, WakeCategory: category, FreezeReason: aman.FreezeNone,
		CurrentSlot: &aman.Slot{Time: slotTime, RunwayGroupID: group, Sequence: number, Revision: 7, Reason: "rate_wtc"},
	}
}

func queueOffer(flightID aman.FlightID, group aman.RunwayGroupID, sequenceNumber int, slotTime time.Time, position int, revision aman.SequenceRevision, expiresAt time.Time) aman.QueueOffer {
	return aman.QueueOffer{
		FlightID: flightID, RunwayGroupID: group,
		CandidateSlot: aman.Slot{Time: slotTime, RunwayGroupID: group, Sequence: sequenceNumber, Revision: revision, Reason: "rate_wtc"},
		QueuePosition: position, ExpiresAt: expiresAt, AirportRevision: revision, Reason: aman.QueueOfferEarlierOccupiedSlot,
	}
}

func bindQueueRevision(input *sequence.Input) {
	for index := range input.Flights {
		if input.Flights[index].CurrentSlot != nil {
			input.Flights[index].CurrentSlot.Revision = input.Revision
		}
		if input.Flights[index].CapturedSlot != nil {
			input.Flights[index].CapturedSlot.Revision = input.Revision
		}
	}
}

func queueState(input sequence.Input, generatedAt time.Time) aman.AirportState {
	state := aman.AirportState{
		Airport: "EKCH", Revision: input.Revision, GeneratedAt: generatedAt, PolicyVersion: "queue-v1", Mode: aman.ModeShadow,
		RunwayGroups: make([]aman.RunwayGroupPolicy, len(input.Policies)), Flights: make([]aman.AMANFlight, len(input.Flights)),
	}
	for index, policy := range input.Policies {
		state.RunwayGroups[index] = aman.RunwayGroupPolicy{ID: policy.RunwayGroupID}
	}
	for index, flight := range input.Flights {
		slot := *flight.CurrentSlot
		state.Flights[index] = aman.AMANFlight{
			ID: flight.ID, VATSIMCID: "CID-" + string(flight.ID), CurrentCallsign: string(flight.ID),
			State: flight.State, DataStatus: aman.DataFresh, FreezeReason: flight.FreezeReason,
			Slot: &slot, UpdatedAt: generatedAt,
		}
	}
	return state
}

func cloneQueueSlot(slot *aman.Slot) *aman.Slot {
	if slot == nil {
		return nil
	}
	copy := *slot
	return &copy
}
