package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/vatsim"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func lifecyclePolicyFixture(t *testing.T) (*sat.StandCapabilityRegistry, *sat.AirlineAssignmentConfig) {
	t.Helper()
	stands, err := sat.LoadStandCapabilities(strings.NewReader("STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30\nSTAND:EKCH:A2:N055.38.42.710:E012.38.33.450:30\nSTAND:EKCH:B1:N055.39.42.710:E012.38.33.450:30\n"))
	if err != nil {
		t.Fatal(err)
	}
	names := []string{sat.FallbackAirlinerDefault, sat.FallbackBusinessVIP, sat.FallbackCargo, sat.FallbackMilitary, sat.FallbackMilitaryHelicopter, sat.FallbackHelicopter, sat.FallbackGAPrivate, sat.FallbackUnknown}
	fallbacks := []string{}
	for _, name := range names {
		fallbacks = append(fallbacks, fmt.Sprintf(`%q:{"stands":{"tier1":{"A1":100},"tier2":{"A2":100},"tier3":{"B1":100}}}`, name))
	}
	policy, err := sat.LoadAirlineAssignment(strings.NewReader(`{"rules":[],"stand_groups":{},"fallback_rules":{`+strings.Join(fallbacks, ",")+`}}`), stands)
	if err != nil {
		t.Fatal(err)
	}
	return stands, policy
}

func TestLifecyclePlanUsesDeparturePolicyAndPersistedEpisodes(t *testing.T) {
	ctx := context.Background()
	stands, policy := lifecyclePolicyFixture(t)
	now := time.Date(2026, 9, 30, 23, 55, 0, 0, time.UTC)
	strip := &models.Strip{Callsign: "SAS123", Session: 1, Origin: "EKCH", Destination: "EDDF", AircraftType: stringPointer("A320"), Bay: "DEP_HIDDEN", CdmData: &models.CdmData{}}
	p := LifecyclePlan{Session: &models.Session{ID: 1, Airport: "EKCH", Name: "LIVE"}, Strips: map[string]*models.Strip{"SAS123": strip}, Assignments: map[string]*models.StandAssignment{}, Blocks: map[string]*models.StandBlock{}, Stands: stands, Policy: policy, Now: now}
	f := &vatsim.DepartureFlightInfo{Callsign: "SAS123", CID: "12345", Revision: 1, Origin: "EKCH", Destination: "EDDF", AircraftType: "A320"}
	flights := map[string]*vatsim.DepartureFlightInfo{"SAS123": f}
	if err := p.Run(ctx, true, flights); err != nil {
		t.Fatal(err)
	}
	if len(p.Assignments) != 0 {
		t.Fatal("prefiles enabled by default")
	}
	p.AllowPrefiles = true
	if err := p.Run(ctx, true, flights); err != nil {
		t.Fatal(err)
	}
	a := p.Assignments[strip.Callsign]
	if a.Stage != StageReserved || !a.ExpiresAt.Equal(now.Add(15*time.Minute)) {
		t.Fatalf("reservation: %+v", a)
	}
	p.Now = now.Add(time.Minute)
	if err := p.Run(ctx, true, flights); err != nil {
		t.Fatal(err)
	}
	if !p.Assignments[strip.Callsign].ExpiresAt.Equal(now.Add(15 * time.Minute)) {
		t.Fatal("identical plan renewed hold")
	}
	stand, _ := stands.Lookup("EKCH", a.Stand)
	strip.EuroscopeSeenAt = &now
	strip.PositionLatitude = &stand.Latitude
	strip.PositionLongitude = &stand.Longitude
	strip.CdmData.Tobt = stringPointer("0010")
	f.Online = true
	f.Latitude, f.Longitude = stand.Latitude, stand.Longitude
	if err := p.Run(ctx, true, flights); err != nil {
		t.Fatal(err)
	}
	a = p.Assignments[strip.Callsign]
	if a.Stage != StageDepartureBlock || a.ExpiresAt != nil || a.ProjectedReleaseAt == nil || !a.ProjectedReleaseAt.Equal(time.Date(2026, 10, 1, 0, 20, 0, 0, time.UTC)) {
		t.Fatalf("observed rollover block: %+v", a)
	}
	p.Now = now.Add(time.Hour)
	if err := p.Run(ctx, true, flights); err != nil {
		t.Fatal(err)
	}
	if p.Assignments[strip.Callsign] == nil {
		t.Fatal("expired projection released physical aircraft")
	}
}

