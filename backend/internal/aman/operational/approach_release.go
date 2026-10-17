package operational

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/release"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Preserve the last published imminent/overdue EAT before a prediction or
// command refresh can replace it. Manual protection does not remove capacity.
func (s *Service) captureLegacyApproachRelease(f aman.AMANFlight, at time.Time) aman.AMANFlight {
	policy := s.deps.Terminal.ApproachRelease
	if policy == nil || !policy.Enabled || f.Expedited || f.ApproachRelease != nil || f.Slot == nil ||
		f.State == aman.StateLanded || f.State == aman.StateRemoved || f.Prediction == nil ||
		f.Prediction.HoldingPlan == nil || f.SelectedHolding == nil || f.SelectedFeederFix == nil ||
		f.STARFamilyIdentity() == "" {
		return f
	}
	p := f.Prediction.HoldingPlan
	if p.ApproachReleaseTime.After(at.Add(time.Duration(policy.LookaheadSeconds) * time.Second)) {
		return f
	}
	digest := f.Prediction.GeometryDigest
	if basis := activeHoldingReleaseBasis(f); basis != nil {
		digest = basis.RouteDigest
	}
	if digest == "" {
		return f
	}
	f.ApproachRelease = &aman.ApproachReleaseReservation{
		Episode:   fmt.Sprintf("%s/%s", *f.SelectedHolding, p.HoldingEntryTime.UTC().Format(time.RFC3339Nano)),
		HoldingID: *f.SelectedHolding, STARFamily: f.STARFamilyIdentity(), FeederFix: *f.SelectedFeederFix,
		RunwayGroupID: f.Slot.RunwayGroupID, HoldingEntryAt: p.HoldingEntryTime,
		ReleaseAt: p.ApproachReleaseTime, Transit: p.PostHoldingTransit, AdditionalApproachTime: p.AdditionalApproachTime,
		RouteDigest: digest, PolicyVersion: release.PolicyVersion,
	}
	return f
}

