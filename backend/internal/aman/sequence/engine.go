// Package sequence generates deterministic, uncommitted AMAN runway-group
// slot candidates. Persistence, revision allocation, and command policy live
// outside this package.
package sequence

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"FlightStrips/internal/aman"
)

// WakeCategory is an airport-policy category used for directional spacing.
// Values are normalized to upper case. Categories absent from a runway-group
// policy are sequenced with that policy's conservative unknown separation.
type WakeCategory string

const WakeUnknown WakeCategory = "UNKNOWN"

// RatePoint changes a runway group's slot interval at EffectiveAt. Each point
// starts a new grid at its exact effective instant.
type RatePoint struct {
	EffectiveAt     time.Time
	ArrivalsPerHour uint32
}

// SeparationRule is the minimum directional spacing from a leading category
// to a trailing category. A policy must provide the complete matrix for every
// category named by its rules.
type SeparationRule struct {
	Leading  WakeCategory
	Trailing WakeCategory
	Minimum  time.Duration
}

// Policy contains all runway-group-local inputs needed by the pure engine.
type Policy struct {
	RunwayGroupID     aman.RunwayGroupID
	Rates             []RatePoint
	EarlyTolerance    time.Duration
	SeparationRules   []SeparationRule
	UnknownSeparation time.Duration
}

// Flight is the narrow sequencing view of an AMAN flight. CapturedSlot is a
// protected slot reference supplied by freeze/manual policy; CurrentSlot is
// used only to report movements and never influences candidate generation.
type Flight struct {
	ID                  aman.FlightID
	RunwayGroupID       aman.RunwayGroupID
	State               aman.FlightState
	OperationalTETA     time.Time
	InitialBaselineTETA *time.Time
	WakeCategory        WakeCategory
	ManualOrder         *int
	FreezeReason        aman.FreezeReason
	CapturedSlot        *aman.Slot
	CurrentSlot         *aman.Slot
}

// Input is a complete, point-in-time pure sequence calculation.
type Input struct {
	Policies []Policy
	Flights  []Flight
}

// CandidateReason explains how a candidate slot was selected.
type CandidateReason string

const (
	ReasonRateWTC           CandidateReason = "rate_wtc"
	ReasonFreezeSuperstable CandidateReason = "freeze_superstable"
	ReasonFreezeManual      CandidateReason = "freeze_manual"
)

// CandidateEntry is an uncommitted slot candidate. It deliberately contains
// no sequence revision; the transaction coordinator owns revision allocation.
type CandidateEntry struct {
	FlightID        aman.FlightID
	RunwayGroupID   aman.RunwayGroupID
	Sequence        int
	Time            time.Time
	OperationalTETA time.Time
	WakeCategory    WakeCategory
	FreezeReason    aman.FreezeReason
	Protected       bool
	Reason          CandidateReason
}

// SlotMovement describes a candidate difference from the supplied current
// slot. Nil From values mean this is a new assignment.
type SlotMovement struct {
	FlightID      aman.FlightID
	RunwayGroupID aman.RunwayGroupID
	FromTime      *time.Time
	FromSequence  *int
	ToTime        time.Time
	ToSequence    int
}

// WarningSeverity distinguishes degraded but usable output from a policy
// conflict that a caller must not commit without explicit policy resolution.
type WarningSeverity string

const (
	SeverityDegraded WarningSeverity = "degraded"
	SeverityConflict WarningSeverity = "conflict"
)

// WarningCode is stable, machine-readable sequence output.
type WarningCode string

const (
	WarningUnknownWakeCategory  WarningCode = "unknown_wake_category"
	WarningProtectedSlotMissing WarningCode = "protected_slot_missing"
	WarningProtectedSlotInvalid WarningCode = "protected_slot_invalid"
	WarningProtectedSpacing     WarningCode = "protected_spacing_conflict"
)

// Warning reports degraded input or an unsatisfied protected constraint.
type Warning struct {
	Severity        WarningSeverity
	Code            WarningCode
	RunwayGroupID   aman.RunwayGroupID
	FlightID        aman.FlightID
	RelatedFlightID *aman.FlightID
}

