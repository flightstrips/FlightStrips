// Package cluster is the opt-in, typed event-write core for the future NATS runtime.
// Nothing in this package is initialized by the current SQL application.
package cluster

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const MaxStateBytes = 1 << 20

func DecodeCommandRequest(data []byte) (*pb.CommandRequest, error) {
	if len(data) > MaxStateBytes {
		return nil, fmt.Errorf("oversized command request")
	}
	r := &pb.CommandRequest{}
	if err := pb.UnmarshalStrict(data, r); err != nil {
		return nil, err
	}
	if _, err := RequestHash(r); err != nil {
		return nil, err
	}
	return r, nil
}

func EncodeCommandRequest(r *pb.CommandRequest) ([]byte, error) {
	if _, err := RequestHash(r); err != nil {
		return nil, err
	}
	b, err := (proto.MarshalOptions{Deterministic: true}).Marshal(r)
	if err == nil && len(b) > MaxStateBytes {
		err = fmt.Errorf("oversized command request")
	}
	return b, err
}

func DecodeCommandReply(data []byte) (*pb.CommandReply, error) {
	if len(data) > MaxStateBytes {
		return nil, fmt.Errorf("oversized command reply")
	}
	r := &pb.CommandReply{}
	if err := pb.UnmarshalStrict(data, r); err != nil {
		return nil, err
	}
	if r.ProtocolRevision != 1 || !canonicalUUID(r.CommandId) || r.Status == pb.CommandReply_STATUS_UNSPECIFIED {
		return nil, fmt.Errorf("invalid command reply")
	}
	if err := validateTyped(r.ProtoReflect()); err != nil {
		return nil, err
	}
	return r, nil
}

func EncodeCommandReply(r *pb.CommandReply) ([]byte, error) {
	if r == nil || r.ProtocolRevision != 1 || !canonicalUUID(r.CommandId) || r.Status == pb.CommandReply_STATUS_UNSPECIFIED {
		return nil, fmt.Errorf("invalid command reply")
	}
	if err := validateTyped(r.ProtoReflect()); err != nil {
		return nil, err
	}
	b, err := (proto.MarshalOptions{Deterministic: true}).Marshal(r)
	if err == nil && len(b) > MaxStateBytes {
		err = fmt.Errorf("oversized command reply")
	}
	return b, err
}

func canonicalUUID(s string) bool {
	id, err := uuid.Parse(s)
	return err == nil && id.String() == s
}

func strictMessage(m proto.Message) error {
	b, err := proto.Marshal(m)
	if err != nil {
		return err
	}
	return pb.UnmarshalStrict(b, m)
}

