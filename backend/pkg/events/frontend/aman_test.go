package frontend

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"

	"github.com/stretchr/testify/require"
)

func TestAMANStateEventMatchesSharedV1Golden(t *testing.T) {
	event, err := NewAMANStateEvent(goldenAMANState(), aman.EffectiveAuthoritative, goldenAMANHealth())
	require.NoError(t, err)

	encoded, err := event.Marshal()
	require.NoError(t, err)
	expected, err := os.ReadFile("testdata/aman-state-v1.json")
	require.NoError(t, err)

	var actualJSON, expectedJSON any
	require.NoError(t, json.Unmarshal(encoded, &actualJSON))
	require.NoError(t, json.Unmarshal(expected, &expectedJSON))
	require.Equal(t, expectedJSON, actualJSON)
}

func TestAMANStateEventIncludesAuthoritativeTrafficPrediction(t *testing.T) {
	state := goldenAMANState()
	effective := state.GeneratedAt.Add(-time.Hour)
	state.RunwayGroups[0].Selected = true
	state.RunwayGroups[0].ActiveRatePerHour = 20
	state.RunwayGroups[0].RateEffectiveAt = &effective
	state.RunwayGroups[0].SelectionSchedule = []aman.RunwayGroupSelectionPoint{{EffectiveAt: effective}}
	state.RunwayGroups[0].RateSchedule = []aman.RunwayGroupRatePoint{{EffectiveAt: effective, ArrivalsPerHour: 20}}

	event, err := NewAMANStateEvent(state, aman.EffectiveAuthoritative, goldenAMANHealth())
	require.NoError(t, err)
	require.Equal(t, "2026-07-22T10:00:00.000Z", event.Data.TrafficPrediction.RangeStart)
	require.Equal(t, "2026-07-22T13:00:00.000Z", event.Data.TrafficPrediction.RangeEnd)
	require.Len(t, event.Data.TrafficPrediction.Buckets, 12)
	require.Equal(t, 1, event.Data.TrafficPrediction.Buckets[1].AirborneCount)
	require.Equal(t, "aman", event.Data.TrafficPrediction.Buckets[1].Flights[0].TimingSource)
	require.EqualValues(t, 20, event.Data.TrafficPrediction.Buckets[1].SelectedRate.ArrivalsPerHour)
}

func TestAMANStateEventSerializesOptionalHoldingFacts(t *testing.T) {
	state := goldenAMANState()
	altitude := int32(12000)
	state.Flights[0].LatestObservation = &aman.FlightObservation{
		FlightID: "flight-123", VATSIMCID: "1234567", Callsign: "SAS123", Origin: "ESSA", Destination: " ekch ",
		ReconciledAt: state.GeneratedAt, SourceStatus: aman.DataFresh,
	}
	state.Flights[0].HoldingClearance = &aman.HoldingClearance{
		Hold: "OLPIB", HoldType: aman.HoldingClearanceEnroute, HoldEAT: "1015",
		ClearedAltitude: &altitude, ObservedAt: state.GeneratedAt,
	}

	event, err := NewAMANStateEvent(state, aman.EffectiveAuthoritative, goldenAMANHealth())
	require.NoError(t, err)
	expectedEAT := "2026-07-22T10:15:00.000Z"
	require.Equal(t, []AMANHoldingEntry{{
		FlightID: "flight-123", Callsign: "SAS123", Holding: "OLPIB", EAT: &expectedEAT,
		ClearedAltitude: &altitude, SourceStatus: "fresh", ObservedAt: "2026-07-22T10:00:00.000Z",
	}}, event.Data.HoldingInformation)

	encoded, err := event.Marshal()
	require.NoError(t, err)
	require.JSONEq(t, `{"flight_id":"flight-123","callsign":"SAS123","holding":"OLPIB","eat":"2026-07-22T10:15:00.000Z","cleared_altitude":12000,"source_status":"fresh","observed_at":"2026-07-22T10:00:00.000Z"}`, firstHoldingJSON(t, encoded))
}

