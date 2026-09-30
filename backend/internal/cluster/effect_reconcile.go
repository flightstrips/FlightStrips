package cluster

import (
	"sort"

	pb "FlightStrips/pkg/events/cluster"
	euroscope "FlightStrips/pkg/events/euroscope"
)

// ReconciledUnknownEffects identifies desired state-setting effects confirmed
// by a later authoritative EuroScope sync. The caller may use the sync's
// observed fields to repair domain state; this never changes the original
// UNKNOWN command outcome into an execution acknowledgment.
func ReconciledUnknownEffects(state *Aggregate, sync *euroscope.SyncEvent) []string {
	if state == nil || sync == nil {
		return nil
	}
	strips := map[string]*euroscope.Strip{}
	for _, strip := range sync.Strips {
		if strip != nil {
			strips[strip.Callsign] = strip
		}
	}
	ids := []string{}
	for id, effect := range state.Effects {
		if effect == nil || effect.Status != pb.EffectRecord_UNKNOWN {
			continue
		}
		fpl := effect.GetSetFlightPlan()
		if fpl == nil {
			continue
		}
		strip := strips[fpl.Callsign]
		if strip == nil {
			continue
		}
		var observed string
		switch fpl.Field {
		case "ROUTE":
			observed = strip.Route
		case "REMARKS":
			observed = strip.Remarks
		case "SID":
			observed = strip.Sid
		case "RUNWAY":
			observed = strip.Runway
		case "EOBT":
			observed = strip.Eobt
		case "AIRCRAFT_TYPE":
			observed = strip.AircraftType
		case "STAND":
			observed = strip.Stand
		default:
			continue
		}
		if observed == fpl.Value {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}
