package cluster

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"FlightStrips/internal/shared"
	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
)

func planStripEdit(request *pb.CommandRequest, state *Aggregate, action *pb.StripAction) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	session, err := stripSession(request, state)
	if err != nil {
		return nil, pb.CommandReply_NOT_FOUND, 0, err
	}
	id := request.GetAggregate().GetSession().GetId()
	actor := request.GetActor()
	if actor.GetKind() != pb.Actor_CONTROLLER || actor.GetSessionId() != id || state.Entities[actor.GetId()].GetValue().GetController() == nil || state.Entities[actor.GetId()].GetValue().GetController().Observer {
		return nil, pb.CommandReply_UNAUTHORIZED, 0, fmt.Errorf("active controller required")
	}
	key := strings.ToUpper(strings.TrimSpace(action.Callsign))
	old := state.Entities[key]
	if old == nil || old.GetValue().GetStrip() == nil {
		return nil, pb.CommandReply_NOT_FOUND, 0, fmt.Errorf("strip not found")
	}
	current := old.Revision
	if request.ExpectedEntityRevision == nil || *request.ExpectedEntityRevision != current {
		return nil, pb.CommandReply_REVISION_CONFLICT, current, fmt.Errorf("stale strip revision")
	}
	s := proto.Clone(old.GetValue().GetStrip()).(*pb.Strip)
	if s.OwnerCid != "" && s.OwnerCid != actor.Id {
		return nil, pb.CommandReply_UNAUTHORIZED, current, fmt.Errorf("strip owned by another controller")
	}
	if s.Validation != nil && s.Validation.Active && (action.GetMove() == nil || !action.GetMove().GetConfirmedRemoval()) {
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("strip is locked by validation")
	}
	var changes []*pb.EntityChange
	switch x := action.GetChange().(type) {
	case *pb.StripAction_SetHeading:
		if x.SetHeading == nil || x.SetHeading.Degrees != nil && (*x.SetHeading.Degrees < 0 || *x.SetHeading.Degrees >= 360) {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("invalid heading")
		}
		s.Heading = x.SetHeading.Degrees
		markStripField(s, "heading")
	case *pb.StripAction_SetSquawk:
		if x.SetSquawk == nil || x.SetSquawk.Code != "" && !validSquawk(x.SetSquawk.Code) {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("invalid squawk")
		}
		s.AssignedSquawk = x.SetSquawk.Code
		markStripField(s, "assigned_squawk")
	case *pb.StripAction_SetRequestedAltitude:
		if x.SetRequestedAltitude == nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("missing altitude")
		}
		s.RequestedAltitude = x.SetRequestedAltitude.Feet
		markStripField(s, "requested_altitude")
	case *pb.StripAction_SetClearedAltitude:
		if x.SetClearedAltitude == nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("missing altitude")
		}
		s.ClearedAltitude = x.SetClearedAltitude.Feet
		markStripField(s, "cleared_altitude")
	case *pb.StripAction_SetBay:
		if x.SetBay == nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("missing bay")
		}
		if err := moveStripBay(state, s, x.SetBay.Bay, session.GetValue().GetSession().Airport, false, false); err != nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, err
		}
	case *pb.StripAction_SetReleasePoint:
		if x.SetReleasePoint == nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("missing release point")
		}
		s.ReleasePoint = x.SetReleasePoint.Point
	case *pb.StripAction_SetMarked:
		if x.SetMarked == nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("missing marked value")
		}
		s.Marked = x.SetMarked.Marked
	case *pb.StripAction_SetRunwayCleared:
		if x.SetRunwayCleared == nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("missing runway flag")
		}
		s.RunwayCleared = x.SetRunwayCleared.Value
	case *pb.StripAction_SetRunwayConfirmed:
		if x.SetRunwayConfirmed == nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("missing runway flag")
		}
		s.RunwayConfirmed = x.SetRunwayConfirmed.Value
	case *pb.StripAction_SetStartRequested:
		if x.SetStartRequested == nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("missing start request")
		}
		s.StartRequested = x.SetStartRequested.Requested
	case *pb.StripAction_SetText:
		if x.SetText == nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("missing text")
		}
		switch x.SetText.Field {
		case pb.SetStripText_ROUTE:
			s.Route = x.SetText.Value
			markStripField(s, "route")
		case pb.SetStripText_REMARKS:
			s.Remarks = x.SetText.Value
			markStripField(s, "remarks")
		case pb.SetStripText_SID:
			s.Sid = x.SetText.Value
			markStripField(s, "sid")
		case pb.SetStripText_RUNWAY:
			s.Runway = x.SetText.Value
			markStripField(s, "runway")
		case pb.SetStripText_REGISTRATION:
			s.Registration = x.SetText.Value
		case pb.SetStripText_LANGUAGE:
			s.Language = x.SetText.Value
		default:
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("unknown strip text field")
		}
	case *pb.StripAction_Move:
		if x.Move == nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("missing move")
		}
		if err := moveStripBay(state, s, x.Move.Bay, session.GetValue().GetSession().Airport, x.Move.GetClearance(), x.Move.GetConfirmedRemoval()); err != nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, err
		}
	case *pb.StripAction_SetOrder:
		if x.SetOrder == nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("missing order")
		}
		changes, err = reorderStrip(state, s, x.SetOrder.InsertAfter)
		if err != nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, err
		}
	case *pb.StripAction_UpdateData:
		if x.UpdateData == nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("missing strip data")
		}
		d := x.UpdateData
		if d.Sid != nil {
			s.Sid = *d.Sid
			markStripField(s, "sid")
		}
		if d.Eobt != nil {
			s.Eobt = d.Eobt
		}
		if d.Route != nil {
			s.Route = *d.Route
			markStripField(s, "route")
		}
		if d.Heading != nil {
			s.Heading = d.Heading
			markStripField(s, "heading")
		}
		if d.AltitudeFeet != nil {
			s.ClearedAltitude = d.AltitudeFeet
			markStripField(s, "cleared_altitude")
		}
		if d.Stand != nil {
			s.Stand = *d.Stand
			markStripField(s, "stand")
		}
		if d.Runway != nil {
			s.Runway = *d.Runway
			markStripField(s, "runway")
		}
		if d.OnBlock != nil {
			s.OnBlock = d.OnBlock
		}
		if d.Remarks != nil {
			s.Remarks = *d.Remarks
			markStripField(s, "remarks")
		}
		if d.AircraftType != nil {
			s.AircraftType = *d.AircraftType
		}
	case *pb.StripAction_MissedApproach:
		if x.MissedApproach == nil || s.Bay != shared.BAY_FINAL && s.Bay != shared.BAY_RWY_ARR {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("invalid missed approach")
		}
		s.Bay = shared.BAY_AIRBORNE
		s.RunwayCleared = false
		s.RunwayConfirmed = false
		s.Sequence, err = endOfStripBay(state, s.Bay, s.Callsign)
		if err != nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, err
		}
	case *pb.StripAction_GenerateSquawk:
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("squawk generation requires an airport allocation planner")
	default:
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("unsupported strip action")
	}
	if len(changes) == 0 {
		if equalStripWithoutRevision(old.GetValue().GetStrip(), s) {
			return &pb.DomainChange{}, pb.CommandReply_COMMITTED, current, nil
		}
		changes = []*pb.EntityChange{stripChange(old, s)}
	}
	if err := checkStripChanges(state, changes); err != nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, err
	}
	return &pb.DomainChange{Changes: changes}, pb.CommandReply_COMMITTED, current, nil
}

