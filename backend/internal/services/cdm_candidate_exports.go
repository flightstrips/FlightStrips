package services

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"FlightStrips/internal/cdm"
	"FlightStrips/internal/cluster"
	"FlightStrips/internal/models"
	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
)

func (c *CdmCandidate) queueExport(state *cluster.Aggregate, data *pb.CdmState, command string, m *models.Strip, revision uint64, config *cdm.CdmAirportConfig) {
	if data.SourceRevision != "" {
		command = data.SourceRevision
	}
	var after *string
	appendIntent := func(kind pb.CdmState_ExportIntent_Kind, label, value string, params *pb.CdmState_ExportData, taxi int) {
		id := lifecycleID(command, "viff/"+m.Callsign+"/"+label)
		prior := after
		after = &id
		if state.Workflows[id] != nil {
			return
		}
		for _, e := range data.PendingExports {
			if e.OperationId == id {
				return
			}
		}
		data.PendingExports = append(data.PendingExports, &pb.CdmState_ExportIntent{OperationId: id, Kind: kind, Value: value, Data: params, TaxiMinutes: int32(taxi), InputRevision: revision, AfterOperationId: prior})
	}
	if data.ReadySyncPending {
		appendIntent(pb.CdmState_ExportIntent_DPI, "ready", "REA/1", nil, 0)
	}
	params, suspend, ok := cdm.ViffProposal(m)
	authoritative := data.ViffRequestSyncPending || data.ReadySyncPending || data.Pushback != nil && !data.Pushback.Completed
	if suspend {
		appendIntent(pb.CdmState_ExportIntent_DPI, "state", "SUSP", nil, 0)
	} else if ok {
		appendIntent(pb.CdmState_ExportIntent_SET_CDM_DATA, "state", "", &pb.CdmState_ExportData{Tobt: params.Tobt, Tsat: params.Tsat, Ttot: params.Ttot, Ctot: params.Ctot, Reason: params.Reason, Asrt: params.Asrt, DepartureInfo: params.DepInfo}, 0)
	} else if authoritative && cdmValue(m.CdmData.Tobt) != "" {
		appendIntent(pb.CdmState_ExportIntent_SET_TOBT, "state", cdmValue(m.CdmData.Tobt), nil, cdm.CandidateTaxiMinutes(m, config))
	} else if authoritative {
		return // Do not acknowledge a request without an authoritative export.
	}
	if data.ViffRequestSyncPending {
		appendIntent(pb.CdmState_ExportIntent_DPI, "clear-request", "REQTOBT/NULL/NULL", nil, 0)
	}
}

func cdmExportParams(callsign string, e *pb.CdmState_ExportData) cdm.SetCdmDataParams {
	return cdm.SetCdmDataParams{Callsign: callsign, Tobt: e.GetTobt(), Tsat: e.GetTsat(), Ttot: e.GetTtot(), Ctot: e.GetCtot(), Reason: e.GetReason(), Asrt: e.GetAsrt(), DepInfo: e.GetDepartureInfo()}
}

