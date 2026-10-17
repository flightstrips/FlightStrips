package main

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/operational"
	"FlightStrips/internal/aman/release"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"time"
)

type replayRebuildStats struct {
	RetainedDueApproachAdmissions int            `json:"retained_due_approach_admissions"`
	InjectedApproachOffers        int            `json:"injected_future_approach_offers"`
	ResetApproachOffers           int            `json:"reset_future_approach_offers"`
	ApproachReservationViolations int            `json:"approach_reservation_violations"`
	States                        map[string]int `json:"states"`
	Attempts                      int            `json:"attempts"`
	Applied                       int            `json:"applied"`
	Unchanged                     int            `json:"unchanged"`
	Rejected                      int            `json:"rejected"`
	RejectionReasons              map[string]int `json:"rejection_reasons,omitempty"`
	RankedHoldingFlights          int            `json:"ranked_holding_flights"`
	Diagnostics                   int            `json:"diagnostics"`
	RepeatedResultViolations      int            `json:"repeated_result_violations"`
	ReleaseOrderViolations        int            `json:"release_order_violations"`
	TimingUsabilityViolations     int            `json:"timing_usability_violations"`
	RefreshedAutomaticTimings     int            `json:"refreshed_automatic_timings"`
	RefreshedTMATimings           int            `json:"refreshed_tma_timings"`
	SnapshotChecks                int            `json:"snapshot_checks"`
	SnapshotTimes                 []string       `json:"snapshot_times,omitempty"`
	SnapshotRejections            int            `json:"snapshot_rejections"`
	InjectedAutomaticCaptures     int            `json:"injected_automatic_captures"`
}

func selectRebuildSnapshots(total, count int, seed int64) map[int]bool {
	selected := map[int]bool{}
	if count <= 0 || total <= 0 {
		return selected
	}
	// Warm the tracking history before selecting positions throughout the session.
	start := total / 10
	indices := rand.New(rand.NewSource(seed)).Perm(total - start)
	for _, index := range indices[:min(count, len(indices))] {
		selected[start+index] = true
	}
	return selected
}

func (s *replayRebuildStats) ApplySnapshot(service *operational.Service, repo *memoryRepository, at time.Time) {
	s.SnapshotTimes = append(s.SnapshotTimes, at.UTC().Format(time.RFC3339Nano))
	for scenario, offset := range []time.Duration{-10 * time.Minute, 2 * time.Hour, 2 * time.Hour} {
		data, err := json.Marshal(repo.state)
		if err != nil {
			panic(err)
		}
		clone := &memoryRepository{}
		if err := json.Unmarshal(data, &clone.state); err != nil {
			panic(err)
		}
		for i := range clone.state.Flights {
			f := &clone.state.Flights[i]
			if !replayTimingIsCurrent(*f, at) || f.Slot == nil || (f.FreezeReason != aman.FreezeTMA && f.FreezeReason != aman.FreezeSuperstable) {
				continue
			}
			// Unknown holding CFL cannot be reset from the public API feed.
			if (aman.ActiveHoldingEpisode(*f) != nil || f.HoldingStack != nil && f.HoldingStack.Confirmed) && (f.HoldingClearance == nil || f.HoldingClearance.ClearedAltitude == nil) {
				continue
			}
			obsolete := at.Add(offset)
			f.Slot.Time = obsolete
			captured := *f.Slot
			f.FrozenSlot, f.FrozenOperationalTETA = &captured, &obsolete
			f.Prediction.OperationalTETA = obsolete
			s.InjectedAutomaticCaptures++
		}
		// The third detached scenario simulates future automatic offers instead
		// of unobserved due offers. The first two preserve all real commitments.
		// Public VATSIM observations do not supply controller offers or CFLs.
		if scenario == 2 {
			for i := range clone.state.Flights {
				f := &clone.state.Flights[i]
				e := aman.ActiveHoldingEpisode(*f)
				if !replayTimingIsCurrent(*f, at) || e == nil || f.HoldingClearance == nil || f.HoldingClearance.ClearedAltitude == nil || f.SelectedHolding == nil || f.SelectedFeederFix == nil || f.STARFamilyIdentity() == "" || f.Slot == nil {
					continue
				}
				if r := f.ApproachRelease; r != nil && r.ReleasedAt != nil {
					continue
				}
				transit, digest := time.Duration(0), f.Prediction.GeometryDigest
				if basis := f.HoldingReleaseBasis; basis != nil {
					transit, digest = basis.PostHoldingTransit, basis.RouteDigest
				} else if f.Prediction.HoldingPlan != nil {
					transit = f.Prediction.HoldingPlan.PostHoldingTransit
				}
				if digest == "" || transit <= 0 {
					continue
				}
				offer := at.Add(30 * time.Minute)
				f.ApproachRelease = &aman.ApproachReleaseReservation{Episode: "injected-obsolete-offer", HoldingID: *f.SelectedHolding, STARFamily: f.STARFamilyIdentity(), FeederFix: *f.SelectedFeederFix, RunwayGroupID: f.Slot.RunwayGroupID, HoldingEntryAt: *e.StartedAt, ReleaseAt: offer, Transit: transit, RouteDigest: digest, PolicyVersion: release.PolicyVersion}
				s.InjectedApproachOffers++
			}
		}
		s.SnapshotChecks++
		previousRejections := s.Rejected
		s.Apply(service, clone, at)
		s.SnapshotRejections += s.Rejected - previousRejections
	}
}

