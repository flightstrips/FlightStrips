package materializer

import (
	"context"
	"errors"
	"testing"
	"time"

	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/navdata/fixture"
	"github.com/stretchr/testify/require"
)

type cachedTerminalReader struct {
	snapshot navdata.ActiveGeometrySnapshot
	err      error
}

func (r cachedTerminalReader) ActiveGeometrySnapshot(context.Context, navdata.AirportID) (navdata.ActiveGeometrySnapshot, error) {
	return r.snapshot, r.err
}

func TestCachedTerminalRefreshNeedsNoCatalogSources(t *testing.T) {
	data := fixture.EKCH()
	config := configFor(data)
	now := data.Provenance.ImportedAt.Add(time.Hour)
	cache := &memoryCache{}
	seed := newMaterializer(t, fixture.New(data), cache, config, &now)
	require.NoError(t, seed.Refresh(t.Context(), Request{Airport: "EKCH"}))
	active, err := cache.ActiveManifest(t.Context(), "EKCH")
	require.NoError(t, err)
	reader := cachedTerminalReader{snapshot: navdata.ActiveGeometrySnapshot{Manifest: active.Candidate, ManifestRevision: active.Revision, Airport: data.Airports["EKCH"], Runways: []navdata.Runway{}, Procedures: data.Procedures}}
	reader.snapshot.Runways, err = config.Runways(t.Context(), data.Version, "EKCH")
	require.NoError(t, err)
	for _, fix := range data.Fixes {
		reader.snapshot.Fixes = append(reader.snapshot.Fixes, fix)
	}
	source := fixture.New(data)
	m, err := New(Dependencies{CachedReferences: reader, Routes: source, Cache: cache, Terminal: config, Now: func() time.Time { return now }})
	require.NoError(t, err)
	require.NoError(t, m.Refresh(t.Context(), Request{Airport: "EKCH"}))
	require.Zero(t, source.Calls(), "cached terminal startup must not contact a source or route resolver")
	require.True(t, m.Health("EKCH").CacheReady)
	require.Empty(t, m.Health("EKCH").Reason)
	updated, err := cache.ActiveManifest(t.Context(), "EKCH")
	require.NoError(t, err)
	require.Equal(t, active.Candidate.AirportDigest, updated.Candidate.AirportDigest)
	require.Equal(t, active.Candidate.FixDigest, updated.Candidate.FixDigest)
	require.Equal(t, active.Candidate.ProcedureDigests, updated.Candidate.ProcedureDigests)
	_, err = cache.Route(t.Context(), "unknown")
	require.Error(t, err)
}

func TestCachedTerminalRefreshRejectsUnavailableAndOutOfDateSnapshots(t *testing.T) {
	for _, kind := range []string{"empty", "corrupt", "expired", "future", "missing terminal", "wrong cycle"} {
		t.Run(kind, func(t *testing.T) {
			data := fixture.EKCH()
			now := data.Provenance.ImportedAt.Add(time.Hour)
			reader := cachedTerminalReader{snapshot: navdata.ActiveGeometrySnapshot{Manifest: navdata.ManifestCandidate{Airport: "EKCH", Version: data.Version, TerminalDigest: "terminal"}}}
			switch kind {
			case "empty", "corrupt":
				reader.err = errors.New(kind + " cache")
			case "expired":
				now = data.Version.EffectiveUntil
			case "future":
				now = data.Version.EffectiveFrom.Add(-time.Second)
			case "missing terminal":
				reader.snapshot.Manifest.TerminalDigest = ""
			case "wrong cycle":
				reader.snapshot.Manifest.Version.Cycle = "0000"
			}
			m, err := New(Dependencies{CachedReferences: reader, Routes: fixture.New(data), Cache: &memoryCache{}, Terminal: configFor(data), Now: func() time.Time { return now }})
			require.NoError(t, err)
			require.Error(t, m.Refresh(t.Context(), Request{Airport: "EKCH"}))
			require.False(t, m.Health("EKCH").CacheReady)
			require.False(t, m.Health("EKCH").TerminalValid)
		})
	}
}
