package airportworkflow

import (
	"FlightStrips/internal/models"
	"time"
)

func SnapshotOf(s *models.Strip) Snapshot {
	if s == nil {
		return Snapshot{}
	}
	var annotations = Snapshot{}.Annotations
	if s.BoardState != nil {
		annotations = s.BoardState.Annotations
		if annotations.REA != nil {
			v := *annotations.REA
			annotations.REA = &v
		}
		if annotations.ATA != nil {
			v := *annotations.ATA
			annotations.ATA = &v
		}
	}
	var removedAt = s.BoardState
	var removed *time.Time
	var reason string
	if removedAt != nil {
		if removedAt.RemovedAt != nil {
			v := *removedAt.RemovedAt
			removed = &v
		}
		reason = removedAt.RemovalReason
	}
	return Snapshot{RemovedAt: removed, RemovalReason: reason, BoardMember: s.BoardState != nil, Local: s.BoardState != nil && s.BoardState.Local, HasFP: s.HasFP, Annotations: annotations, GroundStateKnown: s.State != nil, Exists: true, ID: s.ID, Version: s.Version, Callsign: s.Callsign, Origin: s.Origin, Destination: s.Destination, Bay: s.Bay, GroundState: snapshotValue(s.State), Owner: snapshotValue(s.Owner), Cleared: s.Cleared, PDCState: s.PdcState, AssignedSquawk: snapshotValue(s.AssignedSquawk), TrackingController: s.TrackingController, StartRequested: s.StartReq, Latitude: snapshotValue(s.PositionLatitude), Longitude: snapshotValue(s.PositionLongitude), Altitude: snapshotValue(s.PositionAltitude), EuroscopeSeen: s.EuroscopeSeenAt != nil, Runway: snapshotValue(s.Runway), Landed: s.CdmData != nil && s.CdmData.Aldt != nil}
}
func snapshotValue[T any](p *T) (v T) {
	if p != nil {
		return *p
	}
	return v
}
