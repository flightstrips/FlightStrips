package shared

import (
	"FlightStrips/pkg/events/euroscope"
	"context"
	"fmt"
)

// CommandBinding identifies one live plugin connection, not a CID lookup. The
// transport owns its liveness fence; a reconnect cannot replace this binding.
type CommandBinding interface {
	Session() int32
	CID() string
	Same(CommandBinding) bool
	Validate() error
	Send(euroscope.OutgoingMessage) error
}

type CommandBindingProvider interface {
	CaptureCommandBinding(session int32, cid string) (CommandBinding, error)
}

type AutomaticCommandBindingProvider interface {
	ResolveAutomaticCommandBinding(session int32, preferredCID string) (CommandBinding, error)
}

type humanCommandKey struct{}

// WithoutCommandAuthority is used only when selecting a separately recorded
// deferred proposal. Its recorded source is installed before execution.
func WithoutCommandAuthority(ctx context.Context) context.Context {
	return context.WithValue(ctx, humanCommandKey{}, nil)
}

func WithHumanCommand(ctx context.Context, binding CommandBinding) (context.Context, error) {
	if existing, ok := HumanCommand(ctx); ok {
		if !existing.Same(binding) {
			return ctx, RejectAction("cannot replace initiating command binding")
		}
		return ctx, existing.Validate()
	}
	if binding == nil || binding.Session() <= 0 || binding.CID() == "" {
		return ctx, RejectAction("own plugin binding is required")
	}
	if err := binding.Validate(); err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, humanCommandKey{}, binding), nil
}

// DispatchEuroscopeCommand resolves only genuine automatic work. A supplied
// human context always wins, including when a shared service supplied a CID.
func DispatchEuroscopeCommand(ctx context.Context, provider any, session int32, preferredCID string, event euroscope.OutgoingMessage, legacy func()) error {
	if _, human := HumanCommand(ctx); human {
		return DispatchCommand(ctx, session, event, nil)
	}
	if resolver, ok := provider.(AutomaticCommandBindingProvider); ok {
		binding, err := resolver.ResolveAutomaticCommandBinding(session, preferredCID)
		if err != nil {
			return err
		}
		return binding.Send(event)
	}
	return DispatchCommand(ctx, session, event, legacy)
}

func HumanCommand(ctx context.Context) (CommandBinding, bool) {
	b, ok := ctx.Value(humanCommandKey{}).(CommandBinding)
	return b, ok
}

func ValidateCommandAuthority(ctx context.Context, session int32) error {
	if b, human := HumanCommand(ctx); human {
		if b.Session() != session {
			return RejectAction("command binding belongs to another session")
		}
		return b.Validate()
	}
	return nil
}

// EnsureHumanCommand also protects direct controller use cases outside socket
// dispatch. Narrow test doubles without transport capability remain injectable.
func EnsureHumanCommand(ctx context.Context, provider any, session int32, cid string) (context.Context, error) {
	if b, human := HumanCommand(ctx); human {
		if b.Session() != session || (cid != "" && b.CID() != cid) {
			return ctx, RejectAction("initiating command binding does not match request")
		}
		return ctx, b.Validate()
	}
	p, available := provider.(CommandBindingProvider)
	if !available {
		return ctx, nil
	}
	b, err := p.CaptureCommandBinding(session, cid)
	if err != nil {
		return ctx, err
	}
	return WithHumanCommand(ctx, b)
}

// DispatchCommand never resolves or falls back for human-originated work.
// Automatic callers supply their existing appropriate scoped resolver/send path.
func DispatchCommand(ctx context.Context, session int32, event euroscope.OutgoingMessage, automatic func()) error {
	if b, human := HumanCommand(ctx); human {
		if err := ValidateCommandAuthority(ctx, session); err != nil {
			return err
		}
		return b.Send(event)
	}
	if automatic == nil {
		return fmt.Errorf("automatic command executor is required")
	}
	automatic()
	return nil
}
