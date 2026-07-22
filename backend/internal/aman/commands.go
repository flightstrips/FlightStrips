package aman

import (
	"context"
	"strings"
	"time"
)

// CommandContext contains authority and timing facts derived by the server.
// None of these values are accepted from a command payload.
type CommandContext struct {
	Airport    string
	Actor      string
	Role       string
	ReceivedAt time.Time
}

type MoveFlightCommand struct {
	Metadata       CommandMetadata
	FlightID       FlightID
	RunwayGroupID  RunwayGroupID
	BeforeFlightID *FlightID
	AfterFlightID  *FlightID
}

type LockFlightCommand struct {
	Metadata CommandMetadata
	FlightID FlightID
}

type UnlockFlightCommand struct {
	Metadata CommandMetadata
	FlightID FlightID
}

type SetRateCommand struct {
	Metadata        CommandMetadata
	RunwayGroupID   RunwayGroupID
	ArrivalsPerHour uint32
	EffectiveAt     time.Time
}

type AcceptTETACommand struct {
	Metadata CommandMetadata
	FlightID FlightID
}

type KeepFPLETACommand struct {
	Metadata CommandMetadata
	FlightID FlightID
}

type SetManualETACommand struct {
	Metadata  CommandMetadata
	FlightID  FlightID
	ManualETA time.Time
}

type ResetTETAOverrideCommand struct {
	Metadata CommandMetadata
	FlightID FlightID
}

type ReportGoAroundCommand struct {
	Metadata   CommandMetadata
	FlightID   FlightID
	DetectedAt time.Time
}

// CommandExecution is the transport-safe result of coordinator execution.
// Outcome remains opaque because its schema belongs to the command owner.
type CommandExecution struct {
	CurrentRevision SequenceRevision
	Outcome         CommandOutcome
	Changed         bool
	Duplicate       bool
}

// CommandService is deliberately typed per operation. It prevents a transport
// from constructing a kind plus unrelated nullable fields while preserving the
// coordinator as the sole revision and idempotency owner.
type CommandService interface {
	Component
	CurrentRevision(context.Context, string) (SequenceRevision, error)
	MoveFlight(context.Context, CommandContext, MoveFlightCommand) (CommandExecution, error)
	LockFlight(context.Context, CommandContext, LockFlightCommand) (CommandExecution, error)
	UnlockFlight(context.Context, CommandContext, UnlockFlightCommand) (CommandExecution, error)
	SetRate(context.Context, CommandContext, SetRateCommand) (CommandExecution, error)
	AcceptTETA(context.Context, CommandContext, AcceptTETACommand) (CommandExecution, error)
	KeepFPLETA(context.Context, CommandContext, KeepFPLETACommand) (CommandExecution, error)
	SetManualETA(context.Context, CommandContext, SetManualETACommand) (CommandExecution, error)
	ResetTETAOverride(context.Context, CommandContext, ResetTETAOverrideCommand) (CommandExecution, error)
	ReportGoAround(context.Context, CommandContext, ReportGoAroundCommand) (CommandExecution, error)
}

func (c CommandContext) Validate() error {
	if !trimmed(c.Airport) || !trimmed(c.Actor) || !trimmed(c.Role) {
		return commandInvalid("server-derived airport, actor, and role are required")
	}
	if c.ReceivedAt.IsZero() || c.ReceivedAt.Location() != time.UTC {
		return commandInvalid("server receipt time must be UTC")
	}
	return nil
}

func (c MoveFlightCommand) Validate() error {
	if err := validateCommandMetadata(c.Metadata); err != nil {
		return err
	}
	if !trimmed(string(c.FlightID)) || !trimmed(string(c.RunwayGroupID)) {
		return commandInvalid("move flight and runway group are required")
	}
	if (c.BeforeFlightID == nil) == (c.AfterFlightID == nil) {
		return commandInvalid("move requires exactly one before or after anchor")
	}
	anchor := c.BeforeFlightID
	if anchor == nil {
		anchor = c.AfterFlightID
	}
	if !trimmed(string(*anchor)) || *anchor == c.FlightID {
		return commandInvalid("move anchor must identify another flight")
	}
	return nil
}

func (c LockFlightCommand) Validate() error   { return validateFlightCommand(c.Metadata, c.FlightID) }
func (c UnlockFlightCommand) Validate() error { return validateFlightCommand(c.Metadata, c.FlightID) }
func (c AcceptTETACommand) Validate() error   { return validateFlightCommand(c.Metadata, c.FlightID) }
func (c KeepFPLETACommand) Validate() error   { return validateFlightCommand(c.Metadata, c.FlightID) }
func (c ResetTETAOverrideCommand) Validate() error {
	return validateFlightCommand(c.Metadata, c.FlightID)
}

func (c SetRateCommand) Validate() error {
	if err := validateCommandMetadata(c.Metadata); err != nil {
		return err
	}
	if !trimmed(string(c.RunwayGroupID)) || c.ArrivalsPerHour == 0 || !utc(c.EffectiveAt) {
		return commandInvalid("rate requires a runway group, positive rate, and UTC effective time")
	}
	return nil
}

func (c SetManualETACommand) Validate(receivedAt time.Time) error {
	if err := validateFlightCommand(c.Metadata, c.FlightID); err != nil {
		return err
	}
	if !utc(c.ManualETA) || !c.ManualETA.After(receivedAt) {
		return commandInvalid("manual ETA must be a future UTC value")
	}
	return nil
}

func (c ReportGoAroundCommand) Validate(receivedAt time.Time) error {
	if err := validateFlightCommand(c.Metadata, c.FlightID); err != nil {
		return err
	}
	if !utc(c.DetectedAt) || c.DetectedAt.After(receivedAt) {
		return commandInvalid("go-around detected time must be UTC and not in the future")
	}
	return nil
}

func validateFlightCommand(metadata CommandMetadata, flightID FlightID) error {
	if err := validateCommandMetadata(metadata); err != nil {
		return err
	}
	if !trimmed(string(flightID)) {
		return commandInvalid("flight ID is required")
	}
	return nil
}

func validateCommandMetadata(metadata CommandMetadata) error {
	if !trimmed(metadata.CommandID) {
		return commandInvalid("command ID is required")
	}
	return nil
}

func commandInvalid(message string) error {
	return &DomainError{Class: ErrorInvalidArgument, Message: "AMAN command: " + message}
}

func trimmed(value string) bool { return value != "" && value == strings.TrimSpace(value) }
func utc(value time.Time) bool  { return !value.IsZero() && value.Location() == time.UTC }
