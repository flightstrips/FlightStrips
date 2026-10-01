package cluster

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"FlightStrips/internal/cdm"
	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// ViffClient is the operational vIFF transport. All methods may have sent an
// external write when they return an error, so takeover never retries a call.
type ViffClient interface {
	SetMasterAirport(context.Context, string, string) error
	ClearMasterAirport(context.Context, string, string) error
	IFPSDpi(context.Context, string, string) error
	IFPSSetCdmData(context.Context, cdm.SetCdmDataParams) error
	IFPSSetTobt(context.Context, string, string, int) error
}

type ViffWriteKind string

const (
	ViffSetMaster   ViffWriteKind = "set-master"
	ViffClearMaster ViffWriteKind = "clear-master"
	ViffDpi         ViffWriteKind = "dpi"
	ViffSetCdmData  ViffWriteKind = "set-cdm-data"
	ViffSetTobt     ViffWriteKind = "set-tobt"
)

// OperationID is the stable UUID of the logical CDM transition, reused after
// retry or owner takeover. The request digest in the intent step prevents an
// ID from silently acknowledging a different vIFF write.
type ViffWriteSpec struct {
	OperationID string
	Kind        ViffWriteKind
	Airport     string
	SessionID   int32
	Position    string
	Callsign    string
	Value       string
	TaxiMinutes int
	Data        cdm.SetCdmDataParams
}

type ViffWriteAdapter struct {
	Worker ExternalCallWorker
	Client ViffClient
}

func (a ViffWriteAdapter) Run(ctx context.Context, spec ViffWriteSpec) (bool, error) {
	if a.Client == nil || !canonicalUUID(spec.OperationID) || spec.Airport == "" || spec.Airport != strings.ToUpper(spec.Airport) {
		return false, fmt.Errorf("invalid vIFF write identity")
	}
	switch spec.Kind {
	case ViffSetMaster, ViffClearMaster, ViffDpi, ViffSetCdmData, ViffSetTobt:
	default:
		return false, fmt.Errorf("unsupported vIFF write kind")
	}
	master := spec.Kind == ViffSetMaster || spec.Kind == ViffClearMaster
	if master && (spec.SessionID != 0 || spec.Position == "" || spec.Callsign != "") || !master && (spec.SessionID <= 0 || spec.Position != "" || spec.Callsign == "" || spec.Callsign != strings.ToUpper(spec.Callsign)) {
		return false, fmt.Errorf("vIFF write has wrong owner or target")
	}
	if spec.Kind == ViffSetCdmData && (spec.Data.Callsign != spec.Callsign || spec.Value != "" || spec.TaxiMinutes != 0) || spec.Kind != ViffSetCdmData && spec.Data != (cdm.SetCdmDataParams{}) {
		return false, fmt.Errorf("vIFF CDM data target mismatch")
	}
	if spec.Kind == ViffSetTobt && (spec.Value == "" || spec.TaxiMinutes <= 0) || (spec.Kind == ViffDpi && (spec.Value == "" || spec.TaxiMinutes != 0)) || (master && (spec.Value != "" || spec.TaxiMinutes != 0)) {
		return false, fmt.Errorf("invalid vIFF write parameters")
	}
	var ref *pb.AggregateRef
	if master {
		ref = airportRef(spec.Airport)
	} else {
		ref = sessionRef(spec.SessionID)
	}
	if _, err := Subject(ref); err != nil {
		return false, err
	}
	if !master {
		if err := viffSessionAirport(ctx, a.Worker.Writer, spec.SessionID, spec.Airport); err != nil {
			return false, err
		}
	}
	step := "external/viff/" + string(spec.Kind) + "/" + viffWriteDigest(spec)
	return a.Worker.Run(ctx, ExternalCallSpec{Source: ref, Destination: ref, WorkflowID: spec.OperationID, Step: step,
		Fetch: func(ctx context.Context) (proto.Message, error) {
			var err error
			switch spec.Kind {
			case ViffSetMaster:
				err = a.Client.SetMasterAirport(ctx, spec.Airport, spec.Position)
			case ViffClearMaster:
				err = a.Client.ClearMasterAirport(ctx, spec.Airport, spec.Position)
			case ViffDpi:
				err = a.Client.IFPSDpi(ctx, spec.Callsign, spec.Value)
			case ViffSetCdmData:
				err = a.Client.IFPSSetCdmData(ctx, spec.Data)
			case ViffSetTobt:
				err = a.Client.IFPSSetTobt(ctx, spec.Callsign, spec.Value, spec.TaxiMinutes)
			default:
				return nil, fmt.Errorf("unsupported vIFF write kind")
			}
			if err != nil {
				return nil, err
			}
			return &emptypb.Empty{}, nil
		},
		Commit: func(ctx context.Context, resultID string, result proto.Message) *pb.CommandReply {
			if _, ok := result.(*emptypb.Empty); !ok {
				return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT}
			}
			workflow := &pb.WorkflowRecord{WorkflowId: spec.OperationID, Source: ref, Destination: ref, Step: step, DerivedCommandId: resultID, Status: pb.WorkflowRecord_COMPLETED}
			request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: resultID, Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "viff-result"}, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_AdvanceWorkflow{AdvanceWorkflow: &pb.AdvanceWorkflow{Workflow: workflow}}}}}
			writer := a.Worker.Writer
			writer.Plan = func(_ context.Context, _ *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
				intent, err := state.LookupWorkflow(spec.OperationID)
				if err != nil {
					return nil, pb.CommandReply_UNAVAILABLE, 0, err
				}
				if intent == nil || intent.Status != pb.WorkflowRecord_PENDING || intent.Step != step || !proto.Equal(intent.Source, ref) || !proto.Equal(intent.Destination, ref) || intent.DerivedCommandId != resultID {
					return nil, pb.CommandReply_REVISION_CONFLICT, 0, fmt.Errorf("vIFF intent changed before result commit")
				}
				return &pb.DomainChange{}, pb.CommandReply_COMMITTED, 0, nil
			}
			return writer.Execute(ctx, request)
		},
	})
}

