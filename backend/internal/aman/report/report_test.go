package report

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/replay"
	"github.com/stretchr/testify/require"
)

func TestCompileProducesCanonicalReportAndEvidence(t *testing.T) {
	input := validInput()
	first, err := Compile(input)
	require.NoError(t, err)
	second, err := Compile(input)
	require.NoError(t, err)
	require.True(t, first.Passed)
	require.Equal(t, first.Digest, second.Digest)
	encoded, err := first.CanonicalJSON()
	require.NoError(t, err)
	decoded, err := Decode(bytes.NewReader(encoded))
	require.NoError(t, err)
	require.Equal(t, first, decoded)
	markdown, err := first.Markdown()
	require.NoError(t, err)
	require.Contains(t, markdown, "Status: PASS")
	evidence, err := first.Evidence(input.EvaluatedAt)
	require.NoError(t, err)
	require.Equal(t, first.Digest, evidence.ID)
}

func TestHorizonBoundariesAndExclusionsAreExplicit(t *testing.T) {
	input := validInput()
	input.Policy.Horizons[0].MinimumSeconds = 3600
	input.Policy.Horizons[0].MaximumSeconds = 3600
	report, err := Compile(input)
	require.NoError(t, err)
	metric := report.Horizons[0]
	require.Equal(t, 1, metric.EligibleCount)
	require.InDelta(t, 30, metric.MAESeconds, .001)
	require.Equal(t, 0, metric.ExcludedMissingTruthCount)
	require.True(t, metric.Passed, "the exact inclusive boundary is eligible")
}

func TestInjectedIntegrityFindingsAreCritical(t *testing.T) {
	for _, kind := range []IntegrityKind{IntegrityDuplicateFlight, IntegrityDuplicateOrder, IntegrityDuplicateRevision, IntegrityWTCViolation, IntegrityUnauthorizedFreeze, IntegrityCommandIdempotency, IntegrityUnauthorizedDirectFact} {
		t.Run(string(kind), func(t *testing.T) {
			input := validInput()
			input.Findings = []Finding{{Kind: kind, Detail: "fixture"}}
			report, err := Compile(input)
			require.NoError(t, err)
			require.False(t, report.Passed)
			require.Equal(t, 1, report.Integrity.Critical())
		})
	}
}

func TestSuperstablePromotionRequiresMatchingAuditEvidence(t *testing.T) {
	start := time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("ARRIVAL-22")
	frozenTETA := start.Add(3 * time.Minute)
	oldSlot := aman.Slot{Time: start.Add(9 * time.Minute), RunwayGroupID: group, Sequence: 3, Revision: 7, Reason: "freeze_superstable"}
	newSlot := aman.Slot{Time: start.Add(6 * time.Minute), RunwayGroupID: group, Sequence: 2, Revision: 8, Reason: "freeze_superstable"}
	old := aman.AMANFlight{Callsign: "HOLDING", FreezeReason: aman.FreezeSuperstable, FrozenAt: &start,
		FrozenOperationalTETA: &frozenTETA, Slot: &oldSlot, FrozenSlot: &oldSlot}
	current := old
	current.Slot, current.FrozenSlot = &newSlot, &newSlot
	payload, err := json.Marshal(map[string]any{
		"action": "queue_promotion", "callsign": current.Callsign,
		"from_sequence": oldSlot.Sequence, "from_time": oldSlot.Time,
		"to_sequence": newSlot.Sequence, "to_time": newSlot.Time,
		"runway_group_id": group,
	})
	require.NoError(t, err)
	audit := aman.AuditRecord{Revision: newSlot.Revision, Category: "aman.queue_promotion", Payload: payload}
	result := replay.Result{Outputs: []replay.Output{
		{Index: 0, Outcome: replay.Outcome{AirportState: &aman.AirportState{Revision: oldSlot.Revision, Flights: []aman.AMANFlight{old}}}},
		{Index: 1, Outcome: replay.Outcome{AirportState: &aman.AirportState{Revision: newSlot.Revision, Flights: []aman.AMANFlight{current}}}},
	}}

	require.Equal(t, 1, integrity(replay.Dataset{}, result, nil).UnauthorizedFreezeMoves)
	result.Outputs[1].Outcome.Audit = []aman.AuditRecord{audit}
	require.Zero(t, integrity(replay.Dataset{}, result, nil).UnauthorizedFreezeMoves)
	audit.Payload = []byte(`{"action":"queue_promotion","callsign":"OTHER"}`)
	result.Outputs[1].Outcome.Audit = []aman.AuditRecord{audit}
	require.Equal(t, 1, integrity(replay.Dataset{}, result, nil).UnauthorizedFreezeMoves)
}

func validInput() Input {
	base := time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
	flightID := aman.Callsign("SAS123")
	predicted := base.Add(time.Hour).Add(30 * time.Second)
	errorSeconds := int64(30)
	dataset := replay.Dataset{Version: replay.DatasetVersion, Metadata: replay.Metadata{ID: "fixture", Airport: "EKCH", CodeDigest: "code", ConfigDigest: "config", Geometry: replay.Manifest{Version: "2607", Digest: "nav"}, WeatherDigest: "weather", ClockOrigin: base}, Records: []replay.Record{
		{Index: 0, At: base, Observation: &aman.FlightObservation{Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", ReconciledAt: base, SourceStatus: aman.DataFresh}},
		{Index: 1, At: base.Add(time.Hour), Landing: &replay.LandingTruth{Callsign: flightID, Source: "tower", LandedAt: base.Add(time.Hour)}},
	}}
	digest, _ := dataset.Digest()
	return Input{Dataset: dataset, Replay: replay.Result{DatasetDigest: digest, OutputDigest: "replay", Outputs: []replay.Output{
		{Index: 0, Outcome: replay.Outcome{Prediction: &aman.Prediction{GeneratedAt: base}}},
		{Index: 1, Outcome: replay.Outcome{LandingComparisons: []replay.LandingComparison{{Callsign: flightID, PredictedAt: &predicted, LandedAt: base.Add(time.Hour), ErrorSeconds: &errorSeconds, TruthSource: "tower"}}}},
	}}, Policy: Policy{Version: "policy-v1", Horizons: []Horizon{{Name: "one-hour", MinimumSeconds: 0, MaximumSeconds: 3600, MaximumMAESeconds: 60, MaximumRMSESeconds: 60, MinimumAvailability: 1}}}, Provenance: Provenance{NavigationDigest: "nav", TerminalDigest: "terminal", HoldingDigest: "holding"}, EvaluatedAt: base.Add(2 * time.Hour)}
}
