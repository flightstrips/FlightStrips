package sequence_test

import (
	"slices"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"github.com/stretchr/testify/require"
)

func TestAutomaticOrderReducesSameSTARDelay(t *testing.T) {
	start := testTime()
	policy := simplePolicy("A", start, 30)
	policy.SameSTARSpacing = sequence.SameSTARSpacing{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}
	flights := []sequence.Flight{
		flight("KLM25J", "A", start, "M"),
		flight("AFR89SR", "A", start.Add(time.Minute), "M"),
		flight("SAS367", "A", start.Add(2*time.Minute), "M"),
	}
	flights[0].STARFamily, flights[1].STARFamily, flights[2].STARFamily = "MONAK", "MONAK", "TUDLO"
	// Ordinary STAR predictions identify a holding fix without joining a hold.
	for i := range flights {
		flights[i].HoldingQueueID = flights[i].STARFamily
		flights[i].ArrivalQueueTime = &flights[i].OperationalTETA
	}
	input := sequence.Input{Policies: []sequence.Policy{policy}, Flights: flights}
	result, err := sequence.Generate(input)
	require.NoError(t, err)
	require.False(t, result.HasConflicts())
	require.Equal(t, []aman.Callsign{"KLM25J", "SAS367", "AFR89SR"}, entryIDs(result))
	require.Equal(t, []time.Time{start, start.Add(2 * time.Minute), start.Add(4 * time.Minute)}, entryTimes(result))
	// The original order landed at 0, 4 and 6 minutes (seven minutes of delay).
	require.Equal(t, 3*time.Minute, totalDelay(result))
	slices.Reverse(input.Flights)
	replayed, err := sequence.Generate(input)
	require.NoError(t, err)
	require.Equal(t, result, replayed)
}

func TestAutomaticOrderKeepsEqualCostAndManualOrders(t *testing.T) {
	start := testTime()
	policy := simplePolicy("A", start, 30)
	flights := []sequence.Flight{flight("FIRST", "A", start, "M"), flight("SECOND", "A", start, "M")}
	result, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}, Flights: flights})
	require.NoError(t, err)
	require.Equal(t, []aman.Callsign{"FIRST", "SECOND"}, entryIDs(result))

	policy = continuousCPHPolicy(start)
	flights[0].WakeCategory = "H"
	for i := range flights {
		order := i + 1
		flights[i].ManualOrder = &order
	}
	result, err = sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}, Flights: flights})
	require.NoError(t, err)
	require.Equal(t, []aman.Callsign{"FIRST", "SECOND"}, entryIDs(result))
	require.Equal(t, start.Add(2*time.Minute), result.Entries[1].Time)
}

func TestStableSameSTARSlotExchange(t *testing.T) {
	start := testTime()
	for _, test := range []string{"same STAR", "different STAR", "unknown STAR", "manual", "holding", "frozen", "physical bound", "wake boundary"} {
		t.Run(test, func(t *testing.T) {
			policy := continuousCPHPolicy(start)
			first := queueFlight("FIRST", "A", start.Add(90*time.Second), "M", 1, start)
			second := queueFlight("SECOND", "A", start, "M", 2, start.Add(90*time.Second))
			first.ProtectCurrentSlot, second.ProtectCurrentSlot = true, true
			first.SelectedSTARFamily, second.SelectedSTARFamily = "MONAK", "MONAK"
			switch test {
			case "different STAR":
				second.SelectedSTARFamily = "TUDLO"
			case "unknown STAR":
				second.SelectedSTARFamily = ""
			case "manual":
				order := 1
				first.ManualOrder = &order
			case "holding":
				first.HoldingSlotProtected = true
			case "frozen":
				first.FreezeReason = aman.FreezeTMA
				first.CapturedSlot = cloneQueueSlot(first.CurrentSlot)
			case "physical bound":
				bound := start.Add(time.Minute)
				second.PromotionNotBefore = &bound
			case "wake boundary":
				second.WakeCategory = "H"
			}
			result, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{first, second}})
			require.NoError(t, err)
			if test == "same STAR" {
				require.Equal(t, []aman.Callsign{"SECOND", "FIRST"}, entryIDs(result))
				require.Zero(t, totalDelay(result))
				require.Len(t, result.Movements, 2)
				// Committing the exchanged slots must produce the same order on
				// the next calculation, without another exchange or movement.
				first.CurrentSlot.Time, first.CurrentSlot.Sequence = result.Entries[1].Time, 2
				second.CurrentSlot.Time, second.CurrentSlot.Sequence = result.Entries[0].Time, 1
				repeated, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{policy}, Flights: []sequence.Flight{first, second}})
				require.NoError(t, err)
				require.Equal(t, result.Entries, repeated.Entries)
				require.Empty(t, repeated.Movements)
			} else {
				require.Equal(t, []aman.Callsign{"FIRST", "SECOND"}, entryIDs(result))
			}
			require.Equal(t, []time.Time{start, start.Add(90 * time.Second)}, entryTimes(result))
		})
	}
}

