package aman

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestAMANFlightTerminalIdentityJSONReplayIsDeterministic(t *testing.T) {
	legacy, starFamily, feederFix := "TESPI", "TESPI", "TNO"
	want := AMANFlight{SelectedFeeder: &legacy, SelectedSTARFamily: &starFamily, SelectedFeederFix: &feederFix}
	first, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal first flight: %v", err)
	}
	var restored AMANFlight
	if err := json.Unmarshal(first, &restored); err != nil {
		t.Fatalf("restore flight: %v", err)
	}
	second, err := json.Marshal(restored)
	if err != nil {
		t.Fatalf("marshal replayed flight: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("replayed JSON changed:\nfirst:  %s\nsecond: %s", first, second)
	}
}

func TestAMANFlightSequenceDispositionJSONReplaysDeterministically(t *testing.T) {
	want := AMANFlight{State: StateLanded, SequenceDisposition: SequenceDispositionDesequenced}
	first, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal flight: %v", err)
	}
	var restored AMANFlight
	if err := json.Unmarshal(first, &restored); err != nil {
		t.Fatalf("restore flight: %v", err)
	}
	second, err := json.Marshal(restored)
	if err != nil {
		t.Fatalf("marshal restored flight: %v", err)
	}
	if !reflect.DeepEqual(want, restored) || string(first) != string(second) {
		t.Fatalf("disposition replay changed:\nflight: %#v\nrestored: %#v\nfirst: %s\nsecond: %s", want, restored, first, second)
	}
}

func TestSequenceDispositionValuesAndDefault(t *testing.T) {
	for _, valid := range []SequenceDisposition{SequenceDispositionActive, SequenceDispositionDesequenced} {
		if !valid.Valid() {
			t.Fatalf("disposition %q should be valid", valid)
		}
	}
	if SequenceDisposition("removed").Valid() {
		t.Fatal("unknown disposition should be invalid")
	}
	if got := (SequenceDisposition("")).OrDefault(); got != SequenceDispositionActive {
		t.Fatalf("default disposition = %q, want active", got)
	}
	if !(SequenceDisposition("")).Participates() || !SequenceDispositionActive.Participates() {
		t.Fatal("default and active dispositions must participate in sequencing")
	}
	if SequenceDispositionDesequenced.Participates() {
		t.Fatal("desequenced disposition must not participate in sequencing")
	}
}

func TestAMANFlightIdentityConsumersPreferExplicitValuesWithLegacyFallback(t *testing.T) {
	legacy, family, fix := "TESPI", "TUDLO", "KOR"
	flight := AMANFlight{SelectedFeeder: &legacy, SelectedSTARFamily: &family, SelectedFeederFix: &fix}
	if actual := flight.STARFamilyIdentity(); actual != family {
		t.Fatalf("STAR family = %q, want %q", actual, family)
	}
	feederFix, legacyFamily := flight.TerminalPathIdentity()
	if feederFix != fix || legacyFamily != "" {
		t.Fatalf("terminal identity = (%q, %q), want (%q, empty)", feederFix, legacyFamily, fix)
	}

	flight.SelectedSTARFamily, flight.SelectedFeederFix = nil, nil
	if actual := flight.STARFamilyIdentity(); actual != legacy {
		t.Fatalf("legacy STAR family = %q, want %q", actual, legacy)
	}
	feederFix, legacyFamily = flight.TerminalPathIdentity()
	if feederFix != "" || legacyFamily != legacy {
		t.Fatalf("legacy terminal identity = (%q, %q), want (empty, %q)", feederFix, legacyFamily, legacy)
	}
}

func TestFormatTimeUsesRFC3339MillisecondsIncludingExactSeconds(t *testing.T) {
	instant := time.Date(2026, time.July, 18, 12, 34, 56, 123000000, time.UTC)
	encoded, err := FormatTime(instant)
	if err != nil {
		t.Fatalf("format millisecond timestamp: %v", err)
	}
	if encoded != "2026-07-18T12:34:56.123Z" {
		t.Errorf("formatted timestamp = %q", encoded)
	}

	exactSecond, err := FormatTime(instant.Truncate(time.Second))
	if err != nil {
		t.Fatalf("format exact-second timestamp: %v", err)
	}
	if exactSecond != "2026-07-18T12:34:56.000Z" {
		t.Errorf("formatted exact-second timestamp = %q", exactSecond)
	}
}