func TestAMANStateEventSerializesMissingHoldingDataAsNull(t *testing.T) {
	state := goldenAMANState()
	state.Flights[0].LatestObservation = &aman.FlightObservation{
		FlightID: "flight-123", VATSIMCID: "1234567", Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH",
		ReconciledAt: state.GeneratedAt, SourceStatus: aman.DataFresh,
	}
	state.Flights[0].HoldingClearance = &aman.HoldingClearance{
		Hold: "OLPIB", HoldType: aman.HoldingClearanceEnroute, ObservedAt: state.GeneratedAt,
	}

	event, err := NewAMANStateEvent(state, aman.EffectiveAuthoritative, goldenAMANHealth())
	require.NoError(t, err)
	require.Nil(t, event.Data.HoldingInformation[0].EAT)
	require.Nil(t, event.Data.HoldingInformation[0].ClearedAltitude)
	encoded, err := event.Marshal()
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"eat":null`)
	require.Contains(t, string(encoded), `"cleared_altitude":null`)
}

func firstHoldingJSON(t *testing.T, payload []byte) string {
	t.Helper()
	var event struct {
		Data struct {
			HoldingInformation []json.RawMessage `json:"holding_information"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(payload, &event))
	require.Len(t, event.Data.HoldingInformation, 1)
	return string(event.Data.HoldingInformation[0])
}

func TestAMANStateEventRejectsEffectiveModeHealthFromAnotherState(t *testing.T) {
	health := goldenAMANHealth()
	health.EffectiveMode = aman.EffectiveBlocked

	_, err := NewAMANStateEvent(goldenAMANState(), aman.EffectiveAuthoritative, health)
	require.EqualError(t, err, "map AMAN state event: effective mode and health must belong to the same state")
}

func TestAMANStateEventNormalizesDisabledComponentHealth(t *testing.T) {
	now := time.Date(2026, time.July, 22, 10, 0, 0, 0, time.UTC)
	state := aman.AirportState{
		Airport: "EKCH", GeneratedAt: now, PolicyVersion: "ekch-aman-v1", Mode: aman.ModeDisabled,
		Flights: []aman.AMANFlight{}, RunwayGroups: []aman.RunwayGroupPolicy{},
	}
	health := aman.EvaluateTechnicalHealth(aman.ModeDisabled, aman.ComponentHealth{}, aman.ComponentHealth{}, aman.ComponentHealth{}, aman.ComponentHealth{}, aman.ComponentHealth{}, aman.ComponentHealth{})

	event, err := NewAMANStateEvent(state, aman.EffectiveDisabled, health)
	require.NoError(t, err)
	require.Equal(t, "disabled", event.Data.TechnicalHealth.Status)
	require.Equal(t, "disabled", event.Data.TechnicalHealth.VATSIM.Status)
	require.Equal(t, "disabled", event.Data.TechnicalHealth.Navigation.Status)
	require.Equal(t, "disabled", event.Data.TechnicalHealth.Weather.Status)
	require.Equal(t, "disabled", event.Data.TechnicalHealth.Repository.Status)
	require.Equal(t, "disabled", event.Data.TechnicalHealth.Predictor.Status)
	require.Equal(t, "disabled", event.Data.TechnicalHealth.ReplayValidation.Status)
}

func TestAMANStateEventIncludesPersistedActiveRate(t *testing.T) {
	state := goldenAMANState()
	effective := state.GeneratedAt.Add(time.Minute)
	state.RunwayGroups[0].ActiveRatePerHour = 20
	state.RunwayGroups[0].RateEffectiveAt = &effective
	event, err := NewAMANStateEvent(state, aman.EffectiveAuthoritative, goldenAMANHealth())
	require.NoError(t, err)
	require.Equal(t, uint32(20), *event.Data.RunwayGroups[0].ActiveRatePerHour)
	expected, err := aman.FormatTime(effective)
	require.NoError(t, err)
	require.Equal(t, expected, *event.Data.RunwayGroups[0].RateEffectiveAt)
}

func TestAMANStateEventIncludesRunwaySelectionStateAndSchedule(t *testing.T) {
	state := goldenAMANState()
	effective := state.GeneratedAt.Add(15 * time.Minute)
	conflict := "protected traffic conflict"
	state.RunwayGroups[0].SelectionSchedule = []aman.RunwayGroupSelectionPoint{{
		EffectiveAt: effective, CommandRevision: 7, Source: aman.RunwayGroupSelectionSourceFMPCommand,
	}}
	state.RunwayGroups[0].SelectionConflict = &conflict
	event, err := NewAMANStateEvent(state, aman.EffectiveAuthoritative, goldenAMANHealth())
	require.NoError(t, err)
	require.True(t, event.Data.RunwayGroups[0].Selected)
	require.Equal(t, []string{"2026-07-22T10:15:00.000Z"}, event.Data.RunwayGroups[0].SelectionSchedule)
	require.Equal(t, conflict, *event.Data.RunwayGroups[0].SelectionConflict)
}