// Result is ordered canonically by runway group, slot, and flight ID. It uses
// slices rather than maps so equivalent inputs marshal to byte-identical JSON.
type Result struct {
	Entries   []CandidateEntry
	Movements []SlotMovement
	Warnings  []Warning
}

// HasConflicts reports whether the result contains a protected-policy
// conflict. Degraded unknown-category output remains usable.
func (r Result) HasConflicts() bool {
	return slices.ContainsFunc(r.Warnings, func(w Warning) bool { return w.Severity == SeverityConflict })
}

type preparedPolicy struct {
	Policy
	rates      []RatePoint
	spacing    map[categoryPair]time.Duration
	categories map[WakeCategory]struct{}
	fallback   time.Duration
}

type categoryPair struct{ leading, trailing WakeCategory }

type preparedFlight struct {
	Flight
	category WakeCategory
	known    bool
}

type allocatedEntry struct {
	flight preparedFlight
	time   time.Time
	reason CandidateReason
}

// Generate calculates a candidate sequence without mutating input or
// allocating a committed revision.
func Generate(input Input) (Result, error) {
	policies, err := preparePolicies(input.Policies)
	if err != nil {
		return Result{}, err
	}
	flights, err := prepareFlights(input.Flights, policies)
	if err != nil {
		return Result{}, err
	}

	result := Result{Entries: []CandidateEntry{}, Movements: []SlotMovement{}, Warnings: []Warning{}}
	groupIDs := make([]aman.RunwayGroupID, 0, len(policies))
	for groupID := range policies {
		groupIDs = append(groupIDs, groupID)
	}
	sort.Slice(groupIDs, func(i, j int) bool { return groupIDs[i] < groupIDs[j] })

	for _, groupID := range groupIDs {
		groupFlights := slices.Clone(flights[groupID])
		entries, warnings, err := generateGroup(policies[groupID], groupFlights)
		if err != nil {
			return Result{}, err
		}
		result.Warnings = append(result.Warnings, warnings...)
		for index, entry := range entries {
			sequence := index + 1
			candidate := CandidateEntry{
				FlightID: entry.flight.ID, RunwayGroupID: groupID, Sequence: sequence,
				Time: entry.time, OperationalTETA: entry.flight.OperationalTETA,
				WakeCategory: entry.flight.category, FreezeReason: entry.flight.FreezeReason,
				Protected: entry.flight.FreezeReason != aman.FreezeNone, Reason: entry.reason,
			}
			result.Entries = append(result.Entries, candidate)
			if movement := movementFor(entry.flight, candidate); movement != nil {
				result.Movements = append(result.Movements, *movement)
			}
		}
	}

	sortWarnings(result.Warnings)
	return result, nil
}

