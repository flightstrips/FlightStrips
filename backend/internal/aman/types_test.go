package aman

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

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
		reflect.TypeFor[RawTETASample](),
		reflect.TypeFor[BaselineState](),
		reflect.TypeFor[Slot](),
		reflect.TypeFor[RouteFact](),
		reflect.TypeFor[ETAReview](),
		reflect.TypeFor[GoAroundDetectionState](),
		reflect.TypeFor[LifecycleState](),
		reflect.TypeFor[RunwayGroupPolicy](),
		reflect.TypeFor[AMANFlight](),
		reflect.TypeFor[AirportState](),
		reflect.TypeFor[CommandMetadata](),
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
