package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"fmt"
	"strings"
	"time"
)

func validateCdmState(state *pb.CdmState) error {
	if p := state.Pushback; p != nil {
		if !canonicalUUID(p.OperationId) || p.ExpectedTobt == nil || p.Attempts > 3 || p.Matches > p.Attempts || p.Verified && (!p.Completed || p.Matches == 0) {
			return fmt.Errorf("invalid CDM pushback verification")
		}
	}
	switch state.Deice {
	case "", "L", "M", "H", "J":
	default:
		return fmt.Errorf("invalid CDM deice")
	}
	clock := func(value string) bool {
		format := "1504"
		if len(value) == 6 {
			format = "150405"
		}
		parsed, err := time.Parse(format, value)
		return err == nil && parsed.Format(format) == value
	}
	seen := map[string]bool{}
	for _, intent := range state.PendingExports {
		if intent == nil || !canonicalUUID(intent.OperationId) || seen[intent.OperationId] || intent.InputRevision == 0 {
			return fmt.Errorf("invalid CDM export identity")
		}
		seen[intent.OperationId] = true
		if intent.AfterOperationId != nil && (!canonicalUUID(*intent.AfterOperationId) || *intent.AfterOperationId == intent.OperationId) {
			return fmt.Errorf("invalid CDM export dependency")
		}
		switch intent.Kind {
		case pb.CdmState_ExportIntent_DPI:
			if intent.Data != nil || intent.TaxiMinutes != 0 {
				return fmt.Errorf("invalid CDM DPI fields")
			}
			valid := intent.Value == "REA/1" || intent.Value == "SUSP" || intent.Value == "REQTOBT/NULL/NULL"
			for _, prefix := range []string{"ATOT/", "AOBT/"} {
				if strings.HasPrefix(intent.Value, prefix) && clock(strings.TrimPrefix(intent.Value, prefix)) {
					valid = true
				}
			}
			parts := strings.Split(intent.Value, "/")
			if len(parts) == 3 && parts[0] == "REQTOBT" && clock(parts[1]) && parts[2] == "ATC" {
				valid = true
			}
			if !valid {
				return fmt.Errorf("invalid CDM DPI value")
			}
		case pb.CdmState_ExportIntent_SET_TOBT:
			if !clock(intent.Value) || intent.TaxiMinutes <= 0 || intent.Data != nil {
				return fmt.Errorf("invalid CDM TOBT fields")
			}
		case pb.CdmState_ExportIntent_SET_CDM_DATA:
			if intent.Data == nil || intent.Value != "" || intent.TaxiMinutes != 0 {
				return fmt.Errorf("invalid CDM proposal fields")
			}
			for _, value := range []string{intent.Data.Tobt, intent.Data.Tsat, intent.Data.Ttot, intent.Data.Ctot, intent.Data.Asrt} {
				if value != "" && !clock(value) {
					return fmt.Errorf("invalid CDM proposal clock")
				}
			}
		default:
			return fmt.Errorf("invalid CDM export kind")
		}
	}
	if calc := state.Calculation; calc != nil {
		if calc.TaxiMinutes != nil && *calc.TaxiMinutes < 0 || calc.SequencePosition != nil && *calc.SequencePosition <= 0 {
			return fmt.Errorf("invalid CDM calculation range")
		}
		for _, marker := range calc.ReasonMarkers {
			if marker != nil && marker.RequiredSpacingMinutes != nil && *marker.RequiredSpacingMinutes < 0 {
				return fmt.Errorf("negative CDM spacing")
			}
		}
	}
	return nil
}
