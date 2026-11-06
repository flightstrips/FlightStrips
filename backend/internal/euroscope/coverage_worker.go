package euroscope

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"FlightStrips/internal/shared"
	"FlightStrips/internal/vatsim"
)

type CoverageRadioSource interface {
	Snapshot() vatsim.TransceiverSnapshot
}

type CoverageOptions struct {
	Network        vatsim.SnapshotSource
	Radios         CoverageRadioSource
	Now            func() time.Time
	MaxEvidenceAge time.Duration
	// Optional additional server policy. Enabled definitions and their configured
	// providers/radios are always required, even when this predicate allows entry.
	Authorize func(shared.AuthenticatedUser, string, string) bool
	// Simulation providers supply their own scoped observations. A missing or
	// wrong-environment inventory stays unknown; public LIVE data is never used.
	Simulated func(context.Context) ([]CoverageStaffingEvidence, error)
}

type CoverageCoordinator struct {
	hub     *Hub
	options CoverageOptions
	wake    chan struct{}
}

// ConfigureCoverage installs exactly one application coordinator before workers
// start. It shares the existing source workers, including MA-05's zero-membership
// LIVE demand, and never constructs a poller/cache for an airport membership.
func (hub *Hub) ConfigureCoverage(options CoverageOptions) *CoverageCoordinator {
	if hub.coverage != nil {
		return hub.coverage
	}
	if hub.airports == nil {
		return nil
	}
	enabled := false
	for _, airport := range hub.airports.Airports() {
		definition, _ := hub.airports.Lookup(airport)
		enabled = enabled || definition.MembershipPolicy() == "remaining-responsibilities/v1"
	}
	if !enabled {
		return nil
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	// Three normal shared-feed refresh intervals, independently of airport grace
	// and the plugin startup delay. Health failure makes last-good data stale now.
	if options.MaxEvidenceAge <= 0 {
		options.MaxEvidenceAge = 45 * time.Second
	}
	hub.coverage = &CoverageCoordinator{hub: hub, options: options, wake: make(chan struct{}, 1)}
	return hub.coverage
}

func (hub *Hub) notifyCoverage() {
	if hub.coverage == nil {
		return
	}
	select {
	case hub.coverage.wake <- struct{}{}:
	default:
	}
}

// Reconcile reads each shared generation once and applies deltas using current
// transport evidence. A control update that overtakes the snapshot is retried;
// it cannot attach an airport based on the prior identity/radio declaration.
func (c *CoverageCoordinator) Reconcile(ctx context.Context) (time.Time, error) {
	now := c.options.Now()
	presences := c.hub.ControlPresence()
	network := vatsim.Snapshot{}
	radios := vatsim.TransceiverSnapshot{}
	if c.options.Network != nil {
		network = c.options.Network.Snapshot()
	}
	if c.options.Radios != nil {
		radios = c.options.Radios.Snapshot()
	}
	var simulated []CoverageStaffingEvidence
	var result error
	if c.options.Simulated != nil {
		var err error
		simulated, err = c.options.Simulated(ctx)
		if err != nil {
			result = errors.Join(result, err)
			for i := range simulated {
				simulated[i].Inventory = CoverageStale
			}
		}
	}
	frames := BuildCoverageEvidence(network, radios, presences, now, c.options.MaxEvidenceAge, simulated...)
	var next time.Time
	for _, presence := range presences {
		if ctx.Err() != nil {
			return next, errors.Join(result, ctx.Err())
		}
		root := presence.Root
		t := root.transport
		t.coverageMu.Lock()
		t.mu.RLock()
		current := t.evidenceRevision == presence.EvidenceRevision
		t.mu.RUnlock()
		if current {
			user := root.userSnapshot()
			authorize := func(airport string) bool {
				return user.IsValid() && (c.options.Authorize == nil || c.options.Authorize(user, airport, presence.Evidence.Environment))
			}
			result = errors.Join(result, c.hub.reconcileCoverageLocked(root, presence.Evidence.EligibleAirports, authorize, frames[presence.Evidence.Environment], now))
		}
		for _, takeover := range t.takeovers {
			if next.IsZero() || takeover.Deadline.Before(next) {
				next = takeover.Deadline
			}
		}
		t.coverageMu.Unlock()
	}
	return next, result
}

func (c *CoverageCoordinator) Run(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.wake:
		case <-timer.C:
		}
		next, err := c.Reconcile(ctx)
		if err != nil && ctx.Err() == nil {
			slog.WarnContext(ctx, "Airport coverage reconciliation failed", slog.Any("error", err))
		}
		wait := time.Second // one application health/source check, never one per member
		if !next.IsZero() {
			wait = min(wait, max(time.Duration(0), next.Sub(c.options.Now())))
		}
		timer.Reset(wait)
	}
}
