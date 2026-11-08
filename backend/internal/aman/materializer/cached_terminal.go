package materializer

import (
	"context"
	"fmt"

	"FlightStrips/internal/aman/terminal"
)

// refreshCachedTerminal keeps a validated, current terminal manifest usable
// while its original catalog provider is unavailable. vIFF supplies flight
// routes only: an empty, corrupt, future or expired terminal cache fails closed.
func (m *Materializer) refreshCachedTerminal(ctx context.Context, request Request) error {
	m.markAttempt(request.Airport)
	snapshot, err := m.deps.CachedReferences.ActiveGeometrySnapshot(ctx, request.Airport)
	if err != nil {
		m.invalidateActive(request.Airport, ReasonNoActiveDataset)
		return m.failed(request.Airport, ReasonNoActiveDataset, false, false, fmt.Errorf("route-only navigation requires an existing validated terminal cache for %s: %w", request.Airport, err))
	}
	version := snapshot.Manifest.Version
	now := m.deps.Now().UTC()
	if now.Before(version.EffectiveFrom) || !now.Before(version.EffectiveUntil) {
		m.invalidateActive(request.Airport, ReasonDatasetExpired)
		return m.failed(request.Airport, ReasonDatasetExpired, false, false, fmt.Errorf("cached terminal dataset %s is not currently effective", version.Cycle))
	}
	if snapshot.Manifest.TerminalDigest == "" {
		m.invalidateActive(request.Airport, ReasonTerminalGeometryBad)
		return m.failed(request.Airport, ReasonTerminalGeometryBad, false, false, fmt.Errorf("cached terminal manifest is missing configured geometry"))
	}
	refs := terminal.ReferenceSet{Version: version, Airport: snapshot.Airport, Runways: snapshot.Runways, Fixes: snapshot.Fixes, Procedures: snapshot.Procedures}
	// Rebuild only the terminal fragment, retaining the exact airport, procedure
	// and fix fragment revisions. This also applies checked-in terminal updates.
	fragment, err := m.deps.Terminal.Candidate(refs, now)
	if err != nil {
		m.invalidateActive(request.Airport, ReasonTerminalGeometryBad)
		return m.failed(request.Airport, ReasonTerminalGeometryBad, true, false, err)
	}
	for _, id := range request.FixIDs {
		found := false
		for _, fix := range snapshot.Fixes {
			if fix.ID == id {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("route-only navigation cannot acquire uncached fix %s", id)
		}
	}
	digest, err := m.deps.Cache.PutTerminalFragment(ctx, fragment)
	if err != nil {
		return m.failed(request.Airport, ReasonCandidateInvalid, true, true, err)
	}
	manifest := snapshot.Manifest
	manifest.TerminalDigest, manifest.ExpectedRevision = digest, snapshot.ManifestRevision
	if _, err = m.deps.Cache.ActivateManifest(ctx, manifest); err != nil {
		return m.failed(request.Airport, ReasonCandidateInvalid, true, true, err)
	}
	m.succeeded(request.Airport, version)
	return nil
}
