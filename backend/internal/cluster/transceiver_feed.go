package cluster

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"FlightStrips/internal/vatsim"
	pb "FlightStrips/pkg/events/cluster"
)

const transceiverResource = "transceivers/v3"

// TransceiverFeed is dormant until Task20 binds the global scheduling loop.
// Both replicas derive the same intent UUID for the configured interval slot.
type TransceiverFeed struct {
	state    NavigationWeather
	worker   ExternalCallWorker
	provider *vatsim.TransceiverProvider
}

func NewTransceiverFeed(state NavigationWeather, provider *vatsim.TransceiverProvider) (*TransceiverFeed, error) {
	if state.Writer.Store == nil || state.Objects == nil || provider == nil {
		return nil, fmt.Errorf("transceiver feed requires state, objects and HTTP provider")
	}
	if err := transceiverNATSWriter(state.Writer, true); err != nil {
		return nil, err
	}
	return &TransceiverFeed{state: state, worker: ExternalCallWorker{Writer: state.Writer}, provider: provider}, nil
}

func (f *TransceiverFeed) RefreshInterval() time.Duration { return f.provider.RefreshInterval() }

func (f *TransceiverFeed) SlotID(at time.Time) (string, error) {
	if at.IsZero() {
		return "", fmt.Errorf("missing transceiver refresh slot")
	}
	slot := at.UTC().Truncate(f.RefreshInterval())
	return ProviderEventCommandID("transceiver-refresh", "vatsim", transceiverResource+"/"+slot.Format(time.RFC3339Nano))
}

// Refresh makes one attempt per accepted slot, even after an uncertain call.
// A malformed/empty HTTP result never replaces the previous checkpoint.
func (f *TransceiverFeed) Refresh(ctx context.Context, at time.Time) (bool, error) {
	id, err := f.SlotID(at)
	if err != nil {
		return false, err
	}
	return f.state.FetchProviderPageFor(ctx, f.worker, id, globalRef(), "vatsim", transceiverResource,
		func(ctx context.Context, _ *pb.ProviderCheckpoint, _ *pb.ProviderPage) (*pb.ProviderPage, *pb.ProviderCheckpoint, error) {
			page, err := f.provider.CandidatePage(ctx)
			if err != nil {
				return nil, nil, err
			}
			return &pb.ProviderPage{Provider: "vatsim", Resource: transceiverResource, Parsed: &pb.ProviderPage_Transceivers{Transceivers: page}},
				&pb.ProviderCheckpoint{Provider: "vatsim", Resource: transceiverResource}, nil
		})
}

// Resume is invoked once on owner takeover, before scheduling new slots.
// It only resolves persisted outcomes and never invokes the HTTP provider.
func (f *TransceiverFeed) Resume(ctx context.Context) error { return f.worker.Resume(ctx, globalRef()) }

// TransceiverGeneration is an immutable accepted lookup for one policy pass.
// Its revision is the checkpoint entity revision, not a local poll counter.
type TransceiverGeneration struct {
	Revision uint64
	Sha256   string
	page     *pb.TransceiverFeedPage
}

func (g TransceiverGeneration) GetFrequencies(callsign string) []string {
	if g.page == nil {
		return nil
	}
	callsign = strings.ToUpper(strings.TrimSpace(callsign))
	i, found := slices.BinarySearchFunc(g.page.Clients, callsign, func(c *pb.TransceiverFeedClient, key string) int { return strings.Compare(c.Callsign, key) })
	if !found {
		return nil
	}
	result := make([]string, 0, len(g.page.Clients[i].FrequenciesHz))
	for _, hz := range g.page.Clients[i].FrequenciesHz {
		result = append(result, vatsim.NormalizeFrequency(strconv.FormatUint(hz, 10)))
	}
	return result
}

// TransceiverSource implements PDC/server GetFrequencies without a poller or
// mutable authoritative cache. Each read verifies the accepted typed object.
type TransceiverSource struct{ state NavigationWeather }

func NewTransceiverSource(state NavigationWeather) (*TransceiverSource, error) {
	if state.Writer.Store == nil || state.Objects == nil {
		return nil, fmt.Errorf("transceiver source requires state and objects")
	}
	if err := transceiverNATSWriter(state.Writer, false); err != nil {
		return nil, err
	}
	return &TransceiverSource{state: state}, nil
}

func transceiverNATSWriter(writer Writer, writes bool) error {
	switch writer.Store.(type) {
	case NATSStore, *NATSStore:
		if writer.Projection == nil {
			return fmt.Errorf("NATS transceiver source requires an independent projection")
		}
		if writes && writer.Lease == nil {
			return fmt.Errorf("NATS transceiver worker requires an owner lease")
		}
	}
	return nil
}

func (s *TransceiverSource) Generation(ctx context.Context) (TransceiverGeneration, error) {
	if s == nil {
		return TransceiverGeneration{}, fmt.Errorf("transceiver source unavailable")
	}
	checkpoint, page, revision, err := s.state.CheckpointRevisionFor(ctx, globalRef(), "vatsim", transceiverResource)
	if err != nil {
		return TransceiverGeneration{}, err
	}
	if checkpoint == nil || page == nil || revision == 0 || page.GetTransceivers() == nil {
		return TransceiverGeneration{}, fmt.Errorf("accepted transceiver generation unavailable")
	}
	return TransceiverGeneration{Revision: revision, Sha256: checkpoint.Sha256, page: page.GetTransceivers()}, nil
}

func (s *TransceiverSource) GetFrequencies(callsign string) []string {
	generation, err := s.Generation(context.Background())
	if err != nil {
		return nil
	}
	return generation.GetFrequencies(callsign)
}