func preparePolicies(input []Policy) (map[aman.RunwayGroupID]preparedPolicy, error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("sequence requires at least one runway-group policy")
	}
	result := make(map[aman.RunwayGroupID]preparedPolicy, len(input))
	for _, raw := range input {
		if strings.TrimSpace(string(raw.RunwayGroupID)) == "" {
			return nil, fmt.Errorf("runway-group policy ID is required")
		}
		if _, duplicate := result[raw.RunwayGroupID]; duplicate {
			return nil, fmt.Errorf("duplicate runway-group policy %q", raw.RunwayGroupID)
		}
		if raw.EarlyTolerance < 0 {
			return nil, fmt.Errorf("runway group %q has negative early tolerance", raw.RunwayGroupID)
		}
		if raw.UnknownSeparation <= 0 {
			return nil, fmt.Errorf("runway group %q requires positive unknown-category separation", raw.RunwayGroupID)
		}

		prepared := preparedPolicy{Policy: raw, rates: slices.Clone(raw.Rates), spacing: map[categoryPair]time.Duration{}, categories: map[WakeCategory]struct{}{}, fallback: raw.UnknownSeparation}
		if len(prepared.rates) == 0 {
			return nil, fmt.Errorf("runway group %q requires at least one rate", raw.RunwayGroupID)
		}
		sort.Slice(prepared.rates, func(i, j int) bool { return prepared.rates[i].EffectiveAt.Before(prepared.rates[j].EffectiveAt) })
		for index, rate := range prepared.rates {
			if !validUTC(rate.EffectiveAt) || rate.ArrivalsPerHour == 0 {
				return nil, fmt.Errorf("runway group %q has invalid rate point", raw.RunwayGroupID)
			}
			if index > 0 && rate.EffectiveAt.Equal(prepared.rates[index-1].EffectiveAt) {
				return nil, fmt.Errorf("runway group %q has duplicate rate effective time", raw.RunwayGroupID)
			}
		}

		if len(raw.SeparationRules) == 0 {
			return nil, fmt.Errorf("runway group %q requires WTC separation rules", raw.RunwayGroupID)
		}
		for _, rule := range raw.SeparationRules {
			leading, trailing := normalizeCategory(rule.Leading), normalizeCategory(rule.Trailing)
			if leading == WakeUnknown || trailing == WakeUnknown || rule.Minimum < 0 {
				return nil, fmt.Errorf("runway group %q has invalid WTC separation rule", raw.RunwayGroupID)
			}
			pair := categoryPair{leading, trailing}
			if _, duplicate := prepared.spacing[pair]; duplicate {
				return nil, fmt.Errorf("runway group %q has duplicate WTC separation rule", raw.RunwayGroupID)
			}
			prepared.spacing[pair] = rule.Minimum
			prepared.categories[leading] = struct{}{}
			prepared.categories[trailing] = struct{}{}
			if rule.Minimum > prepared.fallback {
				prepared.fallback = rule.Minimum
			}
		}
		for leading := range prepared.categories {
			for trailing := range prepared.categories {
				if _, ok := prepared.spacing[categoryPair{leading, trailing}]; !ok {
					return nil, fmt.Errorf("runway group %q has incomplete WTC separation matrix", raw.RunwayGroupID)
				}
			}
		}
		result[raw.RunwayGroupID] = prepared
	}
	return result, nil
}

func prepareFlights(input []Flight, policies map[aman.RunwayGroupID]preparedPolicy) (map[aman.RunwayGroupID][]preparedFlight, error) {
	result := make(map[aman.RunwayGroupID][]preparedFlight, len(policies))
	seen := make(map[aman.FlightID]struct{}, len(input))
	for _, raw := range input {
		if strings.TrimSpace(string(raw.ID)) == "" {
			return nil, fmt.Errorf("sequence flight ID is required")
		}
		if _, duplicate := seen[raw.ID]; duplicate {
			return nil, fmt.Errorf("duplicate sequence flight %q", raw.ID)
		}
		seen[raw.ID] = struct{}{}
		policy, ok := policies[raw.RunwayGroupID]
		if !ok {
			return nil, fmt.Errorf("flight %q references unknown runway group %q", raw.ID, raw.RunwayGroupID)
		}
		if !raw.State.Valid() || !raw.FreezeReason.Valid() || !validUTC(raw.OperationalTETA) {
			return nil, fmt.Errorf("flight %q has invalid sequencing state", raw.ID)
		}
		if raw.InitialBaselineTETA != nil && !validUTC(*raw.InitialBaselineTETA) {
			return nil, fmt.Errorf("flight %q has invalid initial baseline TETA", raw.ID)
		}
		if raw.ManualOrder != nil && *raw.ManualOrder < 1 {
			return nil, fmt.Errorf("flight %q has invalid manual order", raw.ID)
		}
		if raw.CurrentSlot != nil {
			if !validUTC(raw.CurrentSlot.Time) || raw.CurrentSlot.Sequence < 1 || raw.CurrentSlot.RunwayGroupID != raw.RunwayGroupID {
				return nil, fmt.Errorf("flight %q has invalid current slot", raw.ID)
			}
		}
		if raw.FreezeReason == aman.FreezeNone && raw.CapturedSlot != nil {
			return nil, fmt.Errorf("flight %q has captured slot without freeze reason", raw.ID)
		}
		category := normalizeCategory(raw.WakeCategory)
		_, known := policy.categories[category]
		if !known {
			category = WakeUnknown
		}
		result[raw.RunwayGroupID] = append(result[raw.RunwayGroupID], preparedFlight{Flight: raw, category: category, known: known})
	}
	return result, nil
}

