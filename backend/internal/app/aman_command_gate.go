package app

import (
	"context"
	"strings"

	"FlightStrips/internal/aman"
)

// amanCommandGate keeps controller authority tied to the current technical
// health snapshot instead of treating configured authoritative mode as a
// permanent mutation grant.
type amanCommandGate struct {
	health   func(context.Context) aman.TechnicalHealth
	commands aman.CommandService
}

func (*amanCommandGate) Name() string { return "health-gated AMAN command service" }

func (g *amanCommandGate) CurrentRevision(ctx context.Context, airport string) (aman.SequenceRevision, error) {
	return g.commands.CurrentRevision(ctx, airport)
}

func (g *amanCommandGate) authorize(ctx context.Context) error {
	health := g.health(ctx)
	if health.AuthorityAllowed {
		return nil
	}
	reason := strings.Join(health.BlockedReasons, ",")
	if reason == "" {
		reason = "technical_authority_unavailable"
	}
	return &aman.DomainError{Class: aman.ErrorReadOnly, Message: "AMAN controller mutations are blocked: " + reason}
}

func (g *amanCommandGate) MoveFlight(ctx context.Context, auth aman.CommandContext, command aman.MoveFlightCommand) (aman.CommandExecution, error) {
	if err := g.authorize(ctx); err != nil {
		return aman.CommandExecution{}, err
	}
	return g.commands.MoveFlight(ctx, auth, command)
}

func (g *amanCommandGate) LockFlight(ctx context.Context, auth aman.CommandContext, command aman.LockFlightCommand) (aman.CommandExecution, error) {
	if err := g.authorize(ctx); err != nil {
		return aman.CommandExecution{}, err
	}
	return g.commands.LockFlight(ctx, auth, command)
}

func (g *amanCommandGate) UnlockFlight(ctx context.Context, auth aman.CommandContext, command aman.UnlockFlightCommand) (aman.CommandExecution, error) {
	if err := g.authorize(ctx); err != nil {
		return aman.CommandExecution{}, err
	}
	return g.commands.UnlockFlight(ctx, auth, command)
}

func (g *amanCommandGate) SetRate(ctx context.Context, auth aman.CommandContext, command aman.SetRateCommand) (aman.CommandExecution, error) {
	if err := g.authorize(ctx); err != nil {
		return aman.CommandExecution{}, err
	}
	return g.commands.SetRate(ctx, auth, command)
}

func (g *amanCommandGate) AcceptTETA(ctx context.Context, auth aman.CommandContext, command aman.AcceptTETACommand) (aman.CommandExecution, error) {
	if err := g.authorize(ctx); err != nil {
		return aman.CommandExecution{}, err
	}
	return g.commands.AcceptTETA(ctx, auth, command)
}

func (g *amanCommandGate) KeepFPLETA(ctx context.Context, auth aman.CommandContext, command aman.KeepFPLETACommand) (aman.CommandExecution, error) {
	if err := g.authorize(ctx); err != nil {
		return aman.CommandExecution{}, err
	}
	return g.commands.KeepFPLETA(ctx, auth, command)
}

func (g *amanCommandGate) SetManualETA(ctx context.Context, auth aman.CommandContext, command aman.SetManualETACommand) (aman.CommandExecution, error) {
	if err := g.authorize(ctx); err != nil {
		return aman.CommandExecution{}, err
	}
	return g.commands.SetManualETA(ctx, auth, command)
}

func (g *amanCommandGate) ResetTETAOverride(ctx context.Context, auth aman.CommandContext, command aman.ResetTETAOverrideCommand) (aman.CommandExecution, error) {
	if err := g.authorize(ctx); err != nil {
		return aman.CommandExecution{}, err
	}
	return g.commands.ResetTETAOverride(ctx, auth, command)
}

func (g *amanCommandGate) ReportGoAround(ctx context.Context, auth aman.CommandContext, command aman.ReportGoAroundCommand) (aman.CommandExecution, error) {
	if err := g.authorize(ctx); err != nil {
		return aman.CommandExecution{}, err
	}
	return g.commands.ReportGoAround(ctx, auth, command)
}

var _ aman.CommandService = (*amanCommandGate)(nil)
