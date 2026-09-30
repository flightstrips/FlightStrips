package cluster

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"FlightStrips/internal/vatsim"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestTransceiverFeedSlotsAndAcceptedReads(t *testing.T) {
	ctx := context.Background()
	_, objects, nav := navFixture(t)
	var body atomic.Value
	body.Store(`[{"callsign":" ekch_twr ","transceivers":[{"frequency":119300999},{"frequency":118105000},{"frequency":118105999}]}]`)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(body.Load().(string)))
	}))
	defer server.Close()
	feed, err := NewTransceiverFeed(nav, vatsim.NewTransceiverProvider(server.URL, time.Minute, server.Client()))
	require.NoError(t, err)
	source, err := NewTransceiverSource(nav)
	require.NoError(t, err)
	require.Nil(t, source.GetFrequencies("EKCH_TWR"))
	at := time.Date(2026, 9, 30, 14, 0, 1, 0, time.UTC)
	id, err := feed.SlotID(at)
	require.NoError(t, err)
	same, err := feed.SlotID(at.Add(40 * time.Second).In(time.FixedZone("test", 7200)))
	require.NoError(t, err)
	require.Equal(t, id, same)
	_, err = feed.SlotID(time.Time{})
	require.Error(t, err)
	sent, err := feed.Refresh(ctx, at)
	require.NoError(t, err)
	require.True(t, sent)
	accepted, err := source.Generation(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"118.105", "119.300"}, source.GetFrequencies(" ekch_twr "))
	frequencies := source.GetFrequencies("EKCH_TWR")
	frequencies[0] = "CORRUPTED"
	require.Equal(t, "118.105", source.GetFrequencies("EKCH_TWR")[0])
	require.Nil(t, source.GetFrequencies("UNKNOWN"))
	sent, err = feed.Refresh(ctx, at.Add(40*time.Second))
	require.NoError(t, err)
	require.False(t, sent)
	require.EqualValues(t, 1, calls.Load())
	for i, invalid := range []string{`[]`, `{`, `[{"callsign":"EKCH_TWR","radios":[{"frequency":118105000}]}]`} {
		body.Store(invalid)
		sent, err = feed.Refresh(ctx, at.Add(time.Duration(i+1)*time.Minute))
		require.Error(t, err)
		require.True(t, sent)
		got, readErr := source.Generation(ctx)
		require.NoError(t, readErr)
		require.Equal(t, accepted.Revision, got.Revision)
		require.Equal(t, accepted.Sha256, got.Sha256)
	}
	// Corrupt/unready source state cannot reuse a node-local cached value.
	unready := nav
	unready.Writer.Projection = &Projection{}
	failedSource, err := NewTransceiverSource(unready)
	require.NoError(t, err)
	require.Nil(t, failedSource.GetFrequencies("EKCH_TWR"))
	checkpoint, page, err := nav.CheckpointFor(ctx, globalRef(), "vatsim", transceiverResource)
	require.NoError(t, err)
	for _, mutate := range []func(*pb.TransceiverFeedPage){
		func(p *pb.TransceiverFeedPage) { p.Clients[0].Callsign = "ekch_twr" },
		func(p *pb.TransceiverFeedPage) { p.Clients[0].FrequenciesHz = []uint64{119300000, 118105000} },
		func(p *pb.TransceiverFeedPage) { p.Clients[0].FrequenciesHz = []uint64{118105000, 118105000} },
		func(p *pb.TransceiverFeedPage) { p.Clients[0].FrequenciesHz = []uint64{118105001} },
		func(p *pb.TransceiverFeedPage) {
			p.Clients = append(p.Clients, proto.Clone(p.Clients[0]).(*pb.TransceiverFeedClient))
		},
		func(p *pb.TransceiverFeedPage) { p.Clients = nil },
		func(p *pb.TransceiverFeedPage) { p.ProtoReflect().SetUnknown([]byte{0x78, 0x01}) },
	} {
		bad := proto.Clone(page).(*pb.ProviderPage)
		mutate(bad.GetTransceivers())
		_, _, err := nav.PublishProvider(bad)
		require.Error(t, err)
	}
	objects.damage(checkpoint.ObjectName)
	require.Nil(t, source.GetFrequencies("EKCH_TWR"))
}

func TestTransceiverNATSConstructorsRequireProjectionAndLease(t *testing.T) {
	_, objects, _ := navFixture(t)
	state := NavigationWeather{Writer: Writer{Store: NATSStore{}}, Objects: objects}
	_, err := NewTransceiverSource(state)
	require.ErrorContains(t, err, "independent projection")
	state.Writer.Projection = &Projection{}
	source, err := NewTransceiverSource(state)
	require.NoError(t, err)
	_, err = NewTransceiverFeed(state, vatsim.NewTransceiverProvider("", 0, nil))
	require.ErrorContains(t, err, "owner lease")
	_, err = NewTransceiverSectorReconciler(source, state.Writer, transceiverFixtureSectors)
	require.ErrorContains(t, err, "owner lease")
}
