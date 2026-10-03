package cluster

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/nats-io/nats.go"
)

type providerObjectCleaner interface {
	List(context.Context) ([]*nats.ObjectInfo, error)
	Delete(string) error
}

var _ providerObjectCleaner = NATSObjects{}

func (a NavigationWeather) currentProviderReferences(ctx context.Context) (map[string]bool, error) {
	refs := map[string]bool{}
	if p := a.Writer.Projection; p != nil {
		if err := p.readyForRead(); err != nil {
			return nil, err
		}
		p.mu.RLock()
		defer p.mu.RUnlock()
		for _, state := range p.states {
			for _, entry := range state.Indexes[pb.EntityKind_PROVIDER_CHECKPOINT] {
				if name := entry.Value.GetProviderCheckpoint().ObjectName; name != "" {
					refs[name] = true
				}
			}
		}
	}
	return refs, nil
}

// Called only after durable replacement, on the serialized provider worker.
// Checkpoints in other aggregates can share a payload and keep it live.
func (a NavigationWeather) deleteReplacedProvider(ctx context.Context, ref *pb.AggregateRef, prior, current *pb.ProviderCheckpoint) error {
	if prior == nil || prior.ObjectName == "" || current.ObjectName == "" || prior.ObjectName == current.ObjectName {
		return nil
	}
	cleaner, ok := a.Objects.(providerObjectCleaner)
	if !ok {
		return nil
	}
	if a.Writer.Lease != nil && !a.Writer.Lease.CanDispatchDurable(ref) {
		return fmt.Errorf("provider cleanup owner changed")
	}
	refs, err := a.currentProviderReferences(ctx)
	if err != nil {
		return err
	}
	if refs[prior.ObjectName] {
		return nil
	}
	if err := cleaner.Delete(prior.ObjectName); err != nil && !errors.Is(err, nats.ErrObjectNotFound) {
		return err
	}
	a.Cache.discard(prior.ObjectName)
	return nil
}

