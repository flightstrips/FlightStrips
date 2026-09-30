package cluster

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
)

// VatsimSessionAdapter applies one committed global generation atomically to
// one live session. The session cursor fences older generations, including
// after a different owner takes over. Provider calls remain global-owned.
type VatsimSessionAdapter struct {
	Source NavigationWeather
	Writer Writer
}

func (a VatsimSessionAdapter) Reconcile(ctx context.Context, sessionID int32) *pb.CommandReply {
	if sessionID < 1 {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT}
	}
	checkpoint, page, revision, err := a.Source.CheckpointRevisionFor(ctx, globalRef(), "vatsim", "network-data/v3")
	if err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_UNAVAILABLE, Detail: err.Error()}
	}
	if checkpoint == nil || page == nil || revision == 0 || checkpoint.Sha256 == "" {
		return &pb.CommandReply{Status: pb.CommandReply_NOT_FOUND, Detail: "VATSIM source generation unavailable"}
	}
	if err := validateProviderPage(page); err != nil || page.GetVatsim() == nil || page.GetVatsim().SnapshotAt == nil {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT, Detail: "invalid VATSIM source generation"}
	}
	id, err := ProviderEventCommandID("vatsim-session", "vatsim", fmt.Sprintf("%d/%d/%s", sessionID, revision, checkpoint.Sha256))
	if err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT, Detail: err.Error()}
	}
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: id, Aggregate: sessionRef(sessionID),
		Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "vatsim-session"},
		Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{
			Key: "vatsim", Value: &pb.EntityRecord{Value: &pb.EntityRecord_VatsimSessionCursor{VatsimSessionCursor: &pb.VatsimSessionCursor{
				Provider: "vatsim", SourceRevision: revision, SourceSha256: checkpoint.Sha256, SnapshotAt: page.GetVatsim().SnapshotAt,
			}}},
		}}}}}
	w := a.Writer
	w.Plan = func(_ context.Context, _ *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		return planVatsimGeneration(state, sessionID, revision, checkpoint.Sha256, page.GetVatsim())
	}
	return w.Execute(ctx, request)
}

