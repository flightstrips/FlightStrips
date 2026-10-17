// Package release plans approach intake without changing runway slots.
package release

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"sort"
	"time"
)

const PolicyVersion = "aman-approach-release-v1"

// StreamSpacing carries the base and additional gaps effective from a rate point.
type StreamSpacing struct {
	EffectiveAt   time.Time
	Minimum, Busy time.Duration
}

// Traffic contains accepted facts only. Unknown traffic remains capacity
// consuming; the planner never invents a departure from an elapsed target.
type Traffic struct {
	// Preserve offer order when fresh readiness or capacity postpones a leader.
	ReservationOrderAt                                        time.Time
	Spacing                                                   []StreamSpacing
	NewHoldingEpisode                                         bool
	WakeCategory                                              string
	Callsign                                                  aman.Callsign
	Active, Eligible, InApproach, Incoming, Unknown, Released bool
	Episode, HoldingID, STARFamily, FeederFix, RouteDigest    string
	RunwayGroupID                                             aman.RunwayGroupID
	EntryAt, ReadyAt, BaselineReleaseAt, SlotAt               time.Time
	Transit, MinimumSpacing, BusySpacing                      time.Duration
	Reservation                                               *aman.ApproachReleaseReservation
}

type Input struct {
	WorkloadUnknown bool
	BlockedReason   string
	At              time.Time
	Policy          *aman.ApproachReleasePolicy
	Traffic         []Traffic
}

type Decision struct {
	Callsign    aman.Callsign
	Reservation *aman.ApproachReleaseReservation
}

type Result struct {
	Buffer    aman.ReleaseBufferSummary
	Decisions []Decision
}

func Allowance(policy aman.ApproachReleasePolicy, count int) time.Duration {
	if count >= policy.TargetBuffer {
		return 0
	}
	seconds := policy.MaxAdvanceSeconds
	if count > policy.TaperStart {
		seconds = seconds * int64(policy.TargetBuffer-count) / int64(policy.TargetBuffer-policy.TaperStart)
	}
	return time.Duration(seconds) * time.Second
}

func sharedStream(a, b Traffic) bool {
	return a.HoldingID != "" && a.HoldingID == b.HoldingID ||
		a.STARFamily != "" && a.STARFamily == b.STARFamily ||
		a.FeederFix != "" && a.FeederFix == b.FeederFix
}

func cloneReservation(r *aman.ApproachReleaseReservation) *aman.ApproachReleaseReservation {
	if r == nil {
		return nil
	}
	copy := *r
	if r.ReleasedAt != nil {
		at := *r.ReleasedAt
		copy.ReleasedAt = &at
	}
	return &copy
}

