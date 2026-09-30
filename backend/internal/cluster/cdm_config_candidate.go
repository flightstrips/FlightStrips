package cluster

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"time"

	"FlightStrips/internal/cdm"
	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CdmConfigCandidateWorker makes each airport configuration refresh one
// owner-fenced provider call. The acquisition callback can be bound directly
// to CdmConfigStore.FetchAirportCandidate, which reuses the legacy parsers.
type CdmConfigCandidateWorker struct {
	State  NavigationWeather
	Worker ExternalCallWorker
	Fetch  func(context.Context, string) (*cdm.CdmAirportConfig, error)
	Now    func() time.Time
}

func (w CdmConfigCandidateWorker) Refresh(ctx context.Context, airport string, deadline time.Time) (bool, error) {
	if w.Fetch == nil || deadline.IsZero() {
		return false, fmt.Errorf("CDM configuration source or deadline unavailable")
	}
	if _, err := Subject(airportRef(airport)); err != nil {
		return false, err
	}
	resource := "airport/" + airport
	id, err := ProviderEventCommandID("cdm-configuration", "cdm", airport+"/"+deadline.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return false, err
	}
	return w.State.FetchProviderPageFenced(ctx, w.Worker, id, airport, "cdm", resource,
		func(ctx context.Context, _ *pb.ProviderCheckpoint, _ *pb.ProviderPage) (*pb.ProviderPage, *pb.ProviderCheckpoint, error) {
			config, err := w.Fetch(ctx, airport)
			if err != nil {
				return nil, nil, err
			}
			now := time.Now().UTC()
			if w.Now != nil {
				now = w.Now().UTC()
			}
			page, err := typedCdmConfig(config, now)
			if err != nil {
				return nil, nil, err
			}
			value := &pb.ProviderPage{Provider: "cdm", Resource: resource, Parsed: &pb.ProviderPage_CdmConfig{CdmConfig: page}}
			encoded, err := (proto.MarshalOptions{Deterministic: true}).Marshal(value)
			if err != nil {
				return nil, nil, err
			}
			sum := sha256.Sum256(encoded)
			return value, &pb.ProviderCheckpoint{Provider: "cdm", Resource: resource, Etag: hex.EncodeToString(sum[:])}, nil
		})
}

func (w CdmConfigCandidateWorker) Read(ctx context.Context, airport string) (*pb.CdmConfigPage, uint64, error) {
	_, page, revision, err := w.State.CheckpointRevisionFor(ctx, airportRef(airport), "cdm", "airport/"+airport)
	if err != nil {
		return nil, 0, err
	}
	if page == nil || page.GetCdmConfig() == nil {
		return nil, revision, nil
	}
	return proto.Clone(page.GetCdmConfig()).(*pb.CdmConfigPage), revision, nil
}

func (w CdmConfigCandidateWorker) Resume(ctx context.Context, airport string) error {
	return w.Worker.Resume(ctx, airportRef(airport))
}

func cdmInt32(value int) (int32, error) {
	if value < math.MinInt32 || value > math.MaxInt32 {
		return 0, fmt.Errorf("CDM integer exceeds int32")
	}
	return int32(value), nil
}

