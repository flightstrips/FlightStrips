package vatsim

import (
	"context"
	"fmt"
	"math"
	"sort"

	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CandidatePage fetches and converts a generation under a caller-owned
// external-call intent. Provider JSON stays in this HTTP adapter package.
func (c *Cache) CandidatePage(ctx context.Context, prior *pb.VatsimPage) (*pb.VatsimPage, error) {
	if c == nil {
		return nil, fmt.Errorf("VATSIM cache unavailable")
	}
	snapshot, _, err := c.fetch(ctx)
	if err != nil {
		return nil, err
	}
	if prior != nil {
		previous, err := snapshotFromTyped(prior)
		if err != nil {
			return nil, err
		}
		snapshot = preserveNewerFlightPlans(previous, snapshot)
	}
	return typedVatsimPage(snapshot)
}

// SnapshotFromPage reconstructs the existing in-memory VATSIM lookup model
// from the global typed checkpoint, without contacting the provider.
func SnapshotFromPage(page *pb.ProviderPage) (Snapshot, error) {
	if page == nil || page.Provider != "vatsim" || page.Resource != "network-data/v3" {
		return Snapshot{}, fmt.Errorf("invalid VATSIM provider page")
	}
	snapshot, err := snapshotFromTyped(page.GetVatsim())
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Timestamp: snapshot.timestamp, flightsByCallsign: snapshot.flightsByCallsign, flightsByCID: snapshot.flightsByCID}, nil
}

func typedVatsimPage(snapshot cacheSnapshot) (*pb.VatsimPage, error) {
	if snapshot.timestamp.IsZero() {
		return nil, fmt.Errorf("missing VATSIM snapshot timestamp")
	}
	page := &pb.VatsimPage{SnapshotAt: timestamppb.New(snapshot.timestamp.UTC())}
	keys := make([]string, 0, len(snapshot.flightsByCallsign))
	for callsign := range snapshot.flightsByCallsign {
		keys = append(keys, callsign)
	}
	sort.Strings(keys)
	for _, callsign := range keys {
		flight := snapshot.flightsByCallsign[callsign]
		if flight.CID == "" || flight.Callsign == "" || flight.State != FlightStateOnline && flight.State != FlightStatePrefile || flight.Altitude < math.MinInt32 || flight.Altitude > math.MaxInt32 || flight.Groundspeed < math.MinInt32 || flight.Groundspeed > math.MaxInt32 {
			return nil, fmt.Errorf("invalid VATSIM flight")
		}
		plan := flight.FlightPlan
		value := &pb.VatsimFlight{Cid: flight.CID, Callsign: flight.Callsign, State: string(flight.State), Latitude: flight.Latitude, Longitude: flight.Longitude,
			Altitude: int32(flight.Altitude), Groundspeed: int32(flight.Groundspeed), FlightPlan: &pb.VatsimFlightPlan{
				FlightRules: plan.FlightRules, Aircraft: plan.Aircraft, AircraftFaa: plan.AircraftFAA, AircraftShort: plan.AircraftShort,
				Origin: plan.Origin, Destination: plan.Destination, Alternate: plan.Alternate, Eobt: plan.EOBT,
				EnrouteDuration: plan.EnrouteDuration, RequestedLevel: plan.RequestedLevel, Remarks: plan.Remarks,
				Route: plan.Route, AssignedSquawk: plan.AssignedSquawk, Revision: plan.Revision}}
		if !flight.LogonTime.IsZero() {
			value.LogonTime = timestamppb.New(flight.LogonTime.UTC())
		}
		if !flight.LastUpdated.IsZero() {
			value.LastUpdated = timestamppb.New(flight.LastUpdated.UTC())
		}
		page.Flights = append(page.Flights, value)
	}
	return page, nil
}

func snapshotFromTyped(page *pb.VatsimPage) (cacheSnapshot, error) {
	if page == nil || page.SnapshotAt == nil || page.SnapshotAt.CheckValid() != nil {
		return cacheSnapshot{}, fmt.Errorf("invalid VATSIM page")
	}
	snapshot := newCacheSnapshot(page.SnapshotAt.AsTime(), page.SnapshotAt.AsTime())
	seen := map[string]bool{}
	for _, value := range page.Flights {
		if value == nil || value.Cid == "" || value.Callsign == "" || seen[value.Callsign] || value.FlightPlan == nil {
			return cacheSnapshot{}, fmt.Errorf("invalid VATSIM flight")
		}
		seen[value.Callsign] = true
		plan := value.FlightPlan
		flight := Flight{CID: value.Cid, Callsign: value.Callsign, State: FlightState(value.State), Latitude: value.Latitude,
			Longitude: value.Longitude, Altitude: int(value.Altitude), Groundspeed: int(value.Groundspeed),
			FlightPlan: FlightPlan{FlightRules: plan.FlightRules, Aircraft: plan.Aircraft, AircraftFAA: plan.AircraftFaa,
				AircraftShort: plan.AircraftShort, Origin: plan.Origin, Destination: plan.Destination, Alternate: plan.Alternate,
				EOBT: plan.Eobt, EnrouteDuration: plan.EnrouteDuration, RequestedLevel: plan.RequestedLevel,
				Remarks: plan.Remarks, Route: plan.Route, AssignedSquawk: plan.AssignedSquawk, Revision: plan.Revision}}
		if value.LogonTime != nil {
			flight.LogonTime = value.LogonTime.AsTime()
		}
		if value.LastUpdated != nil {
			flight.LastUpdated = value.LastUpdated.AsTime()
		}
		snapshot.add(flight)
	}
	return preserveNewerFlightPlans(cacheSnapshot{flightsByCallsign: map[string]Flight{}}, snapshot), nil
}