// Plan is deterministic and does not mutate input. Offered releases reserve
// capacity immediately, even while their aircraft remain outside the TMA.
func Plan(input Input) Result {
	result := Result{Buffer: aman.ReleaseBufferSummary{Status: "disabled"}, Decisions: []Decision{}}
	enabled := input.Policy != nil && input.Policy.Enabled && input.Policy.Validate() == nil
	if input.Policy != nil {
		result.Buffer.Target = input.Policy.TargetBuffer
	}
	if enabled {
		result.Buffer.Status = "ready"
	}
	if input.WorkloadUnknown {
		result.Buffer.Unknown++
	}
	if input.BlockedReason != "" {
		result.Buffer.BlockedReasons = []string{input.BlockedReason}
	}
	traffic := append([]Traffic(nil), input.Traffic...)
	for i := range traffic {
		traffic[i].Spacing = append([]StreamSpacing(nil), traffic[i].Spacing...)
		sort.SliceStable(traffic[i].Spacing, func(a, b int) bool {
			return traffic[i].Spacing[a].EffectiveAt.Before(traffic[i].Spacing[b].EffectiveAt)
		})
	}
	sort.Slice(traffic, func(i, j int) bool {
		a, b := traffic[i], traffic[j]
		if !a.ReadyAt.Equal(b.ReadyAt) {
			return a.ReadyAt.Before(b.ReadyAt)
		}
		if !a.EntryAt.Equal(b.EntryAt) {
			return a.EntryAt.Before(b.EntryAt)
		}
		return a.Callsign < b.Callsign
	})
	lookahead := 2 * time.Minute
	if input.Policy != nil && input.Policy.LookaheadSeconds > 0 {
		lookahead = time.Duration(input.Policy.LookaheadSeconds) * time.Second
	}
	countedReservation := func(r *aman.ApproachReleaseReservation) bool {
		return r != nil && (r.ReleasedAt != nil || !r.ReleaseAt.After(input.At.Add(lookahead)))
	}
	reservations := make(map[aman.Callsign]*aman.ApproachReleaseReservation)
	for _, flight := range traffic {
		if !flight.Active {
			continue
		}
		r := cloneReservation(flight.Reservation)
		// A genuinely new confirmed holding episode replaces the prior admission.
		if r != nil && flight.NewHoldingEpisode && flight.Episode != r.Episode {
			r = nil
		}
		if r != nil {
			if flight.Released && r.ReleasedAt == nil {
				at := input.At
				r.ReleasedAt = &at
			}
			reservations[flight.Callsign] = r
		}
	}
	if reason := revalidateReservations(input, traffic, reservations); reason != "" {
		input.BlockedReason = reason
		result.Buffer.BlockedReasons = []string{reason}
	}
	for _, flight := range traffic {
		if !flight.Active {
			continue
		}
		r := reservations[flight.Callsign]
		if flight.Unknown {
			result.Buffer.Unknown++
		}
		switch {
		case flight.InApproach || r != nil && r.ReleasedAt != nil:
			result.Buffer.InApproach++
		case countedReservation(r):
			result.Buffer.Reserved++
		case flight.Incoming:
			result.Buffer.Incoming++
		}
	}
	if enabled && (result.Buffer.Unknown > 0 || input.BlockedReason != "") {
		result.Buffer.Status = "degraded"
	}

	// Select one oldest eligible head from each stream per sweep. A blocked
	// head does not prevent independent heads from being considered.
	considered := make(map[aman.Callsign]bool)
	for enabled && result.Buffer.Unknown == 0 && input.BlockedReason == "" {
		progressed := false
		var sweep []Traffic
		for _, flight := range traffic {
			if !flight.Active || !flight.Eligible || flight.InApproach || considered[flight.Callsign] || reservations[flight.Callsign] != nil && !reservations[flight.Callsign].ReleaseAt.After(input.At) {
				continue
			}
			olderPending := false
			for _, older := range traffic {
				if older.Callsign == flight.Callsign || !older.Active || !sharedStream(older, flight) ||
					!older.EntryAt.Before(flight.EntryAt) || reservations[older.Callsign] != nil {
					continue
				}
				// An older unready aircraft still owns its physical stream.
				if older.Eligible || !older.EntryAt.IsZero() {
					olderPending = true
					break
				}
			}
			if olderPending {
				continue
			}
			sameSweep := false
			for _, chosen := range sweep {
				if sharedStream(chosen, flight) {
					sameSweep = true
					break
				}
			}
			if sameSweep {
				continue
			}
			sweep = append(sweep, flight)
		}
		for _, flight := range sweep {
			considered[flight.Callsign] = true
			if flight.Transit <= 0 || flight.Episode == "" || flight.HoldingID == "" ||
				flight.STARFamily == "" || flight.FeederFix == "" || flight.RouteDigest == "" ||
				flight.RunwayGroupID == "" || flight.EntryAt.IsZero() || flight.BaselineReleaseAt.IsZero() {
				continue
			}
			existing := reservations[flight.Callsign]
			count := result.Buffer.Projected() + 1
			if countedReservation(existing) {
				count--
			}
			// An incoming aircraft already counted in the lookahead only changes
			// category when its release becomes reserved.
			if flight.Incoming && !countedReservation(existing) {
				count--
			}
			allowance := Allowance(*input.Policy, count)

			at := flight.BaselineReleaseAt.Add(-allowance)
			if flight.ReadyAt.After(at) {
				at = flight.ReadyAt
			}
			if input.At.After(at) {
				at = input.At
			}
			if !at.After(flight.EntryAt) {
				at = flight.EntryAt.Add(time.Second)
			}
			at = earliestStreamRelease(at, flight, releasePredecessors(flight.Callsign, traffic, reservations), reservations, count >= input.Policy.TargetBuffer)
			advance := flight.BaselineReleaseAt.Sub(at)
			if advance < 0 || existing != nil && !at.Before(existing.ReleaseAt) || at.After(input.At.Add(time.Duration(input.Policy.LookaheadSeconds)*time.Second)) {
				continue
			}
			reservations[flight.Callsign] = &aman.ApproachReleaseReservation{
				Episode: flight.Episode, HoldingID: flight.HoldingID, STARFamily: flight.STARFamily, FeederFix: flight.FeederFix,
				RunwayGroupID: flight.RunwayGroupID, HoldingEntryAt: flight.EntryAt, ReleaseAt: at, Transit: flight.Transit, AdditionalApproachTime: advance,
				RouteDigest: flight.RouteDigest, PolicyVersion: PolicyVersion,
			}
			if !countedReservation(existing) {
				if flight.Incoming {
					result.Buffer.Incoming--
				}
				result.Buffer.Reserved++
			}
			progressed = true
		}
		if !progressed {
			break
		}
	}
	for _, flight := range traffic {
		result.Decisions = append(result.Decisions, Decision{Callsign: flight.Callsign, Reservation: reservations[flight.Callsign]})
	}
	sort.Slice(result.Decisions, func(i, j int) bool { return result.Decisions[i].Callsign < result.Decisions[j].Callsign })
	return result
}