func planVatsimGeneration(state *Aggregate, sessionID int32, revision uint64, sha string, page *pb.VatsimPage) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	if state == nil || page == nil || page.SnapshotAt == nil || page.SnapshotAt.CheckValid() != nil || revision == 0 || len(sha) != 64 {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("invalid VATSIM generation")
	}
	sessionEntry := state.Indexes[pb.EntityKind_SESSION][strconv.FormatInt(int64(sessionID), 10)]
	if sessionEntry == nil || sessionEntry.GetValue().GetSession() == nil || sessionEntry.GetValue().GetSession().Tombstoned {
		return nil, pb.CommandReply_NOT_FOUND, 0, fmt.Errorf("live session not found")
	}
	session := sessionEntry.GetValue().GetSession()
	if !strings.EqualFold(session.Name, "LIVE") || session.Airport == "" {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("VATSIM reconciliation requires a live airport session")
	}
	oldCursor := state.Indexes[pb.EntityKind_VATSIM_SESSION_CURSOR]["vatsim"]
	if oldCursor != nil {
		prior := oldCursor.GetValue().GetVatsimSessionCursor()
		if prior == nil || prior.SourceRevision == 0 {
			return nil, pb.CommandReply_UNAVAILABLE, oldCursor.Revision, fmt.Errorf("corrupt VATSIM cursor")
		}
		if prior.SourceRevision > revision || prior.SourceRevision == revision && prior.SourceSha256 != sha {
			return nil, pb.CommandReply_REVISION_CONFLICT, oldCursor.Revision, fmt.Errorf("newer VATSIM generation already applied")
		}
		if prior.SourceRevision == revision {
			return &pb.DomainChange{}, pb.CommandReply_COMMITTED, oldCursor.Revision, nil
		}
	}
	airport := strings.ToUpper(session.Airport)
	seen := make(map[string]bool)
	changes := make([]*pb.EntityChange, 0)
	nextID := session.NextStripId
	flights := append([]*pb.VatsimFlight(nil), page.Flights...)
	sort.Slice(flights, func(i, j int) bool { return flights[i].GetCallsign() < flights[j].GetCallsign() })
	nextOrder := map[string]uint64{}
	for _, flight := range flights {
		if flight == nil || flight.Callsign == "" || flight.Cid == "" || flight.FlightPlan == nil || flight.State != "online" && flight.State != "prefile" || math.IsNaN(flight.Latitude) || math.IsNaN(flight.Longitude) || math.IsInf(flight.Latitude, 0) || math.IsInf(flight.Longitude, 0) {
			return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("invalid VATSIM flight")
		}
		if !strings.EqualFold(flight.FlightPlan.Origin, airport) && !strings.EqualFold(flight.FlightPlan.Destination, airport) {
			continue
		}
		callsign := strings.ToUpper(flight.Callsign)
		if seen[callsign] {
			return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("duplicate VATSIM callsign")
		}
		seen[callsign] = true
		old := state.Indexes[pb.EntityKind_STRIP][callsign]
		var strip *pb.Strip
		if old == nil {
			if nextID == 0 || nextID == math.MaxUint64 {
				return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("strip ID exhausted")
			}
			bay := "ARR_HIDDEN"
			if strings.EqualFold(flight.FlightPlan.Origin, airport) {
				bay = "DEP_HIDDEN"
			}
			strip = &pb.Strip{Id: nextID, Callsign: callsign, Bay: bay, VatsimOnly: true, HasFlightPlan: true}
			sequence := nextOrder[bay]
			if sequence == 0 {
				var err error
				sequence, err = endOfStripBay(state, bay, callsign)
				if err != nil {
					return nil, pb.CommandReply_INVALID_ARGUMENT, 0, err
				}
			}
			if sequence > math.MaxUint64-stripOrderSpacing {
				return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("strip order exhausted")
			}
			strip.Sequence = sequence
			nextOrder[bay] = sequence + stripOrderSpacing
			nextID++
		} else {
			strip = proto.Clone(old.GetValue().GetStrip()).(*pb.Strip)
			if strip.VatsimCid != "" && strip.VatsimCid != flight.Cid {
				return nil, pb.CommandReply_REVISION_CONFLICT, old.Revision, fmt.Errorf("VATSIM CID changed for %s", callsign)
			}
		}
		strip.VatsimCid = flight.Cid
		strip.VatsimSourceRevision = revision
		strip.VatsimPlanRevision = flight.FlightPlan.Revision
		strip.VatsimSeenAt = page.SnapshotAt
		if flight.LastUpdated != nil && flight.LastUpdated.CheckValid() == nil {
			strip.VatsimSeenAt = flight.LastUpdated
		}
		strip.VatsimOnline = flight.State == "online"
		if strip.VatsimOnline {
			strip.VatsimLatitude, strip.VatsimLongitude = &flight.Latitude, &flight.Longitude
		}
		// EuroScope and controller state take precedence once a strip has been
		// observed operationally. Only VATSIM-created strips receive plan text.
		if strip.VatsimOnly {
			strip.Departure = strings.ToUpper(flight.FlightPlan.Origin)
			strip.Destination = strings.ToUpper(flight.FlightPlan.Destination)
			strip.Alternate = flight.FlightPlan.Alternate
			strip.AircraftType = flight.FlightPlan.AircraftShort
			if !stripFieldModified(strip, "route") {
				strip.Route = flight.FlightPlan.Route
			}
			if !stripFieldModified(strip, "remarks") {
				strip.Remarks = flight.FlightPlan.Remarks
			}
			if !stripFieldModified(strip, "assigned_squawk") {
				strip.AssignedSquawk = flight.FlightPlan.AssignedSquawk
			}
		}
		if old == nil || !equalStripWithoutRevision(old.GetValue().GetStrip(), strip) {
			changes = append(changes, stripChange(old, strip))
		}
	}
	for _, old := range state.EntitiesByKind(pb.EntityKind_STRIP) {
		strip := old.GetValue().GetStrip()
		if strip == nil || !strip.VatsimOnly || strip.VatsimCid == "" || seen[strip.Callsign] || strip.VatsimSourceRevision >= revision {
			continue
		}
		if strip.Stand != "" || strip.OwnerCid != "" || state.Indexes[pb.EntityKind_STAND_ASSIGNMENT][strip.Callsign] != nil || len(strip.ControllerModifiedFields) != 0 {
			// A strip with operational state remains visible for its lifecycle
			// worker. Record that this source generation no longer sees it.
			copy := proto.Clone(strip).(*pb.Strip)
			copy.VatsimOnline = false
			copy.VatsimSourceRevision = revision
			if !equalStripWithoutRevision(strip, copy) {
				changes = append(changes, stripChange(old, copy))
			}
			continue
		}
		changes = append(changes, candidateDelete(strip.Callsign, old, pb.EntityKind_STRIP))
	}
	if nextID != session.NextStripId {
		copy := proto.Clone(session).(*pb.Session)
		copy.NextStripId = nextID
		changes = append(changes, candidateUpsert(sessionEntry.Key, sessionEntry, &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: copy}}))
	}
	cursor := &pb.VatsimSessionCursor{Provider: "vatsim", SourceRevision: revision, SourceSha256: sha, SnapshotAt: page.SnapshotAt}
	changes = append(changes, candidateUpsert("vatsim", oldCursor, &pb.EntityRecord{Value: &pb.EntityRecord_VatsimSessionCursor{VatsimSessionCursor: cursor}}))
	sortCandidateChanges(changes)
	if err := checkStripChanges(state, changes); err != nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, err
	}
	return &pb.DomainChange{Changes: changes}, pb.CommandReply_COMMITTED, 0, nil
}

func stripFieldModified(strip *pb.Strip, field string) bool {
	for _, name := range strip.ControllerModifiedFields {
		if name == field {
			return true
		}
	}
	return false
}
