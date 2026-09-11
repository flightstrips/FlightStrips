package coordinationrequest

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrUnauthorized   = errors.New("coordination request command is unauthorized")
	ErrWrongRecipient = errors.New("coordination request is not assigned to the authoritative controller")
)

// CommandContext contains only server-derived authentication and receipt
// facts. Transports must never populate it from request payload data.
type CommandContext struct {
	Airport    string
	Actor      string
	Role       string
	ReceivedAt time.Time
}

type SubmitCommand struct {
	CommandID        string
	ExpectedRevision uint64
	FlightID         FlightID
	Kind             Kind
	Payload          Payload
}

type DecisionCommand struct {
	CommandID        string
	ExpectedRevision uint64
	RequestID        RequestID
	Reason           string
}

type submitRepository interface {
	Submit(context.Context, Request, uint64) (CommitResult, error)
	Get(context.Context, string, RequestID) (Request, error)
	Decide(context.Context, RequestID, Decision, uint64) (CommitResult, error)
	TransferPending(context.Context, OwnershipFact) (TransferResult, error)
}

// ObserveOwnership applies a trusted authoritative tracking-controller fact.
func (s *Service) ObserveOwnership(ctx context.Context, fact OwnershipFact) (TransferResult, error) {
	if s == nil || s.repository == nil {
		return TransferResult{}, errors.New("coordination request service is not configured")
	}
	return s.repository.TransferPending(ctx, fact)
}

func (s *Service) Accept(ctx context.Context, auth CommandContext, command DecisionCommand) (CommitResult, error) {
	return s.decide(ctx, auth, command, StateAccepted)
}
func (s *Service) Reject(ctx context.Context, auth CommandContext, command DecisionCommand) (CommitResult, error) {
	return s.decide(ctx, auth, command, StateRejected)
}

func (s *Service) decide(ctx context.Context, auth CommandContext, command DecisionCommand, state State) (CommitResult, error) {
	if err := s.validateContext(auth); err != nil {
		return CommitResult{}, err
	}
	request, err := s.repository.Get(ctx, auth.Airport, command.RequestID)
	if err != nil {
		return CommitResult{}, err
	}
	recipient, err := s.owners.TrackingController(ctx, auth.Airport, request.FlightID)
	if err != nil {
		return CommitResult{}, err
	}
	if recipient == "" || recipient != request.RecipientController {
		return CommitResult{}, ErrWrongRecipient
	}
	if auth.Role != string(recipient) {
		return CommitResult{}, ErrUnauthorized
	}
	decision := Decision{CommandID: command.CommandID, Airport: auth.Airport, Actor: auth.Actor, Role: auth.Role,
		AuthoritativeRecipient: recipient, RequestID: request.ID, RequestKind: request.Kind,
		BeforeState: StatePending, AfterState: state, Reason: command.Reason, ReceivedAt: auth.ReceivedAt}
	return s.repository.Decide(ctx, request.ID, decision, command.ExpectedRevision)
}

func (s *Service) validateContext(auth CommandContext) error {
	if s == nil || s.repository == nil || s.owners == nil {
		return errors.New("coordination request service is not configured")
	}
	if auth.Airport == "" || auth.Airport != strings.TrimSpace(auth.Airport) || auth.Actor == "" || auth.Actor != strings.TrimSpace(auth.Actor) ||
		auth.Role == "" || auth.Role != strings.TrimSpace(auth.Role) || auth.ReceivedAt.IsZero() || auth.ReceivedAt.Location() != time.UTC {
		return errors.New("server-derived airport, actor, role, and UTC receipt time are required")
	}
	return nil
}

// TrackingControllerResolver reads the flight's current authoritative owner.
// An empty controller with no error means that the flight is unassigned.
type TrackingControllerResolver interface {
	TrackingController(context.Context, string, FlightID) (ControllerID, error)
}

type Service struct {
	repository submitRepository
	owners     TrackingControllerResolver
	fmpRoles   map[string]struct{}
}

func NewService(repository submitRepository, owners TrackingControllerResolver, fmpRoles []string) *Service {
	roles := make(map[string]struct{}, len(fmpRoles))
	for _, role := range fmpRoles {
		if role == strings.TrimSpace(role) && role != "" {
			roles[role] = struct{}{}
		}
	}
	return &Service{repository: repository, owners: owners, fmpRoles: roles}
}

func (s *Service) Submit(ctx context.Context, auth CommandContext, command SubmitCommand) (CommitResult, error) {
	if err := s.validateContext(auth); err != nil {
		return CommitResult{}, err
	}
	if _, authorized := s.fmpRoles[auth.Role]; !authorized {
		return CommitResult{}, ErrUnauthorized
	}
	recipient, err := s.owners.TrackingController(ctx, auth.Airport, command.FlightID)
	if err != nil {
		return CommitResult{}, err
	}
	if recipient != "" && string(recipient) != strings.TrimSpace(string(recipient)) {
		return CommitResult{}, errors.New("authoritative tracking controller is invalid")
	}
	request, err := New(command.CommandID, auth.Airport, command.FlightID, recipient,
		auth.Actor, auth.Role, command.Kind, command.Payload, auth.ReceivedAt)
	if err != nil {
		return CommitResult{}, err
	}
	return s.repository.Submit(ctx, request, command.ExpectedRevision)
}
