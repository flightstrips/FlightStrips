package navdata

import (
	"context"

	"FlightStrips/internal/aman"
)

// Acquisition interfaces are deliberately small. Application composition picks
// the sources a materializer needs; implementations do not negotiate features.
type CycleSource interface {
	LatestVersion(context.Context) (DatasetVersion, error)
}
type AirportSource interface {
	Airport(context.Context, DatasetVersion, AirportID) (Airport, error)
}
type ProcedureSource interface {
	Procedures(context.Context, ProcedureQuery) (ProcedureSet, error)
}
type FixSource interface {
	Fixes(context.Context, FixQuery) (FixSet, error)
}
type RouteResolver interface {
	Resolve(context.Context, RouteQuery) (RouteGeometry, error)
}

// GeometryReader is the cache-only runtime boundary. It must not import data,
// call an acquisition source, or report a source outage for usable cached data.
type GeometryReader interface {
	ActiveVersion(context.Context, AirportID) (DatasetVersion, error)
	Route(context.Context, RouteKey) (RouteGeometry, error)
	TerminalPath(context.Context, AirportID, FeederID, aman.RunwayGroupID) (TerminalPath, error)
}

// Stable provider-independent navigation error aliases.
const (
	ErrorInvalidRequest       = aman.ErrorInvalidArgument
	ErrorNotFound             = aman.ErrorNotFound
	ErrorIncompleteGeometry   = aman.ErrorDegradedOrIncompleteGeometry
	ErrorUnsupportedLeg       = aman.ErrorUnsupportedLeg
	ErrorDatasetMismatch      = aman.ErrorDatasetMismatch
	ErrorSourceUnavailable    = aman.ErrorDependencyUnavailable
	ErrorCorruptCanonicalData = aman.ErrorCorruptData
)
