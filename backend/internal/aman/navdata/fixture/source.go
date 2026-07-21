// Package fixture supplies a local, deterministic source/resolver for contract
// tests. It models no provider transport and is also a replacement-source proof.
package fixture

import (
	"context"
	"slices"
	"sync/atomic"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
)

type Dataset struct {
	Version        navdata.DatasetVersion
	Provenance     navdata.Provenance
	Airports       map[navdata.AirportID]navdata.Airport
	Fixes          map[navdata.FixID]navdata.Fix
	Procedures     []navdata.Procedure
	Routes         map[navdata.RouteKey]navdata.RouteGeometry
	HoldingDigests map[navdata.HoldingID]string
	TerminalPaths  []navdata.TerminalPath
}

type Source struct {
	dataset Dataset
	calls   atomic.Int64
}

func New(data Dataset) *Source { return &Source{dataset: data} }
func (s *Source) Calls() int   { return int(s.calls.Load()) }
func (s *Source) LatestVersion(context.Context) (navdata.DatasetVersion, error) {
	s.calls.Add(1)
	return s.dataset.Version, nil
}
func (s *Source) Airport(_ context.Context, version navdata.DatasetVersion, airport navdata.AirportID) (navdata.Airport, error) {
	s.calls.Add(1)
	if err := matchVersion(s.dataset.Version, version); err != nil {
		return navdata.Airport{}, err
	}
	value, ok := s.dataset.Airports[airport]
	if !ok {
		return navdata.Airport{}, unavailable(aman.ErrorNotFound, "airport was not found")
	}
	return value, nil
}
func (s *Source) Runways(_ context.Context, version navdata.DatasetVersion, airport navdata.AirportID) ([]navdata.Runway, error) {
	s.calls.Add(1)
	if err := matchVersion(s.dataset.Version, version); err != nil {
		return nil, err
	}
	if _, ok := s.dataset.Airports[airport]; !ok {
		return nil, unavailable(aman.ErrorNotFound, "airport was not found")
	}
	return []navdata.Runway{{ID: "22L", Airport: airport, Threshold: navdata.Threshold{Position: navdata.Coordinate{LatitudeDeg: 55.6254111111, LongitudeDeg: 12.6675805556}, CourseTrueDeg: ptr(221.2)}, LengthNM: 3302.0 / 1852.0, Provenance: s.dataset.Provenance}}, nil
}
func (s *Source) Procedures(_ context.Context, query navdata.ProcedureQuery) (navdata.ProcedureSet, error) {
	s.calls.Add(1)
	if err := query.Validate(); err != nil {
		return navdata.ProcedureSet{}, err
	}
	if err := matchVersion(s.dataset.Version, query.Version); err != nil {
		return navdata.ProcedureSet{}, err
	}
	procedures := make([]navdata.Procedure, 0)
	coverage := navdata.CoverageComplete
	for _, procedure := range s.dataset.Procedures {
		if procedure.Airport == query.Airport && matchesProcedure(procedure, query) {
			procedures = append(procedures, procedure)
			for _, leg := range procedure.Legs {
				if !leg.PathTerminator.Supported() {
					coverage = navdata.CoveragePartial
				}
			}
		}
	}
	if len(procedures) == 0 {
		coverage = navdata.CoveragePartial
	}
	return navdata.ProcedureSet{Version: s.dataset.Version, Airport: query.Airport, Procedures: procedures, Coverage: coverage, Provenance: s.dataset.Provenance}, nil
}
func (s *Source) Fixes(_ context.Context, query navdata.FixQuery) (navdata.FixSet, error) {
	s.calls.Add(1)
	if err := query.Validate(); err != nil {
		return navdata.FixSet{}, err
	}
	if err := matchVersion(s.dataset.Version, query.Version); err != nil {
		return navdata.FixSet{}, err
	}
	fixes := make([]navdata.Fix, 0, len(query.Identifiers))
	for _, id := range query.Identifiers {
		if fix, ok := s.dataset.Fixes[id]; ok {
			fixes = append(fixes, fix)
		}
	}
	coverage := navdata.CoverageComplete
	if len(fixes) != len(query.Identifiers) {
		coverage = navdata.CoveragePartial
	}
	return navdata.FixSet{Version: s.dataset.Version, Fixes: fixes, Coverage: coverage, Provenance: s.dataset.Provenance}, nil
}
func (s *Source) Resolve(_ context.Context, query navdata.RouteQuery) (navdata.RouteGeometry, error) {
	s.calls.Add(1)
	key, err := query.Key()
	if err != nil {
		return navdata.RouteGeometry{}, err
	}
	if err := matchVersion(s.dataset.Version, query.Version); err != nil {
		return navdata.RouteGeometry{}, err
	}
	route, ok := s.dataset.Routes[key]
	if !ok {
		return navdata.RouteGeometry{}, unavailable(aman.ErrorNotFound, "route was not found")
	}
	return route, nil
}
func (s *Source) Cache() (navdata.Cache, error) {
	return navdata.NewCache(map[navdata.AirportID]navdata.DatasetVersion{"EKCH": s.dataset.Version}, s.dataset.Routes, s.dataset.TerminalPaths)
}

