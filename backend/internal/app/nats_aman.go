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
		sourceErr = r.reconcileVatsimGeneration(ctx, airport)
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

// Only the single airport supervisor accesses this advisory cache. Durable
// per-observation outcomes remain the authority after restart or takeover.
type vatsimPass struct {
	epoch, revision uint64
	digest          string
}

func (r *natsRuntime) reconcileVatsimGeneration(ctx context.Context, airport string) error {
	ref := airportNATSRef(airport)
	if !r.owner.CanWrite(ref) {
		delete(r.vatsimPasses, airport)
		return fmt.Errorf("VATSIM airport owner unavailable")
	}
	owner, err := r.projection.ReadOwner(ref)
	if err != nil {
		return err
	}
	checkpoint, revision, err := r.source.CheckpointMetadataFor(ctx, globalNATSRef(), "vatsim", "network-data/v3")
	if err != nil {
		return err
	}
	if checkpoint == nil || revision == 0 || checkpoint.Sha256 == "" {
		return fmt.Errorf("committed VATSIM generation unavailable")
	}
	pass := vatsimPass{epoch: owner.GetEpoch(), revision: revision, digest: checkpoint.Sha256}
	if r.vatsimPasses[airport] == pass {
		return nil
	}
	if err := r.aman.ReconcileVatsim(ctx, airport); err != nil {
		return err
	}
	// A successful pass alone may be skipped. Failures retry every second;
	// changing source identity or owner epoch always triggers another pass.
	if r.vatsimPasses == nil {
		r.vatsimPasses = make(map[string]vatsimPass)
	}
	r.vatsimPasses[airport] = pass
	return nil
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
	return w.candidate.ProfileForRequest(ctx, w.airport, resource, request)
}