func generateGroup(policy preparedPolicy, flights []preparedFlight) ([]allocatedEntry, []Warning, error) {
	warnings := []Warning{}
	protected := []preparedFlight{}
	movable := []preparedFlight{}
	for _, flight := range flights {
		if !flight.known {
			warnings = append(warnings, Warning{Severity: SeverityDegraded, Code: WarningUnknownWakeCategory, RunwayGroupID: policy.RunwayGroupID, FlightID: flight.ID})
		}
		if flight.State == aman.StateLanded || flight.State == aman.StateRemoved {
			continue
		}
		if flight.FreezeReason == aman.FreezeNone {
			movable = append(movable, flight)
		} else {
			protected = append(protected, flight)
		}
	}

	sort.Slice(protected, func(i, j int) bool { return flightLess(protected[i], protected[j]) })
	entries := []allocatedEntry{}
	for _, flight := range protected {
		if flight.CapturedSlot == nil {
			warnings = append(warnings, Warning{Severity: SeverityConflict, Code: WarningProtectedSlotMissing, RunwayGroupID: policy.RunwayGroupID, FlightID: flight.ID})
			continue
		}
		if !validUTC(flight.CapturedSlot.Time) || flight.CapturedSlot.RunwayGroupID != policy.RunwayGroupID || flight.CapturedSlot.Sequence < 1 || policy.intervalAt(flight.CapturedSlot.Time) == 0 {
			warnings = append(warnings, Warning{Severity: SeverityConflict, Code: WarningProtectedSlotInvalid, RunwayGroupID: policy.RunwayGroupID, FlightID: flight.ID})
			continue
		}
		reason := ReasonFreezeManual
		if flight.FreezeReason == aman.FreezeSuperstable {
			reason = ReasonFreezeSuperstable
		}
		entries = append(entries, allocatedEntry{flight: flight, time: flight.CapturedSlot.Time, reason: reason})
	}
	sortEntries(entries)
	for index := 1; index < len(entries); index++ {
		leading, trailing := entries[index-1], entries[index]
		if !adjacentValid(policy, leading, trailing) {
			related := leading.flight.ID
			warnings = append(warnings, Warning{Severity: SeverityConflict, Code: WarningProtectedSpacing, RunwayGroupID: policy.RunwayGroupID, FlightID: trailing.flight.ID, RelatedFlightID: &related})
		}
	}

	sort.Slice(movable, func(i, j int) bool { return flightLess(movable[i], movable[j]) })
	for _, flight := range movable {
		candidate, err := findCandidate(policy, entries, flight)
		if err != nil {
			return nil, nil, err
		}
		entries = append(entries, allocatedEntry{flight: flight, time: candidate, reason: ReasonRateWTC})
		sortEntries(entries)
	}
	return entries, warnings, nil
}

func findCandidate(policy preparedPolicy, entries []allocatedEntry, flight preparedFlight) (time.Time, error) {
	lower := flight.OperationalTETA.Add(-policy.EarlyTolerance)
	if candidate, ok := previousGridAtOrBefore(policy, flight.OperationalTETA); ok && candidate.Before(flight.OperationalTETA) {
		for !candidate.Before(lower) {
			valid, earlier, _ := placement(policy, entries, flight, candidate)
			if valid {
				return candidate, nil
			}
			nextTarget := candidate.Add(-time.Nanosecond)
			if earlier.Before(nextTarget) {
				nextTarget = earlier
			}
			candidate, ok = previousGridAtOrBefore(policy, nextTarget)
			if !ok {
				break
			}
		}
	}

	candidate, ok := nextGridAtOrAfter(policy, flight.OperationalTETA)
	if !ok {
		return time.Time{}, fmt.Errorf("runway group %q has no slot grid at or after flight %q TETA", policy.RunwayGroupID, flight.ID)
	}
	for attempts := 0; attempts <= len(entries)+1; attempts++ {
		valid, _, later := placement(policy, entries, flight, candidate)
		if valid {
			return candidate, nil
		}
		nextTarget := candidate.Add(time.Nanosecond)
		if later.After(nextTarget) {
			nextTarget = later
		}
		candidate, ok = nextGridAtOrAfter(policy, nextTarget)
		if !ok {
			break
		}
	}
	return time.Time{}, fmt.Errorf("runway group %q could not allocate flight %q", policy.RunwayGroupID, flight.ID)
}

