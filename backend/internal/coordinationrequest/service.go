package coordinationrequest

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrUnauthorized = errors.New("coordination request submission requires an authorized FMP role")

// CommandContext contains only server-derived authentication, ownership, and
// receipt facts. Transports must never populate it from request payload data.
type CommandContext struct {
	Airport             string
	Actor               string
	Role                string
	RecipientController ControllerID
	ReceivedAt          time.Time
}

type SubmitCommand struct {
	CommandID        string
	ExpectedRevision uint64
	FlightID         FlightID
	Kind             Kind
	Payload          Payload
}

type submitRepository interface {
	Submit(context.Context, Request, uint64) (CommitResult, error)
}

type Service struct {
	repository submitRepository
	fmpRoles   map[string]struct{}
}

func NewService(repository submitRepository, fmpRoles []string) *Service {
	roles := make(map[string]struct{}, len(fmpRoles))
	for _, role := range fmpRoles {
		if role == strings.TrimSpace(role) && role != "" {
			roles[role] = struct{}{}
		}
	}
	return &Service{repository: repository, fmpRoles: roles}
}

func (s *Service) Submit(ctx context.Context, auth CommandContext, command SubmitCommand) (CommitResult, error) {
	if s == nil || s.repository == nil {
		return CommitResult{}, errors.New("coordination request service is not configured")
	}
	if auth.Airport == "" || auth.Airport != strings.TrimSpace(auth.Airport) || auth.Actor == "" || auth.Actor != strings.TrimSpace(auth.Actor) ||
		auth.Role == "" || auth.Role != strings.TrimSpace(auth.Role) || auth.RecipientController == "" ||
		string(auth.RecipientController) != strings.TrimSpace(string(auth.RecipientController)) || auth.ReceivedAt.IsZero() || auth.ReceivedAt.Location() != time.UTC {
		return CommitResult{}, errors.New("server-derived airport, actor, role, recipient, and UTC receipt time are required")
	}
	if _, authorized := s.fmpRoles[auth.Role]; !authorized {
		return CommitResult{}, ErrUnauthorized
	}
	request, err := New(command.CommandID, auth.Airport, command.FlightID, auth.RecipientController,
		auth.Actor, auth.Role, command.Kind, command.Payload, auth.ReceivedAt)
	if err != nil {
		return CommitResult{}, err
	}
	return s.repository.Submit(ctx, request, command.ExpectedRevision)
}
