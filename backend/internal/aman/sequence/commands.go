package sequence

import (
	"fmt"
	"strings"
	"time"

	"FlightStrips/internal/aman"
)

// MoveFlightCommand moves one unfrozen flight immediately before or after one
// anchor. The transport layer derives metadata; policy validates it against
// the supplied point-in-time input.
type MoveFlightCommand struct {
	Metadata       aman.CommandMetadata
	FlightID       aman.FlightID
	RunwayGroupID  aman.RunwayGroupID
	BeforeFlightID *aman.FlightID
	AfterFlightID  *aman.FlightID
}

type ApplyManualFreezeCommand struct {
	Metadata aman.CommandMetadata
	FlightID aman.FlightID
	At       time.Time
}

type ReleaseManualFreezeCommand struct {
	Metadata aman.CommandMetadata
	FlightID aman.FlightID
	At       time.Time
}

type SetRateCommand struct {
	Metadata        aman.CommandMetadata
	RunwayGroupID   aman.RunwayGroupID
	ArrivalsPerHour uint32
	EffectiveAt     time.Time
}

type ApplyGoAroundCommand struct {
	Metadata   aman.CommandMetadata
	FlightID   aman.FlightID
	DetectedAt time.Time
}

type GoAroundPolicy struct {
	Delay      time.Duration
	MaxCascade int
}

func (c MoveFlightCommand) Validate(revision aman.SequenceRevision) error {
	if err := validateMetadata(c.Metadata, revision); err != nil {
		return err
	}
	if !validID(string(c.FlightID)) || !validID(string(c.RunwayGroupID)) {
		return invalidArgument("move flight and runway group are required")
	}
	if (c.BeforeFlightID == nil) == (c.AfterFlightID == nil) {
		return invalidArgument("move requires exactly one before or after anchor")
	}
	anchor := c.BeforeFlightID
	if anchor == nil {
		anchor = c.AfterFlightID
	}
	if !validID(string(*anchor)) || *anchor == c.FlightID {
		return invalidArgument("move anchor must identify another flight")
	}
	return nil
}

func (c ApplyManualFreezeCommand) Validate(revision aman.SequenceRevision) error {
	return validateFlightAction(c.Metadata, revision, c.FlightID, c.At)
}

func (c ReleaseManualFreezeCommand) Validate(revision aman.SequenceRevision) error {
	return validateFlightAction(c.Metadata, revision, c.FlightID, c.At)
}

func (c SetRateCommand) Validate(revision aman.SequenceRevision) error {
	if err := validateMetadata(c.Metadata, revision); err != nil {
		return err
	}
	if !validID(string(c.RunwayGroupID)) || c.ArrivalsPerHour == 0 || !validUTC(c.EffectiveAt) {
		return invalidArgument("rate requires a runway group, positive rate, and UTC effective time")
	}
	return nil
}

func (c ApplyGoAroundCommand) Validate(revision aman.SequenceRevision) error {
	return validateFlightAction(c.Metadata, revision, c.FlightID, c.DetectedAt)
}

func (p GoAroundPolicy) Validate() error {
	if p.Delay <= 0 || p.MaxCascade < 1 {
		return invalidArgument("go-around delay and cascade bound must be positive")
	}
	return nil
}

func validateMetadata(metadata aman.CommandMetadata, revision aman.SequenceRevision) error {
	if !validID(metadata.CommandID) {
		return invalidArgument("command ID is required")
	}
	if metadata.ExpectedRevision != revision {
		return &aman.DomainError{Class: aman.ErrorRevisionConflict, Message: fmt.Sprintf("expected revision %d does not match current revision %d", metadata.ExpectedRevision, revision)}
	}
	return nil
}

func validateFlightAction(metadata aman.CommandMetadata, revision aman.SequenceRevision, flightID aman.FlightID, at time.Time) error {
	if err := validateMetadata(metadata, revision); err != nil {
		return err
	}
	if !validID(string(flightID)) || !validUTC(at) {
		return invalidArgument("flight action requires a flight and UTC time")
	}
	return nil
}

func validID(value string) bool { return value != "" && strings.TrimSpace(value) == value }

func invalidArgument(message string) error {
	return &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: "sequence policy: " + message}
}

func invalidTransition(message string) error {
	return &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "sequence policy: " + message}
}

func notFound(message string) error {
	return &aman.DomainError{Class: aman.ErrorNotFound, Message: "sequence policy: " + message}
}
