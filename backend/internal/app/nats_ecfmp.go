package app

import (
	"FlightStrips/internal/cluster"
	"FlightStrips/internal/ecfmp"
	"FlightStrips/internal/httpresults"
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"net/http"
	"time"
)

func (r *natsRuntime) registerECFMP(mux *http.ServeMux) {
	mux.HandleFunc("/ecfmp/measures", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		values, err := ecfmp.AcceptedMeasures(req.Context(), r.source)
		if err != nil {
			http.Error(w, "failed to fetch measures: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		writeNATSJSON(w, http.StatusOK, values)
	})
	for _, path := range []string{"/ecfmp/test/inject", "/ecfmp/test/clear"} {
		mux.HandleFunc(path, func(w http.ResponseWriter, req *http.Request) {
			if req.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var values []ecfmp.FlowMeasure
			if req.URL.Path == "/ecfmp/test/inject" {
				if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<20)).Decode(&values); err != nil {
					http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
					return
				}
			}
			id, ok := httpresults.CommandID(req)
			if !ok {
				http.Error(w, "invalid Idempotency-Key", http.StatusBadRequest)
				return
			}
			// Use sentinels only at this HTTP input boundary. The owner resolves omitted
			// provider-test intervals once, while the request hash remains retry-stable.
			epoch := time.Unix(0, 0).UTC()
			for i := range values {
				if values[i].StartTime.IsZero() {
					values[i].StartTime = epoch
				}
				if values[i].EndTime.IsZero() {
					values[i].EndTime = epoch.Add(24 * time.Hour)
				}
			}
			page, err := ecfmp.TypedPage(values, epoch)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			name, hash, err := r.source.PublishProvider(&pb.ProviderPage{Provider: "ecfmp", Resource: "flow-measure/test", Parsed: &pb.ProviderPage_Ecfmp{Ecfmp: page}})
			if err != nil {
				http.Error(w, "failed to inject measures", http.StatusServiceUnavailable)
				return
			}
			checkpoint := &pb.ProviderCheckpoint{Provider: "ecfmp", Resource: "flow-measure/test", ObjectName: name, Sha256: hash}
			reply := r.Route(req.Context(), &pb.CommandRequest{ProtocolRevision: 1, CommandId: id, Aggregate: globalNATSRef(), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "ecfmp-http"}, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: "ecfmp.flow-measure/test", Value: &pb.EntityRecord{Value: &pb.EntityRecord_ProviderCheckpoint{ProviderCheckpoint: checkpoint}}}}}}})
			var body any = map[string]int{"injected": len(values)}
			if req.URL.Path == "/ecfmp/test/clear" {
				body = map[string]bool{"cleared": true}
			}
			httpresults.WriteResult(w, id, reply, http.StatusOK, body, func(w http.ResponseWriter, status int, message string) { http.Error(w, message, status) })
		})
	}
}
func (r *natsRuntime) planECFMP(_ context.Context, req *pb.CommandRequest, state *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	cp := req.GetSystem().GetUpdateEntity().GetValue().GetProviderCheckpoint()
	if req.Actor.Kind != pb.Actor_SYSTEM || req.Aggregate.GetGlobal() == nil || cp == nil || cp.Provider != "ecfmp" || cp.Resource != "flow-measure/test" || req.GetSystem().GetUpdateEntity().Key != "ecfmp.flow-measure/test" {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("invalid ECFMP test input")
	}
	page, err := r.source.ReadProvider(cp.ObjectName, cp.Sha256, cp.Provider, cp.Resource)
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, 0, err
	}
	epoch := time.Unix(0, 0).UTC()
	at := time.Now().UTC()
	copy := proto.Clone(page).(*pb.ProviderPage)
	copy.GetEcfmp().FetchedAt = timestamppb.New(at)
	for _, v := range copy.GetEcfmp().Measures {
		if v.StartTime.AsTime().Equal(epoch) {
			v.StartTime = timestamppb.New(at.Add(-time.Hour))
		}
		if v.EndTime.AsTime().Equal(epoch.Add(24 * time.Hour)) {
			v.EndTime = timestamppb.New(at.Add(24 * time.Hour))
		}
	}
	name, hash, err := r.source.PublishProvider(copy)
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, 0, err
	}
	next := proto.Clone(cp).(*pb.ProviderCheckpoint)
	next.ObjectName = name
	next.Sha256 = hash
	old := state.Indexes[pb.EntityKind_PROVIDER_CHECKPOINT]["ecfmp.flow-measure/test"]
	return &pb.DomainChange{Changes: []*pb.EntityChange{{Key: "ecfmp.flow-measure/test", Revision: old.GetRevision() + 1, Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_ProviderCheckpoint{ProviderCheckpoint: next}}}}}}, pb.CommandReply_COMMITTED, old.GetRevision(), nil
}
