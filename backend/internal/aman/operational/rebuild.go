package operational

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"context"
	"encoding/json"
	"sort"
	"time"
)

func (s *Service) RebuildArrivalSequence(ctx context.Context, auth aman.CommandContext, command aman.RebuildArrivalSequenceCommand) (sequence.CommandMutation, error) {
	if !aman.IsFMPRole(auth.Role) {
		return nil, &aman.DomainError{Class: aman.ErrorUnauthorized, Message: "arrival sequence rebuild requires an FMP role"}
	}
	return func(state aman.AirportState) (sequence.CommandChange, error) {
		before := state
		state.Flights = append([]aman.AMANFlight(nil), state.Flights...)
		state.RunwayGroups = append([]aman.RunwayGroupPolicy(nil), state.RunwayGroups...)
		setProtectedSameSTARWarnings(state.RunwayGroups, nil)
		state.ReleaseBuffer = nil
		diagnostics := []aman.RebuildDiagnostic{}
		tma := make(map[aman.Callsign]bool)
		for i := range state.Flights {
			f := &state.Flights[i]
			resetRebuiltTiming(f)
			// Retry the physical calculation when radar is current but the last
			// prediction failed or predates that position. Existing current raw
			// estimates need no extra route/wind request.
			if canRefreshRebuildPrediction(*f, auth.ReceivedAt) && (retainedTimingUnavailable(*f) || !currentRebuildPrediction(*f, auth.ReceivedAt)) {
				updated, err := s.reconcileFlight(withPredictionRefresh(ctx), state, *f, *f.LatestObservation, auth.ReceivedAt)
				if err == nil && (updated.State == aman.StateLanded || updated.State == aman.StateRemoved || !retainedTimingUnavailable(updated)) {
					*f = updated
				}
				if err := ctx.Err(); err != nil {
					return sequence.CommandChange{}, err
				}
			}
			if aman.ActiveHoldingEpisode(*f) == nil && (f.HoldingEpisode == nil || f.HoldingEpisode.PendingAt != nil && f.HoldingEpisode.StartedAt == nil && f.HoldingEpisode.EndedAt == nil) && f.HoldingStack != nil && f.HoldingStack.Confirmed && f.SelectedHolding != nil {
				at := f.HoldingStack.CandidateObservedAt
				if !at.IsZero() && !at.After(auth.ReceivedAt) {
					fix := *f.SelectedHolding
					if f.HoldingClearance != nil && f.HoldingClearance.Hold != "" {
						fix = f.HoldingClearance.Hold
					}
					source := "estimated_detection"
					if pending := f.HoldingEpisode; pending != nil && pending.PendingAt != nil {
						at = *pending.PendingAt
						source = pending.PendingSource
					}
					f.HoldingEpisode = &aman.HoldingEpisode{ID: aman.HoldingEpisodeID(f.Callsign, *f.SelectedHolding, at), HoldingID: *f.SelectedHolding, Fix: fix, StartedAt: &at, Source: source}
				}
			}
			resetRebuiltTiming(f)
			tma[f.Callsign] = rebuildTMATraffic(*f, auth.ReceivedAt)
			if !tma[f.Callsign] && f.State != aman.StatePlanned && f.State != aman.StateLanded && f.State != aman.StateRemoved && f.SequenceDisposition.Participates() &&
				(retainedTimingUnavailable(*f) || !currentRebuildPrediction(*f, auth.ReceivedAt) || f.Prediction.RawTETA.IsZero()) {
				f.Prediction = nil
				diagnostics = append(diagnostics, aman.RebuildDiagnostic{Code: "current_prediction_unavailable", Callsign: f.Callsign, Message: "A current position-based estimate is unavailable; no slot is assigned"})
			}
		}
		input := s.sequenceInput(state)
		included := make(map[aman.Callsign]bool)
		for i := range input.Flights {
			f := &input.Flights[i]
			f.RebuildTMA = tma[f.Callsign]
			included[f.Callsign] = true
		}
		// TMA traffic gets an immediate landing queue even when its old
		// prediction is missing. No old ETA is needed to reserve these slots.
		for _, f := range state.Flights {
			if !tma[f.Callsign] || included[f.Callsign] || !f.SequenceDisposition.Participates() || f.SelectedRunwayGroup == nil ||
				f.State == aman.StatePlanned || f.State == aman.StateLanded || f.State == aman.StateRemoved {
				continue
			}
			wake := ""
			if f.LatestObservation != nil {
				wake = stringValue(f.LatestObservation.WakeCategory)
			}
			input.Flights = append(input.Flights, sequence.Flight{Callsign: f.Callsign, RunwayGroupID: *f.SelectedRunwayGroup, State: f.State,
				OperationalTETA: auth.ReceivedAt, WakeCategory: sequence.WakeCategory(wake), STARFamily: f.STARFamilyIdentity(), RebuildTMA: true})
		}
		for i := range input.Flights {
			f := &input.Flights[i]
			source := state.Flights[flightIndex(state.Flights, f.Callsign)]
			if f.RebuildTMA {
				f.OperationalTETA = auth.ReceivedAt
				if source.Prediction != nil && currentRebuildPrediction(source, auth.ReceivedAt) && !source.Prediction.RawTETA.IsZero() {
					// Physical estimates order the TMA queue, never delay its slots.
					f.OperationalTETA = source.Prediction.RawTETA
				}
				f.SlotNotBefore, f.PromotionNotBefore = nil, nil
				continue
			}
			// Rebuild from physical timing, without smoothing or prior captures.
			f.OperationalTETA = source.Prediction.RawTETA
			if floor := auth.ReceivedAt.Add(time.Second); floor.After(f.OperationalTETA) {
				f.OperationalTETA = floor
			}
			if e := aman.ActiveHoldingEpisode(source); e != nil {
				f.HoldingStartedAt = e.StartedAt
				f.HoldingStackID = e.HoldingID
				if source.HoldingClearance != nil && source.HoldingClearance.ClearedAltitude != nil && source.DataStatus == aman.DataFresh {
					level := int(*source.HoldingClearance.ClearedAltitude)
					f.HoldingAltitudeFeet = &level
				}
				if activeHoldingReleaseBasis(source) == nil && source.Prediction != nil {
					earliest := source.Prediction.RawTETA.Add(time.Second)
					f.SlotNotBefore = &earliest
				}
			} else if source.HoldingEpisode != nil {
				// An approach without entry evidence or an explicitly ended hold
				// cannot keep reserving a stack through its geometric footprint.
				f.HoldingStackID = ""
			}
			if f.HoldingStartedAt == nil || activeHoldingReleaseBasis(source) == nil {
				earliest := f.OperationalTETA
				if floor := auth.ReceivedAt.Add(time.Second); floor.After(earliest) {
					earliest = floor
				}
				f.SlotNotBefore = &earliest
			} else if source.Prediction.HoldingFixETA != nil {
				// A holding aircraft on an outbound leg cannot start the published
				// fix-to-runway journey before it can reach the fix again.
				earliest := source.Prediction.HoldingFixETA.Add(f.HoldingTransit)
				if f.SlotNotBefore == nil || earliest.After(*f.SlotNotBefore) {
					f.SlotNotBefore = &earliest
				}
			}
		}
		decision, err := sequence.ApplyArrivalSequenceRebuild(input, auth.ReceivedAt)
		if err != nil {
			return sequence.CommandChange{}, err
		}
		decision.Outcome.Diagnostics = append(decision.Outcome.Diagnostics, diagnostics...)
		state = s.applyDecision(state, decision.Decision)
		for i := range state.Flights {
			f := &state.Flights[i]
			old := before.Flights[i]
			if f.Slot != nil && old.Slot != nil && f.Slot.Time.Equal(old.Slot.Time) && f.Slot.Sequence == old.Slot.Sequence && f.Slot.RunwayGroupID == old.Slot.RunwayGroupID && f.Slot.Reason == old.Slot.Reason {
				f.Slot = old.Slot
			}
			ranked := inputFor(decision.Input, f.Callsign)
			if ranked.Callsign != "" && f.Slot != nil {
				for _, entry := range decision.Candidate.Entries {
					if entry.Callsign == f.Callsign {
						refreshRebuiltTiming(f, old, ranked, entry.OperationalTETA, auth.ReceivedAt)
						break
					}
				}
			}
			if ranked.RebuildTMA && f.FreezeReason == aman.FreezeTMA {
				if f.TMAEntry != nil {
					entry := *f.TMAEntry
					entry.FreezeTriggered = true
					f.TMAEntry = &entry
				}
			}
			f.HoldingReleaseReservation = nil
			if ranked.ReleaseRank != nil && f.HoldingClearance != nil && f.HoldingClearance.ClearedAltitude != nil {
				if e := aman.ActiveHoldingEpisode(*f); e != nil {
					f.HoldingReleaseReservation = &aman.HoldingReleaseReservation{CommandID: command.Metadata.CommandID, EpisodeID: e.ID, HoldingID: e.HoldingID, Level: int(*f.HoldingClearance.ClearedAltitude), Rank: *ranked.ReleaseRank}
					if previous := old.HoldingReleaseReservation; previous != nil && previous.EpisodeID == e.ID && previous.Level == f.HoldingReleaseReservation.Level && previous.Rank == *ranked.ReleaseRank {
						f.HoldingReleaseReservation = previous
					}
					if e.Source == "estimated_detection" {
						decision.Outcome.Diagnostics = append(decision.Outcome.Diagnostics, aman.RebuildDiagnostic{Code: "estimated_holding_start", Callsign: f.Callsign, Message: "Holding entry was not recorded; duration starts at a confirmed detection"})
					}
				}
			}
		}
		newOrigin := false
		for _, f := range state.Flights {
			if f.HoldingReleaseReservation != nil && f.HoldingReleaseReservation.CommandID == command.Metadata.CommandID {
				newOrigin = true
			}
		}
		if newOrigin {
			for i := range state.Flights {
				if r := state.Flights[i].HoldingReleaseReservation; r != nil {
					copy := *r
					copy.CommandID = command.Metadata.CommandID
					state.Flights[i].HoldingReleaseReservation = &copy
				}
			}
		}
		s.refreshHoldingPlans(&state)
		changed := !statesEqual(before, state)
		if changed {
			for i := range state.Flights {
				if r := state.Flights[i].HoldingReleaseReservation; r != nil {
					copy := *r
					copy.CommandID = command.Metadata.CommandID
					state.Flights[i].HoldingReleaseReservation = &copy
				}
			}
		}
		if !changed {
			state = before
		}
		sort.Slice(decision.Outcome.Diagnostics, func(i, j int) bool {
			a, b := decision.Outcome.Diagnostics[i], decision.Outcome.Diagnostics[j]
			if a.Callsign != b.Callsign {
				return a.Callsign < b.Callsign
			}
			return a.Code < b.Code
		})
		outcome, err := json.Marshal(decision.Outcome)
		if err != nil {
			return sequence.CommandChange{}, err
		}
		overrides := []map[string]any{}
		for i, f := range state.Flights {
			old := before.Flights[i]
			if (old.FreezeReason == aman.FreezeSuperstable || old.FreezeReason == aman.FreezeTMA) && f.FrozenSlot != nil && (old.FrozenSlot == nil || !old.FrozenSlot.Time.Equal(f.FrozenSlot.Time) || old.FrozenOperationalTETA == nil || !old.FrozenOperationalTETA.Equal(*f.FrozenOperationalTETA)) {
				overrides = append(overrides, map[string]any{"callsign": f.Callsign, "before_capture": old.FrozenSlot, "after_capture": f.FrozenSlot, "before_teta": old.FrozenOperationalTETA, "after_teta": f.FrozenOperationalTETA})
			}
		}
		audit, err := json.Marshal(map[string]any{"automatic_capture_overrides": overrides, "terminal_config_version": s.deps.Terminal.ConfigVersion, "action": "rebuild_arrival_sequence", "actor": auth.Actor, "role": auth.Role, "received_at": auth.ReceivedAt, "policy_version": state.PolicyVersion, "movements": decision.Candidate.Movements, "release_order": decision.Outcome.ReleaseOrder, "diagnostics": decision.Outcome.Diagnostics})
		if err != nil {
			return sequence.CommandChange{}, err
		}
		change, err := s.commandChange(state, changed, "rebuild_arrival_sequence", "", nil)
		change.Outcome = outcome
		change.Audit = []sequence.AuditEntry{{Category: "aman.rebuild_arrival_sequence", Payload: audit}}
		return change, err
	}, nil
}

