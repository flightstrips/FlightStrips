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
		keys = append(keys, key)
	}
	sum := sha256.Sum256([]byte(strings.Join(keys, "\x1f")))
	return "gfs-grid-v1/" + hex.EncodeToString(sum[:]), nil
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
			// Persist the provider's grid/hour identity, not one aircraft's
			// transient coordinates. Return requested identities at the read edge.
			canonical := request
			canonical.Samples = append([]predictor.WindSampleRequest(nil), request.Samples...)
			for i := range canonical.Samples {
				canonical.Samples[i] = providerGridSample(canonical.Samples[i])
				canonical.Samples[i].At = canonical.Samples[i].At.UTC().Truncate(time.Hour)
			}
			profile, err := a.Provider.CandidateProfile(ctx, canonical)
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

// ProfileForRequest projects accepted grid/hour levels onto the caller's sample
// identities. Legacy exact-coordinate resources remain readable via Profile.
func (a Candidate) ProfileForRequest(ctx context.Context, airport, resource string, request predictor.WindProfileRequest) (predictor.WindProfile, error) {
	want, err := ResourceForRequest(request)
	if err != nil || want != resource {
		return predictor.WindProfile{}, fmt.Errorf("wind resource differs from request")
	}
	profile, err := a.Profile(ctx, airport, resource)
	if err != nil {
		return predictor.WindProfile{}, err
	}
	return projectGridProfile(profile, request)
}

func projectGridProfile(profile predictor.WindProfile, request predictor.WindProfileRequest) (predictor.WindProfile, error) {
	if len(profile.Samples) != len(request.Samples) {
		return predictor.WindProfile{}, fmt.Errorf("wind sample count differs from request")
	}
	profile.Samples = append([]predictor.WindSample(nil), profile.Samples...)
	for i, sample := range request.Samples {
		want, err := cacheKey(sample)
		got, readErr := cacheKey(predictor.WindSampleRequest{Position: profile.Samples[i].Position, At: profile.Samples[i].At})
		if err != nil || readErr != nil || want != got {
			return predictor.WindProfile{}, fmt.Errorf("wind grid/hour differs from request")
		}
		profile.Samples[i].Position, profile.Samples[i].At = sample.Position, sample.At
		profile.Samples[i].Levels = cloneLevels(profile.Samples[i].Levels)
	}
	return profile, nil
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