// PruneProviderPages migrates old immutable feed generations. It runs on the
// owning provider supervisor, before starting a new fetch. Staged objects newer
// than the accepted payload are protected. Snapshots/logs retain metadata only:
// consumers access objects after replay reaches the current checkpoint.
func (a NavigationWeather) PruneProviderPages(ctx context.Context, ref *pb.AggregateRef) (int, error) {
	cleaner, ok := a.Objects.(providerObjectCleaner)
	if !ok {
		return 0, nil
	}
	if a.Writer.Lease != nil && !a.Writer.Lease.CanDispatchDurable(ref) {
		return 0, fmt.Errorf("provider cleanup owner unavailable")
	}
	if err := a.expireWeatherCheckpoints(ctx, ref, time.Now().UTC()); err != nil {
		return 0, err
	}
	state, err := a.read(ctx, ref)
	if err != nil {
		return 0, err
	}
	checkpoints := map[string]*pb.ProviderCheckpoint{}
	providers := map[string]bool{}
	for _, entry := range state.Indexes[pb.EntityKind_PROVIDER_CHECKPOINT] {
		cp := entry.Value.GetProviderCheckpoint()
		if cp.ObjectName != "" {
			checkpoints[cp.Provider+"/"+digest([]byte(cp.Resource))] = cp
			providers[cp.Provider] = true
		}
	}
	providers["openmeteo"] = true
	refs, err := a.currentProviderReferences(ctx)
	if err != nil {
		return 0, err
	}
	for _, cp := range checkpoints {
		refs[cp.ObjectName] = true
	}
	infos, err := cleaner.List(ctx)
	if errors.Is(err, nats.ErrNoObjectsFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	byName := map[string]*nats.ObjectInfo{}
	for _, info := range infos {
		if info != nil && !info.Deleted {
			byName[info.Name] = info
		}
	}
	deleted := 0
	for _, info := range infos {
		if ctx.Err() != nil {
			return deleted, ctx.Err()
		}
		if info == nil || info.Deleted || refs[info.Name] || !strings.HasPrefix(info.Name, "provider/") {
			continue
		}
		parts := strings.Split(info.Name, "/")
		if len(parts) < 3 || !providers[parts[1]] {
			continue
		}
		var current *pb.ProviderCheckpoint
		if len(parts) == 4 {
			current = checkpoints[parts[1]+"/"+parts[2]]
		} else if len(parts) == 3 {
			data, err := a.Objects.GetBytes(info.Name)
			if errors.Is(err, nats.ErrObjectNotFound) {
				continue
			}
			if err != nil {
				return deleted, err
			}
			object := &pb.ObjectValue{}
			if err := pb.UnmarshalStrict(data, object); err != nil {
				return deleted, err
			}
			page := object.GetProviderPage()
			if page == nil {
				return deleted, fmt.Errorf("invalid legacy provider payload")
			}
			current = checkpoints[page.Provider+"/"+digest([]byte(page.Resource))]
		}
		if current == nil {
			if parts[1] != "openmeteo" {
				continue
			}
			expired, err := a.weatherObjectExpired(info.Name, time.Now().UTC())
			if err != nil {
				return deleted, err
			}
			if !expired {
				continue
			}
			if err := cleaner.Delete(info.Name); err != nil && !errors.Is(err, nats.ErrObjectNotFound) {
				return deleted, err
			}
			a.Cache.discard(info.Name)
			deleted++
			continue
		}
		accepted := byName[current.ObjectName]
		if accepted == nil || !info.ModTime.Before(accepted.ModTime) {
			continue
		}
		if a.Writer.Lease != nil && !a.Writer.Lease.CanDispatchDurable(ref) {
			return deleted, fmt.Errorf("provider cleanup owner changed")
		}
		if err := cleaner.Delete(info.Name); err != nil && !errors.Is(err, nats.ErrObjectNotFound) {
			return deleted, err
		}
		a.Cache.discard(info.Name)
		deleted++
	}
	return deleted, nil
}

func (a NavigationWeather) weatherObjectExpired(name string, now time.Time) (bool, error) {
	data, err := a.Objects.GetBytes(name)
	if errors.Is(err, nats.ErrObjectNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	object := &pb.ObjectValue{}
	if err := pb.UnmarshalStrict(data, object); err != nil {
		return false, err
	}
	page := object.GetProviderPage().GetOpenMeteo()
	return page != nil && page.ExpiresAt != nil && !now.Before(page.ExpiresAt.AsTime()), nil
}

// Expired weather resources are not current feed state. Drop their checkpoint
// durably before deleting bytes, using entity revision CAS against refreshes.
func (a NavigationWeather) expireWeatherCheckpoints(ctx context.Context, ref *pb.AggregateRef, now time.Time) error {
	state, err := a.read(ctx, ref)
	if err != nil {
		return err
	}
	expired := map[string]*pb.EntitySnapshot{}
	for key, entry := range state.Indexes[pb.EntityKind_PROVIDER_CHECKPOINT] {
		cp := entry.Value.GetProviderCheckpoint()
		if cp.Provider != "openmeteo" || cp.ObjectName == "" || cp.AttemptStatus == pb.ProviderCheckpoint_PENDING {
			continue
		}
		old, err := a.weatherObjectExpired(cp.ObjectName, now)
		if err != nil {
			return err
		}
		if old {
			expired[key] = entry
		}
	}
	keys := make([]string, 0, len(expired))
	for key := range expired {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for len(keys) > 0 {
		count := len(keys)
		if count > 128 {
			count = 128
		}
		batch := append([]string(nil), keys[:count]...)
		keys = keys[count:]
		scope, _ := Subject(ref)
		identity := ""
		for _, key := range batch {
			identity += fmt.Sprintf("%s/%d;", key, expired[key].Revision)
		}
		id, _ := ProviderEventCommandID("provider-expire", "openmeteo", scope+"/"+digest([]byte(identity)))
		writer := a.Writer
		writer.Plan = func(_ context.Context, _ *pb.CommandRequest, current *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
			changes := []*pb.EntityChange{}
			for _, key := range batch {
				old := expired[key]
				entry := current.Indexes[pb.EntityKind_PROVIDER_CHECKPOINT][key]
				if entry == nil || entry.Revision != old.Revision {
					continue
				}
				changes = append(changes, &pb.EntityChange{Key: key, Revision: entry.Revision + 1, Operation: &pb.EntityChange_Delete{Delete: &pb.DeleteEntity{Kind: pb.EntityKind_PROVIDER_CHECKPOINT}}})
			}
			return &pb.DomainChange{Changes: changes}, pb.CommandReply_COMMITTED, 0, nil
		}
		request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: id, Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: writer.NodeID}, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: batch[0], Value: expired[batch[0]].Value}}}}}
		reply, _ := writer.ExecuteFresh(ctx, request)
		if reply.Status != pb.CommandReply_COMMITTED {
			return fmt.Errorf("weather expiration: %s: %s", reply.Status, reply.Detail)
		}
		if a.Writer.Projection == nil {
			for _, key := range batch {
				a.Cache.acceptProvider(scope, key, "", expired[key].Revision+1)
			}
		}
	}
	return nil
}
