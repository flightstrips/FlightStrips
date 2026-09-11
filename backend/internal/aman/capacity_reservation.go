package aman

import (
	"strings"
	"time"
)

const DefaultCapacityReservationLabel = "FLIGHT"

// RunwayCapacityReservationID is the stable identity derived from the command
// that created a reservation. It is not a flight identity.
type RunwayCapacityReservationID string

// RunwayCapacityReservation reserves one normal landing opportunity on its
// owning runway group. Start is inclusive and End is exclusive. The accepted
// absolute interval is persisted as-is; later scheduling code must not rewrite
// it from rates or predictions.
type RunwayCapacityReservation struct {
	ID        RunwayCapacityReservationID
	Start     time.Time
	End       time.Time
	Label     string
	CreatedAt time.Time
	CreatedBy string
}

// NewRunwayCapacityReservation derives the durable identity from commandID and
// applies the sole optional-field default before the model is persisted.
func NewRunwayCapacityReservation(commandID string, start, end time.Time, label string, createdAt time.Time, createdBy string) (RunwayCapacityReservation, error) {
	if label == "" {
		label = DefaultCapacityReservationLabel
	}
	reservation := RunwayCapacityReservation{
		ID: RunwayCapacityReservationID(commandID), Start: start, End: end,
		Label: label, CreatedAt: createdAt, CreatedBy: createdBy,
	}
	if err := reservation.Validate(); err != nil {
		return RunwayCapacityReservation{}, err
	}
	return reservation, nil
}

// Validate enforces the canonical persisted representation of one opportunity.
func (r RunwayCapacityReservation) Validate() error {
	if !isTrimmedNonEmpty(string(r.ID)) {
		return invalid("capacity reservation ID is required and must be canonical")
	}
	if !isTrimmedNonEmpty(r.Label) {
		return invalid("capacity reservation label is required and must be canonical")
	}
	if !isTrimmedNonEmpty(r.CreatedBy) {
		return invalid("capacity reservation creator is required and must be canonical")
	}
	if err := requireUTCTime("capacity reservation start", r.Start); err != nil {
		return err
	}
	if err := requireUTCTime("capacity reservation end", r.End); err != nil {
		return err
	}
	if !r.Start.Before(r.End) {
		return invalid("capacity reservation start must be before end")
	}
	return requireUTCTime("capacity reservation creation time", r.CreatedAt)
}

// runwayCapacityReservationLess defines deterministic persistence and replay
// order without depending on insertion order.
func runwayCapacityReservationLess(left, right RunwayCapacityReservation) bool {
	if !left.Start.Equal(right.Start) {
		return left.Start.Before(right.Start)
	}
	if !left.End.Equal(right.End) {
		return left.End.Before(right.End)
	}
	return strings.Compare(string(left.ID), string(right.ID)) < 0
}