// Aircraft already in approach receive the immediate TMA landing queue.
// A published holding pattern can lie inside the TMA; actual holding evidence
// keeps those aircraft in the rebuild's release queue.
func rebuildTMATraffic(f aman.AMANFlight, at time.Time) bool {
	return f.TMAEntry != nil && f.TMAEntry.LastContainment == aman.TMAInside &&
		!f.TMAEntry.LastObservedAt.After(at) &&
		aman.ActiveHoldingEpisode(f) == nil && aman.ConfirmedActiveHoldingSince(f) == nil
}

func inputFor(input sequence.Input, id aman.Callsign) sequence.Flight {
	for _, f := range input.Flights {
		if f.Callsign == id {
			return f
		}
	}
	return sequence.Flight{}
}

func currentRebuildPrediction(flight aman.AMANFlight, now time.Time) bool {
	p := flight.Prediction
	if p == nil || p.InputObservedAt.IsZero() || p.InputObservedAt.After(now) || now.Sub(p.InputObservedAt) > tmaSurveillanceFresh {
		return false
	}
	if o := flight.LatestObservation; o != nil {
		if o.Missing || o.SourceStatus != aman.DataFresh {
			return false
		}
		if o.Surveillance != nil && o.Surveillance.ObservedAt != nil && o.Surveillance.ObservedAt.UTC().Truncate(time.Second).After(p.InputObservedAt) {
			return false
		}
	}
	return true
}