func typedCdmConfig(source *cdm.CdmAirportConfig, fetched time.Time) (*pb.CdmConfigPage, error) {
	if source == nil || len(source.Airport) != 4 || fetched.IsZero() {
		return nil, fmt.Errorf("incomplete CDM configuration")
	}
	rate, err := cdmInt32(source.DefaultRate)
	if err != nil {
		return nil, err
	}
	rateLvo, err := cdmInt32(source.DefaultRateLvo)
	if err != nil {
		return nil, err
	}
	taxi, err := cdmInt32(source.DefaultTaxiMinutes)
	if err != nil {
		return nil, err
	}
	light, err := cdmInt32(source.DeiceConfig.Light)
	if err != nil {
		return nil, err
	}
	medium, err := cdmInt32(source.DeiceConfig.Medium)
	if err != nil {
		return nil, err
	}
	heavy, err := cdmInt32(source.DeiceConfig.Heavy)
	if err != nil {
		return nil, err
	}
	super, err := cdmInt32(source.DeiceConfig.Super)
	if err != nil {
		return nil, err
	}
	page := &pb.CdmConfigPage{Airport: source.Airport, FetchedAt: timestamppb.New(fetched), DefaultRate: rate, DefaultRateLvo: rateLvo, DefaultTaxiMinutes: taxi, LvoActive: source.LvoActive,
		ActiveArrivalRunways: append([]string(nil), source.ActiveArrivalRunways...), ActiveDepartureRunways: append([]string(nil), source.ActiveDepartureRunways...),
		Deice: &pb.CdmConfigDeice{Light: light, Medium: medium, Heavy: heavy, Super: super}}
	for _, item := range source.Rates {
		if item.Airport != source.Airport {
			return nil, fmt.Errorf("foreign CDM rate")
		}
		page.Rates = append(page.Rates, &pb.CdmConfigRate{ArrRwyYes: append([]string(nil), item.ArrRwyYes...), ArrRwyNo: append([]string(nil), item.ArrRwyNo...), DepRwyYes: append([]string(nil), item.DepRwyYes...), DepRwyNo: append([]string(nil), item.DepRwyNo...), DependentRwy: append([]string(nil), item.DependentRwy...), Rates: append([]string(nil), item.Rates...), RatesLvo: append([]string(nil), item.RatesLvo...)})
	}
	for _, item := range source.SidIntervals {
		if item.Airport != source.Airport {
			return nil, fmt.Errorf("foreign CDM SID interval")
		}
		page.SidIntervals = append(page.SidIntervals, &pb.CdmConfigSidInterval{Runway: item.Runway, Sid1: item.Sid1, Sid2: item.Sid2, Value: item.Value})
	}
	for _, item := range source.TaxiZones {
		if item.Airport != source.Airport {
			return nil, fmt.Errorf("foreign CDM taxi zone")
		}
		minutes, err := cdmInt32(item.Minutes)
		if err != nil {
			return nil, err
		}
		zone := &pb.CdmConfigTaxiZone{Runway: item.Runway, Minutes: minutes}
		for _, point := range item.Polygon {
			zone.Polygon = append(zone.Polygon, &pb.CdmConfigTaxiPoint{Latitude: point.Lat, Longitude: point.Lon})
		}
		for _, minute := range item.RemoteTaxiMinutes {
			value, err := cdmInt32(minute)
			if err != nil {
				return nil, err
			}
			zone.RemoteTaxiMinutes = append(zone.RemoteTaxiMinutes, value)
		}
		page.TaxiZones = append(page.TaxiZones, zone)
	}
	for _, item := range source.Delays {
		if item.Airport != source.Airport {
			return nil, fmt.Errorf("foreign CDM delay")
		}
		page.Delays = append(page.Delays, &pb.CdmConfigDelay{Runway: item.Runway, Time: item.Time, Type: item.Type})
	}
	for _, item := range source.DeiceConfig.Platform {
		value, err := cdmInt32(item.Time)
		if err != nil {
			return nil, err
		}
		page.Deice.Platforms = append(page.Deice.Platforms, &pb.CdmConfigDeicePlatform{Name: item.Name, Time: value})
	}
	if err := validateProviderPage(&pb.ProviderPage{Provider: "cdm", Resource: "airport/" + source.Airport, Parsed: &pb.ProviderPage_CdmConfig{CdmConfig: page}}); err != nil {
		return nil, err
	}
	return page, nil
}

// OperationalCdmConfig rebuilds the existing CDM policy model from a verified
// typed airport checkpoint for the eventual candidate runtime binding.
func OperationalCdmConfig(page *pb.CdmConfigPage) (*cdm.CdmAirportConfig, error) {
	if err := validateProviderPage(&pb.ProviderPage{Provider: "cdm", Resource: "airport/" + page.GetAirport(), Parsed: &pb.ProviderPage_CdmConfig{CdmConfig: page}}); err != nil {
		return nil, err
	}
	config := &cdm.CdmAirportConfig{Airport: page.Airport, DefaultRate: int(page.DefaultRate), DefaultRateLvo: int(page.DefaultRateLvo), DefaultTaxiMinutes: int(page.DefaultTaxiMinutes), LvoActive: page.LvoActive,
		ActiveArrivalRunways: append([]string(nil), page.ActiveArrivalRunways...), ActiveDepartureRunways: append([]string(nil), page.ActiveDepartureRunways...),
		DeiceConfig: cdm.CdmDeiceConfig{Light: int(page.Deice.Light), Medium: int(page.Deice.Medium), Heavy: int(page.Deice.Heavy), Super: int(page.Deice.Super)}}
	for _, item := range page.Rates {
		config.Rates = append(config.Rates, cdm.CdmRate{Airport: page.Airport, ArrRwyYes: append([]string(nil), item.ArrRwyYes...), ArrRwyNo: append([]string(nil), item.ArrRwyNo...), DepRwyYes: append([]string(nil), item.DepRwyYes...), DepRwyNo: append([]string(nil), item.DepRwyNo...), DependentRwy: append([]string(nil), item.DependentRwy...), Rates: append([]string(nil), item.Rates...), RatesLvo: append([]string(nil), item.RatesLvo...)})
	}
	for _, item := range page.SidIntervals {
		config.SidIntervals = append(config.SidIntervals, cdm.CdmSidInterval{Airport: page.Airport, Runway: item.Runway, Sid1: item.Sid1, Sid2: item.Sid2, Value: item.Value})
	}
	for _, item := range page.TaxiZones {
		zone := cdm.CdmTaxiZone{Airport: page.Airport, Runway: item.Runway, Minutes: int(item.Minutes)}
		for _, point := range item.Polygon {
			zone.Polygon = append(zone.Polygon, cdm.CdmTaxiPoint{Lat: point.Latitude, Lon: point.Longitude})
		}
		for _, minute := range item.RemoteTaxiMinutes {
			zone.RemoteTaxiMinutes = append(zone.RemoteTaxiMinutes, int(minute))
		}
		config.TaxiZones = append(config.TaxiZones, zone)
	}
	for _, item := range page.Delays {
		config.Delays = append(config.Delays, cdm.CdmDelay{Airport: page.Airport, Runway: item.Runway, Time: item.Time, Type: item.Type})
	}
	for _, item := range page.Deice.Platforms {
		config.DeiceConfig.Platform = append(config.DeiceConfig.Platform, cdm.CdmDeicePlatformConfig{Name: item.Name, Time: int(item.Time)})
	}
	return config, nil
}