func TestWholeSecondsPreservesDurationWithoutNanosecondSerialization(t *testing.T) {
	seconds, err := WholeSeconds(90 * time.Second)
	if err != nil {
		t.Fatalf("whole seconds: %v", err)
	}
	if seconds != 90 {
		t.Errorf("whole seconds = %d", seconds)
	}

	if _, err := WholeSeconds(1500 * time.Millisecond); err == nil {
		t.Error("expected non-whole duration to be rejected")
	}
}

func TestDomainTypesDoNotDeclareWireJSONTags(t *testing.T) {
	types := []reflect.Type{
		reflect.TypeFor[FlightObservation](),
		reflect.TypeFor[PlannedTiming](),
		reflect.TypeFor[FlightPlanFact](),
		reflect.TypeFor[SurveillanceFact](),
		reflect.TypeFor[Prediction](),
		reflect.TypeFor[FeederETAState](),
		reflect.TypeFor[RawTETASample](),
		reflect.TypeFor[BaselineState](),
		reflect.TypeFor[Slot](),
		reflect.TypeFor[QueueOffer](),
		reflect.TypeFor[RouteFact](),
		reflect.TypeFor[ETAReview](),
		reflect.TypeFor[OperationalException](),
		reflect.TypeFor[GoAroundDetectionState](),
		reflect.TypeFor[LifecycleState](),
		reflect.TypeFor[TMAEntryState](),
		reflect.TypeFor[RunwayGap](),
		reflect.TypeFor[RunwayGroupPolicy](),
		reflect.TypeFor[AMANFlight](),
		reflect.TypeFor[AirportState](),
		reflect.TypeFor[CommandMetadata](),
		reflect.TypeFor[Warning](),
		reflect.TypeFor[WarningSnapshot](),
	}
	for _, domainType := range types {
		for index := range domainType.NumField() {
			field := domainType.Field(index)
			if tag := field.Tag.Get("json"); tag != "" {
				t.Errorf("%s.%s declares wire JSON tag %q", domainType.Name(), field.Name, tag)
			}
		}
	}
}

func TestRunwayGapJSONReplayIsAdditiveAndDeterministic(t *testing.T) {
	var legacy RunwayGroupPolicy
	if err := json.Unmarshal([]byte(`{"ID":"north","Selected":true}`), &legacy); err != nil {
		t.Fatalf("decode legacy runway group: %v", err)
	}
	if legacy.Gaps != nil {
		t.Fatalf("legacy gaps = %#v, want nil", legacy.Gaps)
	}

	now := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	state := AirportState{
		Airport: "EKCH", GeneratedAt: now, PolicyVersion: "gap-v1", Mode: ModeReadOnly,
		RunwayGroups: []RunwayGroupPolicy{{ID: "north", Gaps: []RunwayGap{
			{ID: "gap-1", Start: now.Add(time.Hour), End: now.Add(70 * time.Minute), Label: "approach stop", CreatedAt: now, CreatedBy: "controller-1"},
			{ID: "gap-2", Start: now.Add(80 * time.Minute), End: now.Add(90 * time.Minute), Label: "runway inspection", CreatedAt: now.Add(time.Minute), CreatedBy: "controller-2"},
		}}},
	}
	if err := state.Validate(); err != nil {
		t.Fatalf("validate runway gaps: %v", err)
	}
	first, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal runway gaps: %v", err)
	}
	var restored AirportState
	if err := json.Unmarshal(first, &restored); err != nil {
		t.Fatalf("restore runway gaps: %v", err)
	}
	if err := restored.Validate(); err != nil {
		t.Fatalf("validate restored runway gaps: %v", err)
	}
	second, err := json.Marshal(restored)
	if err != nil {
		t.Fatalf("marshal restored runway gaps: %v", err)
	}
	if string(first) != string(second) || sha256.Sum256(first) != sha256.Sum256(second) {
		t.Fatalf("runway gap replay changed canonical bytes:\nfirst:  %s\nsecond: %s", first, second)
	}
}

