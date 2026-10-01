package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"time"
)

const QuotaWindowRetention = 48 * time.Hour

// RetireQuotaWindows emits accepted deletions, preserving current minute,
// hour and day counters. Lifetime command deduplication remains independent.
func RetireQuotaWindows(state *Aggregate, changes []*pb.EntityChange, current time.Time) []*pb.EntityChange {
	cutoff := current.Add(-QuotaWindowRetention)
	for id, prior := range state.Indexes[pb.EntityKind_PROVIDER_QUOTA] {
		if prior.GetValue().GetProviderQuota().GetWindowStart().AsTime().Before(cutoff) {
			changes = append(changes, &pb.EntityChange{Key: id, Revision: prior.Revision + 1, Operation: &pb.EntityChange_Delete{Delete: &pb.DeleteEntity{Kind: pb.EntityKind_PROVIDER_QUOTA}}})
		}
	}
	return changes
}
