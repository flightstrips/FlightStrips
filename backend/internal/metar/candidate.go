package metar

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Candidate is only constructed by the future NATS runtime. The existing SQL
// poller and its frontend sends remain unchanged until Task 20.
type Candidate struct {
	Provider      *Poller
	State         cluster.NavigationWeather
	AirportWorker cluster.ExternalCallWorker
	GlobalWorker  cluster.ExternalCallWorker
	Session       cluster.AtisSessionAdapter
}

// NewCandidateProvider reuses the production HTTP parsers without starting
// Poller.Start or requiring a PostgreSQL session repository.
func NewCandidateProvider(client *http.Client, metarURL, atisURL string) *Poller {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if metarURL == "" {
		metarURL = "https://metar.vatsim.net"
	}
	if atisURL == "" {
		atisURL = "https://data.vatsim.net/v3/afv-atis-data.json"
	}
	return &Poller{httpClient: client, metarBaseURL: metarURL, atisDataURL: atisURL}
}

func pollID(family, key string, deadline time.Time) (string, error) {
	if deadline.IsZero() {
		return "", fmt.Errorf("missing provider poll deadline")
	}
	return cluster.ProviderEventCommandID(family, family, key+"/"+deadline.UTC().Format(time.RFC3339Nano))
}

// FetchMetar reserves the caller's durable global quota slot after the
// airport intent and before the HTTP call. Reserve must route to the global
// owner with this exact workflow ID; an uncertain reservation blocks the call.
func (a Candidate) FetchMetar(ctx context.Context, airport string, deadline time.Time, ttl time.Duration, reserve func(context.Context, string) (bool, error)) (bool, error) {
	if a.Provider == nil || reserve == nil {
		return false, fmt.Errorf("METAR provider or global reservation unavailable")
	}
	id, err := pollID("metar-poll", airport, deadline)
	if err != nil {
		return false, err
	}
	return a.State.FetchWeatherFenced(ctx, a.AirportWorker, id, airport, "metar", ttl,
		func(ctx context.Context) (bool, error) { return reserve(ctx, id) },
		func(ctx context.Context) (*pb.WeatherObservation, error) {
			value, err := a.Provider.fetch(ctx, airport)
			if err != nil {
				return nil, err
			}
			return &pb.WeatherObservation{Metar: value}, nil
		})
}

func feedEntry(value *ATIS) *pb.AtisFeedEntry {
	if value == nil {
		return nil
	}
	return &pb.AtisFeedEntry{Callsign: value.Callsign, Code: value.Code, Frequency: value.Frequency,
		TextLines: append([]string(nil), value.Text...), LastUpdated: timestamppb.New(value.LastUpdated.UTC())}
}

// FetchAtisFeed is one global-owned request for the AFV feed, persisted as a
// typed immutable page. Session owners derive airport presentation from it.
func (a Candidate) FetchAtisFeed(ctx context.Context, deadline time.Time) (bool, error) {
	if a.Provider == nil {
		return false, fmt.Errorf("ATIS provider unavailable")
	}
	id, err := pollID("afv-atis-poll", "all", deadline)
	if err != nil {
		return false, err
	}
	ref := &pb.AggregateRef{Target: &pb.AggregateRef_Global{Global: &pb.GlobalRef{}}}
	return a.State.FetchProviderPageFor(ctx, a.GlobalWorker, id, ref, "afv-atis", "feed",
		func(ctx context.Context, _ *pb.ProviderCheckpoint, _ *pb.ProviderPage) (*pb.ProviderPage, *pb.ProviderCheckpoint, error) {
			entries, err := a.Provider.fetchAllAtisData(ctx)
			if err != nil {
				return nil, nil, err
			}
			page := &pb.AtisFeedPage{FetchedAt: timestamppb.Now()}
			keys := make([]string, 0, len(entries))
			for airport := range entries {
				keys = append(keys, airport)
			}
			sort.Strings(keys)
			for _, airport := range keys {
				value := entries[airport]
				page.Airports = append(page.Airports, &pb.AtisFeedAirport{Airport: airport, Arrival: feedEntry(value.arr), Departure: feedEntry(value.dep)})
			}
			return &pb.ProviderPage{Provider: "afv-atis", Resource: "feed", Parsed: &pb.ProviderPage_AtisFeed{AtisFeed: page}},
				&pb.ProviderCheckpoint{Provider: "afv-atis", Resource: "feed"}, nil
		})
}

func (a Candidate) ApplySession(ctx context.Context, sessionID int32, airport string, at time.Time) error {
	if at.IsZero() {
		return fmt.Errorf("missing ATIS application time")
	}
	ref := &pb.AggregateRef{Target: &pb.AggregateRef_Global{Global: &pb.GlobalRef{}}}
	checkpoint, page, revision, err := a.State.CheckpointRevisionFor(ctx, ref, "afv-atis", "feed")
	if err != nil {
		return err
	}
	if checkpoint == nil || page == nil || page.GetAtisFeed() == nil || revision == 0 {
		return fmt.Errorf("committed ATIS feed unavailable")
	}
	var feed *pb.AtisFeedAirport
	for _, entry := range page.GetAtisFeed().Airports {
		if entry.Airport == airport {
			feed = entry
			break
		}
	}
	weather, err := a.State.Weather(ctx, airport, "metar", at)
	if err != nil {
		return err
	}
	metar, metarTime := "", int64(0)
	if weather != nil {
		metar, metarTime = weather.Observation.Metar, weather.FetchedAt.AsTime().UnixNano()
	}
	value := &pb.Atis{Airport: airport, Metar: metar, ObservedAt: page.GetAtisFeed().FetchedAt,
		SourceRevision: fmt.Sprintf("%020d:%s:%020d", revision, checkpoint.Sha256, metarTime)}
	if feed != nil {
		if feed.Arrival != nil {
			value.ArrivalCode = feed.Arrival.Code
		}
		if feed.Departure != nil {
			value.DepartureCode = feed.Departure.Code
		}
		present := feed.Arrival
		if present == nil {
			present = feed.Departure
		}
		if present != nil {
			value.Code, value.Text = present.Code, strings.Join(present.TextLines, "\n")
		}
	}
	reply := a.Session.Apply(ctx, sessionID, value)
	if reply == nil || reply.Status != pb.CommandReply_COMMITTED || reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		return fmt.Errorf("ATIS presentation not committed: %v", reply)
	}
	return nil
}
