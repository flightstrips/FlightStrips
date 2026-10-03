package euroscopebinary

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/server"
	"FlightStrips/internal/shared"
	pb "FlightStrips/pkg/events/cluster"
	modelvalues "FlightStrips/pkg/models"
	"google.golang.org/protobuf/proto"
)

// RetainedAircraft consumes the accepted global generation, session cursor and
// lifecycle state. Unavailable/stale input is an error, so expiry fails closed.
func (c *DeadlineCandidate) RetainedAircraft(state *cluster.Aggregate, callsign string) (bool, error) {
	strip := state.Indexes[pb.EntityKind_STRIP][callsign].GetValue().GetStrip()
	if strip == nil {
		return false, nil
	}
	// Task 19a owns protected bookings/physical occupancy. Controller edits and
	// coordination also keep an operational strip out of disconnect deletion.
	if strip.OwnerCid != "" || strip.Stand != "" || strip.Manual || len(strip.ControllerModifiedFields) != 0 {
		return true, nil
	}
	for _, coordination := range state.EntitiesByKind(pb.EntityKind_COORDINATION) {
		if coordination.Value.GetCoordination().Callsign == callsign {
			return true, nil
		}
	}
	assignment := state.Indexes[pb.EntityKind_STAND_ASSIGNMENT][callsign].GetValue().GetStandAssignment()
	if assignment != nil && (assignment.ExpiresAt == nil || assignment.ExpiresAt.AsTime().After(c.clock()) || assignment.Stage == "ON_STAND" || assignment.Stage == "ARRIVED") {
		return true, nil
	}
	seed := state.Indexes[pb.EntityKind_SESSION][fmt.Sprint(state.Ref.GetSession().Id)].Value.GetSession()
	if !strings.EqualFold(seed.Name, "LIVE") {
		return false, nil
	}
	checkpoint, page, revision, err := c.Source.CheckpointRevisionFor(context.Background(), globalCandidateRef(), "vatsim", "network-data/v3")
	if err != nil {
		return false, err
	}
	cursor := state.Indexes[pb.EntityKind_VATSIM_SESSION_CURSOR]["vatsim"].GetValue().GetVatsimSessionCursor()
	if checkpoint == nil || page.GetVatsim() == nil || page.GetVatsim().SnapshotAt == nil || cursor == nil || cursor.SourceRevision != revision || cursor.SourceSha256 != checkpoint.Sha256 || c.clock().Sub(page.GetVatsim().SnapshotAt.AsTime()) > 2*time.Minute || page.GetVatsim().SnapshotAt.AsTime().After(c.clock().Add(time.Second)) {
		return false, fmt.Errorf("aircraft retention requires a fresh accepted session VATSIM generation")
	}
	for _, flight := range page.GetVatsim().Flights {
		if flight.Callsign == callsign {
			return true, nil
		}
	}
	positions, _, err := c.Router.Projection.ObservationSnapshot(seed.Id)
	if err != nil {
		return false, err
	}
	for _, position := range positions {
		if position.Value.AircraftKey != callsign {
			continue
		}
		if position.Stale {
			return false, fmt.Errorf("retention occupancy awaits owner/master sync")
		}
		if position.Value.GetPosition() != nil {
			return true, nil
		}
	}
	return false, nil
}

func (c *DeadlineCandidate) operationalControllers(ctx context.Context, state *cluster.Aggregate) ([]*pb.Controller, error) {
	controllers, err := cluster.SharedEuroScopeControllers(c.Router.Projection, state, c.clock())
	if err != nil {
		return nil, err
	}
	if synced, err := c.Router.Projection.OperationalSync(state.Ref); err != nil {
		return nil, err
	} else if synced != nil {
		for _, workflow := range state.Workflows {
			parts := strings.Split(workflow.Step, "/")
			if len(parts) == 3 && parts[0] == "euroscope-controller" && workflow.GetSourceRevision() == state.Master.GetEpoch() && workflow.Status == pb.WorkflowRecord_COMPLETED && state.Indexes[pb.EntityKind_SESSION_DEADLINE]["controller-offline."+parts[1]] != nil {
				found := false
				for _, controller := range controllers {
					if controller.Callsign == parts[1] {
						found = true
						break
					}
				}
				if !found {
					observed := &pb.Controller{Callsign: parts[1], Position: parts[2]}
					for _, entity := range state.EntitiesByKind(pb.EntityKind_CONTROLLER) {
						if entity.Value.GetController().Callsign == parts[1] {
							observed = entity.Value.GetController()
							break
						}
					}
					controllers = append(controllers, observed)
				}
			}
		}
	}
	result := []*pb.Controller{}
	for _, controller := range controllers {
		model := &models.Controller{Callsign: controller.Callsign, Position: controller.Position, Observer: controller.Observer}
		if shared.IsOperationalPositionController(model) {
			result = append(result, controller)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Cid < result[j].Cid })
	return result, nil
}

