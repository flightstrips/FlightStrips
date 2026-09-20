package metrics

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// RecordStandPublication separates normal batching from degraded publication.
// Do not label these metrics with session IDs, callsigns, or error strings.
func RecordStandPublication(ctx context.Context, mode string, assignments int) {
	switch mode {
	case "snapshot", "assignment_fallback", "block_fallback":
	default:
		mode = "other"
	}
	get().satPublicationAssignments.Record(ctx, int64(max(assignments, 0)), metric.WithAttributes(attribute.String("mode", mode)))
}

// RecordStandWebsocketFanout records even messages with no recipients. The
// histogram count is the source rate; its sum is enqueue attempts, not writes.
func RecordStandWebsocketFanout(ctx context.Context, eventType string, fanout int) {
	switch eventType {
	case "stand_assignment_update", "stand_assignment_removed", "stand_status_snapshot", "stand_block_update":
	default:
		return
	}
	get().satWebsocketFanout.Record(ctx, int64(max(fanout, 0)), metric.WithAttributes(attribute.String("type", eventType)))
}
