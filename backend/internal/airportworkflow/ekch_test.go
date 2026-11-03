package airportworkflow

import (
	"FlightStrips/internal/shared"
	"testing"
)

func TestEKCHProductionDecisions(t *testing.T) {
	b, err := Bind(definition(t), EKCHDescriptor())
	if err != nil {
		t.Fatal(err)
	}
	c := Context{Session: 42, Source: HumanSource, GroundOnline: true, Strip: Snapshot{Exists: true, Origin: "EKCH", Destination: "EGLL", Bay: shared.BAY_TAXI_LWR}}
	for _, kind := range []ObservationKind{FullFlight, GroundState} {
		d, err := b.Observe(c, Observation{Kind: kind, Origin: "EKCH", Destination: "EGLL", GroundState: "TAXI"})
		if err != nil || *d.Bay != shared.BAY_TAXI_LWR {
			t.Fatalf("specific taxi placement: %+v %v", d, err)
		}
	}
	c.Strip.Exists = false
	c.Strip.Bay = ""
	c.GroundOnline = false
	d, err := b.Observe(c, Observation{Kind: FullFlight, Origin: "EKCH", Destination: "EGLL", GroundState: "TAXI"})
	if err != nil || *d.Bay != shared.BAY_TAXI_LWR {
		t.Fatalf("solo tower: %+v %v", d, err)
	}
	c.Strip.Exists = true
	c.Strip.Bay = shared.BAY_DEPART
	d, err = b.Observe(c, Observation{Kind: Position, Latitude: 55.6181, Longitude: 12.656, Altitude: 2000})
	if err != nil || *d.Bay != shared.BAY_AIRBORNE {
		t.Fatalf("airborne: %+v %v", d, err)
	}
	c.Strip.Bay = shared.BAY_HIDDEN_DEP
	d, err = b.Observe(c, Observation{Kind: GroundState, GroundState: "TAXI"})
	if err != nil || *d.Bay != shared.BAY_HIDDEN_DEP {
		t.Fatalf("terminal departure: %+v %v", d, err)
	}
	l, err := b.Lifecycle(c, Lifecycle{Origin: "EKCH", Destination: "EGLL", Latitude: 55.6181, Longitude: 12.656, Altitude: 17})
	if err != nil || !l.Restart {
		t.Fatalf("refile: %+v %v", l, err)
	}
	c.Strip.EuroscopeSeen = true
	l, _ = b.Lifecycle(c, Lifecycle{Origin: "EKCH", Destination: "EGLL", Latitude: 55.6181, Longitude: 12.656, Altitude: 17})
	if l.Restart {
		t.Fatal("restarted a completed observed departure")
	}
	c.Strip.Bay = shared.BAY_TAXI
	coord, err := b.Coordinate(c, Coordination{TargetIsTower: true, SQPosition: "planner", DeliveryPosition: "delivery"})
	if err != nil || !coord.PromoteTaxi || coord.ClearanceOwner != "planner" {
		t.Fatalf("coordination: %+v %v", coord, err)
	}
	c.Strip.Bay = shared.BAY_NOT_CLEARED
	if _, err := b.Action(c, "strip.move", Move{Bay: shared.BAY_TAXI}); err == nil {
		t.Fatal("uncleared move allowed")
	}
	d, err = b.Action(c, "strip.move", Move{Bay: shared.BAY_CLEARED, Clearance: true})
	if err != nil || d.Cleared == nil || !*d.Cleared {
		t.Fatalf("clearance: %+v %v", d, err)
	}
}

func TestEKCHPreservesLegacyMoveAndNilStateSemantics(t *testing.T) {
	b, err := Bind(definition(t), EKCHDescriptor())
	if err != nil {
		t.Fatal(err)
	}
	c := Context{Session: 1, Source: HumanSource, GroundOnline: true, Strip: Snapshot{Exists: true, Origin: "EKCH", Destination: "EGLL", Bay: shared.BAY_PUSH}}
	d, err := b.Observe(c, Observation{Kind: FullFlight, Origin: "EKCH", Destination: "EGLL"})
	if err != nil || *d.Bay != shared.BAY_NOT_CLEARED {
		t.Fatalf("nil state must not preserve PUSH: %+v %v", d, err)
	}
	for _, bay := range []string{shared.BAY_UNKNOWN, shared.BAY_HIDDEN_DEP, shared.BAY_DEP_HIDDEN} {
		if _, err := b.Action(c, "strip.move", Move{Bay: bay}); err == nil {
			t.Fatalf("internal destination %s accepted", bay)
		}
	}
	c.Strip.Origin, c.Strip.Destination = "EGLL", "EKCH"
	d, err = b.Action(c, "strip.move", Move{Bay: shared.BAY_TWY_ARR})
	if err != nil || d.GroundState != nil {
		t.Fatalf("arrival taxi must preserve state: %+v %v", d, err)
	}
	d, err = b.Action(c, "strip.move", Move{Bay: shared.BAY_STAND})
	if err != nil || d.GroundState == nil || *d.GroundState != "PARK" {
		t.Fatalf("arrival parking: %+v %v", d, err)
	}
}
