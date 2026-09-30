package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestCdmExportValidationRejectsUnreplayableIntents(t *testing.T) {
	valid := &pb.CdmState{Callsign: "SAS1", PendingExports: []*pb.CdmState_ExportIntent{{OperationId: uuid.NewString(), Kind: pb.CdmState_ExportIntent_SET_CDM_DATA, InputRevision: 1, Data: &pb.CdmState_ExportData{Tobt: "120500", Tsat: "120500", Ttot: "121500", Ctot: "1230", Asrt: "1201"}}}}
	if err := validateTyped(valid.ProtoReflect()); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*pb.CdmState)
	}{
		{"unknown kind", func(v *pb.CdmState) { v.PendingExports[0].Kind = 99 }},
		{"missing payload", func(v *pb.CdmState) { v.PendingExports[0].Data = nil }},
		{"invalid identity", func(v *pb.CdmState) { v.PendingExports[0].OperationId = "new-attempt" }},
		{"self dependency", func(v *pb.CdmState) { v.PendingExports[0].AfterOperationId = &v.PendingExports[0].OperationId }},
		{"invalid clock", func(v *pb.CdmState) { v.PendingExports[0].Data.Tsat = "246000" }},
		{"unknown deice", func(v *pb.CdmState) { v.Deice = "opaque" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := proto.Clone(valid).(*pb.CdmState)
			test.mutate(value)
			if validateTyped(value.ProtoReflect()) == nil {
				t.Fatal("invalid replay intent admitted")
			}
		})
	}
}