// RequestHash is computed by the Go owner on a normalized typed request. The
// transport command ID is the only field removed before deterministic encoding.
func RequestHash(request *pb.CommandRequest) (string, error) {
	if request == nil || request.GetProtocolRevision() != 1 || !canonicalUUID(request.GetCommandId()) || request.GetActor() == nil || request.GetCommand() == nil {
		return "", fmt.Errorf("invalid command envelope")
	}
	copy := proto.Clone(request).(*pb.CommandRequest)
	if err := normalize(copy.ProtoReflect()); err != nil {
		return "", err
	}
	if _, err := Subject(copy.GetAggregate()); err != nil {
		return "", err
	}
	if copy.GetActor().GetKind() == pb.Actor_KIND_UNSPECIFIED || strings.TrimSpace(copy.GetActor().GetId()) == "" {
		return "", fmt.Errorf("invalid actor")
	}
	if copy.GetClient() != nil && copy.GetClient().GetAction() == nil {
		return "", fmt.Errorf("missing client action")
	}
	if copy.GetSystem() != nil && copy.GetSystem().GetAction() == nil {
		return "", fmt.Errorf("missing system action")
	}
	copy.CommandId = ""
	if err := strictMessage(copy); err != nil {
		return "", err
	}
	if err := validateTyped(copy.ProtoReflect()); err != nil {
		return "", err
	}
	b, err := (proto.MarshalOptions{Deterministic: true}).Marshal(copy)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

// normalize changes only fields whose ordering or identifier spelling is
// semantically immaterial. Ordered routes, strips, and AMAN sequences remain ordered.
func normalize(m protoreflect.Message) error {
	if len(m.GetUnknown()) != 0 {
		return fmt.Errorf("unknown fields in %s", m.Descriptor().FullName())
	}
	var failure error
	m.Range(func(f protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		name := string(f.Name())
		if f.IsList() {
			list := v.List()
			for i := 0; i < list.Len(); i++ {
				if f.Kind() == protoreflect.MessageKind {
					failure = normalize(list.Get(i).Message())
				} else if f.Kind() == protoreflect.EnumKind && f.Enum().Values().ByNumber(list.Get(i).Enum()) == nil {
					failure = fmt.Errorf("unknown enum in %s", f.FullName())
				}
				if failure != nil {
					return false
				}
			}
			if f.Kind() == protoreflect.StringKind && (name == "recipients" || name == "runway_group_ids") {
				values := make([]string, list.Len())
				for i := range values {
					values[i] = strings.TrimSpace(list.Get(i).String())
				}
				sort.Strings(values)
				for i, s := range values {
					list.Set(i, protoreflect.ValueOfString(s))
				}
			}
		} else if f.Kind() == protoreflect.MessageKind {
			if f.Message().FullName() == "google.protobuf.Timestamp" {
				ts := v.Message().Interface().(*timestamppb.Timestamp)
				if failure = ts.CheckValid(); failure != nil {
					return false
				}
				ts.Nanos = int32(ts.AsTime().UTC().Nanosecond())
			} else {
				failure = normalize(v.Message())
			}
		} else if f.Kind() == protoreflect.EnumKind && f.Enum().Values().ByNumber(v.Enum()) == nil {
			failure = fmt.Errorf("unknown enum in %s", f.FullName())
		} else if f.Kind() == protoreflect.StringKind {
			s := v.String()
			if name == "icao" || name == "airport" || name == "callsign" || name == "flight_callsign" || name == "sector" || name == "runway" || name == "stand" || name == "observed_stand" {
				s = strings.ToUpper(strings.TrimSpace(s))
			}
			if name == "cid" || name == "to_cid" || name == "target_cid" || name == "controller_cid" {
				s = strings.TrimSpace(s)
			}
			m.Set(f, protoreflect.ValueOfString(s))
		}
		return failure == nil
	})
	return failure
}

func Subject(ref *pb.AggregateRef) (string, error) {
	if ref == nil {
		return "", fmt.Errorf("missing aggregate")
	}
	switch x := ref.GetTarget().(type) {
	case *pb.AggregateRef_Global:
		if x.Global == nil {
			break
		}
		return "fs.v1.state.global", nil
	case *pb.AggregateRef_Airport:
		if x.Airport == nil || len(x.Airport.Icao) != 4 || strings.ToUpper(x.Airport.Icao) != x.Airport.Icao {
			break
		}
		for _, c := range x.Airport.Icao {
			if c < 'A' || c > 'Z' {
				return "", fmt.Errorf("invalid ICAO")
			}
		}
		return "fs.v1.state.airport." + x.Airport.Icao, nil
	case *pb.AggregateRef_Session:
		if x.Session == nil || x.Session.Id < 1 {
			break
		}
		return fmt.Sprintf("fs.v1.state.session.%d", x.Session.Id), nil
	}
	return "", fmt.Errorf("invalid aggregate")
}

func validateTyped(message protoreflect.Message) error {
	if checkpoint, ok := message.Interface().(*pb.ProviderCheckpoint); ok {
		legacy := checkpoint.AttemptId == "" && checkpoint.AttemptStatus == pb.ProviderCheckpoint_ATTEMPT_STATUS_UNSPECIFIED
		if !legacy && (!canonicalUUID(checkpoint.AttemptId) || checkpoint.AttemptStatus == pb.ProviderCheckpoint_ATTEMPT_STATUS_UNSPECIFIED) {
			return fmt.Errorf("invalid provider attempt identity/status")
		}
		if checkpoint.AcceptedRevision > 0 && (checkpoint.ObjectName == "" || checkpoint.Sha256 == "") {
			return fmt.Errorf("accepted provider revision requires an object")
		}
	}
	if state, ok := message.Interface().(*pb.CdmState); ok {
		if err := validateCdmState(state); err != nil {
			return err
		}
	}
	if len(message.GetUnknown()) != 0 {
		return fmt.Errorf("unknown fields in %s", message.Descriptor().FullName())
	}
	var failure error
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		check := func(item protoreflect.Value) error {
			switch field.Kind() {
			case protoreflect.MessageKind:
				if field.Message().FullName() == "google.protobuf.Timestamp" {
					return item.Message().Interface().(*timestamppb.Timestamp).CheckValid()
				}
				if field.Message().FullName() == "google.protobuf.Duration" {
					return item.Message().Interface().(*durationpb.Duration).CheckValid()
				}
				return validateTyped(item.Message())
			case protoreflect.DoubleKind, protoreflect.FloatKind:
				x := item.Float()
				if math.IsNaN(x) || math.IsInf(x, 0) {
					return fmt.Errorf("nonfinite %s", field.FullName())
				}
				name := string(field.Name())
				if strings.Contains(name, "latitude") && (x < -90 || x > 90) {
					return fmt.Errorf("latitude out of range")
				}
				if strings.Contains(name, "longitude") && (x < -180 || x > 180) {
					return fmt.Errorf("longitude out of range")
				}
				if (strings.Contains(name, "heading") || strings.Contains(name, "course") || strings.Contains(name, "track")) && (x < 0 || x >= 360) {
					return fmt.Errorf("course out of range")
				}
			case protoreflect.EnumKind:
				if field.Enum().Values().ByNumber(item.Enum()) == nil {
					return fmt.Errorf("unknown enum in %s", field.FullName())
				}
			case protoreflect.StringKind:
				if !validDomainToken(string(field.FullName()), item.String()) {
					return fmt.Errorf("invalid domain token in %s", field.FullName())
				}
			}
			return nil
		}
		if field.IsList() {
			list := value.List()
			for i := 0; i < list.Len(); i++ {
				failure = check(list.Get(i))
				if failure != nil {
					return false
				}
			}
		} else {
			failure = check(value)
		}
		return failure == nil
	})
	return failure
}