func TestAMANStateEventProjectsProtectedSameSTARWarningIdentity(t *testing.T) {
	state := goldenAMANState()
	state.RunwayGroups[0].SequenceWarnings = []aman.RunwayGroupSequenceWarning{{
		Code: "protected_same_star_spacing", FlightID: "TRAIL", RelatedFlightID: "LEAD", STARFamily: "MONAK",
	}}
	event, err := NewAMANStateEvent(state, aman.EffectiveAuthoritative, goldenAMANHealth())
	require.NoError(t, err)
	require.Equal(t, []AMANRunwayGroupSequenceWarning{{
		Code: "protected_same_star_spacing", FlightID: "TRAIL", RelatedFlightID: "LEAD", STARFamily: "MONAK",
	}}, event.Data.RunwayGroups[0].SequenceWarnings)
}

func TestAMANStateEventProjectsCompleteCurrentWarnings(t *testing.T) {
	state := goldenAMANState()
	state.RunwayGroups[0].SequenceWarnings = []aman.RunwayGroupSequenceWarning{{
		Code: "protected_same_star_spacing", FlightID: "TRAIL", RelatedFlightID: "LEAD", STARFamily: "MONAK",
	}}
	health := goldenAMANHealth()
	reason := "airac_expired"
	health.Status, health.Ready = aman.HealthDegraded, false
	health.Navigation = aman.ComponentHealth{Status: aman.HealthDegraded, Reason: reason}

	event, err := NewAMANStateEvent(state, aman.EffectiveAuthoritative, health)
	require.NoError(t, err)
	require.Len(t, event.Data.Warnings, 2)
	require.Equal(t, AMANWarning{
		ID:     `warning:"sequence"/-/"protected_same_star_spacing"/"ARRIVAL-22"/"TRAIL"/"LEAD"`,
		Source: "sequence", Severity: "error", Code: "protected_same_star_spacing",
		RunwayGroupID: stringPointer(&state.RunwayGroups[0].ID), FlightID: stringPointer(&state.RunwayGroups[0].SequenceWarnings[0].FlightID),
		RelatedFlightID: stringPointer(&state.RunwayGroups[0].SequenceWarnings[0].RelatedFlightID),
		Message:         "Flights TRAIL and LEAD conflict with protected MONAK spacing on runway group ARRIVAL-22",
	}, event.Data.Warnings[0])
	require.Equal(t, "technical_health", event.Data.Warnings[1].Source)
	require.Equal(t, "navigation", *event.Data.Warnings[1].Component)
	require.Equal(t, "airac_expired", event.Data.Warnings[1].Code)

	clearEvent, err := NewAMANStateEvent(goldenAMANState(), aman.EffectiveAuthoritative, goldenAMANHealth())
	require.NoError(t, err)
	require.NotNil(t, clearEvent.Data.Warnings)
	require.Empty(t, clearEvent.Data.Warnings)
	encoded, err := clearEvent.Marshal()
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"warnings":[]`)
}

func TestAMANWarningsAreAdditiveForLegacyV1Decoders(t *testing.T) {
	event, err := NewAMANStateEvent(goldenAMANState(), aman.EffectiveAuthoritative, goldenAMANHealth())
	require.NoError(t, err)
	encoded, err := event.Marshal()
	require.NoError(t, err)

	var legacy struct {
		Version int `json:"version"`
		Data    struct {
			Airport string `json:"airport"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(encoded, &legacy))
	require.Equal(t, AMANWireVersion, legacy.Version)
	require.Equal(t, "EKCH", legacy.Data.Airport)
}

func TestAMANStateEventProjectsActiveRunwayGroupsInConfiguredOrder(t *testing.T) {
	state := goldenAMANState()
	state.RunwayGroups = append(state.RunwayGroups, aman.RunwayGroupPolicy{ID: "ARRIVAL-04"})
	state.ActiveRunwayGroups = []aman.RunwayGroupID{"ARRIVAL-04", "ARRIVAL-22"}

	event, err := NewAMANStateEvent(state, aman.EffectiveAuthoritative, goldenAMANHealth())
	require.NoError(t, err)
	require.Equal(t, []string{"ARRIVAL-22", "ARRIVAL-04"}, event.Data.ActiveRunwayGroups)
}