func TestAirportStateValidatesCanonicalRunwayGaps(t *testing.T) {
	now := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	gap := RunwayGap{ID: "gap-1", Start: now.Add(time.Hour), End: now.Add(70 * time.Minute), Label: "approach stop", CreatedAt: now, CreatedBy: "controller-1"}
	valid := AirportState{Airport: "EKCH", GeneratedAt: now, PolicyVersion: "gap-v1", Mode: ModeReadOnly, RunwayGroups: []RunwayGroupPolicy{{ID: "north", Gaps: []RunwayGap{gap}}}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("validate runway gap: %v", err)
	}

	for name, mutate := range map[string]func(*AirportState){
		"empty ID":      func(state *AirportState) { state.RunwayGroups[0].Gaps[0].ID = "" },
		"unclean label": func(state *AirportState) { state.RunwayGroups[0].Gaps[0].Label = " reason " },
		"empty creator": func(state *AirportState) { state.RunwayGroups[0].Gaps[0].CreatedBy = "" },
		"non-UTC start": func(state *AirportState) {
			state.RunwayGroups[0].Gaps[0].Start = now.In(time.FixedZone("CEST", 2*60*60))
		},
		"non-UTC end": func(state *AirportState) {
			state.RunwayGroups[0].Gaps[0].End = gap.End.In(time.FixedZone("CEST", 2*60*60))
		},
		"non-UTC created": func(state *AirportState) {
			state.RunwayGroups[0].Gaps[0].CreatedAt = now.In(time.FixedZone("CEST", 2*60*60))
		},
		"empty interval":   func(state *AirportState) { state.RunwayGroups[0].Gaps[0].End = gap.Start },
		"reverse interval": func(state *AirportState) { state.RunwayGroups[0].Gaps[0].End = gap.Start.Add(-time.Second) },
		"duplicate ID": func(state *AirportState) {
			state.RunwayGroups = append(state.RunwayGroups, RunwayGroupPolicy{ID: "south", Gaps: []RunwayGap{gap}})
		},
		"non-canonical order": func(state *AirportState) {
			earlier := gap
			earlier.ID, earlier.Start, earlier.End = "gap-0", gap.Start.Add(-time.Minute), gap.End.Add(-time.Minute)
			state.RunwayGroups[0].Gaps = append(state.RunwayGroups[0].Gaps, earlier)
		},
		"overlapping intervals": func(state *AirportState) {
			overlap := gap
			overlap.ID, overlap.Start, overlap.End = "gap-2", gap.End.Add(-time.Minute), gap.End.Add(time.Minute)
			state.RunwayGroups[0].Gaps = append(state.RunwayGroups[0].Gaps, overlap)
		},
		"touching intervals": func(state *AirportState) {
			touching := gap
			touching.ID, touching.Start, touching.End = "gap-2", gap.End, gap.End.Add(time.Minute)
			state.RunwayGroups[0].Gaps = append(state.RunwayGroups[0].Gaps, touching)
		},
	} {
		t.Run(name, func(t *testing.T) {
			state := valid
			state.RunwayGroups = append([]RunwayGroupPolicy(nil), valid.RunwayGroups...)
			state.RunwayGroups[0].Gaps = append([]RunwayGap(nil), valid.RunwayGroups[0].Gaps...)
			mutate(&state)
			assertInvalidArgument(t, state.Validate())
		})
	}
}

func TestTMAEntryStateRejectsInvalidObservationTime(t *testing.T) {
	state := TMAEntryState{LastContainment: TMAOutside, LastObservedAt: time.Date(2026, time.July, 18, 14, 0, 0, 0, time.FixedZone("CEST", 2*60*60))}
	assertInvalidArgument(t, state.Validate())
	state.LastObservedAt = time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	state.LastContainment = "unknown"
	assertInvalidArgument(t, state.Validate())
}