func validDomainToken(field, value string) bool {
	var allowed string
	switch field {
	case "flightstrips.cluster.v1.Strip.pdc_state", "flightstrips.cluster.v1.PdcSequence.state", "flightstrips.cluster.v1.PdcEffect.state":
		allowed = "NONE REQUESTED REQUESTED_WITH_FAULTS CLEARED CONFIRMED NO_RESPONSE FAILED REVERT_TO_VOICE"
	case "flightstrips.cluster.v1.PdcSequence.request_channel":
		allowed = "WEB CPDLC"
	case "flightstrips.cluster.v1.AmanAirport.effective_mode":
		allowed = "disabled shadow read_only authoritative blocked"
	case "flightstrips.cluster.v1.AmanFlight.state":
		allowed = "planned airborne unstable stable landed go_around removed"
	case "flightstrips.cluster.v1.AmanFlight.sequence_disposition":
		allowed = "active desequenced"
	case "flightstrips.cluster.v1.AmanFlight.data_status":
		allowed = "fresh stale disconnected"
	case "flightstrips.cluster.v1.AmanFlight.freeze_reason":
		allowed = "none superstable tma manual"
	case "flightstrips.cluster.v1.AmanPrediction.confidence", "flightstrips.cluster.v1.AmanBaseline.confidence":
		allowed = "unknown low medium high"
	case "flightstrips.cluster.v1.NavProcedureFragment.coverage", "flightstrips.cluster.v1.NavFixFragment.coverage", "flightstrips.cluster.v1.NavTerminalPath.coverage", "flightstrips.cluster.v1.NavRouteGeometry.coverage":
		allowed = "complete partial unsupported unavailable"
	case "flightstrips.cluster.v1.NavData.validation_state":
		allowed = "candidate validated"
	case "flightstrips.cluster.v1.AmanRouteFact.state":
		allowed = "active cleared expired"
	default:
		return true
	}
	for _, token := range strings.Fields(allowed) {
		if value == token {
			return true
		}
	}
	return false
}
