package diagnostics

import (
	"FlightStrips/internal/aman"
	"bytes"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"log/slog"
	"testing"
	"time"
)

func TestCommittedPromotionExplainsL8ToL2WithoutChangingPrediction(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })
	now := time.Date(2026, 10, 19, 18, 0, 0, 0, time.UTC)
	old := aman.AMANFlight{Callsign: "SAS123", State: aman.StateStable, Prediction: &aman.Prediction{RawTETA: now.Add(20 * time.Minute), OperationalTETA: now.Add(20 * time.Minute)}, Slot: &aman.Slot{Time: now.Add(28 * time.Minute), RunwayGroupID: "A", Sequence: 1}}
	updated := old
	slot := *old.Slot
	slot.Time = slot.Time.Add(-6 * time.Minute)
	updated.Slot = &slot
	before := aman.AirportState{Airport: "EKCH", SessionID: 482, Revision: 7, Flights: []aman.AMANFlight{old}}
	after := before
	after.Revision++
	after.Flights = []aman.AMANFlight{updated}
	payload := []byte(`{"callsign":"SAS123","operation":"star_exchange"}`)
	LogCommittedChanges(t.Context(), before, after, []aman.AuditRecord{{Category: "aman.sequence_operation", Payload: payload}, {Category: "aman.queue_promotion", Payload: []byte(`{"callsign":"SAS123"}`)}}, "capacity-change")
	lines := bytes.Split(bytes.TrimSpace(logs.Bytes()), []byte("\n"))
	var change map[string]any
	require.NoError(t, json.Unmarshal(lines[len(lines)-1], &change))
	require.Equal(t, "AMAN guidance change committed", change["msg"])
	require.EqualValues(t, -480, change["previous_guidance_seconds"])
	require.EqualValues(t, -120, change["guidance_seconds"])
	require.EqualValues(t, -360, change["target_movement_seconds"])
	require.Equal(t, change["previous_raw_eta"], change["raw_eta"])
	require.Equal(t, []any{"star_exchange", "vacancy_promotion", "controller_command"}, change["operations"])
}

func TestSequenceRenumberingDoesNotLookLikeAnOrderChange(t *testing.T) {
	now := time.Date(2026, 10, 19, 18, 0, 0, 0, time.UTC)
	f := aman.AMANFlight{Callsign: "SAS123", State: aman.StateStable, Slot: &aman.Slot{Time: now, RunwayGroupID: "A", Sequence: 2}}
	state := aman.AirportState{Flights: []aman.AMANFlight{f}}
	before := sequenceNeighbours(state)
	state.Flights[0].Slot.Sequence = 1
	require.Equal(t, before, sequenceNeighbours(state))
}