func (s *Service) refreshApproachReleases(state *aman.AirportState, at time.Time) {
	policy := s.deps.Terminal.ApproachRelease
	if policy == nil {
		return
	}
	input := release.Input{At: at, Policy: policy, WorkloadUnknown: s.tmaVolume == nil}
	horizon := at.Add(time.Duration(policy.LookaheadSeconds) * time.Second)
	for _, f := range state.Flights {
		traffic := release.Traffic{Callsign: f.Callsign, Active: f.State != aman.StateLanded && f.State != aman.StateRemoved && f.State != aman.StatePlanned,
			STARFamily: f.STARFamilyIdentity(), FeederFix: stringValue(f.SelectedFeederFix), HoldingID: stringValue(f.SelectedHolding),
			Reservation: f.ApproachRelease}
		if f.SelectedRunwayGroup != nil {
			traffic.RunwayGroupID = *f.SelectedRunwayGroup
		} else if f.ApproachRelease != nil {
			traffic.RunwayGroupID = f.ApproachRelease.RunwayGroupID
		}
		// Rate policy survives loss of holding timing, prediction or assignment.
		// Due/observed admissions still require the schedule at their entry time.
		traffic.Spacing = s.releaseStreamSpacings(*state, traffic.RunwayGroupID, traffic.STARFamily)
		if f.HoldingStack != nil {
			traffic.EntryAt = f.HoldingStack.FirstObservedAt
		} else if f.HoldingClearance != nil {
			traffic.EntryAt = f.HoldingClearance.ObservedAt
		}
		observation := f.LatestObservation
		if observation != nil {
			traffic.WakeCategory = stringValue(observation.WakeCategory)
		}
		fresh := f.DataStatus == aman.DataFresh && observation != nil && observation.Surveillance != nil &&
			observation.Surveillance.ObservedAt != nil && !observation.Surveillance.ObservedAt.After(at) &&
			at.Sub(*observation.Surveillance.ObservedAt) <= tmaSurveillanceFresh
		held := holdingStackID(f) != "" || f.HoldingClearance != nil && f.HoldingClearance.HoldType == aman.HoldingClearanceEnroute && f.HoldingClearance.Hold != ""
		if f.TMAEntry != nil && f.TMAEntry.LastContainment == aman.TMAInside && aman.ConfirmedActiveHoldingSince(f) == nil {
			traffic.InApproach = true
		}
		if traffic.Active && !fresh {
			traffic.Unknown = true
		}
		if fresh && f.ApproachRelease != nil && f.RouteProgress != nil {
			// Evidence must be newer than the accepted EAT, and the retained holding
			// fix must no longer occur in the physical remaining route.
			traffic.Released = observedPastReservedHold(f, *f.ApproachRelease)
		}
		if traffic.Active && !held && !traffic.InApproach && (f.Prediction == nil || !f.Prediction.Publishable) {
			traffic.Unknown = true
		}
		if f.Prediction != nil && f.Prediction.Publishable && fresh {
			if !held && !traffic.InApproach {
				entry := s.predictedTMAEntry(f)
				traffic.Incoming = entry != nil && !entry.After(horizon)
				if entry == nil && f.State != aman.StatePlanned {
					traffic.Unknown = true
				}
			}
			if f.Slot != nil && populateApproachReleaseTiming(f, &traffic, at) {
				traffic.NewHoldingEpisode = f.HoldingStack != nil && f.HoldingStack.Confirmed && f.HoldingStack.HoldingID == traffic.HoldingID && (f.ApproachRelease == nil || f.ApproachRelease.HoldingID != traffic.HoldingID || f.ApproachRelease.ReleasedAt != nil && f.HoldingStack.FirstObservedAt.After(*f.ApproachRelease.ReleasedAt))
				traffic.Episode = fmt.Sprintf("%s/%s", traffic.HoldingID, traffic.EntryAt.UTC().Format(time.RFC3339Nano))
				if r := f.ApproachRelease; r != nil && r.HoldingID == traffic.HoldingID && !traffic.NewHoldingEpisode {
					traffic.Episode = r.Episode
				}
				traffic.Eligible = f.Prediction.HoldingPlan != nil && traffic.Active && f.SequenceDisposition.Participates() && traffic.STARFamily != "" &&
					traffic.FeederFix != "" && traffic.RouteDigest != "" && !retainedTimingUnavailable(f) &&
					f.FreezeReason != aman.FreezeManual && f.ManualOrder == nil
				if prior := f.ApproachRelease; prior != nil && prior.HoldingID == traffic.HoldingID && !traffic.NewHoldingEpisode {
					traffic.ReservationOrderAt = prior.ReleaseAt
					var reason string
					traffic.Reservation, reason = refreshAcceptedApproachRelease(f, traffic, *policy, at)
					if reason != "" {
						input.BlockedReason = reason
					}
				}
				if p := f.Prediction.HoldingPlan; f.ApproachRelease == nil && p != nil && held && !p.ApproachReleaseTime.After(horizon) && traffic.Eligible {
					traffic.Reservation = &aman.ApproachReleaseReservation{Episode: traffic.Episode, HoldingID: traffic.HoldingID,
						STARFamily: traffic.STARFamily, FeederFix: traffic.FeederFix, RunwayGroupID: traffic.RunwayGroupID,
						HoldingEntryAt: traffic.EntryAt, ReleaseAt: p.ApproachReleaseTime, Transit: traffic.Transit, RouteDigest: traffic.RouteDigest, PolicyVersion: release.PolicyVersion}
				}
			} else if traffic.Reservation != nil && !traffic.InApproach && !traffic.Released {
				// An accepted admission cannot make missing physical timing safe.
				traffic.Unknown = true
			}
		}
		// Holding timing disappears after physical release, but an accepted
		// admission still owns its original journey and absorption budget.
		// Check the retained facts independently of fresh prediction availability.
		if traffic.Active && traffic.Reservation != nil && f.Slot != nil && !traffic.NewHoldingEpisode {
			r := *traffic.Reservation
			r.AdditionalApproachTime = max(time.Duration(0), f.Slot.Time.Sub(r.ReleaseAt)-r.Transit)
			traffic.Reservation = &r
			if r.AdditionalApproachTime > time.Duration(policy.MaxAdvanceSeconds)*time.Second && input.BlockedReason == "" {
				input.BlockedReason = "approach_absorption_budget_exceeded"
			}
		}
		// An explicit FMP expedite is an immediate intake commitment even if
		// surveillance still shows the hold. It consumes one buffer place until
		// landing/removal or a successful return to the managed sequence.
		if f.Expedited {
			traffic.Incoming = true
		}
		input.Traffic = append(input.Traffic, traffic)
	}
	result := release.Plan(input)
	state.ReleaseBuffer = &result.Buffer
	facts := make(map[aman.Callsign]release.Traffic, len(input.Traffic))
	for _, f := range input.Traffic {
		facts[f.Callsign] = f
	}
	decisions := make(map[aman.Callsign]*aman.ApproachReleaseReservation, len(result.Decisions))
	for _, d := range result.Decisions {
		decisions[d.Callsign] = d.Reservation
	}
	for i := range state.Flights {
		f := &state.Flights[i]
		f.ApproachRelease = decisions[f.Callsign]
		r := f.ApproachRelease
		if f.Expedited || r == nil || r.HoldingID != stringValue(f.SelectedHolding) || f.Prediction == nil || f.Slot == nil ||
			retainedTimingUnavailable(*f) {
			continue
		}
		fact := facts[f.Callsign]
		if f.Prediction.HoldingPlan == nil && (fact.Released || r.ReleasedAt != nil) {
			continue
		}
		p := aman.HoldingPlan{HoldingEntryTime: fact.EntryAt}
		if p.HoldingEntryTime.IsZero() {
			p.HoldingEntryTime = r.HoldingEntryAt
		}
		if !r.ReleaseAt.After(p.HoldingEntryTime) {
			if r.HoldingEntryAt.IsZero() || !r.ReleaseAt.After(r.HoldingEntryAt) {
				continue
			}
			p.HoldingEntryTime = r.HoldingEntryAt
		}
		p.ApproachReleaseTime = r.ReleaseAt
		p.ExpectedHoldingDuration = r.ReleaseAt.Sub(p.HoldingEntryTime)
		// The physical journey remains distinct from planned vectoring/absorption.
		p.PostHoldingTransit = r.Transit
		p.AdditionalApproachTime = max(time.Duration(0), f.Slot.Time.Sub(r.ReleaseAt)-p.PostHoldingTransit)
		r.AdditionalApproachTime = p.AdditionalApproachTime
		prediction := *f.Prediction
		prediction.HoldingPlan = &p
		f.Prediction = &prediction
		applyDerivedFeederETA(f, holdingFeederETA(state.Authoritative, *f, prediction, s.deps.Terminal))
	}
}