// placement returns whether candidate is valid plus safe bounds for the next
// earlier/later attempt. Crossing an adjacent entry recomputes both directional
// WTC boundaries, so allocation never validates only one side of an insertion.
func placement(policy preparedPolicy, entries []allocatedEntry, flight preparedFlight, candidate time.Time) (bool, time.Time, time.Time) {
	index := sort.Search(len(entries), func(i int) bool { return !entries[i].time.Before(candidate) })
	valid := true
	earlier, later := candidate.Add(-time.Nanosecond), candidate.Add(time.Nanosecond)
	if index > 0 {
		leading := entries[index-1]
		required := requiredGap(policy, leading.flight, flight, candidate)
		if candidate.Sub(leading.time) < required {
			valid = false
			boundEarlier := leading.time.Add(-requiredGap(policy, flight, leading.flight, leading.time))
			if boundEarlier.Before(earlier) {
				earlier = boundEarlier
			}
			boundLater := leading.time.Add(required)
			if boundLater.After(later) {
				later = boundLater
			}
		}
	}
	if index < len(entries) {
		trailing := entries[index]
		required := requiredGap(policy, flight, trailing.flight, trailing.time)
		if trailing.time.Sub(candidate) < required {
			valid = false
			boundEarlier := trailing.time.Add(-required)
			if boundEarlier.Before(earlier) {
				earlier = boundEarlier
			}
			boundLater := trailing.time.Add(requiredGap(policy, trailing.flight, flight, trailing.time))
			if boundLater.After(later) {
				later = boundLater
			}
		}
	}
	return valid, earlier, later
}

func adjacentValid(policy preparedPolicy, leading, trailing allocatedEntry) bool {
	if !trailing.time.After(leading.time) {
		return false
	}
	return trailing.time.Sub(leading.time) >= requiredGap(policy, leading.flight, trailing.flight, trailing.time)
}

func requiredGap(policy preparedPolicy, leading, trailing preparedFlight, trailingAt time.Time) time.Duration {
	wtc := policy.fallback
	if leading.known && trailing.known {
		wtc = policy.spacing[categoryPair{leading.category, trailing.category}]
	}
	base := policy.intervalAt(trailingAt)
	if base > wtc {
		return base
	}
	return wtc
}

func (p preparedPolicy) intervalAt(at time.Time) time.Duration {
	index := sort.Search(len(p.rates), func(i int) bool { return p.rates[i].EffectiveAt.After(at) }) - 1
	if index < 0 {
		return 0
	}
	return rateInterval(p.rates[index].ArrivalsPerHour)
}

func rateInterval(rate uint32) time.Duration {
	nanoseconds := uint64(time.Hour)
	return time.Duration((nanoseconds + uint64(rate) - 1) / uint64(rate))
}

func nextGridAtOrAfter(policy preparedPolicy, target time.Time) (time.Time, bool) {
	for index, rate := range policy.rates {
		if index+1 < len(policy.rates) && !target.Before(policy.rates[index+1].EffectiveAt) {
			continue
		}
		start := rate.EffectiveAt
		if target.Before(start) {
			return start, true
		}
		interval := rateInterval(rate.ArrivalsPerHour)
		delta := target.Sub(start)
		steps := delta / interval
		if delta%interval != 0 {
			steps++
		}
		candidate := start.Add(steps * interval)
		if index+1 == len(policy.rates) || candidate.Before(policy.rates[index+1].EffectiveAt) {
			return candidate, true
		}
	}
	return time.Time{}, false
}

