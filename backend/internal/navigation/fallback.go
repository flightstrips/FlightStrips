package navigation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"FlightStrips/internal/aman/materializer"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/navdata/sector"
	"FlightStrips/internal/aman/terminal"
)

func sectorMaterializer(local *sector.Adapter, config terminal.Configuration, cache materializerCache, now func() time.Time) (*materializer.Materializer, error) {
	return materializer.New(materializer.Dependencies{Cycles: local, Airports: local, Runways: config, Procedures: local, Fixes: local, Routes: local, Cache: cache, Terminal: config, Now: now})
}

type fallbackRoutes struct {
	primary                navdata.RouteResolver
	local                  *sector.Adapter
	references             navdata.GeometrySnapshotReader
	primaryRequiresCatalog bool
	checkFiledRoute        bool
	catalogUnavailable     atomic.Bool
}

func (r *fallbackRoutes) Resolve(ctx context.Context, q navdata.RouteQuery) (navdata.RouteGeometry, error) {
	if err := ctx.Err(); err != nil {
		return navdata.RouteGeometry{}, err
	}
	if err := q.Validate(); err != nil {
		return navdata.RouteGeometry{}, err
	}
	// AIRAC.NET cannot parse a dataset imported from a local file. Its catalog
	// refresh will restore the primary manifest after the provider recovers.
	version, localErr := r.local.LatestVersion(ctx)
	var primaryErr error
	var localGeometry *navdata.RouteGeometry
	if !r.primaryRequiresCatalog || !r.catalogUnavailable.Load() && (localErr != nil || !q.Version.Equal(version)) {
		geometry, err := r.primary.Resolve(ctx, q)
		if err == nil {
			if !r.checkFiledRoute {
				return geometry, nil
			}
			// A successful bulk response is not proof that vIFF has processed a
			// controller's current ITEM15. Compare locally resolved geometry on
			// every refresh, so a stale response cannot replace the amendment.
			current, resolveErr := r.local.Resolve(ctx, q)
			if resolveErr != nil {
				if filedWaypointsMatch(q, geometry, r.local.HasFix) {
					return geometry, nil
				}
				return navdata.RouteGeometry{}, errors.Join(fmt.Errorf("vIFF route does not match current filed waypoints"), resolveErr)
			}
			if routeMatchesFiledGeometry(geometry, current) {
				return geometry, nil
			}
			localGeometry = &current
			err = fmt.Errorf("vIFF route geometry does not match current filed route")
		}
		primaryErr = err
	}
	if err := ctx.Err(); err != nil {
		return navdata.RouteGeometry{}, err
	}
	var geometry navdata.RouteGeometry
	if localGeometry != nil {
		geometry = *localGeometry
	} else {
		var err error
		geometry, err = r.local.Resolve(ctx, q)
		if err != nil {
			return navdata.RouteGeometry{}, errors.Join(primaryErr, err)
		}
	}
	// A same-cycle sector file can supplement an online manifest, but it must
	// join at the cached terminal's actual coordinates, not just the same name.
	snapshot, err := r.references.ActiveGeometrySnapshot(ctx, q.Destination)
	if err != nil {
		return navdata.RouteGeometry{}, err
	}
	if !snapshot.Manifest.Version.Equal(q.Version) {
		return navdata.RouteGeometry{}, fmt.Errorf("navigation manifest changed during sector fallback")
	}
	last := geometry.Legs[len(geometry.Legs)-1]
	for _, fix := range snapshot.Fixes {
		if fix.ID == *q.TerminalEntry && navdata.CoordinateDistanceNM(fix.Position, *last.ToPosition) < 0.05 {
			slog.WarnContext(ctx, "Navigation route uses sector fallback", "callsign", q.Callsign, "airport", q.Destination, "primary_error", primaryErr)
			return geometry, nil
		}
	}
	return navdata.RouteGeometry{}, fmt.Errorf("sector fallback join does not match cached terminal coordinates")
}

type fallbackImporter struct {
	primary, local, retained *materializer.Materializer
	routes                   *fallbackRoutes
	mu                       sync.RWMutex
	active                   *materializer.Materializer
}

func (f *fallbackImporter) Refresh(ctx context.Context, request materializer.Request) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	err := f.primary.Refresh(ctx, request)
	if f.routes.primaryRequiresCatalog && ctx.Err() == nil {
		f.routes.catalogUnavailable.Store(err != nil)
	}
	if err == nil {
		f.active = f.primary
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// Preserve a usable current manifest and its route keys across an outage.
	current, currentErr := f.retained.GeometryReader().ActiveVersion(ctx, request.Airport)
	local, localErr := f.routes.local.LatestVersion(ctx)
	fileChanged := currentErr == nil && localErr == nil && strings.HasPrefix(current.SourceRevision, "sector-") && !current.Equal(local)
	if !fileChanged {
		if retainedErr := f.retained.Refresh(ctx, request); retainedErr == nil {
			f.active = f.retained
			slog.WarnContext(ctx, "Navigation provider unavailable; retaining terminal cache with sector route fallback", "airport", request.Airport, "error", err)
			return nil
		}
	}
	// A cold database can be bootstrapped entirely from SCT + terminal config.
	if fallbackErr := f.local.Refresh(ctx, request); fallbackErr != nil {
		return errors.Join(err, fallbackErr)
	}
	f.active = f.local
	slog.WarnContext(ctx, "Navigation provider unavailable; activated sector fallback", "airport", request.Airport, "error", err)
	return nil
}

func (f *fallbackImporter) MaterializeRoute(ctx context.Context, q navdata.RouteQuery, resolver string) (navdata.RouteKey, error) {
	// The primary materializer's resolver already tries local geometry. Keep
	// cache writes outside provider fallback so storage failures are not retried.
	return f.primary.MaterializeRoute(ctx, q, resolver+"+"+sector.ResolverVersion)
}
func (f *fallbackImporter) Health(airport navdata.AirportID) materializer.Health {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.active != nil {
		return f.active.Health(airport)
	}
	return f.primary.Health(airport)
}
