package cluster

import (
	"context"
	"fmt"
	"sort"
	"time"

	pb "FlightStrips/pkg/events/cluster"
)

// AiracCandidateWorker fences one airport import, stores the typed page, and
// activates only a manifest whose every fragment was published and read back.
// The acquisition callback is the AIRAC.NET boundary; it must convert every
// provider field needed for replay to NavData before returning.
type AiracCandidateWorker struct {
	State  NavigationWeather
	Worker ExternalCallWorker
	Fetch  func(context.Context, string, *pb.ProviderCheckpoint, *pb.ProviderPage) (*pb.AiracPage, *pb.ProviderCheckpoint, error)
}

func (w AiracCandidateWorker) Import(ctx context.Context, airport string, deadline time.Time) (bool, error) {
	if w.Fetch == nil || deadline.IsZero() {
		return false, fmt.Errorf("AIRAC source or import deadline unavailable")
	}
	if _, err := Subject(airportRef(airport)); err != nil {
		return false, err
	}
	resource := "airport/" + airport
	id, err := ProviderEventCommandID("airac-import", "airacnet", airport+"/"+deadline.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return false, err
	}
	sent, err := w.State.FetchProviderPageFenced(ctx, w.Worker, id, airport, "airacnet", resource,
		func(ctx context.Context, prior *pb.ProviderCheckpoint, previous *pb.ProviderPage) (*pb.ProviderPage, *pb.ProviderCheckpoint, error) {
			page, checkpoint, err := w.Fetch(ctx, airport, prior, previous)
			if err != nil {
				return nil, nil, err
			}
			if checkpoint == nil {
				return nil, nil, fmt.Errorf("AIRAC checkpoint unavailable")
			}
			checkpoint.Provider, checkpoint.Resource = "airacnet", resource
			return &pb.ProviderPage{Provider: "airacnet", Resource: resource, Parsed: &pb.ProviderPage_Airac{Airac: page}}, checkpoint, nil
		})
	if err != nil || !sent {
		return sent, err
	}
	return true, w.ActivateCurrent(ctx, airport)
}

// Resume first resolves provider uncertainty without calling AIRAC.NET, then
// completes a verified manifest switch from the last committed typed page.
func (w AiracCandidateWorker) Resume(ctx context.Context, airport string) error {
	if err := w.Worker.Resume(ctx, airportRef(airport)); err != nil {
		return err
	}
	checkpoint, page, _, err := w.State.CheckpointRevisionFor(ctx, airportRef(airport), "airacnet", "airport/"+airport)
	if err != nil {
		return err
	}
	if checkpoint == nil || page == nil {
		return nil
	}
	return w.ActivateCurrent(ctx, airport)
}

func (w AiracCandidateWorker) ActivateCurrent(ctx context.Context, airport string) error {
	checkpoint, page, revision, err := w.State.CheckpointRevisionFor(ctx, airportRef(airport), "airacnet", "airport/"+airport)
	if err != nil {
		return err
	}
	if checkpoint == nil || checkpoint.Sha256 == "" || page == nil || page.GetAirac() == nil || revision == 0 || len(page.GetAirac().Fragments) == 0 {
		return fmt.Errorf("committed AIRAC page unavailable")
	}
	cycle := ""
	objects := make([]*pb.NavObjectRef, 0, len(page.GetAirac().Fragments))
	kinds := map[string]int{}
	for _, fragment := range page.GetAirac().Fragments {
		if fragment == nil || fragment.Airport != airport || fragment.Version == nil || fragment.Version.Cycle == "" || cycle != "" && fragment.Version.Cycle != cycle {
			return fmt.Errorf("AIRAC page contains mismatched fragments")
		}
		cycle = fragment.Version.Cycle
		object, err := w.State.PublishNav(fragment)
		if err != nil {
			return err
		}
		kinds[object.Kind]++
		objects = append(objects, object)
	}
	if kinds["airport"] != 1 || kinds["fix"] != 1 || kinds["terminal"] != 1 {
		return fmt.Errorf("AIRAC page lacks a complete airport, fix and terminal set")
	}
	sort.Slice(objects, func(i, j int) bool {
		if objects[i].Kind != objects[j].Kind {
			return objects[i].Kind < objects[j].Kind
		}
		return objects[i].ObjectName < objects[j].ObjectName
	})
	manifest := &pb.NavManifest{Airport: airport, Cycle: cycle, Active: true, Objects: objects, SourceRevision: revision, SourceSha256: checkpoint.Sha256}
	manifest.Digest = manifestDigest(manifest)
	id, err := ProviderEventCommandID("airac-activation", "airacnet", fmt.Sprintf("%s/%d/%s", airport, revision, checkpoint.Sha256))
	if err != nil {
		return err
	}
	reply, err := w.State.ActivateManifest(ctx, id, manifest)
	if err != nil {
		return err
	}
	if reply == nil || reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		return fmt.Errorf("AIRAC manifest not activated: %v", reply)
	}
	return nil
}
