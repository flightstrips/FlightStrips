package aman

import (
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

// runwayGapLess defines persistence and replay order without treating overlap
// or touching intervals as equivalent. GAP unioning is deliberately out of
// scope for the persisted interval model.
func runwayGapLess(left, right RunwayGap) bool {
	if !left.Start.Equal(right.Start) {
		return left.Start.Before(right.Start)
	}
	if !left.End.Equal(right.End) {
		return left.End.Before(right.End)
	}
	return strings.Compare(string(left.ID), string(right.ID)) < 0
}
