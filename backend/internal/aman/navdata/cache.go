package navdata

import (
	"context"
	"fmt"
	"slices"

	"FlightStrips/internal/aman"
)

// Cache is an in-memory implementation of the runtime reader contract. It is
// intentionally populated by its caller, so a runtime read cannot trigger an
// import or a source request.
type Cache struct {
	active   map[AirportID]DatasetVersion
	routes   map[RouteKey]RouteGeometry
	terminal map[terminalPathKey]TerminalPath
}

type terminalPathKey struct {
	airport     AirportID
	feeder      FeederID
	runwayGroup aman.RunwayGroupID
}

func (c Cache) ActiveVersion(_ context.Context, airport AirportID) (DatasetVersion, error) {
	version, ok := c.active[airport]
	if !ok {
		return DatasetVersion{}, domainError(aman.ErrorNotFound, "cached active dataset was not found")
	}
	return version, nil
}
func (c Cache) Route(_ context.Context, key RouteKey) (RouteGeometry, error) {
	route, ok := c.routes[key]
	if !ok {
		return RouteGeometry{}, domainError(aman.ErrorNotFound, "cached route geometry was not found")
	}
	return cloneRoute(route), nil
}
func (c Cache) TerminalPath(_ context.Context, airport AirportID, feeder FeederID, runwayGroup aman.RunwayGroupID) (TerminalPath, error) {
	path, ok := c.terminal[terminalPathKey{airport, feeder, runwayGroup}]
	if !ok {
		return TerminalPath{}, domainError(aman.ErrorNotFound, "cached terminal path was not found")
	}
	return cloneTerminal(path), nil
}

func NewCache(active map[AirportID]DatasetVersion, routes map[RouteKey]RouteGeometry, paths []TerminalPath) (Cache, error) {
	activeCopy := make(map[AirportID]DatasetVersion, len(active))
	for airport, version := range active {
		if !validIdentifier(string(airport)) {
			return Cache{}, invalid("cached active airport is invalid")
		}
		if err := version.Validate(); err != nil {
			return Cache{}, err
		}
		activeCopy[airport] = version
	}
	routeCopy := make(map[RouteKey]RouteGeometry, len(routes))
	for key, route := range routes {
		if key == "" {
			return Cache{}, invalid("cached route key is required")
		}
		if err := route.Validate(); err != nil {
			return Cache{}, err
		}
		routeCopy[key] = cloneRoute(route)
	}
	terminal := make(map[terminalPathKey]TerminalPath, len(paths))
	for _, path := range paths {
		if err := path.Validate(); err != nil {
			return Cache{}, err
		}
		key := terminalPathKey{path.Airport, path.Feeder, path.RunwayGroup}
		if _, found := terminal[key]; found {
			return Cache{}, invalid("cached terminal path is duplicated")
		}
		terminal[key] = cloneTerminal(path)
	}
	return Cache{active: activeCopy, routes: routeCopy, terminal: terminal}, nil
}

func cloneRoute(value RouteGeometry) RouteGeometry {
	value.Legs = cloneLegs(value.Legs)
	value.HoldingIDs = slices.Clone(value.HoldingIDs)
	value.Unresolved = slices.Clone(value.Unresolved)
	return value
}
func cloneTerminal(value TerminalPath) TerminalPath {
	value.Legs = cloneLegs(value.Legs)
	value.HoldingIDs = slices.Clone(value.HoldingIDs)
	value.Unresolved = slices.Clone(value.Unresolved)
	return value
}
func cloneLegs(values []ProcedureLeg) []ProcedureLeg {
	result := make([]ProcedureLeg, len(values))
	for index, value := range values {
		result[index] = value
		result[index].FromFix = clonePtr(value.FromFix)
		result[index].ToFix = clonePtr(value.ToFix)
		result[index].CourseTrueDeg = clonePtr(value.CourseTrueDeg)
		result[index].DistanceNM = clonePtr(value.DistanceNM)
		result[index].HoldingID = clonePtr(value.HoldingID)
	}
	return result
}
func clonePtr[T any](value *T) *T {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func domainError(class aman.ErrorClass, message string) error {
	return &aman.DomainError{Class: class, Message: fmt.Sprintf("navigation cache: %s", message)}
}