func TestAMANStateEventOmitsActiveRunwayGroupsForLegacyState(t *testing.T) {
	event, err := NewAMANStateEvent(goldenAMANState(), aman.EffectiveAuthoritative, goldenAMANHealth())
	require.NoError(t, err)
	encoded, err := event.Marshal()
	require.NoError(t, err)
	require.NotContains(t, string(encoded), `"active_runway_groups"`)
}

func TestAMANFlightProjectsTMAFreezeForNewAndLegacyV1Decoders(t *testing.T) {
	state := goldenAMANState()
	state.Flights[0].FreezeReason = aman.FreezeTMA
	mapped, err := mapAMANFlight(state.GeneratedAt, state.Flights[0])
	require.NoError(t, err)
	require.Equal(t, AMANFreezeTMA, mapped.FreezeReason)

	encoded, err := json.Marshal(mapped)
	require.NoError(t, err)
	var legacy struct {
		FlightID     string `json:"flight_id"`
		FreezeReason string `json:"freeze_reason"`
	}
	require.NoError(t, json.Unmarshal(encoded, &legacy))
	require.Equal(t, "flight-123", legacy.FlightID)
	require.Equal(t, "tma", legacy.FreezeReason)
}

func TestAMANFlightProjectsDesequencedDispositionAdditively(t *testing.T) {
	state := goldenAMANState()
	state.Flights[0].SequenceDisposition = aman.SequenceDispositionDesequenced
	mapped, err := mapAMANFlight(state.GeneratedAt, state.Flights[0])
	require.NoError(t, err)
	require.Equal(t, "desequenced", mapped.SequenceDisposition)

	encoded, err := json.Marshal(mapped)
	require.NoError(t, err)
	var legacy struct {
		FlightID string `json:"flight_id"`
	}
	require.NoError(t, json.Unmarshal(encoded, &legacy))
	require.Equal(t, "flight-123", legacy.FlightID)
}

func TestAMANFlightRejectsUnknownFreezeReason(t *testing.T) {
	state := goldenAMANState()
	state.Flights[0].FreezeReason = aman.FreezeReason("future")
	_, err := mapAMANFlight(state.GeneratedAt, state.Flights[0])
	require.EqualError(t, err, `unsupported freeze reason "future"`)
}