func TestStableSlotExchangeReportsFinalSpacingConflicts(t *testing.T) {
	start := testTime()
	for _, frozen := range []bool{false, true} {
		t.Run(map[bool]string{false: "exchange resolves conflict", true: "frozen conflict remains"}[frozen], func(t *testing.T) {
			first := queueFlight("FIRST", "A", start.Add(90*time.Second), "M", 1, start)
			second := queueFlight("SECOND", "A", start, "L", 2, start.Add(90*time.Second))
			first.ProtectCurrentSlot, second.ProtectCurrentSlot = true, true
			first.SelectedSTARFamily, second.SelectedSTARFamily = "MONAK", "MONAK"
			if frozen {
				first.FreezeReason = aman.FreezeTMA
				first.CapturedSlot = cloneQueueSlot(first.CurrentSlot)
			}
			unknown := flight("UNKNOWN", "A", start.Add(20*time.Minute), "unconfigured")
			result, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{continuousCPHPolicy(start)}, Flights: []sequence.Flight{first, second, unknown}})
			require.NoError(t, err)
			require.Equal(t, frozen, result.HasConflicts())
			require.Contains(t, result.Warnings, sequence.Warning{Severity: sequence.SeverityDegraded, Code: sequence.WarningUnknownWakeCategory, RunwayGroupID: "A", Callsign: "UNKNOWN"})
			if frozen {
				require.Equal(t, []aman.Callsign{"FIRST", "SECOND", "UNKNOWN"}, entryIDs(result))
				require.Contains(t, result.Warnings, sequence.Warning{Severity: sequence.SeverityConflict, Code: sequence.WarningProtectedSpacing, RunwayGroupID: "A", Callsign: "SECOND", RelatedCallsign: flightIDPointer("FIRST")})
			} else {
				require.Equal(t, []aman.Callsign{"SECOND", "FIRST", "UNKNOWN"}, entryIDs(result))
				require.Len(t, result.Warnings, 1)
			}
		})
	}
}

func TestStableSTAROrderBreaksEqualDelayTies(t *testing.T) {
	start := testTime()
	flights := []sequence.Flight{
		queueFlight("BACK", "A", start.Add(2*time.Minute), "M", 1, start.Add(10*time.Minute)),
		queueFlight("MIDDLE", "A", start.Add(time.Minute), "M", 2, start.Add(12*time.Minute)),
		queueFlight("FRONT", "A", start, "M", 3, start.Add(14*time.Minute)),
	}
	for i := range flights {
		f := &flights[i]
		f.ProtectCurrentSlot = true
		f.SelectedSTARFamily = "MONAK"
		entry := f.OperationalTETA.Add(-time.Minute)
		f.STARProgress = &sequence.STARProgress{Fix: "MONAK-FEEDER", ETA: &entry, TerminalDigest: "terminal"}
	}
	input := sequence.Input{Policies: []sequence.Policy{continuousCPHPolicy(start)}, Flights: flights}
	result, err := sequence.Generate(input)
	require.NoError(t, err)
	require.False(t, result.HasConflicts())
	require.Equal(t, []aman.Callsign{"FRONT", "MIDDLE", "BACK"}, entryIDs(result))
	require.Equal(t, 33*time.Minute, totalDelay(result), "restore order without increasing total delay")
	for i, entry := range result.Entries {
		require.Equal(t, time.Duration(10+i)*time.Minute, entry.Time.Sub(entry.OperationalTETA))
	}
}

func TestStablePassedSTAROrderUsesMatchingRouteProgress(t *testing.T) {
	start := testTime()
	for _, scenario := range []string{"matching route", "different route", "different terminal", "close progress", "passed versus approaching"} {
		t.Run(scenario, func(t *testing.T) {
			back := queueFlight("BACK", "A", start, "M", 1, start.Add(10*time.Minute))
			front := queueFlight("FRONT", "A", start, "M", 2, start.Add(12*time.Minute))
			for _, f := range []*sequence.Flight{&back, &front} {
				f.ProtectCurrentSlot = true
				f.SelectedSTARFamily = "MONAK"
			}
			back.STARProgress = &sequence.STARProgress{Fix: "FEEDER", Passed: true, DistanceToGoNM: 30, RemainingFixes: []string{"A", "B", "RWY"}, TerminalDigest: "terminal"}
			front.STARProgress = &sequence.STARProgress{Fix: "FEEDER", Passed: true, DistanceToGoNM: 20, RemainingFixes: []string{"B", "RWY"}, TerminalDigest: "terminal"}
			wantSwap := scenario == "matching route" || scenario == "passed versus approaching"
			switch scenario {
			case "different route":
				front.STARProgress.RemainingFixes = []string{"OTHER", "RWY"}
			case "different terminal":
				front.STARProgress.TerminalDigest = "other"
			case "close progress":
				front.STARProgress.DistanceToGoNM = 29.8
			case "passed versus approaching":
				back.STARProgress.Passed = false
				eta := start.Add(5 * time.Minute)
				back.STARProgress.ETA = &eta
			}
			result, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{continuousCPHPolicy(start)}, Flights: []sequence.Flight{back, front}})
			require.NoError(t, err)
			if wantSwap {
				require.Equal(t, []aman.Callsign{"FRONT", "BACK"}, entryIDs(result))
			} else {
				require.Equal(t, []aman.Callsign{"BACK", "FRONT"}, entryIDs(result))
			}
		})
	}
}