func canRefreshRebuildPrediction(flight aman.AMANFlight, now time.Time) bool {
	if flight.State == aman.StatePlanned || flight.State == aman.StateLanded || flight.State == aman.StateRemoved || !flight.SequenceDisposition.Participates() {
		return false
	}
	o := flight.LatestObservation
	return o != nil && !o.Missing && o.SourceStatus == aman.DataFresh && o.Surveillance != nil && o.Surveillance.ObservedAt != nil && !o.Surveillance.ObservedAt.After(now) && now.Sub(*o.Surveillance.ObservedAt) <= tmaSurveillanceFresh
}

func refreshRebuiltTiming(flight *aman.AMANFlight, before aman.AMANFlight, input sequence.Flight, estimate time.Time, at time.Time) {
	if input.ReleaseRank != nil || input.RebuildTMA {
		// The landing estimate now includes the newly allocated holding wait.
		estimate = flight.Slot.Time
	}
	reason := aman.OperationalReasonPredicted
	freeze := aman.FreezeNone
	if input.RebuildTMA || before.FreezeReason == aman.FreezeTMA {
		freeze, reason = aman.FreezeTMA, aman.OperationalReasonTMAFreeze
	} else if before.FreezeReason == aman.FreezeSuperstable {
		freeze, reason = aman.FreezeSuperstable, aman.OperationalReasonSuperstableFreeze
	}
	if flight.Prediction != nil {
		p := *flight.Prediction
		p.OperationalTETA, p.OperationalReason = estimate, reason
		flight.Prediction = &p
		flight.RawTETASamples = []aman.RawTETASample{{TETA: p.RawTETA, GeneratedAt: p.GeneratedAt}}
	}
	flight.FreezeReason = freeze
	if freeze == aman.FreezeNone {
		return
	}
	// Preserve capture provenance only for a semantic no-op. A changed ETA or
	// slot reason requires a fresh capture even if the landing time is unchanged.
	if before.FreezeReason == freeze && before.FrozenOperationalTETA != nil && before.FrozenOperationalTETA.Equal(estimate) && before.FrozenSlot != nil && before.FrozenSlot.Time.Equal(flight.Slot.Time) && before.FrozenSlot.Sequence == flight.Slot.Sequence && before.FrozenSlot.RunwayGroupID == flight.Slot.RunwayGroupID && before.FrozenSlot.Reason == flight.Slot.Reason {
		flight.FrozenAt, flight.FrozenOperationalTETA, flight.FrozenSlot = before.FrozenAt, before.FrozenOperationalTETA, before.FrozenSlot
		return
	}
	captured := *flight.Slot
	flight.FrozenAt, flight.FrozenOperationalTETA, flight.FrozenSlot = &at, &estimate, &captured
}