func TestAMANStateEventActiveSetIsAdditiveForLegacyV1Decoders(t *testing.T) {
	state := goldenAMANState()
	state.ActiveRunwayGroups = []aman.RunwayGroupID{"ARRIVAL-22"}
	event, err := NewAMANStateEvent(state, aman.EffectiveAuthoritative, goldenAMANHealth())
	require.NoError(t, err)
	encoded, err := event.Marshal()
	require.NoError(t, err)

	var legacy struct {
		Data struct {
			RunwayGroups []AMANRunwayGroup `json:"runway_groups"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(encoded, &legacy))
	require.Equal(t, "ARRIVAL-22", legacy.Data.RunwayGroups[0].ID)
	require.True(t, legacy.Data.RunwayGroups[0].Selected)
}

func TestProjectAMANTimelineConfigOrdersMappingsAndPreservesUnusedSides(t *testing.T) {
	tespi, tudlo, ernov := navdata.STARFamilyID("TESPI"), navdata.STARFamilyID("TUDLO"), navdata.STARFamilyID("ERNOV")
	source := []navdata.TimelineMapping{{ID: 3, Left: &ernov}, {ID: 1, Left: &tespi, Right: &tudlo}}
	projected := ProjectAMANTimelineConfig("EKCH-AIP-2609-V4", source)
	require.Equal(t, &AMANTimelineConfig{Version: "EKCH-AIP-2609-V4", Mappings: []AMANTimelineMapping{
		{ID: 1, Left: stringPointer(&tespi), Right: stringPointer(&tudlo)},
		{ID: 3, Left: stringPointer(&ernov), Right: nil},
	}}, projected)
	require.Equal(t, navdata.TimelineMappingID(3), source[0].ID, "projection must not mutate source order")
}

func TestAMANTimelineConfigIsOptionalAndAdditiveForLegacyV1Decoders(t *testing.T) {
	event, err := NewAMANStateEvent(goldenAMANState(), aman.EffectiveAuthoritative, goldenAMANHealth())
	require.NoError(t, err)
	require.Nil(t, ProjectAMANTimelineConfig("", nil))
	tespi := navdata.STARFamilyID("TESPI")
	event.Data.TimelineConfig = ProjectAMANTimelineConfig("mapping-v1", []navdata.TimelineMapping{{ID: 1, Left: &tespi}})
	encoded, err := event.Marshal()
	require.NoError(t, err)

	var legacy struct {
		Version int `json:"version"`
		Data    struct {
			Airport string `json:"airport"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(encoded, &legacy))
	require.Equal(t, AMANWireVersion, legacy.Version)
	require.Equal(t, "EKCH", legacy.Data.Airport)
}

func TestAMANFlightOmitsNonPublishablePredictionData(t *testing.T) {
	state := goldenAMANState()
	state.Flights[0].Prediction.Publishable = false
	reason := "missing_essential_data:surveillance"
	state.Flights[0].Prediction.DegradationReason = &reason

	mapped, err := mapAMANFlight(state.GeneratedAt, state.Flights[0])
	require.NoError(t, err)
	require.Nil(t, mapped.RawTETA)
	require.Nil(t, mapped.OperationalTETA)
	require.Nil(t, mapped.Confidence)
	require.Nil(t, mapped.Provenance)
	require.Nil(t, mapped.InputAgeSeconds)
	require.Nil(t, mapped.DistanceToGoNM)
	require.Nil(t, mapped.GainLossSeconds)
	require.NotNil(t, mapped.Slot, "protected slot publication is independent from prediction publication")
}

func TestAMANFlightRoundsLegacyFractionalInputAgeForWire(t *testing.T) {
	state := goldenAMANState()
	state.GeneratedAt = state.GeneratedAt.Add(time.Minute + 600*time.Millisecond)
	state.Flights[0].Prediction.OperationalTETA = state.Flights[0].Prediction.OperationalTETA.Add(600 * time.Millisecond)
	mapped, err := mapAMANFlight(state.GeneratedAt, state.Flights[0])
	require.NoError(t, err)
	require.Equal(t, "TESPI", *mapped.Star)
	require.Equal(t, "TESPI", *mapped.STARFamily)
	require.Equal(t, "TNO", *mapped.FeederFix)
	require.Equal(t, "ROSBI", *mapped.HoldingFix)
	require.EqualValues(t, 121, *mapped.InputAgeSeconds)
	require.EqualValues(t, 61, *mapped.GainLossSeconds)
}

func TestAMANFlightSerializesFeederETAProvenanceWithoutInventingPassedTime(t *testing.T) {
	state := goldenAMANState()
	mapped, err := mapAMANFlight(state.GeneratedAt, state.Flights[0])
	require.NoError(t, err)
	require.Equal(t, "2026-07-22T10:12:00.000Z", *mapped.FeederFixETA)
	require.Equal(t, "route", *mapped.FeederFixETASource)
	require.False(t, *mapped.FeederFixPassed)

	state.Flights[0].FeederETA = &aman.FeederETAState{Source: aman.FeederETASourcePassed, Passed: true}
	mapped, err = mapAMANFlight(state.GeneratedAt, state.Flights[0])
	require.NoError(t, err)
	require.Nil(t, mapped.FeederFixETA)
	require.Equal(t, "passed", *mapped.FeederFixETASource)
	require.True(t, *mapped.FeederFixPassed)
}

func TestAMANFlightPublishesPendingGoAroundEvidence(t *testing.T) {
	state := goldenAMANState()
	detectedAt := state.GeneratedAt.Add(-time.Minute)
	state.Flights[0].GoAroundConfirmation = &aman.GoAroundConfirmation{
		EpisodeID: "flight-123/go-around/1", Reason: "climb", DetectedAt: detectedAt,
		EvidenceTimes: []time.Time{detectedAt.Add(-time.Second), detectedAt}, Status: aman.GoAroundConfirmationPending,
	}

	mapped, err := mapAMANFlight(state.GeneratedAt, state.Flights[0])
	require.NoError(t, err)
	require.Equal(t, "flight-123/go-around/1", mapped.GoAroundConfirmation.EpisodeID)
	require.Equal(t, "pending", mapped.GoAroundConfirmation.Status)
	require.Len(t, mapped.GoAroundConfirmation.EvidenceTimes, 2)
	require.Nil(t, mapped.GoAroundConfirmation.DecidedAt)
}

func TestAMANCommandRejectionCarriesStableCorrelation(t *testing.T) {
	event, err := NewAMANCommandRejectedEvent("command-7", 9, &aman.DomainError{
		Class: aman.ErrorRevisionConflict, Message: "revision changed",
	}, true)
	require.NoError(t, err)
	require.Equal(t, AMANCommandRejectedEvent{Version: 1, Data: AMANCommandRejection{
		CommandID: "command-7", Code: "revision_conflict", Message: "revision changed", CurrentRevision: 9, Retryable: true,
	}}, event)
}

func goldenAMANState() aman.AirportState {
	now := time.Date(2026, time.July, 22, 10, 0, 0, 0, time.UTC)
	runwayGroup := aman.RunwayGroupID("ARRIVAL-22")
	starFamily, feederFix, holding := "TESPI", "TNO", "ROSBI"
	dtg := 87.5
	return aman.AirportState{
		Airport: "EKCH", Revision: 7, GeneratedAt: now, PolicyVersion: "ekch-aman-v1",
		Mode: aman.ModeAuthoritative, Authoritative: true,
		RunwayGroups: []aman.RunwayGroupPolicy{{ID: runwayGroup, Selected: true}},
		Flights: []aman.AMANFlight{{
			ID: "flight-123", VATSIMCID: "1234567", CurrentCallsign: "SAS123",
			State: aman.StateStable, DataStatus: aman.DataFresh, SelectedRunwayGroup: &runwayGroup,
			SelectedFeeder: &starFamily, SelectedSTARFamily: &starFamily, SelectedFeederFix: &feederFix, SelectedHolding: &holding,
			FeederETA:       &aman.FeederETAState{ETA: timePointer(now.Add(12 * time.Minute)), Source: aman.FeederETASourceRoute},
			ActiveRouteFact: &aman.RouteFact{ID: "route-fact-1", Fix: "SOK", ObservedAt: now.Add(-2 * time.Minute), State: aman.RouteFactActive},
			Prediction: &aman.Prediction{
				RawTETA: now.Add(20 * time.Minute), OperationalTETA: now.Add(19 * time.Minute), OperationalReason: aman.OperationalReasonSmoothed,
				GeneratedAt: now, InputObservedAt: now.Add(-time.Minute), Confidence: aman.ConfidenceHigh, Publishable: true,
				DatasetVersion: "2607", GeometryDigest: "geometry-sha256", DistanceToGoNM: &dtg,
				HoldingFixETA: timePointer(now.Add(10 * time.Minute)), HoldingPlan: &aman.HoldingPlan{HoldingEntryTime: now.Add(10 * time.Minute), ApproachReleaseTime: now.Add(18 * time.Minute), ExpectedHoldingDuration: 8 * time.Minute, PostHoldingTransit: 10 * time.Minute}, ModelVersion: "performance-wind-v1", ConfigVersion: "ekch-v1", Sources: []string{"vatsim", "airacnet"},
			},
			FreezeReason: aman.FreezeNone,
			Slot:         &aman.Slot{Time: now.Add(18 * time.Minute), RunwayGroupID: runwayGroup, Sequence: 3, Revision: 7, Reason: "rate_wtc"},
			Order:        intPointer(3), QueueOffers: []aman.QueueOffer{}, UpdatedAt: now,
		}},
	}
}

func goldenAMANHealth() aman.TechnicalHealth {
	now := time.Date(2026, time.July, 22, 10, 0, 0, 0, time.UTC)
	age := float64(0)
	ready := aman.ComponentHealth{Status: aman.HealthReady, UpdatedAt: &now, AgeSeconds: &age}
	return aman.TechnicalHealth{
		Enabled: true, Mode: aman.ModeAuthoritative, DesiredMode: aman.ModeAuthoritative,
		EffectiveMode: aman.EffectiveAuthoritative, AuthorityAllowed: true, Ready: true, Status: aman.HealthReady,
		BlockedReasons: []string{}, VATSIM: ready, Navigation: ready, Weather: ready,
		Repository: ready, Predictor: ready, ReplayValidation: ready,
	}
}

func timePointer(value time.Time) *time.Time { return &value }
func intPointer(value int) *int              { return &value }