// The public feed has no controller CFL or holding annotations. This explicitly
// opt-in overlay exercises those inputs (including assignments before entry);
// it is never reported as recorded controller or holding truth.
func applyReplayHoldingLevels(state *aman.AirportState, at time.Time) {
	for i := range state.Flights {
		f := &state.Flights[i]
		if f.LatestObservation == nil || f.LatestObservation.Surveillance == nil ||
			f.LatestObservation.Surveillance.AltitudeFeet == nil || f.SelectedHolding == nil || f.Prediction == nil || f.Prediction.HoldingFixETA == nil {
			continue
		}
		fix := ""
		if f.HoldingEpisode != nil && f.HoldingEpisode.HoldingID == *f.SelectedHolding {
			fix = f.HoldingEpisode.Fix
		}
		if holdingFix := holdingFixFromCalculation(f.Prediction); holdingFix != nil && fix == "" {
			fix = holdingFix.id
		}
		if f.HoldingReleaseBasis != nil {
			fix = f.HoldingReleaseBasis.Fix
		}
		if fix == "" {
			continue
		}
		if aman.ActiveHoldingEpisode(*f) != nil && f.HoldingClearance != nil && f.HoldingClearance.ClearedAltitude != nil {
			continue
		}
		level := int32(math.Round(float64(*f.LatestObservation.Surveillance.AltitudeFeet)/1000) * 1000)
		f.ReplaceHoldingClearance(&aman.HoldingClearance{Hold: fix, HoldType: aman.HoldingClearanceEnroute, ClearedAltitude: &level, ObservedAt: at})
	}
}

