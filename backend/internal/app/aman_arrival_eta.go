package app

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/compatibility"
	"FlightStrips/internal/models"
)

// amanArrivalETAMaxAge bounds how old an AMAN prediction may be before the
// strip's ArrivalETA is cleared instead of showing a stale estimate.
const amanArrivalETAMaxAge = 5 * time.Minute

// arrivalETAStripStore is the strip persistence needed to project AMAN's
// operational TETA onto the strip ArrivalETA field read by stand allocation.
type arrivalETAStripStore interface {
	compatibility.ArrivalETAStore
	List(ctx context.Context, session int32) ([]*models.Strip, error)
}

// amanArrivalETAProjection owns the AMAN side of the ArrivalETA field. In
// read-only and authoritative modes the legacy VATSIM writer is disabled, so
// without this projection stand assignments would never receive an ETA.
type amanArrivalETAProjection struct {
	mode   aman.RolloutMode
	store  arrivalETAStripStore
	writer compatibility.Writer
	notify func(session int32, callsign string)
}

func newAMANArrivalETAProjection(mode aman.RolloutMode, store arrivalETAStripStore, notify func(int32, string)) *amanArrivalETAProjection {
	if store == nil {
		return nil
	}
	return &amanArrivalETAProjection{
		mode: mode, store: store, notify: notify,
		writer: compatibility.NewWriter(mode, amanArrivalETAMaxAge, store),
	}
}

// project writes each AMAN flight's projected ETA to its strip when the value
// differs from what the strip holds. Only Time and Source are compared:
// GeneratedAt moves on every prediction and must not bump strip versions.
func (p *amanArrivalETAProjection) project(ctx context.Context, state aman.AirportState, health aman.TechnicalHealth, now time.Time) {
	if p == nil || state.SessionID == 0 || len(state.Flights) == 0 {
		return
	}
	// A blocked rollout gate owns neither ETA writer (see
	// aman.OwnershipForRolloutGate); leave the last accepted value in place.
	if !health.AuthorityAllowed {
		return
	}
	strips, err := p.store.List(ctx, state.SessionID)
	if err != nil {
		slog.WarnContext(ctx, "AMAN arrival ETA projection failed to list strips", slog.Int("session", int(state.SessionID)), slog.Any("error", err))
		return
	}
	current := make(map[string]*models.Strip, len(strips))
	for _, strip := range strips {
		if strip != nil {
			current[normalizeAMANCallsign(strip.Callsign)] = strip
		}
	}
	for _, flight := range state.Flights {
		strip, found := current[normalizeAMANCallsign(flight.Callsign)]
		if !found {
			continue
		}
		projected := compatibility.ProjectArrivalETA(p.mode, flight.Prediction, now, amanArrivalETAMaxAge)
		if sameArrivalETA(strip.ArrivalETA, projected) {
			continue
		}
		updated, err := p.writer.Apply(ctx, state.SessionID, strip.Callsign, flight.Prediction, now)
		if err != nil {
			slog.WarnContext(ctx, "AMAN arrival ETA projection failed", slog.Int("session", int(state.SessionID)), slog.String("callsign", strip.Callsign), slog.Any("error", err))
			continue
		}
		if updated && p.notify != nil && strip.EuroscopeSeenAt != nil {
			p.notify(state.SessionID, strip.Callsign)
		}
	}
}

func sameArrivalETA(existing, projected *models.ArrivalETA) bool {
	existingEmpty := existing == nil || existing.Time.IsZero()
	if projected == nil {
		return existingEmpty
	}
	return !existingEmpty && existing.Time.Equal(projected.Time) && existing.Source == projected.Source
}

func normalizeAMANCallsign(callsign string) string {
	return strings.ToUpper(strings.TrimSpace(callsign))
}

func (p *amanTransport) projectArrivalETAs(ctx context.Context, state aman.AirportState, health aman.TechnicalHealth) {
	p.mu.RLock()
	projection := p.arrivalETA
	p.mu.RUnlock()
	if projection == nil {
		return
	}
	now := time.Now().UTC()
	if p.now != nil {
		now = p.now().UTC()
	}
	projection.project(ctx, state, health, now)
}

func (p *amanTransport) setArrivalETAProjection(projection *amanArrivalETAProjection) {
	p.mu.Lock()
	p.arrivalETA = projection
	p.mu.Unlock()
}