func TestLifecyclePlanPhysicalDisplacementRetainsAdvisory(t *testing.T) {
	stands, policy := lifecyclePolicyFixture(t)
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	p := LifecyclePlan{Session: &models.Session{ID: 1, Airport: "EKCH", Name: "LIVE"}, Strips: map[string]*models.Strip{}, Assignments: map[string]*models.StandAssignment{}, Blocks: map[string]*models.StandBlock{}, Stands: stands, Policy: policy, Now: now}
	flights := map[string]*vatsim.DepartureFlightInfo{}
	for _, key := range []string{"ARR1", "ARR2", "ARR3"} {
		p.Strips[key] = &models.Strip{Callsign: key, Session: 1, Origin: "EDDF", Destination: "EKCH", AircraftType: stringPointer("A320"), ArrivalETA: &models.ArrivalETA{Time: now.Add(20 * time.Minute), Source: "AMAN"}, CdmData: &models.CdmData{}}
		flights[key] = &vatsim.DepartureFlightInfo{Callsign: key, CID: "12345", Revision: 1, Online: true, Origin: "EDDF", Destination: "EKCH", AircraftType: "A320"}
	}
	if err := p.Run(context.Background(), false, flights); err != nil {
		t.Fatal(err)
	}
	stand, _ := stands.Lookup("EKCH", "A1")
	p.Strips["DEP"] = &models.Strip{Callsign: "DEP", Session: 1, Origin: "EKCH", Destination: "EDDF", AircraftType: stringPointer("A320"), EuroscopeSeenAt: &now, PositionLatitude: &stand.Latitude, PositionLongitude: &stand.Longitude, CdmData: &models.CdmData{}}
	flights["DEP"] = &vatsim.DepartureFlightInfo{Callsign: "DEP", CID: "54321", Revision: 1, Online: true, Origin: "EKCH", Destination: "EDDF", AircraftType: "A320", Latitude: stand.Latitude, Longitude: stand.Longitude}
	if err := p.Run(context.Background(), true, flights); err != nil {
		t.Fatal(err)
	}
	if a := p.Assignments["DEP"]; a == nil || a.Stand != "A1" || a.Stage != StageDepartureBlock {
		t.Fatal("physical departure did not claim the stand")
	}
	if a := p.Assignments["ARR1"]; a == nil || a.Stand != "" || a.ConflictReason == nil {
		t.Fatalf("displaced arrival lost advisory state: %+v", a)
	}
}

func TestLifecyclePlanUsesCommittedBlockAdjacency(t *testing.T) {
	stands, policy := lifecyclePolicyFixture(t)
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	strip := &models.Strip{Callsign: "SAS123", Session: 1, Origin: "EKCH", Destination: "EDDF", AircraftType: stringPointer("A320"), CdmData: &models.CdmData{}}
	p := LifecyclePlan{Session: &models.Session{ID: 1, Airport: "EKCH", Name: "LIVE"}, Strips: map[string]*models.Strip{"SAS123": strip}, Assignments: map[string]*models.StandAssignment{}, Blocks: map[string]*models.StandBlock{"B1": {Stand: "B1", Manual: true}}, BlockAdjacency: map[string][]string{"B1": {"A1"}}, Stands: stands, Policy: policy, Now: now, AllowPrefiles: true}
	flights := map[string]*vatsim.DepartureFlightInfo{"SAS123": {Callsign: "SAS123", CID: "12345", Revision: 1, Origin: "EKCH", Destination: "EDDF", AircraftType: "A320"}}
	if err := p.Run(context.Background(), true, flights); err != nil {
		t.Fatal(err)
	}
	if a := p.Assignments["SAS123"]; a == nil || a.Stand != "A2" {
		t.Fatalf("selection ignored committed block adjacency: %+v", a)
	}
}