func matchesProcedure(procedure navdata.Procedure, query navdata.ProcedureQuery) bool {
	if len(query.Kinds) > 0 && !slices.Contains(query.Kinds, procedure.Kind) {
		return false
	}
	if len(query.Identifiers) > 0 && !slices.Contains(query.Identifiers, procedure.ID) {
		return false
	}
	if len(query.Runways) == 0 {
		return true
	}
	for _, runway := range procedure.Runways {
		if slices.Contains(query.Runways, runway) {
			return true
		}
	}
	return false
}
func matchVersion(expected, actual navdata.DatasetVersion) error {
	if !expected.Equal(actual) {
		return unavailable(aman.ErrorDatasetMismatch, "dataset version does not match fixture")
	}
	return nil
}
func unavailable(class aman.ErrorClass, message string) error {
	return &aman.DomainError{Class: class, Message: message}
}

var _ navdata.CycleSource = (*Source)(nil)
var _ navdata.AirportSource = (*Source)(nil)
var _ navdata.RunwaySource = (*Source)(nil)
var _ navdata.ProcedureSource = (*Source)(nil)
var _ navdata.FixSource = (*Source)(nil)
var _ navdata.RouteResolver = (*Source)(nil)
var _ navdata.GeometryReader = (navdata.Cache{})