// Clear the old release ordering when a controller changes its sequence.
func invalidateHoldingRebuild(state *aman.AirportState, id aman.Callsign) {
	index := flightIndex(state.Flights, id)
	if index < 0 {
		return
	}
	reservation := state.Flights[index].HoldingReleaseReservation
	if reservation == nil {
		return
	}
	for i := range state.Flights {
		if r := state.Flights[i].HoldingReleaseReservation; r != nil && r.CommandID == reservation.CommandID {
			state.Flights[i].HoldingReleaseReservation = nil
		}
	}
}

// resetRebuiltTiming discards the previous plan while keeping observations,
// routes and physical holding evidence needed to calculate its replacement.
func resetRebuiltTiming(f *aman.AMANFlight) {
	f.Slot, f.Order, f.ManualOrder, f.QueueOffers = nil, nil, nil, nil
	f.FreezeReason, f.FrozenAt, f.FrozenOperationalTETA, f.FrozenSlot = aman.FreezeNone, nil, nil, nil
	f.HoldingReleaseReservation, f.ApproachRelease, f.RunwayGapException = nil, nil, nil
	f.ETAReview, f.PredictionFilter, f.RawTETASamples = nil, nil, nil
	// Route timing and passed-fix facts describe physical progress, not the
	// discarded plan. Remove manual/holding clocks and expose that evidence.
	feeder := f.DerivedFeederETA
	if feeder == nil {
		feeder = f.FeederETA
	}
	if feeder != nil && feeder.Source != aman.FeederETASourceRoute && feeder.Source != aman.FeederETASourcePassed {
		feeder = nil
	}
	f.FeederETA = nil
	applyDerivedFeederETA(f, feeder)
	if f.Prediction != nil {
		p := *f.Prediction
		p.OperationalTETA, p.OperationalReason = p.RawTETA, aman.OperationalReasonPredicted
		p.HoldingPlan, p.HoldingPlanBlockedBy = nil, nil
		f.Prediction = &p
	}
}
