//go:build task22fault

package faultgate

import (
	pb "FlightStrips/pkg/events/cluster"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

func State(point string, data []byte, sequence uint64) {
	event := &pb.StateEvent{}
	if pb.UnmarshalStrict(data, event) != nil {
		return
	}
	kind := "domain"
	if effect := event.GetEffectChanged(); effect != nil {
		kind = "effect-" + effect.Status.String()
	}
	Reach(point+"-"+kind, event.GetCommandId(), sequence)
}

// Reach reads a fixture-owned control file. Ordinary binaries exclude this
// implementation entirely. No payload, token, credential or effect body is
// written to the checkpoint; only the command identity and timing are recorded.
func Reach(point, commandID string, sequence uint64) {
	dir := os.Getenv("TASK22_GATE_DIR")
	if dir == "" || commandID == "" {
		return
	}
	var control struct{ Point, CommandID string }
	data, err := os.ReadFile(filepath.Join(dir, "control.json"))
	if err != nil || json.Unmarshal(data, &control) != nil || control.Point != point || control.CommandID != commandID {
		return
	}
	checkpoint, _ := json.Marshal(struct {
		Point     string    `json:"point"`
		CommandID string    `json:"command_id"`
		Sequence  uint64    `json:"stream_sequence"`
		PID       int       `json:"pid"`
		At        time.Time `json:"at"`
	}{point, commandID, sequence, os.Getpid(), time.Now().UTC()})
	temporary := filepath.Join(dir, "reached.tmp")
	if os.WriteFile(temporary, checkpoint, 0600) != nil || os.Rename(temporary, filepath.Join(dir, "reached.json")) != nil {
		panic("fault checkpoint unavailable")
	}
	// Removing the control releases the barrier. The harness normally kills
	// the exact child PID after verifying the reached checkpoint instead.
	for {
		if _, err := os.Stat(filepath.Join(dir, "control.json")); os.IsNotExist(err) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}