func ptr[T any](value T) *T { return &value }
func EKCH() Dataset {
	from := timeUTC(2026, 7, 16)
	until := timeUTC(2026, 8, 13)
	imported := timeUTC(2026, 7, 17)
	version := navdata.DatasetVersion{Cycle: "2608", SourceRevision: "fixture-r1", EffectiveFrom: from, EffectiveUntil: until}
	provenance := navdata.Provenance{SourceID: "fixture", SourceRevision: "fixture-r1", ImportedAt: imported, EffectiveFrom: from, EffectiveUntil: until}
	kemax, sok, rosbi := navdata.FixID("KEMAX"), navdata.FixID("SOK"), navdata.FixID("ROSBI")
	ha, hf, hm := navdata.HoldingID("KEMAX-HA"), navdata.HoldingID("SOK-HF"), navdata.HoldingID("ROSBI-HM")
	course := 220.0
	distance := 5.0
	holdTime := int64(60)
	hold := func(id navdata.HoldingID, fix navdata.FixID, termination navdata.HoldingTermination) navdata.HoldingPattern {
		return navdata.HoldingPattern{ID: id, Fix: fix, InboundCourseTrueDeg: course, TurnDirection: navdata.TurnRight, LegTimeSeconds: &holdTime, Termination: termination, Provenance: provenance}
	}
	sid := navdata.Procedure{ID: "KEMAX3A", Airport: "EKCH", Kind: navdata.ProcedureSID, Runways: []navdata.RunwayID{"22L"}, Provenance: provenance, Holdings: []navdata.HoldingPattern{hold(ha, kemax, navdata.HoldingToAltitude)}, Legs: []navdata.ProcedureLeg{{ID: "SID-1", PathTerminator: navdata.PathTF, ToFix: &kemax, CourseTrueDeg: &course, DistanceNM: &distance}, {ID: "SID-HOLD", PathTerminator: navdata.PathHA, ToFix: &kemax, HoldingID: &ha}}}
	star := navdata.Procedure{ID: "SOK1P", Airport: "EKCH", Kind: navdata.ProcedureSTAR, Runways: []navdata.RunwayID{"22L"}, Provenance: provenance, Holdings: []navdata.HoldingPattern{hold(hf, sok, navdata.HoldingToFix)}, Legs: []navdata.ProcedureLeg{{ID: "STAR-HOLD", PathTerminator: navdata.PathHF, ToFix: &sok, HoldingID: &hf}}}
	approach := navdata.Procedure{ID: "ILS22L", Airport: "EKCH", Kind: navdata.ProcedureApproach, Runways: []navdata.RunwayID{"22L"}, Provenance: provenance, Holdings: []navdata.HoldingPattern{hold(hm, rosbi, navdata.HoldingManual)}, Legs: []navdata.ProcedureLeg{{ID: "APP-HOLD", PathTerminator: navdata.PathHM, ToFix: &rosbi, HoldingID: &hm}, {ID: "VECTOR", PathTerminator: navdata.PathUnsupported}}}
	query := navdata.RouteQuery{Version: version, Origin: "ENGM", Destination: "EKCH", FiledRoute: "DCT KEMAX", ArrivalProcedure: ptr(navdata.ProcedureID("SOK1P")), Runway: ptr(navdata.RunwayID("22L")), RunwayGroup: ptr(aman.RunwayGroupID("SOUTH"))}
	routeLegs := []navdata.ProcedureLeg{{ID: "DCT-KEMAX", PathTerminator: navdata.PathDF, ToFix: &kemax, DistanceNM: &distance}, {ID: "VECTOR", PathTerminator: navdata.PathUnsupported}}
	route := navdata.RouteGeometry{Version: version, Legs: routeLegs, HoldingIDs: []navdata.HoldingID{hf}, TotalDistanceNM: distance, Coverage: navdata.CoveragePartial, Unresolved: []string{"vector"}, Provenance: provenance}
	digest, _ := navdata.RouteGeometryDigest(query, route)
	route.Digest = digest
	key, _ := query.Key()
	haDigest, _ := navdata.HoldingDigest(sid.Holdings[0])
	hfDigest, _ := navdata.HoldingDigest(star.Holdings[0])
	hmDigest, _ := navdata.HoldingDigest(approach.Holdings[0])
	return Dataset{Version: version, Provenance: provenance, Airports: map[navdata.AirportID]navdata.Airport{"EKCH": {ID: "EKCH", Name: "Copenhagen", Position: navdata.Coordinate{LatitudeDeg: 55.618, LongitudeDeg: 12.656}, Provenance: provenance}}, Fixes: map[navdata.FixID]navdata.Fix{kemax: {ID: kemax, Position: navdata.Coordinate{LatitudeDeg: 55.8, LongitudeDeg: 12.4}, Provenance: provenance}, sok: {ID: sok, Position: navdata.Coordinate{LatitudeDeg: 55.7, LongitudeDeg: 12.2}, Provenance: provenance}, rosbi: {ID: rosbi, Position: navdata.Coordinate{LatitudeDeg: 55.6, LongitudeDeg: 12.7}, Provenance: provenance}}, Procedures: []navdata.Procedure{sid, star, approach}, Routes: map[navdata.RouteKey]navdata.RouteGeometry{key: route}, HoldingDigests: map[navdata.HoldingID]string{ha: haDigest, hf: hfDigest, hm: hmDigest}, TerminalPaths: []navdata.TerminalPath{{Version: version, Airport: "EKCH", Feeder: "SOK", RunwayGroup: "SOUTH", Legs: star.Legs, HoldingIDs: []navdata.HoldingID{hf}, Coverage: navdata.CoverageComplete, Provenance: provenance, Digest: "fixture-terminal-sok"}}}
}

func timeUTC(year, month, day int) time.Time {
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}