func (s *replayRebuildStats) Apply(service *operational.Service, repo *memoryRepository, at time.Time) {
	s.Attempts++
	if s.States == nil {
		s.States = map[string]int{}
	}
	for _, f := range repo.state.Flights {
		s.States[string(f.State)]++
		if f.Slot != nil {
			s.States["with_slot"]++
		}
		if f.LatestObservation != nil && f.LatestObservation.Surveillance != nil {
			s.States["with_surveillance"]++
		}
		if f.HoldingStack != nil && f.HoldingStack.Confirmed {
			s.States["confirmed_holding"]++
		}
	}
	if s.Attempts == 1 {
		stateReport, _ := json.Marshal(s.States)
		fmt.Printf("Initial rebuild input: %s\n", stateReport)
	}
	command := aman.RebuildArrivalSequenceCommand{Metadata: aman.CommandMetadata{CommandID: fmt.Sprintf("replay-rebuild-%d", s.Attempts), ExpectedRevision: repo.state.Revision}}
	auth := aman.CommandContext{Airport: "EKCH", Role: "EKDK_FMP", Actor: "replay", ReceivedAt: at}
	mutation, err := service.RebuildArrivalSequence(context.Background(), auth, command)
	if err != nil {
		s.Rejected++
		s.recordRejection(err)
		return
	}
	detached, err := json.Marshal(repo.state)
	if err != nil {
		panic(err)
	}
	var input aman.AirportState
	if err := json.Unmarshal(detached, &input); err != nil {
		panic(err)
	}
	change, err := mutation(input)
	if err != nil {
		s.Rejected++
		s.recordRejection(err)
		return
	}
	repeated, err := mutation(input)
	if err != nil || string(change.Outcome) != string(repeated.Outcome) {
		s.RepeatedResultViolations++
	}
	firstState, _ := json.Marshal(change.State)
	secondState, _ := json.Marshal(repeated.State)
	if string(firstState) != string(secondState) {
		s.RepeatedResultViolations++
	}
	var outcome aman.ArrivalSequenceRebuildOutcome
	if err := json.Unmarshal(change.Outcome, &outcome); err != nil {
		panic(err)
	}
	s.RankedHoldingFlights += len(outcome.ReleaseOrder)
	s.Diagnostics += len(outcome.Diagnostics)
	s.validateTimingRecovery(input, change.State, outcome, at)
	if !change.Changed {
		s.Unchanged++
		return
	}
	// This benchmark exercises the operational mutation directly. Advance the
	// in-memory revision and discard stale offers; coordinator/CAS/deduplication
	// and offer publication are covered by the command integration tests.
	change.State.Revision++
	change.State.GeneratedAt = at
	for i := range change.State.Flights {
		f := &change.State.Flights[i]
		if f.Slot != nil {
			f.Slot.Revision = change.State.Revision
		}
		f.QueueOffers = nil
	}
	if err := change.State.Validate(); err != nil {
		panic(err)
	}
	repo.state = change.State
	s.Applied++
	for _, later := range repo.state.Flights {
		if later.Prediction == nil || later.Prediction.HoldingPlan == nil {
			continue
		}
		for _, older := range repo.state.Flights {
			if precedes, rebuilt := aman.RebuiltHoldingPrecedes(older, later); rebuilt && precedes && older.Prediction != nil && older.Prediction.HoldingPlan != nil {
				a, b := older.Prediction.HoldingPlan.ApproachReleaseTime, later.Prediction.HoldingPlan.ApproachReleaseTime
				if b.Before(a) || older.SelectedHolding != nil && later.SelectedHolding != nil && *older.SelectedHolding == *later.SelectedHolding && b.Sub(a) < time.Minute {
					s.ReleaseOrderViolations++
				}
			}
		}
	}
}

func replayTimingIsCurrent(f aman.AMANFlight, at time.Time) bool {
	if f.State == aman.StatePlanned || f.State == aman.StateLanded || f.State == aman.StateRemoved || !f.SequenceDisposition.Participates() || f.DataStatus != aman.DataFresh || f.Prediction == nil || !f.Prediction.Publishable || f.FreezeReason == aman.FreezeManual || f.Prediction.OperationalReason == aman.OperationalReasonManualOverride {
		return false
	}
	p := f.Prediction
	if o := f.LatestObservation; o != nil {
		if o.Missing || o.SourceStatus != aman.DataFresh || o.Surveillance != nil && o.Surveillance.ObservedAt != nil && o.Surveillance.ObservedAt.UTC().Truncate(time.Second).After(p.InputObservedAt) {
			return false
		}
	}
	return !p.InputObservedAt.IsZero() && !p.InputObservedAt.After(at) && at.Sub(p.InputObservedAt) <= 2*time.Minute
}

func (s *replayRebuildStats) recordRejection(err error) {
	if s.RejectionReasons == nil {
		s.RejectionReasons = map[string]int{}
	}
	s.RejectionReasons[err.Error()]++
}