func TestFlexibleSTAROrderUsesPhysicalEvidence(t *testing.T) {
	start := testTime()
	for _, scenario := range []string{"equal delay", "wake saving would reverse physical order"} {
		t.Run(scenario, func(t *testing.T) {
			first := flight("FIRST", "A", start, "M")
			second := flight("SECOND", "A", start, "M")
			first.SelectedSTARFamily, second.SelectedSTARFamily = "MONAK", "MONAK"
			later := start.Add(time.Minute)
			first.STARProgress = &sequence.STARProgress{Fix: "FEEDER", ETA: &later, TerminalDigest: "terminal"}
			second.STARProgress = &sequence.STARProgress{Fix: "FEEDER", ETA: &start, TerminalDigest: "terminal"}
			if scenario != "equal delay" {
				first.STARProgress.ETA, second.STARProgress.ETA = &start, &later
				second.WakeCategory = "L"
			}
			result, err := sequence.Generate(sequence.Input{Policies: []sequence.Policy{continuousCPHPolicy(start)}, Flights: []sequence.Flight{first, second}})
			require.NoError(t, err)
			if scenario == "equal delay" {
				require.Equal(t, []aman.Callsign{"SECOND", "FIRST"}, entryIDs(result))
				require.Equal(t, 90*time.Second, totalDelay(result))
			} else {
				require.Equal(t, []aman.Callsign{"FIRST", "SECOND"}, entryIDs(result))
				require.Equal(t, 3*time.Minute, totalDelay(result))
			}
		})
	}
}

func TestVacancyPromotionsRetainExchangedSTAROrder(t *testing.T) {
	start := testTime()
	for _, scenario := range []string{"both compact", "one compacts", "neither compacts"} {
		t.Run(scenario, func(t *testing.T) {
			back := queueFlight("BACK", "A", start.Add(2*time.Minute), "M", 1, start.Add(10*time.Minute))
			front := queueFlight("FRONT", "A", start, "M", 2, start.Add(12*time.Minute))
			for _, f := range []*sequence.Flight{&back, &front} {
				f.ProtectCurrentSlot = true
				f.SelectedSTARFamily = "MONAK"
				eta := f.OperationalTETA.Add(-time.Minute)
				f.STARProgress = &sequence.STARProgress{Fix: "FEEDER", ETA: &eta, TerminalDigest: "terminal"}
			}
			at := start.Add(-time.Minute)
			wantTimes := []time.Time{start, start.Add(2 * time.Minute)}
			wantPromotions := 2
			if scenario == "one compacts" {
				at = start.Add(10 * time.Minute)
				wantTimes = []time.Time{start.Add(10 * time.Minute), start.Add(690 * time.Second)}
				wantPromotions = 1
			} else if scenario == "neither compacts" {
				at = start.Add(12 * time.Minute)
				wantTimes = []time.Time{start.Add(10 * time.Minute), start.Add(12 * time.Minute)}
				wantPromotions = 0
			}
			input := sequence.Input{Revision: 7, Policies: []sequence.Policy{continuousCPHPolicy(start)}, Flights: []sequence.Flight{back, front}}
			result, promotions, err := sequence.GenerateWithVacancyPromotions(input, nil, at)
			require.NoError(t, err)
			require.False(t, result.HasConflicts())
			require.Equal(t, []aman.Callsign{"FRONT", "BACK"}, entryIDs(result))
			require.Equal(t, wantTimes, entryTimes(result))
			require.Len(t, promotions, wantPromotions)
			require.Len(t, result.Movements, 2)
			for _, movement := range result.Movements {
				original := back.CurrentSlot
				if movement.Callsign == front.Callsign {
					original = front.CurrentSlot
				}
				require.Equal(t, original.Time, *movement.FromTime)
				require.Equal(t, original.Sequence, *movement.FromSequence)
			}
			for _, promotion := range promotions {
				original := back.CurrentSlot
				if promotion.Callsign == front.Callsign {
					original = front.CurrentSlot
				}
				require.Equal(t, *original, promotion.From)
			}
			require.Equal(t, start.Add(10*time.Minute), back.CurrentSlot.Time, "input reservations must remain unchanged")
			require.Equal(t, start.Add(12*time.Minute), front.CurrentSlot.Time)
		})
	}
}

func totalDelay(result sequence.Result) time.Duration {
	var delay time.Duration
	for _, entry := range result.Entries {
		delay += max(time.Duration(0), entry.Time.Sub(entry.OperationalTETA))
	}
	return delay
}