func observedPastReservedHold(f aman.AMANFlight, r aman.ApproachReleaseReservation) bool {
	if f.LatestObservation == nil || f.LatestObservation.Surveillance == nil || f.LatestObservation.Surveillance.ObservedAt == nil ||
		!f.LatestObservation.Surveillance.ObservedAt.After(r.ReleaseAt) || f.Prediction == nil || !f.Prediction.Publishable || f.Prediction.InputObservedAt.Before(*f.LatestObservation.Surveillance.ObservedAt) || f.Prediction.HoldingFixETA != nil {
		return false
	}
	// Remaining leg identities are accepted physical route progress, not an
	// assigned clearance or proximity to a racetrack.
	fix := holdingFixFromID(f, r.HoldingID)
	if len(f.Prediction.RemainingRouteFixes) > 0 {
		for _, to := range f.Prediction.RemainingRouteFixes {
			if to == fix {
				return false
			}
		}
		return true
	}
	// Legacy performance predictions retain the same evidence in Calculation.
	// Missing/empty geometry never proves release, including older RETA rows.
	if f.Prediction.Calculation == nil || len(f.Prediction.Calculation.Legs) == 0 {
		return false
	}
	for _, leg := range f.Prediction.Calculation.Legs {
		if leg.To == fix {
			return false
		}
	}
	return true
}

func holdingFixFromID(f aman.AMANFlight, id string) string {
	if f.HoldingReleaseBasis != nil && f.HoldingReleaseBasis.HoldingID == id {
		return f.HoldingReleaseBasis.Fix
	}
	if f.HoldingClearance != nil && f.HoldingClearance.Hold != "" {
		return strings.ToUpper(f.HoldingClearance.Hold)
	}
	return id
}

// Physical timing is available independently of a positive-duration holding
// plan. In particular, an infeasible slot must not hide an accepted EAT.
func populateApproachReleaseTiming(f aman.AMANFlight, t *release.Traffic, at time.Time) bool {
	t.RouteDigest = f.Prediction.GeometryDigest
	if p := f.Prediction.HoldingPlan; p != nil {
		t.EntryAt, t.ReadyAt, t.Transit = p.HoldingEntryTime, p.HoldingEntryTime, p.PostHoldingTransit
	} else if entry := f.Prediction.HoldingFixETA; entry != nil {
		t.EntryAt, t.ReadyAt = *entry, *entry
		t.Transit = f.Prediction.RawTETA.Sub(*entry)
	}
	if basis := activeHoldingReleaseBasis(f); basis != nil {
		t.EntryAt, t.ReadyAt = basis.HoldingEntryTime, at
		t.Transit, t.RouteDigest = basis.PostHoldingTransit, basis.RouteDigest
	} else if f.HoldingStack != nil && f.HoldingStack.Confirmed && t.Transit > 0 {
		t.EntryAt, t.ReadyAt = f.HoldingStack.FirstObservedAt, at
	}
	t.SlotAt = f.Slot.Time
	t.BaselineReleaseAt = f.Slot.Time.Add(-t.Transit)
	return t.Transit > 0 && !t.EntryAt.IsZero() && t.RouteDigest != ""
}

