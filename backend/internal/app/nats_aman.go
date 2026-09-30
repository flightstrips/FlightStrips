package app

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/predictor"
	"FlightStrips/internal/aman/predictor/openmeteo"
	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"errors"
	"fmt"
	"time"
)

func (r *natsRuntime) planAMAN(ctx context.Context, req *pb.CommandRequest, state *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	if r.aman == nil {
		return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("AMAN disabled")
	}
	actor := req.GetActor()
	if actor.Kind != pb.Actor_CONTROLLER || actor.SessionId == nil {
		return nil, pb.CommandReply_UNAUTHORIZED, 0, fmt.Errorf("AMAN requires an authenticated controller session")
	}
	session, err := r.projection.Read(sessionNATSRef(*actor.SessionId))
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, 0, err
	}
	seed := session.Indexes[pb.EntityKind_SESSION][fmt.Sprint(*actor.SessionId)].GetValue().GetSession()
	controller := session.Indexes[pb.EntityKind_CONTROLLER][actor.Id].GetValue().GetController()
	if seed == nil || seed.Tombstoned || seed.Airport != req.Aggregate.GetAirport().Icao || controller == nil || controller.Observer {
		return nil, pb.CommandReply_UNAUTHORIZED, 0, fmt.Errorf("controller is not bound to the airport")
	}
	_, presence, err := r.projection.ObservationSnapshot(*actor.SessionId)
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, 0, err
	}
	connected := false
	for _, entry := range presence {
		client := entry.Value.GetClient()
		if client != nil && client.Cid == actor.Id && r.projection.RequireLiveSocket(*actor.SessionId, client.ConnectionId, actor.Id, client.Kind) == nil {
			connected = true
			break
		}
	}
	if !connected {
		return nil, pb.CommandReply_UNAUTHORIZED, 0, fmt.Errorf("controller disconnected")
	}
	return r.aman.PlanCommand(ctx, req, state, controller.Callsign)
}
func (r *natsRuntime) reconcileAMAN(ctx context.Context, airport string, now time.Time, cfg aman.RuntimeConfig) error {
	var sourceErr error
	if cfg.SourceMode.UsesVATSIM() {
		sourceErr = r.aman.ReconcileVatsim(ctx, airport)
	}
	interval := cfg.ReconciliationInterval
	if interval <= 0 {
		interval = time.Minute
	}
	// Hybrid surveillance also evaluates shared KV changes at its accepted tick.
	if cfg.SourceMode.UsesEuroScope() && cfg.SurveillanceInterval > 0 && cfg.SurveillanceInterval < interval {
		interval = cfg.SurveillanceInterval
	}
	return errors.Join(sourceErr, natsReply(r.aman.Reconcile(ctx, airport, slot(now, interval))))
}

type natsWind struct {
	candidate openmeteo.Candidate
	runtime   *natsRuntime
	airport   string
	at        time.Time
}

func (w natsWind) WindProfile(ctx context.Context, request predictor.WindProfileRequest) (predictor.WindProfile, error) {
	_, resource, err := w.candidate.Fetch(ctx, w.airport, request, slot(w.at, time.Minute), func(ctx context.Context, id string) (bool, error) {
		return w.runtime.reserveQuota(ctx, id, "openmeteo", slot(w.at, time.Minute), 500)
	})
	if err != nil {
		return predictor.WindProfile{}, err
	}
	return w.candidate.Profile(ctx, w.airport, resource)
}
