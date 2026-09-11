package aman

import (
	"slices"
	"sort"
	"strings"
	"time"
)

// RunwayGapID is the stable audit identity of a persisted runway GAP.
type RunwayGapID string

// RunwayGap is a runway-owned interval during which normal landing capacity is
// unavailable. Its interval is start-inclusive and end-exclusive: [Start, End).
//
// This type records only the accepted absolute interval. Input normalization,
// overlap handling, sequencing effects, and operator commands belong to later
// layers and must not change the persisted interval implicitly.
type RunwayGap struct {
	ID        RunwayGapID
	Start     time.Time
	End       time.Time
	Label     string
	CreatedAt time.Time
	CreatedBy string
}

type RunwayGapException struct {
	GapID         RunwayGapID
	FlightID      FlightID
	RunwayGroupID RunwayGroupID
	Opportunity   time.Time
	CommandID     string
}

func (e RunwayGapException) Validate() error {
	if !isTrimmedNonEmpty(string(e.GapID)) || !isTrimmedNonEmpty(string(e.FlightID)) ||
		!isTrimmedNonEmpty(string(e.RunwayGroupID)) || !isTrimmedNonEmpty(e.CommandID) {
		return invalid("runway gap exception identity is incomplete")
	}
	return requireUTCTime("runway gap exception opportunity", e.Opportunity)
}

// RunwayGapMergeInput describes one accepted GAP before it is combined with
// persisted runway capacity. CommandID becomes the stable identity of the
// resulting union; authorization and command handling remain outside this
// pure domain operation.
type RunwayGapMergeInput struct {
	RunwayGroupID RunwayGroupID
	CommandID     string
	Interval      RunwayGapInterval
	Label         string
	CreatedAt     time.Time
	CreatedBy     string
}

// RunwayGapMergeResult contains an isolated copy of the runway groups and the
// identities removed from persisted state. ReplacedIDs uses canonical interval
// order so an audit payload does not depend on input slice order.
type RunwayGapMergeResult struct {
	RunwayGroups []RunwayGroupPolicy
	Union        RunwayGap
	ReplacedIDs  []RunwayGapID
}

// MergeRunwayGap inserts a GAP and unions every overlapping or touching GAP on
// the same runway group. The accepted input owns the union's label and creation
// provenance; replaced objects contribute only their bounds and audit IDs.
func MergeRunwayGap(groups []RunwayGroupPolicy, input RunwayGapMergeInput) (RunwayGapMergeResult, error) {
	if !isTrimmedNonEmpty(string(input.RunwayGroupID)) {
		return RunwayGapMergeResult{}, invalid("runway gap runway group is required")
	}
	union := RunwayGap{
		ID: RunwayGapID(input.CommandID), Start: input.Interval.Start(), End: input.Interval.End(),
		Label: input.Label, CreatedAt: input.CreatedAt, CreatedBy: input.CreatedBy,
	}
	if err := union.Validate(); err != nil {
		return RunwayGapMergeResult{}, err
	}

	result := RunwayGapMergeResult{RunwayGroups: slices.Clone(groups), Union: union}
	found := false
	for groupIndex := range result.RunwayGroups {
		group := &result.RunwayGroups[groupIndex]
		group.Gaps = slices.Clone(group.Gaps)
		for _, gap := range group.Gaps {
			if gap.ID == union.ID {
				return RunwayGapMergeResult{}, invalid("runway gap command ID already exists")
			}
		}
		if group.ID != input.RunwayGroupID {
			continue
		}
		if found {
			return RunwayGapMergeResult{}, invalid("runway gap runway group is not unique")
		}
		found = true

		sort.Slice(group.Gaps, func(i, j int) bool { return runwayGapLess(group.Gaps[i], group.Gaps[j]) })
		retained := make([]RunwayGap, 0, len(group.Gaps)+1)
		merged := make([]bool, len(group.Gaps))
		for changed := true; changed; {
			changed = false
			for index, gap := range group.Gaps {
				if merged[index] || gap.End.Before(union.Start) || union.End.Before(gap.Start) {
					continue
				}
				merged[index], changed = true, true
				if gap.Start.Before(union.Start) {
					union.Start = gap.Start
				}
				if gap.End.After(union.End) {
					union.End = gap.End
				}
			}
		}
		for index, gap := range group.Gaps {
			if merged[index] {
				result.ReplacedIDs = append(result.ReplacedIDs, gap.ID)
			} else {
				retained = append(retained, gap)
			}
		}
		retained = append(retained, union)
		sort.Slice(retained, func(i, j int) bool { return runwayGapLess(retained[i], retained[j]) })
		group.Gaps = retained
	}
	if !found {
		return RunwayGapMergeResult{}, invalid("runway gap runway group does not exist")
	}
	result.Union = union
	return result, nil
}

// Validate enforces the canonical persisted representation of a runway GAP.
func (g RunwayGap) Validate() error {
	if !isTrimmedNonEmpty(string(g.ID)) {
		return invalid("runway gap ID is required and must be canonical")
	}
	if !isTrimmedNonEmpty(g.Label) {
		return invalid("runway gap label is required and must be canonical")
	}
	if !isTrimmedNonEmpty(g.CreatedBy) {
		return invalid("runway gap creator is required and must be canonical")
	}
	if err := requireUTCTime("runway gap start", g.Start); err != nil {
		return err
	}
	if err := requireUTCTime("runway gap end", g.End); err != nil {
		return err
	}
	if err := requireUTCTime("runway gap creation time", g.CreatedAt); err != nil {
		return err
	}
	if !g.Start.Before(g.End) {
		return invalid("runway gap start must be before end")
	}
	return nil
}

// runwayGapLess defines deterministic persistence, replay, and audit order.
func runwayGapLess(left, right RunwayGap) bool {
	if !left.Start.Equal(right.Start) {
		return left.Start.Before(right.Start)
	}
	if !left.End.Equal(right.End) {
		return left.End.Before(right.End)
	}
	return strings.Compare(string(left.ID), string(right.ID)) < 0
}
