package cluster

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
)

func ControllerObservationID(session int32, callsign string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("flightstrips/euroscope-controller/%d/%s", session, callsign))).String()
}

// SharedEuroScopeControllers includes authenticated live sockets and the last
// accepted controller set from the currently live, synced master. Its workflow
// source revision is the master epoch, never a node's private socket count.
func SharedEuroScopeControllers(projection *Projection, state *Aggregate, now time.Time) ([]*pb.Controller, error) {
	id := state.Ref.GetSession().GetId()
	_, entries, err := projection.ObservationSnapshot(id)
	if err != nil {
		return nil, err
	}
	live, err := (ControllerSector{Store: RoutedLifecycleStore{Projection: projection}}).OperationalControllers(context.Background(), id, entries, now)
	if err != nil {
		return nil, err
	}
	byCallsign := map[string]*pb.Controller{}
	for _, controller := range live {
		if !controller.Observer {
			byCallsign[controller.Callsign] = controller
		}
	}
	synced, err := projection.OperationalSync(state.Ref)
	if err != nil {
		return nil, err
	}
	if synced != nil && state.Master != nil {
		for _, workflow := range state.Workflows {
			parts := strings.Split(workflow.Step, "/")
			if len(parts) != 3 || parts[0] != "euroscope-controller" || workflow.GetSourceRevision() != state.Master.Epoch || workflow.Status != pb.WorkflowRecord_PENDING {
				continue
			}
			if byCallsign[parts[1]] != nil {
				continue
			}
			controller := &pb.Controller{Callsign: parts[1], Position: parts[2]}
			for _, entity := range state.EntitiesByKind(pb.EntityKind_CONTROLLER) {
				if entity.Value.GetController().Callsign == parts[1] {
					controller = entity.Value.GetController()
					break
				}
			}
			byCallsign[parts[1]] = controller
		}
	}
	result := []*pb.Controller{}
	for _, controller := range byCallsign {
		result = append(result, controller)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Callsign < result[j].Callsign })
	return result, nil
}
