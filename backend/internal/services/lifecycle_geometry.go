package services

import (
	"context"
	"math"
	"strings"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/sat"
)

type lifecycleGeometryContextKey struct{}

// This cache lives only within one sequential reconciliation. The registry is
// read-only; its identity and exact coordinates completely determine geometry.
// Store only the name, so mutable planning never borrows registry data.
type lifecycleGeometry struct {
	registry *sat.StandCapabilityRegistry
	airport  string
	entries  map[string]lifecycleGeometryEntry
	current  map[string]bool
	lookup   func(*sat.StandCapabilityRegistry, string, float64, float64) (string, bool)
}

type lifecycleGeometryEntry struct {
	latitude, longitude uint64
	stand               string
	found               bool
}

func (g *lifecycleGeometry) prepare(registry *sat.StandCapabilityRegistry, airport string, positions []cluster.KVPosition) {
	airport = strings.ToUpper(strings.TrimSpace(airport))
	if g.registry != registry || g.airport != airport {
		g.entries = nil
	}
	g.registry, g.airport = registry, airport
	if g.entries == nil {
		g.entries = make(map[string]lifecycleGeometryEntry)
	}
	if g.current == nil {
		g.current = make(map[string]bool, len(positions))
	} else {
		clear(g.current)
	}
	for _, position := range positions {
		if position.Value != nil {
			g.current[position.Value.AircraftKey] = true
		}
	}
	for aircraft := range g.entries {
		if !g.current[aircraft] {
			delete(g.entries, aircraft)
		}
	}
}

func (g *lifecycleGeometry) standAt(aircraft string, latitude, longitude float64) (string, bool) {
	lat, lon := math.Float64bits(latitude), math.Float64bits(longitude)
	if entry, ok := g.entries[aircraft]; ok && entry.latitude == lat && entry.longitude == lon {
		return entry.stand, entry.found
	}
	lookup := g.lookup
	if lookup == nil {
		lookup = physicalStandAt
	}
	stand, found := lookup(g.registry, g.airport, latitude, longitude)
	g.entries[aircraft] = lifecycleGeometryEntry{latitude: lat, longitude: lon, stand: stand, found: found}
	return stand, found
}

func physicalStandAt(registry *sat.StandCapabilityRegistry, airport string, latitude, longitude float64) (string, bool) {
	stand, found := registry.StandAtPosition(airport, latitude, longitude)
	return stand.Name, found
}

func lifecycleGeometryFrom(ctx context.Context) *lifecycleGeometry {
	geometry, _ := ctx.Value(lifecycleGeometryContextKey{}).(*lifecycleGeometry)
	return geometry
}