func wakeSpacing(leading, trailing string) time.Duration {
	for _, rule := range sequence.CPHSeparationRules() {
		if string(rule.Leading) == leading && string(rule.Trailing) == trailing {
			return rule.Minimum
		}
	}
	return 3 * time.Minute
}

// Search each rate period rather than retaining a spacing value from a later
// baseline target. Crossing a rate boundary can make its first instant legal.
func earliestStreamRelease(start time.Time, flight Traffic, traffic []Traffic, reservations map[aman.Callsign]*aman.ApproachReleaseReservation, busy bool) time.Time {
	at := ceilReleaseTime(start)
	for {
		candidate, next := streamReleaseBoundary(at, flight, traffic, reservations, busy)
		candidate = ceilReleaseTime(candidate)
		if !next.IsZero() && !candidate.Before(next) {
			at = ceilReleaseTime(next)
			continue
		}
		return candidate
	}
}

// streamReleaseBoundary checks spacing at a fixed entry time without rounding
// or moving across rate periods. Due/observed entries cannot be retimed.
func streamReleaseBoundary(at time.Time, flight Traffic, traffic []Traffic, reservations map[aman.Callsign]*aman.ApproachReleaseReservation, busy bool) (time.Time, time.Time) {
	minimum, extra, next := streamSpacingAt(flight, at)
	candidate := at
	for _, other := range traffic {
		r := reservations[other.Callsign]
		if other.Callsign == flight.Callsign || r == nil || !sharedStream(other, flight) {
			continue
		}
		spacing := max(minimum, wakeSpacing(other.WakeCategory, flight.WakeCategory))
		if busy {
			spacing = max(spacing, extra)
		}
		if flight.HoldingID == other.HoldingID {
			spacing = max(spacing, time.Minute)
		}
		leadingAt := r.ReleaseAt
		if r.ReleasedAt != nil && r.ReleasedAt.After(leadingAt) {
			leadingAt = *r.ReleasedAt
		}
		boundary := leadingAt.Add(spacing)
		if boundary.After(candidate) {
			candidate = boundary
		}
	}
	return candidate, next
}

