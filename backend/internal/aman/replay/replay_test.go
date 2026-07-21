package replay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"github.com/stretchr/testify/require"
)

func TestReplayIsByteStableAndRestartsAtEveryCheckpoint(t *testing.T) {
	dataset := fixtureDataset()
	firstProcessor := &recordingProcessor{}
	firstClock := &recordingClock{}
	runner := mustRunner(t, firstClock, firstProcessor)
	full, err := runner.Replay(context.Background(), dataset)
	require.NoError(t, err)
	require.Equal(t, "e3c547662ea568a0b7c278f8e1299710adf5abc58c2462a28b658ea57139ef9c", full.OutputDigest)

	secondProcessor := &recordingProcessor{}
	second := mustRunner(t, &recordingClock{}, secondProcessor)
	again, err := second.Replay(context.Background(), dataset)
	require.NoError(t, err)
	require.Equal(t, full.DatasetDigest, again.DatasetDigest)
	require.Equal(t, full.OutputDigest, again.OutputDigest)
	require.Equal(t, full.Outputs, again.Outputs)

	for index := range dataset.Records {
		prefixProcessor := &recordingProcessor{}
		prefix := mustRunner(t, &recordingClock{}, prefixProcessor)
		partial, err := prefix.ReplayTo(context.Background(), dataset, uint64(index))
		require.NoError(t, err)

		restoredProcessor := &recordingProcessor{}
		restored := mustRunner(t, &recordingClock{}, restoredProcessor)
		resumed, err := restored.Resume(context.Background(), dataset, partial.Checkpoint)
		require.NoError(t, err)
		require.Equal(t, full.Outputs[index+1:], resumed.Outputs, "restart at record %d", index)
		require.Equal(t, partial.Outputs, full.Outputs[:index+1])
	}
	require.Equal(t, dataset.Records[len(dataset.Records)-1].At, firstClock.current)
}

func TestReplayBindsCallsignCorrectionsAndAllowsCIDReuseAfterRetirement(t *testing.T) {
	dataset := fixtureDataset()
	processor := &recordingProcessor{}
	runner := mustRunner(t, &recordingClock{}, processor)
	_, err := runner.Replay(context.Background(), dataset)
	require.NoError(t, err)

	var observations []aman.FlightObservation
	for _, input := range processor.inputs {
		if input.Record.Observation != nil {
			observations = append(observations, *input.Record.Observation)
		}
	}
	require.Len(t, observations, 3)
	require.Equal(t, aman.FlightID("fixture/0"), observations[0].FlightID)
	require.Equal(t, observations[0].FlightID, observations[1].FlightID, "callsign correction must retain replay identity")
	require.Equal(t, "SAS124", observations[1].Callsign)
	require.Equal(t, aman.FlightID("fixture/3"), observations[2].FlightID, "retired CID may start a separate lifetime")
}

func TestDatasetRejectsCorruptAndIncompleteFixtures(t *testing.T) {
	dataset := fixtureDataset()
	dataset.Metadata.ConfigDigest = ""
	require.ErrorIs(t, dataset.Validate(), ErrInvalidDataset)

	dataset = fixtureDataset()
	dataset.Records[1].Index = 9
	require.ErrorIs(t, dataset.Validate(), ErrInvalidDataset)

	dataset = fixtureDataset()
	dataset.Records[0].Policy = &PolicyChange{Version: "also-present"}
	require.ErrorIs(t, dataset.Validate(), ErrInvalidDataset)

	encoded, err := json.Marshal(fixtureDataset())
	require.NoError(t, err)
	_, err = Decode(bytes.NewReader(append(encoded, []byte(` {"unexpected":true}`)...)))
	require.Error(t, err)
	_, err = Decode(bytes.NewReader([]byte(`{"version":"aman-replay/v1","unexpected":true}`)))
	require.Error(t, err)
}

