package services

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/metrics"
	"FlightStrips/internal/models"
	pb "FlightStrips/pkg/events/cluster"
)

// TrafficCandidate publishes a read-only projection gauge from the accepted
// session owner. Its optional local sampling interval only coalesces metrics;
// it cannot change domain state or establish authority.
type TrafficCandidate struct {
	Writer cluster.Writer
	Now    func() time.Time
	Record func(context.Context, string, string, int64, int64, int64, int64)
}

func NewTrafficCandidate(writer cluster.Writer) (*TrafficCandidate, error) {
	if writer.Store == nil || writer.Projection == nil || writer.Lease == nil {
		return nil, fmt.Errorf("traffic requires projection and accepted owner writer")
	}
	return &TrafficCandidate{Writer: writer, Now: time.Now, Record: metrics.RecordTrafficSnapshot}, nil
}

// Traffic is bound to SessionWork.Traffic at cutover. Recheck readiness and
// ownership immediately before publication, including after projection reads.
func (c *TrafficCandidate) Traffic(ctx context.Context, id int32) error {
	ref := sessionRef(id)
	if c.Writer.Projection == nil || c.Writer.Lease == nil || c.Record == nil || c.Now == nil {
		return fmt.Errorf("traffic candidate is incomplete")
	}
	if err := c.Writer.Projection.Ready(); err != nil {
		return err
	}
	if !lifecycleOwnerCanPlan(c.Writer.Lease, c.Writer.Projection.Async != nil, ref) {
		return fmt.Errorf("traffic session is not owned")
	}
	state, err := c.Writer.Projection.ReadEntityKinds(ref, pb.EntityKind_SESSION, pb.EntityKind_STRIP)
	if err != nil {
		return err
	}
	seed := state.Indexes[pb.EntityKind_SESSION][strconv.Itoa(int(id))].GetValue().GetSession()
	if seed == nil || seed.Tombstoned {
		return fmt.Errorf("traffic session is unavailable")
	}
	strips := make([]*models.Strip, 0, len(state.Indexes[pb.EntityKind_STRIP]))
	for _, entry := range state.EntitiesByKind(pb.EntityKind_STRIP) {
		s := entry.GetValue().GetStrip()
		strips = append(strips, &models.Strip{Bay: s.Bay, CdmData: &models.CdmData{Aldt: lifecycleClock(s.Aldt), Aobt: lifecycleClock(s.Aobt)}})
	}
	snapshot := buildTrafficSnapshot(strips, c.Now().UTC())
	if err = c.Writer.Projection.Ready(); err != nil {
		return err
	}
	if !lifecycleOwnerCanPlan(c.Writer.Lease, c.Writer.Projection.Async != nil, ref) {
		return fmt.Errorf("traffic lease lost before publication")
	}
	c.Record(ctx, seed.Name, seed.Airport, snapshot.onStand, snapshot.taxiing, snapshot.arr15m, snapshot.dep15m)
	return nil
}
