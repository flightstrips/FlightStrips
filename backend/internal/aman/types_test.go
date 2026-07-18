package aman

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPredictionJSONUsesUTCInstantsAndExplicitNulls(t *testing.T) {
	instant := time.Date(2026, time.July, 18, 12, 34, 56, 123000000, time.UTC)
	prediction := Prediction{
		RawTETA:           instant,
		OperationalTETA:   instant,
		OperationalReason: "smoothed",
		GeneratedAt:       instant,
		InputObservedAt:   instant,
		Confidence:        ConfidenceHigh,
		DatasetVersion:    "2026-07",
		GeometryDigest:    "abc123",
		ModelVersion:      "model-v1",
		ConfigVersion:     "config-v1",
		Sources:           []string{"vatsim", "nav-cache"},
	}

	encoded, err := json.Marshal(prediction)
	if err != nil {
		t.Fatalf("marshal prediction: %v", err)
	}
	jsonText := string(encoded)
	for _, want := range []string{
		`"raw_teta":"2026-07-18T12:34:56.123Z"`,
		`"operational_teta":"2026-07-18T12:34:56.123Z"`,
		`"degradation_reason":null`,
		`"distance_to_go_nm":null`,
		`"holding_fix_eta":null`,
		`"sources":["vatsim","nav-cache"]`,
	} {
		if !strings.Contains(jsonText, want) {
			t.Errorf("prediction JSON %s does not contain %s", jsonText, want)
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
	if err := flight.Validate(); err != nil {
		t.Fatalf("validate frozen flight: %v", err)
	}
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
		OperationalReason: "raw",
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
		ID:           "flight-1",
		State:        StateStable,
		DataStatus:   DataFresh,
		FreezeReason: FreezeNone,
		UpdatedAt:    now,
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