func TestCheckpointRejectsDatasetMismatchAndUnsortedBindings(t *testing.T) {
	dataset := fixtureDataset()
	runner := mustRunner(t, &recordingClock{}, &recordingProcessor{})
	partial, err := runner.ReplayTo(context.Background(), dataset, 1)
	require.NoError(t, err)

	changed := fixtureDataset()
	changed.Metadata.WeatherDigest = "other-weather"
	_, err = runner.Resume(context.Background(), changed, partial.Checkpoint)
	require.ErrorIs(t, err, ErrInvalidCheckpoint)

	partial.Checkpoint.Bindings = []CIDBinding{{VATSIMCID: "200", FlightID: "fixture/0"}, {VATSIMCID: "100", FlightID: "fixture/1"}}
	_, err = runner.Resume(context.Background(), dataset, partial.Checkpoint)
	require.ErrorIs(t, err, ErrInvalidCheckpoint)
}

type recordingClock struct{ current time.Time }

func (c *recordingClock) Set(value time.Time) { c.current = value }

type fixtureIDs struct{}

func (fixtureIDs) FlightID(datasetID string, index uint64) aman.FlightID {
	return aman.FlightID(fmt.Sprintf("%s/%d", datasetID, index))
}

type recordingProcessor struct {
	inputs []Input
}

func (p *recordingProcessor) Apply(_ context.Context, input Input) (Outcome, error) {
	p.inputs = append(p.inputs, input)
	return Outcome{Audit: []aman.AuditRecord{{Airport: "EKCH", Revision: aman.SequenceRevision(input.Record.Index + 1), Category: "replay", RecordedAt: input.Record.At}}}, nil
}

func (p *recordingProcessor) Snapshot(context.Context) ([]byte, error) {
	return json.Marshal(p.inputs)
}

func (p *recordingProcessor) Restore(_ context.Context, snapshot []byte) error {
	return json.Unmarshal(snapshot, &p.inputs)
}

func mustRunner(t *testing.T, clock Clock, processor Processor) *Runner {
	t.Helper()
	runner, err := NewRunner(Dependencies{Clock: clock, IDs: fixtureIDs{}, Processor: processor})
	require.NoError(t, err)
	return runner
}

func fixtureDataset() Dataset {
	base := time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
	return Dataset{
		Version:  DatasetVersion,
		Metadata: Metadata{ID: "fixture", Airport: "EKCH", CodeDigest: "code-v1", ConfigDigest: "config-v1", Geometry: Manifest{Version: "2607", Digest: "geometry-v1"}, WeatherDigest: "weather-v1", ClockOrigin: base},
		Records: []Record{
			{Index: 0, At: base, Observation: observation(base, "SAS123")},
			{Index: 1, At: base.Add(time.Minute), Observation: observation(base.Add(time.Minute), "SAS124")},
			{Index: 2, At: base.Add(2 * time.Minute), Retire: &Retirement{FlightID: "fixture/0", VATSIMCID: "100"}},
			{Index: 3, At: base.Add(3 * time.Minute), Observation: observation(base.Add(3*time.Minute), "SAS125")},
		},
	}
}

func observation(at time.Time, callsign string) *aman.FlightObservation {
	return &aman.FlightObservation{VATSIMCID: "100", Callsign: callsign, Origin: "ESSA", Destination: "EKCH", ReconciledAt: at, SourceStatus: aman.DataFresh}
}

func TestOutputDigestDoesNotDependOnProcessorSliceCapacity(t *testing.T) {
	outputs := []Output{{Index: 1}, {Index: 2}}
	first, err := digestOutputs(outputs)
	require.NoError(t, err)
	outputs = slices.Grow(outputs[:1], 20)
	outputs = append(outputs, Output{Index: 2})
	second, err := digestOutputs(outputs)
	require.NoError(t, err)
	require.Equal(t, first, second)
}

func TestDecodeReportsReaderErrors(t *testing.T) {
	_, err := Decode(errorReader{})
	require.Error(t, err)
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("read failure") }
