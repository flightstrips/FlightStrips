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

type SelectRunwayGroupCommand struct {
	Metadata      CommandMetadata
	RunwayGroupID RunwayGroupID
	EffectiveAt   time.Time
}

// SetActiveRunwayGroupsCommand replaces the complete active landing-runway
// set. Authority, airport identity, and receipt time belong to CommandContext.
type SetActiveRunwayGroupsCommand struct {
	Metadata       CommandMetadata
	RunwayGroupIDs []RunwayGroupID
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

// SetManualFeederETACommand and ResetManualFeederETACommand are domain
// commands used by the authenticated AMAN transport.
type SetManualFeederETACommand struct {
	Metadata  CommandMetadata
	FlightID  FlightID
	FeederETA time.Time
}

type ResetManualFeederETACommand struct {
	Metadata CommandMetadata
	FlightID FlightID
}

// RecomputeFlightCommand deliberately carries no prediction fields. The
// operational owner rebuilds the physical prediction from its persisted,
// authoritative observation at the revision named by Metadata.
type RecomputeFlightCommand struct {
	Metadata CommandMetadata
	FlightID FlightID
}

type ReportGoAroundCommand struct {
	Metadata   CommandMetadata
	FlightID   FlightID
	DetectedAt time.Time
}

type ConfirmGoAroundCommand struct {
	Metadata  CommandMetadata
	FlightID  FlightID
	EpisodeID string
}

type RejectGoAroundCommand struct {
	Metadata  CommandMetadata
	FlightID  FlightID
	EpisodeID string
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
	SelectRunwayGroup(context.Context, CommandContext, SelectRunwayGroupCommand) (CommandExecution, error)
	SetActiveRunwayGroups(context.Context, CommandContext, SetActiveRunwayGroupsCommand) (CommandExecution, error)
	AcceptTETA(context.Context, CommandContext, AcceptTETACommand) (CommandExecution, error)
	KeepFPLETA(context.Context, CommandContext, KeepFPLETACommand) (CommandExecution, error)
	SetManualETA(context.Context, CommandContext, SetManualETACommand) (CommandExecution, error)
	ResetTETAOverride(context.Context, CommandContext, ResetTETAOverrideCommand) (CommandExecution, error)
	SetManualFeederETA(context.Context, CommandContext, SetManualFeederETACommand) (CommandExecution, error)
	ResetManualFeederETA(context.Context, CommandContext, ResetManualFeederETACommand) (CommandExecution, error)
	RecomputeFlight(context.Context, CommandContext, RecomputeFlightCommand) (CommandExecution, error)
	ReportGoAround(context.Context, CommandContext, ReportGoAroundCommand) (CommandExecution, error)
	ConfirmGoAround(context.Context, CommandContext, ConfirmGoAroundCommand) (CommandExecution, error)
	RejectGoAround(context.Context, CommandContext, RejectGoAroundCommand) (CommandExecution, error)
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

func (c SetManualFeederETACommand) Validate() error {
	if err := validateFlightCommand(c.Metadata, c.FlightID); err != nil {
		return err
	}
	if !utc(c.FeederETA) {
		return commandInvalid("manual feeder ETA must be a UTC value")
	}
	return nil
}

func (c ResetManualFeederETACommand) Validate() error {
	return validateFlightCommand(c.Metadata, c.FlightID)
}

func (c RecomputeFlightCommand) Validate() error {
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

func (c SelectRunwayGroupCommand) Validate() error {
	if err := validateCommandMetadata(c.Metadata); err != nil {
		return err
	}
	if !trimmed(string(c.RunwayGroupID)) || !utc(c.EffectiveAt) {
		return commandInvalid("runway selection requires a runway group and UTC effective time")
	}
	return nil
}

func (c SetActiveRunwayGroupsCommand) Validate() error {
	if err := validateCommandMetadata(c.Metadata); err != nil {
		return err
	}
	if len(c.RunwayGroupIDs) == 0 {
		return commandInvalid("active runway group set cannot be empty")
	}
	seen := make(map[RunwayGroupID]struct{}, len(c.RunwayGroupIDs))
	for _, id := range c.RunwayGroupIDs {
		if !trimmed(string(id)) {
			return commandInvalid("active runway group ID is required")
		}
		if _, exists := seen[id]; exists {
			return commandInvalid("active runway group IDs must be unique")
		}
		seen[id] = struct{}{}
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

func (c ConfirmGoAroundCommand) Validate() error {
	return validateGoAroundDecision(c.Metadata, c.FlightID, c.EpisodeID)
}
func (c RejectGoAroundCommand) Validate() error {
	return validateGoAroundDecision(c.Metadata, c.FlightID, c.EpisodeID)
}

func validateGoAroundDecision(metadata CommandMetadata, flightID FlightID, episodeID string) error {
	if err := validateFlightCommand(metadata, flightID); err != nil {
		return err
	}
	if !trimmed(episodeID) {
		return commandInvalid("go-around episode ID is required")
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
