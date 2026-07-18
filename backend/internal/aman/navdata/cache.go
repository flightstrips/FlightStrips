package navdata

import (
	"context"
	"fmt"

	"FlightStrips/internal/aman"
)

// Cache is an in-memory implementation of the runtime reader contract. It is
// intentionally populated by its caller, so a runtime read cannot trigger an
// import or a source request.
type Cache struct {
	Active   map[AirportID]DatasetVersion
	Routes   map[RouteKey]RouteGeometry
	Terminal map[terminalPathKey]TerminalPath
}

type terminalPathKey struct {
	airport     AirportID
	feeder      FeederID
	runwayGroup aman.RunwayGroupID
}

func (c Cache) ActiveVersion(_ context.Context, airport AirportID) (DatasetVersion, error) {
	version, ok := c.Active[airport]
	if !ok {
		return DatasetVersion{}, domainError(aman.ErrorNotFound, "cached active dataset was not found")
	}
	return version, nil
}
func (c Cache) Route(_ context.Context, key RouteKey) (RouteGeometry, error) {
	route, ok := c.Routes[key]
	if !ok {
		return RouteGeometry{}, domainError(aman.ErrorNotFound, "cached route geometry was not found")
	}
	return route, nil
}
func (c Cache) TerminalPath(_ context.Context, airport AirportID, feeder FeederID, runwayGroup aman.RunwayGroupID) (TerminalPath, error) {
	path, ok := c.Terminal[terminalPathKey{airport, feeder, runwayGroup}]
	if !ok {
		return TerminalPath{}, domainError(aman.ErrorNotFound, "cached terminal path was not found")
	}
	return path, nil
}

func NewCache(active map[AirportID]DatasetVersion, routes map[RouteKey]RouteGeometry, paths []TerminalPath) Cache {
	terminal := make(map[terminalPathKey]TerminalPath, len(paths))
	for _, path := range paths {
		terminal[terminalPathKey{path.Airport, path.Feeder, path.RunwayGroup}] = path
	}
	return Cache{Active: active, Routes: routes, Terminal: terminal}
}

func domainError(class aman.ErrorClass, message string) error {
	return &aman.DomainError{Class: class, Message: fmt.Sprintf("navigation cache: %s", message)}
}
