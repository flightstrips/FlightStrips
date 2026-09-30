package vatsim

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestTransceiverCandidateUsesActualParser(t *testing.T) {
	for _, body := range []string{
		`[{"callsign":" ekch_a_twr ","frequency":119300999},{"callsign":"EKCH_A_TWR","transceivers":[{"frequency":118105000},{"frequency":119300000},{"frequency":-1}]},{"callsign":"ekch_b_twr","frequency":118105000},{"callsign":"","frequency":118105000}]`,
		`{"transceivers":[{"callsign":"ekch_b_twr","frequency":118105000},{"callsign":" ekch_a_twr ","transceivers":[{"frequency":119300000},{"frequency":118105000},{"frequency":118105999}]}]}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
		provider := NewTransceiverProvider(server.URL, 45*time.Second, server.Client())
		page, err := provider.CandidatePage(context.Background())
		server.Close()
		require.NoError(t, err)
		require.NoError(t, ValidateTransceiverPage(page))
		require.Equal(t, 45*time.Second, provider.RefreshInterval())
		require.Len(t, page.Clients, 2)
		require.Equal(t, "EKCH_A_TWR", page.Clients[0].Callsign)
		require.Equal(t, []uint64{118105000, 119300000}, page.Clients[0].FrequenciesHz)
		require.Equal(t, "EKCH_B_TWR", page.Clients[1].Callsign)
		encoded, err := proto.Marshal(page)
		require.NoError(t, err)
		decoded := &pb.TransceiverFeedPage{}
		require.NoError(t, proto.Unmarshal(encoded, decoded))
		require.True(t, proto.Equal(page, decoded))
	}
}

func TestTransceiverCandidateRejectsMalformedEmptyAndNoncanonical(t *testing.T) {
	for _, body := range []string{`{`, `[]`, `null`, `{}`, `{"transceivers":[]}`, `[{"callsign":"EKCH_TWR","frequency":-1}]`, `[{"callsign":"EKCH_TWR","radios":[{"frequency":118105000}]}]`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
		_, err := NewTransceiverProvider(server.URL, 0, server.Client()).CandidatePage(context.Background())
		server.Close()
		require.Error(t, err, body)
	}
	provider := NewTransceiverProvider("", 0, nil)
	require.Equal(t, defaultRefreshInterval, provider.RefreshInterval())
}
