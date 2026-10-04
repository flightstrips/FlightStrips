package sequence_test

import (
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"github.com/stretchr/testify/require"
)

func continuousCPHPolicy(start time.Time) sequence.Policy {
	return sequence.Policy{RunwayGroupID: "A", ContinuousSpacing: true,
		Rates:           []sequence.RatePoint{{EffectiveAt: start, ArrivalsPerHour: 40}},
		SeparationRules: sequence.CPHSeparationRules(), UnknownSeparation: 3 * time.Minute}
}

func TestContinuousSpacingDoesNotAccumulateRateGridRoundingAfterHeavies(t *testing.T) {
	start := testTime()
	policy := continuousCPHPolicy(start)
	result, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{
		flight("ONE", "A", start, "H"), flight("TWO", "A", start.Add(time.Second), "H"),
		flight("THREE", "A", start.Add(2*time.Second), "M"), flight("FOUR", "A", start.Add(3*time.Second), "M"),
	}})
	require.NoError(t, err)
	require.False(t, result.HasConflicts())
	require.Equal(t, []time.Time{start, start.Add(2 * time.Minute), start.Add(4 * time.Minute), start.Add(330 * time.Second)}, entryTimes(result))

	policy.SameSTARSpacing = sequence.SameSTARSpacing{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}
	first, second := flight("ONE", "A", start, "H"), flight("TWO", "A", start, "M")
	first.STARFamily, second.STARFamily = "MONAK", "MONAK"
	result, err = sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{first, second}})
	require.NoError(t, err)
	require.Equal(t, start.Add(3*time.Minute), entryFor(t, result, "TWO").Time, "same-STAR capacity remains enforced")
}

func TestContinuousSpacingRespectsGapsClosuresAndRateChanges(t *testing.T) {
	start := testTime()
	for _, block := range []string{"gap", "closure"} {
		t.Run(block, func(t *testing.T) {
			policy := continuousCPHPolicy(start)
			end := start.Add(160 * time.Second)
			if block == "gap" {
				policy.Gaps = []sequence.Gap{{Start: start.Add(110 * time.Second), End: end}}
			} else {
				policy.Closures = []aman.RunwayClosure{{Start: start.Add(110 * time.Second), End: &end}}
			}
			policy.Rates = append(policy.Rates, sequence.RatePoint{EffectiveAt: end, ArrivalsPerHour: 20})
			result, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{
				flight("ONE", "A", start, "H"), flight("TWO", "A", start, "M"), flight("THREE", "A", start, "M"),
			}})
			require.NoError(t, err)
			require.False(t, result.HasConflicts())
			require.Equal(t, []time.Time{start, start.Add(3 * time.Minute), start.Add(6 * time.Minute)}, entryTimes(result))
		})
	}
}

func TestContinuousVacancyCompactionPreservesCommittedHoldingAndFrozenOrder(t *testing.T) {
	start := testTime()
	input := sequence.Input{Revision: 7, Policies: []sequence.Policy{continuousCPHPolicy(start)}, Flights: []sequence.Flight{
		queueFlight("ONE", "A", start, "H", 1, start),
		queueFlight("TWO", "A", start, "H", 2, start.Add(3*time.Minute)),
		queueFlight("THREE", "A", start, "M", 3, start.Add(6*time.Minute)),
		queueFlight("MANUAL", "A", start, "M", 4, start.Add(10*time.Minute)),
		queueFlight("TMA", "A", start, "M", 5, start.Add(12*time.Minute)),
	}}
	for index := range input.Flights {
		f := &input.Flights[index]
		f.ProtectCurrentSlot = true
		if index < 3 {
			f.HoldingSlotProtected = true
			f.FreezeReason = aman.FreezeSuperstable
		} else if index == 3 {
			f.FreezeReason = aman.FreezeManual
		} else {
			f.FreezeReason = aman.FreezeTMA
		}
		f.CapturedSlot = cloneQueueSlot(f.CurrentSlot)
		frozenAt, frozenTETA := start.Add(-time.Minute), start
		f.FrozenAt = &frozenAt
		f.FrozenOperationalTETA = &frozenTETA
	}
	result, promotions, err := sequence.GenerateWithVacancyPromotions(input, nil, start.Add(-time.Second))
	require.NoError(t, err)
	require.False(t, result.HasConflicts())
	require.Len(t, promotions, 2)
	require.Equal(t, []time.Time{start, start.Add(2 * time.Minute), start.Add(4 * time.Minute), start.Add(10 * time.Minute), start.Add(12 * time.Minute)}, entryTimes(result))
}

func TestContinuousVacancyOfferAcceptsOffGridTimeAndChecksSeparation(t *testing.T) {
	start := testTime()
	input := sequence.Input{Revision: 7, Policies: []sequence.Policy{continuousCPHPolicy(start)}, Flights: []sequence.Flight{
		queueFlight("LEAD", "A", start, "H", 1, start),
		queueFlight("TARGET", "A", start.Add(2*time.Minute), "M", 2, start.Add(4*time.Minute)),
	}}
	input.Flights[1].ProtectCurrentSlot = true
	offer := queueOffer("TARGET", "A", 2, start.Add(2*time.Minute), 1, 7, start.Add(time.Minute))
	result, promotions, err := sequence.GenerateWithVacancyPromotions(input, []aman.QueueOffer{offer}, start)
	require.NoError(t, err)
	require.Len(t, promotions, 1)
	require.Equal(t, start.Add(2*time.Minute), entryFor(t, result, "TARGET").Time)
}

func TestContinuousGoAroundCascadeUsesExactWakeSpacing(t *testing.T) {
	start := testTime()
	goAround := withState(flight("GO", "A", start, "H"), aman.StateGoAround)
	goAround.CurrentSlot = slot(start, "A", 1)
	stable := queueFlight("STABLE", "A", start.Add(2*time.Minute), "H", 2, start.Add(2*time.Minute))
	stable.ProtectCurrentSlot = true
	frozen := protectedFlight("SUPER", "A", start.Add(4*time.Minute), "M", start.Add(4*time.Minute), aman.FreezeSuperstable)
	frozen.CurrentSlot = slot(start.Add(4*time.Minute), "A", 3)
	input := sequence.Input{Revision: 9, Policies: []sequence.Policy{continuousCPHPolicy(start)}, Flights: []sequence.Flight{goAround, stable, frozen}}
	decision, err := sequence.ApplyGoAround(input, sequence.GoAroundPolicy{Delay: 90 * time.Second, MaxCascade: 2}, sequence.ApplyGoAroundCommand{
		Metadata: aman.CommandMetadata{CommandID: "exact-wake-go-around", ExpectedRevision: 9}, Callsign: "GO", DetectedAt: start,
	})
	require.NoError(t, err)
	require.False(t, decision.Candidate.HasConflicts())
	require.Equal(t, start.Add(90*time.Second), entryFor(t, decision.Candidate, "GO").Time)
	require.Equal(t, start.Add(210*time.Second), entryFor(t, decision.Candidate, "STABLE").Time)
	require.Equal(t, start.Add(330*time.Second), entryFor(t, decision.Candidate, "SUPER").Time)
	require.Equal(t, start.Add(330*time.Second), policyFlight(t, decision.Input, "SUPER").CapturedSlot.Time)
}
