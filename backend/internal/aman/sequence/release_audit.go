package sequence

import (
	"FlightStrips/internal/aman"
	"encoding/json"
	"reflect"
	"time"
)

// ApproachReleaseAuditRecords is shared by automatic reconciliation and the
// command coordinator, so every accepted reservation change is transactional.
func ApproachReleaseAuditRecords(before, after aman.AirportState, at time.Time) []aman.AuditRecord {
	prior := make(map[aman.Callsign]*aman.ApproachReleaseReservation, len(before.Flights))
	for _, f := range before.Flights {
		prior[f.Callsign] = f.ApproachRelease
	}
	var records []aman.AuditRecord
	for _, f := range after.Flights {
		if reflect.DeepEqual(prior[f.Callsign], f.ApproachRelease) {
			continue
		}
		payload, _ := json.Marshal(map[string]any{"callsign": f.Callsign, "before": prior[f.Callsign], "after": f.ApproachRelease})
		records = append(records, aman.AuditRecord{Airport: after.Airport, Revision: after.Revision, Category: "aman.approach_release", Payload: payload, RecordedAt: at})
	}
	return records
}
