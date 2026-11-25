package app

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/compatibility"
	"FlightStrips/internal/models"
)

// amanArrivalETARefreshAfter rewrites an unchanged AMAN ETA well before it
// stops being fresh, so the VATSIM fallback does not take over a flight AMAN is
// still predicting.
const amanArrivalETARefreshAfter = compatibility.FreshFor / 2

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

	// sessionLocks serializes projection per session. PublishAMANState and
	// PublishAMANAuthority can overlap; without this, a publication that listed
	// the strips before a newer one wrote could store its older prediction last.
	sessionLocks sync.Map // int32 -> *sync.Mutex
}

func (p *amanArrivalETAProjection) lockSession(session int32) func() {
	lock, _ := p.sessionLocks.LoadOrStore(session, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func newAMANArrivalETAProjection(mode aman.RolloutMode, store arrivalETAStripStore, notify func(int32, string)) *amanArrivalETAProjection {
	if store == nil {
		return nil
	}
	return &amanArrivalETAProjection{
		mode: mode, store: store, notify: notify,
		writer: compatibility.NewWriter(mode, compatibility.FreshFor, store),
	}
}

// project writes each AMAN flight's projected ETA to its strip when the value
// differs from what the strip holds, or when the stored AMAN value is nearing
// the end of its freshness window. Flights without a usable prediction are left
// alone: the VATSIM reconciler fills them in as the fallback estimate.
func (p *amanArrivalETAProjection) project(ctx context.Context, state aman.AirportState, health aman.TechnicalHealth, now time.Time) {
	if p == nil || state.SessionID == 0 || len(state.Flights) == 0 {
		return
	}
	// A blocked rollout gate owns neither ETA writer (see
	// aman.OwnershipForRolloutGate); leave the last accepted value in place.
	if !health.AuthorityAllowed {
		return
	}
	defer p.lockSession(state.SessionID)()
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
		projected := compatibility.ProjectArrivalETA(p.mode, flight.Prediction, now, compatibility.FreshFor)
		if projected == nil {
			continue
		}
		if newerAMANArrivalETA(strip.ArrivalETA, projected) {
			// An overlapping publication already stored a newer prediction.
			continue
		}
		if sameArrivalETA(strip.ArrivalETA, projected) && !strip.ArrivalETA.CalculatedAt.Before(now.Add(-amanArrivalETARefreshAfter)) {
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
	return existing != nil && !existing.Time.IsZero() &&
		existing.Time.Equal(projected.Time) && existing.Source == projected.Source
}

// newerAMANArrivalETA reports whether the stored ETA is an AMAN prediction
// generated after the projected one, which must not be overwritten.
func newerAMANArrivalETA(existing, projected *models.ArrivalETA) bool {
	return existing != nil && existing.Source == compatibility.ArrivalETASource &&
		existing.CalculatedAt.After(projected.CalculatedAt)
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
