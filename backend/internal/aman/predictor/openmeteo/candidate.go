package openmeteo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"FlightStrips/internal/aman/predictor"
	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Candidate is dormant until the NATS runtime wires the airport owner and a
// global-owner quota command. The HTTP adapter remains the only JSON parser.
type Candidate struct {
	Provider *Adapter
	State    cluster.NavigationWeather
	Worker   cluster.ExternalCallWorker
}

func ResourceForRequest(request predictor.WindProfileRequest) (string, error) {
	if len(request.Samples) == 0 {
		return "", fmt.Errorf("empty wind request")
	}
	keys := make([]string, 0, len(request.Samples))
	for _, sample := range request.Samples {
		key, err := cacheKey(sample)
		if err != nil {
			return "", err
		}
		keys = append(keys, fmt.Sprintf("%s/%0.9f/%0.9f/%s", key, sample.Position.LatitudeDegrees, sample.Position.LongitudeDegrees, sample.At.UTC().Format(time.RFC3339Nano)))
	}
	sum := sha256.Sum256([]byte(strings.Join(keys, "\x1f")))
	return "gfs/" + hex.EncodeToString(sum[:]), nil
}

func (a Candidate) Fetch(ctx context.Context, airport string, request predictor.WindProfileRequest, deadline time.Time, reserve func(context.Context, string) (bool, error)) (bool, string, error) {
	if a.Provider == nil || reserve == nil || deadline.IsZero() || airport == "" {
		return false, "", fmt.Errorf("Open-Meteo provider, quota route, airport or deadline unavailable")
	}
	resource, err := ResourceForRequest(request)
	if err != nil {
		return false, "", err
	}
	_, cached, err := a.State.Checkpoint(ctx, airport, "openmeteo", resource)
	if err != nil {
		return false, resource, err
	}
	if cached != nil && cached.GetOpenMeteo() != nil && a.Provider.now().UTC().Before(cached.GetOpenMeteo().ExpiresAt.AsTime()) {
		return false, resource, nil
	}
	id, err := cluster.ProviderEventCommandID("openmeteo-wind", "openmeteo", airport+"/"+resource+"/"+deadline.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return false, "", err
	}
	ref := &pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: airport}}}
	sent, err := a.State.FetchProviderPageReserved(ctx, a.Worker, id, ref, "openmeteo", resource,
		func(ctx context.Context) (bool, error) { return reserve(ctx, id) },
		func(ctx context.Context, _ *pb.ProviderCheckpoint, _ *pb.ProviderPage) (*pb.ProviderPage, *pb.ProviderCheckpoint, error) {
			profile, err := a.Provider.CandidateProfile(ctx, request)
			if err != nil {
				return nil, nil, err
			}
			page := &pb.OpenMeteoPage{SourceId: profile.SourceID, SourceRevision: profile.SourceRevision,
				ObservedAt: timestamppb.New(profile.ObservedAt), ExpiresAt: timestamppb.New(profile.ExpiresAt)}
			for _, sample := range profile.Samples {
				item := &pb.OpenMeteoSample{LatitudeDegrees: sample.Position.LatitudeDegrees, LongitudeDegrees: sample.Position.LongitudeDegrees, ForecastAt: timestamppb.New(sample.At)}
				for _, level := range sample.Levels {
					item.Levels = append(item.Levels, &pb.OpenMeteoWindLevel{AltitudeFeet: level.AltitudeFeet, EastKnots: level.EastKnots, NorthKnots: level.NorthKnots})
				}
				page.Samples = append(page.Samples, item)
			}
			return &pb.ProviderPage{Provider: "openmeteo", Resource: resource, Parsed: &pb.ProviderPage_OpenMeteo{OpenMeteo: page}},
				&pb.ProviderCheckpoint{Provider: "openmeteo", Resource: resource}, nil
		})
	return sent, resource, err
}

func (a Candidate) Profile(ctx context.Context, airport, resource string) (predictor.WindProfile, error) {
	checkpoint, page, err := a.State.Checkpoint(ctx, airport, "openmeteo", resource)
	if err != nil {
		return predictor.WindProfile{}, err
	}
	if checkpoint == nil || page == nil || page.GetOpenMeteo() == nil {
		return predictor.WindProfile{}, fmt.Errorf("committed Open-Meteo profile unavailable")
	}
	value := page.GetOpenMeteo()
	result := predictor.WindProfile{SourceID: value.SourceId, SourceRevision: value.SourceRevision, ObservedAt: value.ObservedAt.AsTime(), ExpiresAt: value.ExpiresAt.AsTime()}
	for _, sample := range value.Samples {
		item := predictor.WindSample{Position: predictor.WindCoordinate{LatitudeDegrees: sample.LatitudeDegrees, LongitudeDegrees: sample.LongitudeDegrees}, At: sample.ForecastAt.AsTime()}
		for _, level := range sample.Levels {
			item.Levels = append(item.Levels, predictor.WindLevel{AltitudeFeet: level.AltitudeFeet, EastKnots: level.EastKnots, NorthKnots: level.NorthKnots})
		}
		result.Samples = append(result.Samples, item)
	}
	return result, nil
}