// PlanTransceiverSectors runs the complete sector/layout/route policy against
// one immutable accepted frequency generation. It does not mutate the shared
// candidate's live reader while another socket is being admitted.
func (c *DeadlineCandidate) PlanTransceiverSectors(ctx context.Context, req *pb.CommandRequest, state *cluster.Aggregate, generation cluster.TransceiverGeneration) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	local := &DeadlineCandidate{Router: c.Router, Source: c.Source, Now: c.Now, Coverage: generation.GetFrequencies}
	changes, err := local.reconcileChanges(ctx, state)
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, state.Revision, err
	}
	return &pb.DomainChange{Changes: changes}, pb.CommandReply_COMMITTED, state.Revision, nil
}

func (c *DeadlineCandidate) reconcileChanges(ctx context.Context, state *cluster.Aggregate) ([]*pb.EntityChange, error) {
	id := state.Ref.GetSession().GetId()
	seed := state.Indexes[pb.EntityKind_SESSION][fmt.Sprint(id)].GetValue().GetSession()
	if seed == nil || seed.Tombstoned {
		return nil, fmt.Errorf("session reconciliation seed unavailable")
	}
	controllers, err := c.operationalControllers(ctx, state)
	if err != nil {
		return nil, err
	}
	positions, _, err := c.Router.Projection.ObservationSnapshot(id)
	if err != nil {
		return nil, err
	}
	session := &models.Session{ID: id, Airport: seed.Airport, Name: seed.Name, ActiveRunways: modelvalues.ActiveRunways{}}
	for _, runway := range seed.Runways {
		if runway.Arrival {
			session.ActiveRunways.ArrivalRunways = append(session.ActiveRunways.ArrivalRunways, runway.Name)
		}
		if runway.Departure {
			session.ActiveRunways.DepartureRunways = append(session.ActiveRunways.DepartureRunways, runway.Name)
		}
	}
	if len(session.ActiveRunways.ArrivalRunways) == 0 || len(session.ActiveRunways.DepartureRunways) == 0 {
		return nil, nil // Initial runway report has not arrived; keep current allocations.
	}
	coverage := []config.ControllerCoverage{}
	roles := []*config.Position{}
	byPosition := map[string]*pb.Controller{}
	byCID := map[string]*pb.Controller{}
	for _, controller := range controllers {
		if controller.Cid != "" {
			byCID[controller.Cid] = controller
		}
		position, err := config.GetPositionByName(controller.Callsign)
		if err != nil {
			position, err = config.GetPositionBasedOnFrequency(controller.Position)
		}
		if err != nil {
			continue
		}
		role := *position
		role.Frequency = controller.Position
		roles = append(roles, &role)
		entry := config.ControllerCoverage{Name: position.Name, Frequency: controller.Position}
		if c.Coverage != nil {
			entry.CoveredFrequencies = c.Coverage(controller.Callsign)
		}
		coverage = append(coverage, entry)
		if byPosition[controller.Position] == nil || byPosition[controller.Position].Cid == "" && controller.Cid != "" {
			byPosition[controller.Position] = controller
		}
	}
	active := session.ActiveRunways.GetAllActiveRunways()
	sectorGroups := config.GetControllerSectorsWithCoverage(coverage, active)
	wanted := map[string]*pb.SectorOwner{}
	modelOwners := []*models.SectorOwner{}
	for frequency, sectors := range sectorGroups {
		keys, identifier, priority := []string{}, "", -1
		ordered := append([]config.Sector(nil), sectors...)
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].KeyOrName() < ordered[j].KeyOrName() })
		for _, sector := range ordered {
			keys = append(keys, sector.KeyOrName())
			if sector.NamePriority > priority {
				identifier, priority = sector.Name, sector.NamePriority
			}
		}
		owner := byPosition[frequency]
		if owner == nil {
			continue
		}
		modelOwners = append(modelOwners, &models.SectorOwner{Session: id, Position: frequency, Sector: keys, Identifier: identifier})
		for _, sector := range keys {
			wanted[sector] = &pb.SectorOwner{Sector: sector, ControllerCid: owner.Cid, Position: frequency, Identifier: identifier}
		}
	}
	changes := []*pb.EntityChange{}
	for _, old := range state.EntitiesByKind(pb.EntityKind_SECTOR_OWNER) {
		if wanted[old.Key] == nil {
			changes = append(changes, &pb.EntityChange{Key: old.Key, Revision: old.Revision + 1, Operation: &pb.EntityChange_Delete{Delete: &pb.DeleteEntity{Kind: pb.EntityKind_SECTOR_OWNER}}})
		}
	}
	for key, owner := range wanted {
		old := state.Indexes[pb.EntityKind_SECTOR_OWNER][key]
		if old == nil || !proto.Equal(old.Value.GetSectorOwner(), owner) {
			changes = append(changes, replaceEntity(old, key, &pb.EntityRecord{Value: &pb.EntityRecord_SectorOwner{SectorOwner: owner}}))
		}
	}
	layouts := config.GetLayouts(roles, active)
	for _, old := range state.EntitiesByKind(pb.EntityKind_CONTROLLER) {
		controller := proto.Clone(old.Value.GetController()).(*pb.Controller)
		if layout := layouts[controller.Position]; layout != nil && controller.LayoutId != *layout {
			controller.LayoutId, controller.Revision = *layout, old.Revision+1
			changes = append(changes, replaceEntity(old, old.Key, &pb.EntityRecord{Value: &pb.EntityRecord_Controller{Controller: controller}}))
		}
	}
	for _, old := range state.EntitiesByKind(pb.EntityKind_STRIP) {
		s := old.Value.GetStrip()
		pending := false
		for _, coord := range state.EntitiesByKind(pb.EntityKind_COORDINATION) {
			if coord.Value.GetCoordination().Callsign == s.Callsign {
				pending = true
				break
			}
		}
		if pending {
			continue
		}
		ownerFrequency := ""
		if controller := byCID[s.OwnerCid]; controller != nil {
			ownerFrequency = controller.Position
		}
		previous := []string{}
		for _, cid := range s.NextControllers {
			if controller := byCID[cid]; controller != nil {
				previous = append(previous, controller.Position)
			}
		}
		model := &models.Strip{ID: int32(s.Id), Version: int32(old.Revision), Session: id, Callsign: s.Callsign, Origin: s.Departure, Destination: s.Destination, Runway: &s.Runway, Stand: &s.Stand, Owner: &ownerFrequency, Bay: s.Bay, NextOwners: previous}
		for _, position := range positions {
			if position.Value.AircraftKey == s.Callsign && !position.Stale && position.Value.GetPosition() != nil {
				p := position.Value.GetPosition()
				model.PositionLatitude, model.PositionLongitude, model.PositionAltitude = &p.Latitude, &p.Longitude, &p.AltitudeFeet
			}
		}
		owners, _, update, err := server.ComputeCandidateRoute(model, session, modelOwners, coverage)
		if err != nil {
			return nil, err
		}
		if !update {
			continue
		}
		copy := proto.Clone(s).(*pb.Strip)
		copy.NextControllers = nil
		seen := map[string]bool{}
		for _, frequency := range owners {
			if controller := byPosition[frequency]; controller != nil && controller.Cid != "" && !seen[controller.Cid] {
				copy.NextControllers = append(copy.NextControllers, controller.Cid)
				seen[controller.Cid] = true
			}
		}
		if !proto.Equal(copy, s) {
			copy.Revision = old.Revision + 1
			changes = append(changes, replaceEntity(old, old.Key, &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: copy}}))
		}
	}
	sortChanges(changes)
	return changes, nil
}