func TestPredictionRejectsUnknownAsZeroAndNonUTC(t *testing.T) {
	base := validPrediction()
	base.DistanceToGoNM = float64Ptr(-1)
	assertInvalidArgument(t, base.Validate())

	base = validPrediction()
	base.GeneratedAt = base.GeneratedAt.In(time.FixedZone("CEST", 2*60*60))
	assertInvalidArgument(t, base.Validate())

	base = validPrediction()
	base.Sources = nil
	assertInvalidArgument(t, base.Validate())
}

func TestFeederETAStateValidatesTimingAndPassedProvenance(t *testing.T) {
	now := time.Date(2026, time.September, 11, 20, 0, 0, 0, time.UTC)
	for _, source := range []FeederETASource{FeederETASourceRoute, FeederETASourceHolding, FeederETASourceManual} {
		state := FeederETAState{ETA: &now, Source: source}
		if err := state.Validate(); err != nil {
			t.Fatalf("validate %q feeder ETA: %v", source, err)
		}
	}
	if err := (FeederETAState{Source: FeederETASourcePassed, Passed: true}).Validate(); err != nil {
		t.Fatalf("validate passed feeder: %v", err)
	}

	nonUTC := now.In(time.FixedZone("CEST", 2*60*60))
	for _, invalid := range []FeederETAState{
		{ETA: &now, Source: "landing"},
		{ETA: &nonUTC, Source: FeederETASourceRoute},
		{Source: FeederETASourceRoute},
		{ETA: &now, Source: FeederETASourcePassed},
		{ETA: &now, Source: FeederETASourcePassed, Passed: true},
		{Source: FeederETASourceManual, Passed: true},
	} {
		assertInvalidArgument(t, invalid.Validate())
	}
}

func TestAMANFlightFeederETAJSONAcceptsLegacyAbsenceAndReplaysNewState(t *testing.T) {
	var legacy AMANFlight
	if err := json.Unmarshal([]byte(`{"SelectedFeederFix":"TNO"}`), &legacy); err != nil {
		t.Fatalf("decode legacy flight: %v", err)
	}
	if legacy.FeederETA != nil {
		t.Fatal("legacy flight invented feeder ETA state")
	}

	eta := time.Date(2026, time.September, 11, 20, 10, 0, 0, time.UTC)
	manual := eta.Add(-time.Minute)
	want := AMANFlight{
		FeederETA:        &FeederETAState{ETA: &manual, Source: FeederETASourceManual},
		DerivedFeederETA: &FeederETAState{ETA: &eta, Source: FeederETASourceRoute},
	}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("encode feeder ETA state: %v", err)
	}
	var restored AMANFlight
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatalf("restore feeder ETA state: %v", err)
	}
	if !reflect.DeepEqual(want.FeederETA, restored.FeederETA) {
		t.Fatalf("restored feeder ETA = %#v, want %#v", restored.FeederETA, want.FeederETA)
	}
	if !reflect.DeepEqual(want.DerivedFeederETA, restored.DerivedFeederETA) {
		t.Fatalf("restored derived feeder ETA = %#v, want %#v", restored.DerivedFeederETA, want.DerivedFeederETA)
	}
}

func TestFlightFreezeHasOneCanonicalRepresentation(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	flight := validFlight(now)
	flight.FrozenAt = &now
	assertInvalidArgument(t, flight.Validate())

	flight = validFlight(now)
	flight.FreezeReason = FreezeSuperstable
	assertInvalidArgument(t, flight.Validate())

	flight.FrozenAt = &now
	freezeTETA := now.Add(10 * time.Minute)
	flight.FrozenOperationalTETA = &freezeTETA
	slot := Slot{Time: now.Add(11 * time.Minute), RunwayGroupID: "north", Sequence: 1, Reason: "spacing"}
	flight.Slot = &slot
	frozenSlot := slot
	flight.FrozenSlot = &frozenSlot
	if err := flight.Validate(); err != nil {
		t.Fatalf("validate frozen flight: %v", err)
	}
}