func refreshAcceptedApproachRelease(f aman.AMANFlight, t release.Traffic, policy aman.ApproachReleasePolicy, at time.Time) (*aman.ApproachReleaseReservation, string) {
	prior := f.ApproachRelease
	r := *prior
	r.RunwayGroupID = t.RunwayGroupID
	if t.STARFamily != "" {
		r.STARFamily = t.STARFamily
	}
	if t.FeederFix != "" {
		r.FeederFix = t.FeederFix
	}
	r.Transit, r.RouteDigest = t.Transit, t.RouteDigest
	due := r.ReleasedAt != nil || !r.ReleaseAt.After(at)
	if !due {
		earliest := t.BaselineReleaseAt.Add(-time.Duration(policy.MaxAdvanceSeconds) * time.Second)
		earliest = maxTime(earliest, t.ReadyAt, t.EntryAt.Add(time.Second))
		if earliest.After(r.ReleaseAt) {
			r.ReleaseAt = earliest
		}
		r.HoldingEntryAt = t.EntryAt
	}
	conflict := due && (f.Slot.Time.Sub(r.ReleaseAt) < t.Transit ||
		!t.Released && !t.InApproach && !r.ReleaseAt.After(t.EntryAt))
	if !due {
		// A future offer may still advance into its newly earlier slot. Only
		// infeasible readiness, or an offer that cannot be advanced, blocks it.
		conflict = maxTime(t.ReadyAt, t.EntryAt.Add(time.Second)).After(t.BaselineReleaseAt) ||
			!t.Eligible && f.Slot.Time.Sub(r.ReleaseAt) < t.Transit
	}
	if conflict && due {
		// Preserve the accepted journey and target, while the new physical
		// timing remains visible in the trajectory and requires review.
		r.Transit, r.RouteDigest = prior.Transit, prior.RouteDigest
	}
	r.AdditionalApproachTime = max(time.Duration(0), f.Slot.Time.Sub(r.ReleaseAt)-r.Transit)
	if conflict {
		return &r, "approach_release_timing_conflict"
	}
	if r.AdditionalApproachTime > time.Duration(policy.MaxAdvanceSeconds)*time.Second {
		return &r, "approach_absorption_budget_exceeded"
	}
	return &r, ""
}

func maxTime(values ...time.Time) time.Time {
	var latest time.Time
	for _, at := range values {
		if at.After(latest) {
			latest = at
		}
	}
	return latest
}

func (s *Service) releaseStreamSpacings(state aman.AirportState, groupID aman.RunwayGroupID, starFamily string) []release.StreamSpacing {
	multiplier := 1
	for _, policy := range s.deps.Terminal.STARFamilyPolicies {
		if string(policy.STARFamily) == starFamily && policy.SameSTARSpacing.Enabled {
			multiplier = int(policy.SameSTARSpacing.MinimumEmptySlots) + 1
		}
	}
	var result []release.StreamSpacing
	for _, group := range state.RunwayGroups {
		if group.ID != groupID {
			continue
		}
		for _, rate := range sequenceRates(group) {
			if rate.ArrivalsPerHour == 0 {
				continue
			}
			base := time.Hour / time.Duration(rate.ArrivalsPerHour)
			result = append(result, release.StreamSpacing{EffectiveAt: rate.EffectiveAt, Minimum: base, Busy: base * time.Duration(multiplier)})
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].EffectiveAt.Before(result[j].EffectiveAt) })
	return result
}

func (s *Service) predictedTMAEntry(f aman.AMANFlight) *time.Time {
	if s.tmaVolume == nil || f.Prediction == nil || !f.Prediction.Publishable || isPreliminaryPrediction(f.Prediction) || f.Prediction.Calculation == nil ||
		f.LatestObservation == nil || f.LatestObservation.Surveillance == nil ||
		f.LatestObservation.Surveillance.AltitudeFeet == nil || f.LatestObservation.Surveillance.ObservedAt == nil ||
		f.Prediction.InputObservedAt.Before(*f.LatestObservation.Surveillance.ObservedAt) {
		return nil
	}
	at := f.Prediction.InputObservedAt
	altitude := float64(*f.LatestObservation.Surveillance.AltitudeFeet)
	for i, leg := range f.Prediction.Calculation.Legs {
		endAltitude := altitude
		for _, segment := range f.Prediction.Calculation.Segments {
			if segment.RouteLegIndex == i {
				endAltitude = segment.EndAltitudeFeet
			}
		}
		if fraction, ok := s.tmaVolume.EntryFraction(leg.StartLatitude, leg.StartLongitude, altitude, leg.EndLatitude, leg.EndLongitude, endAltitude); ok {
			entry := at.Add(time.Duration(float64(leg.Duration) * fraction))
			return &entry
		}
		at = at.Add(leg.Duration)
		altitude = endAltitude
	}
	return nil
}