func previousGridAtOrBefore(policy preparedPolicy, target time.Time) (time.Time, bool) {
	index := sort.Search(len(policy.rates), func(i int) bool { return policy.rates[i].EffectiveAt.After(target) }) - 1
	if index < 0 {
		return time.Time{}, false
	}
	rate := policy.rates[index]
	interval := rateInterval(rate.ArrivalsPerHour)
	return rate.EffectiveAt.Add((target.Sub(rate.EffectiveAt) / interval) * interval), true
}

func flightLess(a, b preparedFlight) bool {
	aProtected, bProtected := a.FreezeReason != aman.FreezeNone, b.FreezeReason != aman.FreezeNone
	if aProtected != bProtected {
		return aProtected
	}
	if a.ManualOrder != nil || b.ManualOrder != nil {
		if a.ManualOrder == nil {
			return false
		}
		if b.ManualOrder == nil {
			return true
		}
		if *a.ManualOrder != *b.ManualOrder {
			return *a.ManualOrder < *b.ManualOrder
		}
	}
	if aPriority, bPriority := lifecyclePriority(a.State), lifecyclePriority(b.State); aPriority != bPriority {
		return aPriority < bPriority
	}
	if !a.OperationalTETA.Equal(b.OperationalTETA) {
		return a.OperationalTETA.Before(b.OperationalTETA)
	}
	if a.InitialBaselineTETA != nil || b.InitialBaselineTETA != nil {
		if a.InitialBaselineTETA == nil {
			return false
		}
		if b.InitialBaselineTETA == nil {
			return true
		}
		if !a.InitialBaselineTETA.Equal(*b.InitialBaselineTETA) {
			return a.InitialBaselineTETA.Before(*b.InitialBaselineTETA)
		}
	}
	return a.ID < b.ID
}

func lifecyclePriority(state aman.FlightState) int {
	switch state {
	case aman.StateStable:
		return 0
	case aman.StateUnstable:
		return 1
	case aman.StateAirborne:
		return 2
	case aman.StatePlanned:
		return 3
	case aman.StateGoAround:
		return 4
	default:
		return 5
	}
}

func sortEntries(entries []allocatedEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if !entries[i].time.Equal(entries[j].time) {
			return entries[i].time.Before(entries[j].time)
		}
		return flightLess(entries[i].flight, entries[j].flight)
	})
}

func movementFor(flight preparedFlight, candidate CandidateEntry) *SlotMovement {
	if flight.CurrentSlot != nil && flight.CurrentSlot.Time.Equal(candidate.Time) && flight.CurrentSlot.Sequence == candidate.Sequence {
		return nil
	}
	movement := &SlotMovement{FlightID: flight.ID, RunwayGroupID: candidate.RunwayGroupID, ToTime: candidate.Time, ToSequence: candidate.Sequence}
	if flight.CurrentSlot != nil {
		fromTime, fromSequence := flight.CurrentSlot.Time, flight.CurrentSlot.Sequence
		movement.FromTime, movement.FromSequence = &fromTime, &fromSequence
	}
	return movement
}

func sortWarnings(warnings []Warning) {
	sort.Slice(warnings, func(i, j int) bool {
		a, b := warnings[i], warnings[j]
		if a.RunwayGroupID != b.RunwayGroupID {
			return a.RunwayGroupID < b.RunwayGroupID
		}
		if a.FlightID != b.FlightID {
			return a.FlightID < b.FlightID
		}
		if a.Severity != b.Severity {
			return a.Severity < b.Severity
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return relatedID(a.RelatedFlightID) < relatedID(b.RelatedFlightID)
	})
}

func relatedID(id *aman.FlightID) aman.FlightID {
	if id == nil {
		return ""
	}
	return *id
}

func normalizeCategory(category WakeCategory) WakeCategory {
	normalized := WakeCategory(strings.ToUpper(strings.TrimSpace(string(category))))
	if normalized == "" || normalized == WakeUnknown {
		return WakeUnknown
	}
	return normalized
}

func validUTC(value time.Time) bool {
	return !value.IsZero() && value.Location() == time.UTC
}