func TestBaselineStateRejectsCorruptHeldProvenance(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	baseline := BaselineState{
		ArrivalAt: now.Add(time.Hour), AirborneSensedAt: now, Source: BaselineSourceAirborneGreatCircle,
		Confidence: ConfidenceLow, FlightPlanObservedAt: now, ModelVersion: "baseline-v1", ConfigVersion: "config-v1",
		SpeedDefaultsVersion: "speed-v1",
	}
	degradation := BaselineDegradationGreatCircleUsed
	baseline.DegradationReason = &degradation
	if err := baseline.Validate(); err != nil {
		t.Fatalf("validate great-circle baseline: %v", err)
	}

	baseline.ArrivalAt = now
	assertInvalidArgument(t, baseline.Validate())
	baseline.ArrivalAt = now.Add(time.Hour)
	baseline.Source = BaselineSourcePlannedEOBTFiledEET
	assertInvalidArgument(t, baseline.Validate())
	baseline.Source = BaselineSourceAirborneGreatCircle
	invalidDegradation := BaselineDegradationReason("unknown")
	baseline.DegradationReason = &invalidDegradation
	assertInvalidArgument(t, baseline.Validate())
}

func TestFlightCallsignCorrectionKeepsFlightIDAndVATSIMCID(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	flight := validFlight(now)
	originalID := flight.ID
	originalCID := flight.VATSIMCID

	flight.CurrentCallsign = "SAS124"
	if err := flight.Validate(); err != nil {
		t.Fatalf("validate corrected callsign: %v", err)
	}
	if flight.ID != originalID || flight.VATSIMCID != originalCID {
		t.Fatalf("callsign correction changed stable identity: ID=%q CID=%q", flight.ID, flight.VATSIMCID)
	}
}

func TestActiveFlightRejectsEmptyOrUntrimmedProviderIdentity(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	flight := validFlight(now)
	flight.VATSIMCID = ""
	assertInvalidArgument(t, flight.Validate())

	flight = validFlight(now)
	flight.CurrentCallsign = " SAS123"
	assertInvalidArgument(t, flight.Validate())
}

func TestAirportStateRejectsMismatchedSlotRevisionAndDuplicateFlight(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	flight := validFlight(now)
	flight.Slot = &Slot{
		Time:          now.Add(10 * time.Minute),
		RunwayGroupID: "22L",
		Sequence:      1,
		Revision:      4,
		Reason:        "rate",
	}
	state := AirportState{
		Airport:       "EKCH",
		Revision:      5,
		GeneratedAt:   now,
		PolicyVersion: "v1",
		Mode:          ModeReadOnly,
		Flights:       []AMANFlight{flight},
		RunwayGroups:  []RunwayGroupPolicy{{ID: "22L"}},
	}
	assertInvalidArgument(t, state.Validate())

	flight.Slot.Revision = state.Revision
	state.Flights = append(state.Flights, flight)
	assertInvalidArgument(t, state.Validate())
}

