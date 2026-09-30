package vatsimcandidate

import (
	"context"
	"fmt"
	"time"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/vatsim"
	pb "FlightStrips/pkg/events/cluster"
)

// Fetch is dormant until the NATS application runtime replaces the SQL cache
// worker. The status and network-data HTTP requests share one global intent.
type Fetch struct {
	Cache  *vatsim.Cache
	State  cluster.NavigationWeather
	Worker cluster.ExternalCallWorker
}

func (a Fetch) Global(ctx context.Context, deadline time.Time) (bool, error) {
	if a.Cache == nil || deadline.IsZero() {
		return false, fmt.Errorf("VATSIM cache or poll deadline unavailable")
	}
	id, err := cluster.ProviderEventCommandID("vatsim-feed", "vatsim", deadline.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return false, err
	}
	ref := &pb.AggregateRef{Target: &pb.AggregateRef_Global{Global: &pb.GlobalRef{}}}
	return a.State.FetchProviderPageFor(ctx, a.Worker, id, ref, "vatsim", "network-data/v3",
		func(ctx context.Context, _ *pb.ProviderCheckpoint, prior *pb.ProviderPage) (*pb.ProviderPage, *pb.ProviderCheckpoint, error) {
			var old *pb.VatsimPage
			if prior != nil {
				old = prior.GetVatsim()
			}
			page, err := a.Cache.CandidatePage(ctx, old)
			if err != nil {
				return nil, nil, err
			}
			return &pb.ProviderPage{Provider: "vatsim", Resource: "network-data/v3", Parsed: &pb.ProviderPage_Vatsim{Vatsim: page}},
				&pb.ProviderCheckpoint{Provider: "vatsim", Resource: "network-data/v3"}, nil
		})
}
