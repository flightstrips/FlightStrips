package openmeteo

import (
	"FlightStrips/internal/aman/predictor"
	"testing"
	"time"
)

func TestCandidateResourceUsesGridAndForecastHour(t *testing.T) {
	at := time.Date(2026, 10, 3, 12, 1, 0, 0, time.UTC)
	first := predictor.WindProfileRequest{Samples: []predictor.WindSampleRequest{{Position: predictor.WindCoordinate{LatitudeDegrees: 55.61, LongitudeDegrees: 12.01}, At: at}}}
	second := predictor.WindProfileRequest{Samples: []predictor.WindSampleRequest{{Position: predictor.WindCoordinate{LatitudeDegrees: 55.62, LongitudeDegrees: 12.02}, At: at.Add(20 * time.Minute)}}}
	a, err := ResourceForRequest(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ResourceForRequest(second)
	if err != nil || a != b {
		t.Fatalf("same model identity split: %s %s %v", a, b, err)
	}
	second.Samples[0].At = at.Add(time.Hour)
	b, _ = ResourceForRequest(second)
	if a == b {
		t.Fatal("forecast hours shared a resource")
	}
	second.Samples[0].At = at
	second.Samples[0].Position.LatitudeDegrees = 56
	b, _ = ResourceForRequest(second)
	if a == b {
		t.Fatal("different model cells shared a resource")
	}
}

func TestGridProfilePreservesRequestedSampleIdentity(t *testing.T) {
	at := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	profile := predictor.WindProfile{Samples: []predictor.WindSample{{Position: predictor.WindCoordinate{LatitudeDegrees: 55.5, LongitudeDegrees: 12}, At: at, Levels: []predictor.WindLevel{{AltitudeFeet: 1000, EastKnots: 5}}}}}
	request := predictor.WindProfileRequest{Samples: []predictor.WindSampleRequest{{Position: predictor.WindCoordinate{LatitudeDegrees: 55.61, LongitudeDegrees: 12.01}, At: at.Add(time.Minute)}}}
	got, err := projectGridProfile(profile, request)
	if err != nil {
		t.Fatal(err)
	}
	if got.Samples[0].Position != request.Samples[0].Position || !got.Samples[0].At.Equal(request.Samples[0].At) {
		t.Fatal("caller identity lost")
	}
	got.Samples[0].Levels[0].EastKnots = 99
	if profile.Samples[0].Levels[0].EastKnots != 5 || !profile.Samples[0].At.Equal(at) {
		t.Fatal("cached profile mutated")
	}
	request.Samples[0].At = at.Add(time.Hour)
	if _, err := projectGridProfile(profile, request); err == nil {
		t.Fatal("different forecast hour accepted")
	}
}