func viffWriteDigest(spec ViffWriteSpec) string {
	h := sha256.New()
	write := func(value string) {
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(value)))
		_, _ = h.Write(size[:])
		_, _ = h.Write([]byte(value))
	}
	for _, value := range []string{spec.OperationID, string(spec.Kind), spec.Airport, strconv.FormatInt(int64(spec.SessionID), 10), spec.Position, spec.Callsign, spec.Value, strconv.Itoa(spec.TaxiMinutes), spec.Data.Callsign, spec.Data.Tobt, spec.Data.Tsat, spec.Data.Ttot, spec.Data.Ctot, spec.Data.Reason, spec.Data.Asrt, spec.Data.DepInfo} {
		write(value)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func viffSessionAirport(ctx context.Context, writer Writer, sessionID int32, airport string) error {
	ref := sessionRef(sessionID)
	subject, err := Subject(ref)
	if err != nil {
		return err
	}
	state, err := writer.load(ctx, subject, ref)
	if err != nil {
		return err
	}
	entry := state.Indexes[pb.EntityKind_SESSION][strconv.FormatInt(int64(sessionID), 10)]
	if entry == nil || entry.GetValue().GetSession() == nil || entry.GetValue().GetSession().Airport != airport || entry.GetValue().GetSession().Tombstoned {
		return fmt.Errorf("vIFF session airport is unavailable or changed")
	}
	return nil
}

func (a ViffWriteAdapter) Resume(ctx context.Context, airport string, sessionID int32) error {
	if sessionID > 0 {
		return a.Worker.Resume(ctx, sessionRef(sessionID))
	}
	return a.Worker.Resume(ctx, airportRef(airport))
}