func releasePredecessors(callsign aman.Callsign, traffic []Traffic, reservations map[aman.Callsign]*aman.ApproachReleaseReservation) []Traffic {
	self := reservations[callsign]
	if self == nil {
		return traffic
	}
	var preceding []Traffic
	for _, flight := range traffic {
		if r := reservations[flight.Callsign]; r != nil && !r.ReleaseAt.After(self.ReleaseAt) {
			preceding = append(preceding, flight)
		}
	}
	return preceding
}

func streamSpacingAt(flight Traffic, at time.Time) (time.Duration, time.Duration, time.Time) {
	minimum, busy := flight.MinimumSpacing, flight.BusySpacing
	for _, point := range flight.Spacing {
		if point.EffectiveAt.After(at) {
			return minimum, busy, point.EffectiveAt
		}
		minimum, busy = point.Minimum, point.Busy
	}
	return minimum, busy, time.Time{}
}

func ceilReleaseTime(at time.Time) time.Time {
	rounded := at.Truncate(time.Second)
	if rounded.Before(at) {
		rounded = rounded.Add(time.Second)
	}
	return rounded
}

// Validate spacing for irrevocable due/observed commitments without moving
// their EATs. Repair future offers before counting their intake; a postponed
// leader must also constrain later offers in its stream.
func revalidateReservations(input Input, traffic []Traffic, reservations map[aman.Callsign]*aman.ApproachReleaseReservation) string {
	ordered := append([]Traffic(nil), traffic...)
	orderAt := func(f Traffic, r *aman.ApproachReleaseReservation) time.Time {
		if !f.ReservationOrderAt.IsZero() {
			return f.ReservationOrderAt
		}
		return r.ReleaseAt
	}
	sort.Slice(ordered, func(i, j int) bool {
		a, b := reservations[ordered[i].Callsign], reservations[ordered[j].Callsign]
		if a == nil || b == nil {
			return a != nil && b == nil
		}
		left, right := orderAt(ordered[i], a), orderAt(ordered[j], b)
		if !left.Equal(right) {
			return left.Before(right)
		}
		return ordered[i].Callsign < ordered[j].Callsign
	})
	var preceding []Traffic
	reason := ""
	for _, flight := range ordered {
		r := reservations[flight.Callsign]
		if r == nil {
			continue
		}
		if r.ReleasedAt != nil || !r.ReleaseAt.After(input.At) {
			entryAt := r.ReleaseAt
			if r.ReleasedAt != nil && r.ReleasedAt.After(entryAt) {
				entryAt = *r.ReleasedAt
			}
			boundary, _ := streamReleaseBoundary(entryAt, flight, preceding, reservations, false)
			if boundary.After(entryAt) {
				reason = "approach_release_spacing_conflict"
			}
		} else if !flight.Unknown && flight.Transit > 0 && !flight.EntryAt.IsZero() && !flight.ReadyAt.IsZero() {
			previousRelease := r.ReleaseAt
			start := r.ReleaseAt
			if flight.ReadyAt.After(start) {
				start = flight.ReadyAt
			}
			if !start.After(flight.EntryAt) {
				start = flight.EntryAt.Add(time.Second)
			}
			r.ReleaseAt = earliestStreamRelease(start, flight, preceding, reservations, false)
			r.HoldingEntryAt = flight.EntryAt
			if !flight.BaselineReleaseAt.IsZero() {
				r.AdditionalApproachTime = max(time.Duration(0), flight.BaselineReleaseAt.Sub(r.ReleaseAt))
				if r.ReleaseAt.After(previousRelease) && r.ReleaseAt.After(flight.BaselineReleaseAt) {
					reason = "approach_release_timing_conflict"
				}
			}
		}
		preceding = append(preceding, flight)
	}
	return reason
}