func TestLifecyclePlanArrivalStagesProtectionCancellationAndRetention(t *testing.T) {
	ctx := context.Background()
	stands, policy := lifecyclePolicyFixture(t)
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	eta := models.ArrivalETA{Time: now.Add(30 * time.Minute), Source: "AMAN"}
	strip := &models.Strip{Callsign: "SAS123", Session: 1, Origin: "EDDF", Destination: "EKCH", AircraftType: stringPointer("A320"), Bay: "ARR_HIDDEN", CdmData: &models.CdmData{}, ArrivalETA: &eta, VatsimSeenAt: &now}
	p := LifecyclePlan{Session: &models.Session{ID: 1, Airport: "EKCH", Name: "LIVE"}, Strips: map[string]*models.Strip{"SAS123": strip}, Assignments: map[string]*models.StandAssignment{}, Blocks: map[string]*models.StandBlock{}, Stands: stands, Policy: policy, Now: now}
	f := &vatsim.DepartureFlightInfo{Callsign: "SAS123", CID: "12345", Revision: 1, Online: true, Origin: "EDDF", Destination: "EKCH", AircraftType: "A320"}
	flights := map[string]*vatsim.DepartureFlightInfo{"SAS123": f}
	if err := p.Run(ctx, false, flights); err != nil {
		t.Fatal(err)
	}
	if p.Assignments[strip.Callsign].Stage != StageEstimated {
		t.Fatal("estimated missing")
	}
	eta.Time = now.Add(8 * time.Minute)
	if err := p.Run(ctx, false, flights); err != nil {
		t.Fatal(err)
	}
	if p.Assignments[strip.Callsign].Stage != StageAssigned {
		t.Fatal("assigned missing")
	}
	eta.Time = now.Add(time.Minute)
	if err := p.Run(ctx, false, flights); err != nil {
		t.Fatal(err)
	}
	original := p.Assignments[strip.Callsign].Stand
	if p.Assignments[strip.Callsign].Stage != StageConfirmed {
		t.Fatal("confirmed missing")
	}
	p.Blocks[original] = &models.StandBlock{Stand: original, Manual: true}
	eta.Time = now.Add(20 * time.Minute)
	if err := p.Run(ctx, false, flights); err != nil {
		t.Fatal(err)
	}
	if p.Assignments[strip.Callsign].Stand != original {
		t.Fatal("confirmed stand moved")
	}
	delete(p.Blocks, original)
	if err := p.Run(ctx, false, map[string]*vatsim.DepartureFlightInfo{}); err != nil {
		t.Fatal(err)
	}
	if p.Assignments[strip.Callsign] == nil {
		t.Fatal("arrival dropout skipped grace")
	}
	p.Now = now.Add(6 * time.Minute)
	if err := p.Run(ctx, false, map[string]*vatsim.DepartureFlightInfo{}); err != nil {
		t.Fatal(err)
	}
	if p.Assignments[strip.Callsign] != nil {
		t.Fatal("disappeared arrival retained beyond grace")
	}
	p.Now = now
	stand, _ := stands.Lookup("EKCH", "A1")
	strip.PositionLatitude = &stand.Latitude
	strip.PositionLongitude = &stand.Longitude
	alt := int32(0)
	strip.PositionAltitude = &alt
	strip.State = stringPointer("PARK")
	strip.CdmData.Aldt = stringPointer("1150")
	if err := p.Run(ctx, false, flights); err != nil {
		t.Fatal(err)
	}
	a := p.Assignments[strip.Callsign]
	if a == nil || a.ObservedStand == nil || !a.ExpiresAt.Equal(now.Add(20*time.Minute)) {
		t.Fatalf("landed retention: %+v", a)
	}
	p.Now = now.Add(21 * time.Minute)
	if err := p.Run(ctx, false, flights); err != nil {
		t.Fatal(err)
	}
	if p.Assignments[strip.Callsign] != nil {
		t.Fatal("arrival recreated after ALDT retention")
	}
}