func validSquawk(code string) bool {
	if len(code) != 4 {
		return false
	}
	for _, c := range code {
		if c < '0' || c > '7' {
			return false
		}
	}
	return true
}

func markStripField(strip *pb.Strip, field string) {
	for _, existing := range strip.ControllerModifiedFields {
		if existing == field {
			return
		}
	}
	strip.ControllerModifiedFields = append(strip.ControllerModifiedFields, field)
	sort.Strings(strip.ControllerModifiedFields)
}

func moveStripBay(state *Aggregate, s *pb.Strip, target, airport string, clearance, confirmedRemoval bool) error {
	if !validStripBay(target) {
		return fmt.Errorf("invalid bay")
	}
	if confirmedRemoval && target != shared.BAY_HIDDEN && target != shared.BAY_HIDDEN_DEP && target != shared.BAY_ARR_HIDDEN {
		return fmt.Errorf("confirmed removal requires hidden bay")
	}
	if clearance && target != shared.BAY_CLEARED {
		return fmt.Errorf("clearance requires cleared bay")
	}
	if s.Bay == shared.BAY_NOT_CLEARED && target != shared.BAY_NOT_CLEARED && target != shared.BAY_HIDDEN && !clearance {
		return fmt.Errorf("uncleared strip cannot leave not-cleared bay")
	}
	if s.Destination == airport && s.Departure != airport && target == shared.BAY_NOT_CLEARED {
		return fmt.Errorf("arrival cannot enter departure clearance bay")
	}
	if s.Bay != target {
		sequence, err := endOfStripBay(state, target, s.Callsign)
		if err != nil {
			return err
		}
		s.Bay, s.Sequence = target, sequence
		if target == shared.BAY_NOT_CLEARED {
			s.RunwayCleared = false
			s.RunwayConfirmed = false
		}
		if target == shared.BAY_CLEARED {
			s.StartRequested = false
		}
	}
	return nil
}