// resumeExports uses only persisted typed intent parameters. Every external
// attempt/result is delegated to Task 19. A failed/uncertain dependency blocks
// successors; takeover never starts another unsafe attempt under that UUID.
func (c *CdmCandidate) resumeExports(ctx context.Context, id int32) error {
	if c.Writes.Client == nil {
		return nil
	}
	state, err := c.Writer.Read(ctx, sessionRef(id))
	if err != nil {
		return err
	}
	seed, err := cdmSeed(state, id)
	if err != nil {
		return err
	}
	if !c.usesViff(seed) {
		return nil
	}
	keys := []string{}
	for key := range state.Indexes[pb.EntityKind_CDM_STATE] {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		for {
			if err := c.admission(id); err != nil {
				return err
			}
			state, err = c.Writer.Read(ctx, sessionRef(id))
			if err != nil {
				return err
			}
			entity := state.Indexes[pb.EntityKind_CDM_STATE][key]
			value := entity.GetValue().GetCdmState()
			if value == nil || len(value.PendingExports) == 0 {
				break
			}
			intent := value.PendingExports[0]
			if intent.Kind == pb.CdmState_ExportIntent_SET_TOBT && value.Recalculation != pb.CdmState_NONE {
				break // A derived export cannot precede the local sequence commit.
			}
			if after := intent.AfterOperationId; after != nil {
				previous := state.Workflows[*after]
				if previous == nil || previous.Status != pb.WorkflowRecord_COMPLETED {
					if previous != nil && (previous.Status == pb.WorkflowRecord_FAILED || previous.Status == pb.WorkflowRecord_SUPERSEDED) {
						if err = c.finishExport(ctx, id, key, intent, pb.WorkflowRecord_SUPERSEDED); err != nil {
							return err
						}
						continue
					}
					break
				}
			}
			if existing := state.Workflows[intent.OperationId]; existing != nil && existing.Status == pb.WorkflowRecord_FAILED {
				if err = c.finishExport(ctx, id, key, intent, pb.WorkflowRecord_FAILED); err != nil {
					return err
				}
				continue
			}
			spec := cluster.ViffWriteSpec{OperationID: intent.OperationId, Airport: seed.Airport, SessionID: id, Callsign: key, Value: intent.Value, TaxiMinutes: int(intent.TaxiMinutes)}
			switch intent.Kind {
			case pb.CdmState_ExportIntent_DPI:
				spec.Kind = cluster.ViffDpi
			case pb.CdmState_ExportIntent_SET_CDM_DATA:
				spec.Kind = cluster.ViffSetCdmData
				spec.Data = cdmExportParams(key, intent.Data)
			case pb.CdmState_ExportIntent_SET_TOBT:
				spec.Kind = cluster.ViffSetTobt
			default:
				return fmt.Errorf("unsupported CDM export intent")
			}
			strip := state.Indexes[pb.EntityKind_STRIP][key]
			stale := strip == nil
			if !stale {
				m := cdmModel(state, strip, id)
				if spec.Kind == cluster.ViffSetCdmData {
					current, _, ok := cdm.ViffProposal(m)
					stale = !ok || current != spec.Data
				}
				if spec.Kind == cluster.ViffSetTobt {
					stale = cdmValue(m.CdmData.Tobt) != spec.Value
				}
				if spec.Value == "REA/1" {
					stale = cdmValue(m.CdmData.Status) != "REA"
				}
			}
			if stale && state.Workflows[intent.OperationId] == nil {
				if err = c.finishExport(ctx, id, key, intent, pb.WorkflowRecord_SUPERSEDED); err != nil {
					return err
				}
				continue
			}
			_, callErr := c.Writes.Run(ctx, spec)
			state, err = c.Writer.Read(ctx, sessionRef(id))
			if err != nil {
				return err
			}
			result := state.Workflows[intent.OperationId]
			if result == nil || result.Status == pb.WorkflowRecord_PENDING {
				if callErr != nil {
					return callErr
				}
				break
			}
			if result.Status == pb.WorkflowRecord_FAILED {
				if err = c.finishExport(ctx, id, key, intent, pb.WorkflowRecord_FAILED); err != nil {
					return err
				}
				continue
			}
			if result.Status != pb.WorkflowRecord_COMPLETED {
				return fmt.Errorf("invalid vIFF result state")
			}
			if err = c.finishExport(ctx, id, key, intent, pb.WorkflowRecord_COMPLETED); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *CdmCandidate) finishExport(ctx context.Context, id int32, key string, intent *pb.CdmState_ExportIntent, status pb.WorkflowRecord_Status) error {
	req := cdmSystem(id, lifecycleID(intent.OperationId, "apply-result/"+status.String()), 0, &pb.SystemCommand{Action: &pb.SystemCommand_RemoveEntity{RemoveEntity: &pb.RemoveEntity{Key: key, Kind: pb.EntityKind_CDM_STATE}}})
	w := c.Writer
	w.Plan = func(_ context.Context, _ *pb.CommandRequest, state *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		old := state.Indexes[pb.EntityKind_CDM_STATE][key]
		value := old.GetValue().GetCdmState()
		if value == nil {
			return &pb.DomainChange{}, pb.CommandReply_COMMITTED, 0, nil
		}
		copy := proto.Clone(value).(*pb.CdmState)
		found := false
		copy.PendingExports = nil
		for _, item := range value.PendingExports {
			if item.OperationId == intent.OperationId {
				if !proto.Equal(item, intent) {
					return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("CDM intent changed")
				}
				found = true
			} else {
				copy.PendingExports = append(copy.PendingExports, proto.Clone(item).(*pb.CdmState_ExportIntent))
			}
		}
		if !found {
			return &pb.DomainChange{}, pb.CommandReply_COMMITTED, 0, nil
		}
		change := &pb.DomainChange{}
		if status == pb.WorkflowRecord_COMPLETED {
			if state.Workflows[intent.OperationId].GetStatus() != pb.WorkflowRecord_COMPLETED {
				return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("CDM provider result unproven")
			}
			if strings.HasPrefix(intent.Value, "ATOT/") {
				copy.AtotViffPending = false
			}
			if intent.Value == "REQTOBT/NULL/NULL" {
				copy.ViffRequestSyncPending = false
			}
			if (intent.Kind == pb.CdmState_ExportIntent_SET_CDM_DATA || intent.Kind == pb.CdmState_ExportIntent_SET_TOBT || intent.Value == "SUSP") && intent.AfterOperationId != nil && copy.ReadySyncPending {
				otherReady := false
				for _, queued := range copy.PendingExports {
					if queued.Value == "REA/1" {
						otherReady = true
					}
				}
				if !otherReady {
					copy.ReadySyncPending = false
				}
			}
		} else if status == pb.WorkflowRecord_SUPERSEDED {
			ref := sessionRef(id)
			change.Workflows = append(change.Workflows, &pb.WorkflowRecord{WorkflowId: intent.OperationId, Source: ref, Destination: ref, Step: "cdm-export-superseded", DerivedCommandId: req.CommandId, Status: pb.WorkflowRecord_SUPERSEDED, SourceRevision: &intent.InputRevision})
		} else if status == pb.WorkflowRecord_FAILED {
			if state.Workflows[intent.OperationId].GetStatus() != pb.WorkflowRecord_FAILED {
				return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("uncertain provider outcome unproven")
			}
		}
		change.Changes = append(change.Changes, candidateUpsert(key, old, &pb.EntityRecord{Value: &pb.EntityRecord_CdmState{CdmState: copy}}))
		return change, pb.CommandReply_COMMITTED, 0, nil
	}
	return cdmReply(w.Execute(ctx, req))
}
