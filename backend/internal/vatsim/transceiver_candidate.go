package vatsim

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TransceiverProvider is an HTTP boundary, with no cache, callback or poller.
// Its parser and normalization are shared with the current production cache.
type TransceiverProvider struct {
	client   *http.Client
	url      string
	interval time.Duration
}

func NewTransceiverProvider(url string, interval time.Duration, client *http.Client) *TransceiverProvider {
	if strings.TrimSpace(url) == "" {
		url = defaultTransceiversURL
	}
	if interval <= 0 {
		interval = defaultRefreshInterval
	}
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &TransceiverProvider{client: client, url: url, interval: interval}
}

func (p *TransceiverProvider) RefreshInterval() time.Duration { return p.interval }

// CandidatePage is called only after the global owner's external intent.
// Round to the same whole kilohertz used by NormalizeFrequency, so frequencies
// that present identically also have one canonical persisted hertz value.
func (p *TransceiverProvider) CandidatePage(ctx context.Context) (*pb.TransceiverFeedPage, error) {
	if p == nil {
		return nil, fmt.Errorf("transceiver provider unavailable")
	}
	snapshot, err := p.fetch(ctx)
	if err != nil {
		return nil, err
	}
	page := &pb.TransceiverFeedPage{FetchedAt: timestamppb.Now()}
	keys := make([]string, 0, len(snapshot))
	for callsign := range snapshot {
		keys = append(keys, callsign)
	}
	slices.Sort(keys)
	for _, callsign := range keys {
		client := &pb.TransceiverFeedClient{Callsign: callsign}
		for _, frequency := range snapshot[callsign] {
			parts := strings.Split(frequency, ".")
			mhz, err := strconv.ParseUint(parts[0], 10, 64)
			if err != nil {
				return nil, err
			}
			khz, err := strconv.ParseUint(parts[1], 10, 64)
			if err != nil {
				return nil, err
			}
			hz := mhz*1_000_000 + khz*1_000
			if hz == 0 {
				continue
			}
			client.FrequenciesHz = append(client.FrequenciesHz, hz)
		}
		slices.Sort(client.FrequenciesHz)
		if len(client.FrequenciesHz) > 0 {
			page.Clients = append(page.Clients, client)
		}
	}
	if len(page.Clients) == 0 {
		return nil, fmt.Errorf("transceiver payload has no normalized frequencies")
	}
	return page, nil
}

// ValidateTransceiverPage rejects noncanonical storage values; decoding must
// never silently sort, discard or normalize durable data during replay.
func ValidateTransceiverPage(page *pb.TransceiverFeedPage) error {
	if page == nil || page.FetchedAt == nil || page.FetchedAt.CheckValid() != nil || len(page.Clients) == 0 {
		return fmt.Errorf("invalid transceiver feed")
	}
	previous := ""
	for _, client := range page.Clients {
		if client == nil || client.Callsign == "" || client.Callsign != normalizeCallsign(client.Callsign) || client.Callsign <= previous || len(client.FrequenciesHz) == 0 {
			return fmt.Errorf("invalid canonical transceiver client")
		}
		previous = client.Callsign
		var prior uint64
		for _, hz := range client.FrequenciesHz {
			if hz <= prior || hz > math.MaxInt64 || hz%1_000 != 0 {
				return fmt.Errorf("invalid canonical transceiver frequency")
			}
			prior = hz
		}
	}
	return nil
}
