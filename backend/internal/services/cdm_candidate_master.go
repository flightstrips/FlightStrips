package services

import (
	"context"
	"fmt"

	"FlightStrips/internal/cdm"
	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
)

// ReconcileMaster is the concrete airport-owner replacement for detached
// registerMasterAsync/deregisterMaster. Its identity names accepted registry
// and vIFF master observations; Task 19 owns provider polling of those pages.
func (c *CdmCandidate) ReconcileMaster(ctx context.Context, airport string) error {
	if c.Writes.Client == nil {
		return nil
	}
	ref := &pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: airport}}}
	if !c.Writer.Lease.CanWrite(ref) {
		return fmt.Errorf("CDM airport is not owned")
	}
	if err := c.Writes.Resume(ctx, airport, 0); err != nil {
		return err
	}
	state, err := c.Writer.Read(ctx, globalRef())
	if err != nil {
		return err
	}
	active := false
	for _, entry := range state.EntitiesByKind(pb.EntityKind_SESSION_REGISTRY) {
		s := entry.GetValue().GetSessionRegistry()
		if s.Airport == airport && cdm.ViffEnabledSession(s.Name) && s.State == pb.SessionRegistry_ACTIVE {
			active = true
		}
	}
	masters, revision, err := c.Reads.ReadMasters(ctx, airport)
	if err != nil || masters == nil {
		return fmt.Errorf("accepted vIFF master page unavailable: %v", err)
	}
	registered := false
	for _, m := range masters.Masters {
		if m.Airport == airport && m.Position == cdm.DefaultMasterPosition {
			registered = true
		}
	}
	if active == registered {
		return nil
	}
	fresh, err := c.Writer.Read(ctx, globalRef())
	if err != nil || fresh.Revision != state.Revision {
		return fmt.Errorf("CDM registry changed before master reconciliation")
	}
	_, latest, err := c.Reads.ReadMasters(ctx, airport)
	if err != nil || latest != revision {
		return fmt.Errorf("vIFF master observation changed")
	}
	kind := cluster.ViffSetMaster
	if !active {
		kind = cluster.ViffClearMaster
	}
	id := lifecycleID(fmt.Sprintf("cdm-master/%s/%d/%t", airport, revision, active), "provider")
	_, err = c.Writes.Run(ctx, cluster.ViffWriteSpec{OperationID: id, Kind: kind, Airport: airport, Position: cdm.DefaultMasterPosition})
	return err
}