func (s *replayRebuildStats) validateTimingRecovery(before, after aman.AirportState, outcome aman.ArrivalSequenceRebuildOutcome, at time.Time) {
	exempt := map[aman.Callsign]bool{}
	retainedAdmissions := map[aman.Callsign]bool{}
	blockedHolds := map[string]bool{}
	for _, d := range outcome.Diagnostics {
		if d.Code == "approach_admission_retained" {
			retainedAdmissions[d.Callsign] = true
		}
		if d.Code == "current_prediction_unavailable" {
			exempt[d.Callsign] = true
		}
		if d.Code == "holding_evidence_unavailable" {
			for _, f := range before.Flights {
				if f.Callsign == d.Callsign && f.SelectedHolding != nil {
					blockedHolds[*f.SelectedHolding] = true
				}
			}
		}
	}
	for _, old := range before.Flights {
		if !replayTimingIsCurrent(old, at) || exempt[old.Callsign] || old.SelectedHolding != nil && blockedHolds[*old.SelectedHolding] && (aman.ActiveHoldingEpisode(old) != nil || old.HoldingStack != nil && old.HoldingStack.Confirmed) {
			continue
		}
		for _, f := range after.Flights {
			if f.Callsign != old.Callsign || f.Slot == nil || f.Prediction == nil {
				continue
			}
			if old.ApproachRelease != nil && old.ApproachRelease.ReleasedAt == nil && old.ApproachRelease.ReleaseAt.After(at) {
				if f.ApproachRelease == nil || !f.ApproachRelease.ReleaseAt.Equal(old.ApproachRelease.ReleaseAt) {
					s.ResetApproachOffers++
				}
				if old.ApproachRelease.Episode == "injected-obsolete-offer" && f.ApproachRelease != nil && f.ApproachRelease.Episode == old.ApproachRelease.Episode {
					s.ApproachReservationViolations++
				}
			}
			if f.ApproachRelease != nil && f.Prediction.HoldingPlan != nil && f.SelectedHolding != nil && f.ApproachRelease.HoldingID == *f.SelectedHolding && !f.ApproachRelease.ReleaseAt.Equal(f.Prediction.HoldingPlan.ApproachReleaseTime) {
				s.ApproachReservationViolations++
			}
			s.RefreshedAutomaticTimings++
			if f.FreezeReason == aman.FreezeTMA {
				s.RefreshedTMATimings++
			}
			earliest := old.Prediction.RawTETA
			if floor := at.Add(time.Second); floor.After(earliest) {
				earliest = floor
			}
			retainedAdmission := false
			if prior := old.ApproachRelease; prior != nil && (prior.ReleasedAt != nil || !prior.ReleaseAt.After(at)) {
				retainedAdmission = retainedAdmissions[f.Callsign] && f.ApproachRelease != nil && f.ApproachRelease.ReleaseAt.Equal(prior.ReleaseAt)
				if !retainedAdmission {
					s.ApproachReservationViolations++
				} else {
					s.RetainedDueApproachAdmissions++
				}
			}
			if aman.ValidHoldingReleaseReservation(f) != nil && f.Prediction.HoldingPlan != nil {
				earliest = at.Add(time.Second + f.Prediction.HoldingPlan.PostHoldingTransit)
				if old.Prediction.HoldingFixETA != nil {
					fixArrival := old.Prediction.HoldingFixETA.Add(f.Prediction.HoldingPlan.PostHoldingTransit)
					if fixArrival.After(earliest) {
						earliest = fixArrival
					}
				}
				if !retainedAdmission && !f.Prediction.HoldingPlan.ApproachReleaseTime.After(at) {
					s.TimingUsabilityViolations++
				}
			}
			if f.Slot.Time.Before(earliest) || !f.Prediction.OperationalTETA.After(at) {
				s.TimingUsabilityViolations++
			}
			if (f.FreezeReason == aman.FreezeTMA || f.FreezeReason == aman.FreezeSuperstable) && (f.FrozenSlot == nil || !f.FrozenSlot.Time.Equal(f.Slot.Time) || f.FrozenOperationalTETA == nil || !f.FrozenOperationalTETA.Equal(f.Prediction.OperationalTETA)) {
				s.TimingUsabilityViolations++
			}
		}
	}
}