func TestAirportStateValidatesAdditiveActiveRunwaySetWithLegacySelection(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	valid := AirportState{
		Airport: "EKCH", GeneratedAt: now, PolicyVersion: "v1", Mode: ModeReadOnly,
		RunwayGroups:       []RunwayGroupPolicy{{ID: "north", Selected: true}, {ID: "south"}},
		ActiveRunwayGroups: []RunwayGroupID{"north", "south"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("validate active runway set: %v", err)
	}

	invalid := valid
	invalid.ActiveRunwayGroups = []RunwayGroupID{}
	assertInvalidArgument(t, invalid.Validate())
	invalid.ActiveRunwayGroups = []RunwayGroupID{"north", "north"}
	assertInvalidArgument(t, invalid.Validate())
	invalid.ActiveRunwayGroups = []RunwayGroupID{"unknown"}
	assertInvalidArgument(t, invalid.Validate())
	invalid.ActiveRunwayGroups = []RunwayGroupID{"south"}
	assertInvalidArgument(t, invalid.Validate())
	invalid.RunwayGroups = []RunwayGroupPolicy{{ID: "north"}, {ID: "south"}}
	invalid.ActiveRunwayGroups = []RunwayGroupID{"north"}
	assertInvalidArgument(t, invalid.Validate())
}

func TestQueueOfferRequiresMatchingFlightSlotAndAirportRevision(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	flight := validFlight(now)
	flight.Slot = &Slot{Time: now.Add(3 * time.Minute), RunwayGroupID: "north", Sequence: 3, Revision: 4, Reason: "rate_wtc"}
	flight.QueueOffers = []QueueOffer{{
		FlightID: flight.ID, RunwayGroupID: "north",
		CandidateSlot: Slot{Time: now.Add(time.Minute), RunwayGroupID: "north", Sequence: 1, Revision: 4, Reason: "rate_wtc"},
		QueuePosition: 1, ExpiresAt: now.Add(time.Minute), AirportRevision: 4, Reason: QueueOfferEarlierOccupiedSlot,
	}}
	state := AirportState{
		Airport: "EKCH", Revision: 4, GeneratedAt: now, PolicyVersion: "v1", Mode: ModeReadOnly,
		Flights: []AMANFlight{flight}, RunwayGroups: []RunwayGroupPolicy{{ID: "north"}},
	}
	if err := state.Validate(); err != nil {
		t.Fatalf("validate queue offer state: %v", err)
	}

	state.Flights[0].QueueOffers[0].AirportRevision = 3
	assertInvalidArgument(t, state.Validate())
	state.Flights[0].QueueOffers[0].AirportRevision = 4
	state.Flights[0].QueueOffers[0].FlightID = "another-flight"
	assertInvalidArgument(t, state.Validate())
	state.Flights[0].QueueOffers[0].FlightID = flight.ID
	state.Flights[0].QueueOffers[0].ExpiresAt = now
	assertInvalidArgument(t, state.Validate())
}

func TestFlightObservationUsesNeutralUnitsAndOptionalFacts(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	track := 359.9
	observation := FlightObservation{
		FlightID:     "flight-1",
		VATSIMCID:    "1234567",
		Callsign:     "SAS123",
		Origin:       "ESSA",
		Destination:  "EKCH",
		ReconciledAt: now,
		SourceStatus: DataFresh,
		Surveillance: &SurveillanceFact{
			LatitudeDegrees:  55.6,
			LongitudeDegrees: 12.6,
			TrackTrueDegrees: &track,
			ObservedAt:       &now,
		},
	}
	if err := observation.Validate(); err != nil {
		t.Fatalf("validate observation: %v", err)
	}

	track = 360
	assertInvalidArgument(t, observation.Validate())
}

func TestStableErrorClasses(t *testing.T) {
	classes := []ErrorClass{
		ErrorInvalidArgument,
		ErrorNotFound,
		ErrorRevisionConflict,
		ErrorUnauthorized,
		ErrorInvalidTransition,
		ErrorDependencyUnavailable,
		ErrorDegradedOrIncompleteGeometry,
		ErrorUnsupportedLeg,
		ErrorDatasetMismatch,
		ErrorCorruptData,
		ErrorActiveFlightConflict,
	}
	for _, class := range classes {
		if !class.Valid() {
			t.Errorf("error class %q is not valid", class)
		}
	}
}

func validPrediction() Prediction {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	return Prediction{
		RawTETA:           now.Add(15 * time.Minute),
		OperationalTETA:   now.Add(15 * time.Minute),
		OperationalReason: OperationalReasonPredicted,
		GeneratedAt:       now,
		InputObservedAt:   now,
		Confidence:        ConfidenceMedium,
		DatasetVersion:    "2026-07",
		GeometryDigest:    "abc123",
		ModelVersion:      "model-v1",
		ConfigVersion:     "config-v1",
		Sources:           []string{},
	}
}

func validFlight(now time.Time) AMANFlight {
	return AMANFlight{
		ID:              "flight-1",
		VATSIMCID:       "1234567",
		CurrentCallsign: "SAS123",
		State:           StateStable,
		DataStatus:      DataFresh,
		FreezeReason:    FreezeNone,
		UpdatedAt:       now,
	}
}

func float64Ptr(value float64) *float64 {
	return &value
}

func assertInvalidArgument(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected invalid argument error")
	}
	var domainError *DomainError
	if !errors.As(err, &domainError) || domainError.Class != ErrorInvalidArgument {
		t.Fatalf("expected invalid argument domain error, got %v", err)
	}
}
