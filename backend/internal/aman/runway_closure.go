package aman

import (
	"strings"
	"time"
)

// RunwayClosureID is the stable audit identity of a persisted runway closure.
type RunwayClosureID string

// RunwayClosure is a runway-owned capacity interval. Start is inclusive and
// End is exclusive; a nil End keeps the runway closed until explicit removal.
// Command input normalization and operational effects are owned by later
// slices and cannot rewrite an accepted interval implicitly.
type RunwayClosure struct {
	ID        RunwayClosureID
	Start     time.Time
	End       *time.Time
	Reason    string
	CreatedAt time.Time
	CreatedBy string
}

// Validate enforces the canonical persisted representation of a closure.
func (c RunwayClosure) Validate() error {
	if !isTrimmedNonEmpty(string(c.ID)) {
		return invalid("runway closure ID is required and must be canonical")
	}
	if !isTrimmedNonEmpty(c.Reason) {
		return invalid("runway closure reason is required and must be canonical")
	}
	if !isTrimmedNonEmpty(c.CreatedBy) {
		return invalid("runway closure creator is required and must be canonical")
	}
	if err := requireUTCTime("runway closure start", c.Start); err != nil {
		return err
	}
	if c.End != nil {
		if err := requireUTCTime("runway closure end", *c.End); err != nil {
			return err
		}
		if !c.Start.Before(*c.End) {
			return invalid("runway closure start must be before end")
		}
	}
	return requireUTCTime("runway closure creation time", c.CreatedAt)
}

// runwayClosureLess defines deterministic persistence and replay order.
func runwayClosureLess(left, right RunwayClosure) bool {
	if !left.Start.Equal(right.Start) {
		return left.Start.Before(right.Start)
	}
	if left.End == nil || right.End == nil {
		if left.End != nil {
			return true
		}
		if right.End != nil {
			return false
		}
	} else if !left.End.Equal(*right.End) {
		return left.End.Before(*right.End)
	}
	return strings.Compare(string(left.ID), string(right.ID)) < 0
}