type orderedStrip struct {
	key      string
	seq      uint64
	strip    *pb.Strip
	tactical *pb.TacticalStrip
	snapshot *pb.EntitySnapshot
}

func reorderStrip(state *Aggregate, target *pb.Strip, after *pb.StripRef) ([]*pb.EntityChange, error) {
	items := make([]orderedStrip, 0)
	for _, e := range state.Entities {
		if s := e.GetValue().GetStrip(); s != nil && s.Bay == target.Bay && s.Callsign != target.Callsign {
			items = append(items, orderedStrip{key: e.Key, seq: s.Sequence, strip: s, snapshot: e})
		}
		if t := e.GetValue().GetTacticalStrip(); t != nil && t.Bay == target.Bay {
			items = append(items, orderedStrip{key: e.Key, seq: t.Sequence, tactical: t, snapshot: e})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].seq != items[j].seq {
			return items[i].seq < items[j].seq
		}
		return items[i].key < items[j].key
	})
	index := 0
	if after != nil {
		found := false
		for i, item := range items {
			if item.strip != nil && after.GetFlightCallsign() != "" && item.strip.Callsign == after.GetFlightCallsign() || item.tactical != nil && after.GetTacticalId() != 0 && item.tactical.Id == after.GetTacticalId() {
				index, found = i+1, true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("order predecessor absent from bay")
		}
	}
	prev := uint64(0)
	if index > 0 {
		prev = items[index-1].seq
	}
	next := uint64(0)
	if index < len(items) {
		next = items[index].seq
	}
	if next == 0 && prev <= math.MaxUint64-stripOrderSpacing {
		target.Sequence = prev + stripOrderSpacing
	} else if next > prev+stripMinOrderGap {
		target.Sequence = prev + (next-prev)/2
	} else {
		items = append(items, orderedStrip{})
		copy(items[index+1:], items[index:])
		items[index] = orderedStrip{key: target.Callsign, strip: target, snapshot: state.Entities[target.Callsign]}
		if uint64(len(items)) > math.MaxUint64/stripOrderSpacing {
			return nil, fmt.Errorf("strip order exhausted")
		}
		changes := make([]*pb.EntityChange, 0)
		for i, item := range items {
			sequence := uint64(i+1) * stripOrderSpacing
			if item.strip != nil {
				if item.strip.Sequence == sequence {
					continue
				}
				copy := proto.Clone(item.strip).(*pb.Strip)
				copy.Sequence = sequence
				changes = append(changes, stripChange(item.snapshot, copy))
			} else if item.tactical != nil && item.tactical.Sequence != sequence {
				copy := proto.Clone(item.tactical).(*pb.TacticalStrip)
				copy.Sequence = sequence
				copy.Revision = item.snapshot.Revision + 1
				changes = append(changes, &pb.EntityChange{Key: item.key, Revision: copy.Revision, Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_TacticalStrip{TacticalStrip: copy}}}})
			}
		}
		sort.Slice(changes, func(i, j int) bool {
			ki, _ := changeKind(changes[i])
			kj, _ := changeKind(changes[j])
			if ki != kj {
				return ki < kj
			}
			return changes[i].Key < changes[j].Key
		})
		return changes, nil
	}
	if target.Sequence == state.Entities[target.Callsign].GetValue().GetStrip().Sequence {
		return nil, nil
	}
	return []*pb.EntityChange{stripChange(state.Entities[target.Callsign], target)}, nil
}
