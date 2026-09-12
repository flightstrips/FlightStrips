package coordinationrequest

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Kind string
type State string
type RequestID string
type FlightID string
type ControllerID string
type RecipientStatus string
type ExpiryReason string

const (
	KindRouteDirect Kind = "route_direct"
	KindSpeed       Kind = "speed"

	StatePending    State = "pending"
	StateAccepted   State = "accepted"
	StateRejected   State = "rejected"
	StateSuperseded State = "superseded"
	StateExpired    State = "expired"

	RecipientAssigned   RecipientStatus = "assigned"
	RecipientUnassigned RecipientStatus = "unassigned"

	ExpiryFlightCompleted      ExpiryReason = "flight_completed"
	ExpiryGoAroundConfirmed    ExpiryReason = "go_around_confirmed"
	ExpiryAuthoritativeRemoval ExpiryReason = "authoritative_removal"
	ExpiryDesequenced          ExpiryReason = "desequenced"
)

type RouteDirectPayload struct {
	Route    string `json:"route,omitempty"`
	DirectTo string `json:"direct_to,omitempty"`
}

type SpeedPayload struct {
	Requested string `json:"requested"`
}

type Payload struct {
	RouteDirect *RouteDirectPayload `json:"route_direct,omitempty"`
	Speed       *SpeedPayload       `json:"speed,omitempty"`
}

// Decision is the immutable audit of an accept or reject command. Identity
// and authority fields are copied from trusted server context at receipt.
type Decision struct {
	CommandID              string       `json:"command_id"`
	Airport                string       `json:"airport"`
	Actor                  string       `json:"actor"`
	Role                   string       `json:"role"`
	AuthoritativeRecipient ControllerID `json:"authoritative_recipient"`
	RequestID              RequestID    `json:"request_id"`
	RequestKind            Kind         `json:"request_kind"`
	BeforeState            State        `json:"before_state"`
	AfterState             State        `json:"after_state"`
	Reason                 string       `json:"reason,omitempty"`
	ReceivedAt             time.Time    `json:"received_at"`
}

// RecipientTransfer records the authoritative ownership fact that moved a
// pending request. An empty NewRecipient is the explicit unassigned state.
type RecipientTransfer struct {
	OwnershipFact     string       `json:"ownership_fact"`
	OwnershipRevision uint64       `json:"ownership_revision"`
	PreviousRecipient ControllerID `json:"previous_recipient,omitempty"`
	NewRecipient      ControllerID `json:"new_recipient,omitempty"`
	TransferredAt     time.Time    `json:"transferred_at"`
}

// Expiry records the authoritative operational fact that made a pending
// request irrelevant without discarding its identity or prior history.
type Expiry struct {
	FactID       string       `json:"fact_id"`
	FactRevision uint64       `json:"fact_revision"`
	Reason       ExpiryReason `json:"reason"`
	ExpiredAt    time.Time    `json:"expired_at"`
}

// ClearanceAudit links agreement to the later controller fact. It is
// evidence only: attaching it never changes request state or AMAN inputs.
type ClearanceAudit struct {
	FactID     string    `json:"fact_id"`
	Kind       Kind      `json:"kind"`
	Value      string    `json:"value"`
	Issuer     string    `json:"issuer"`
	ObservedAt time.Time `json:"observed_at"`
}

// Request is the durable aggregate. CommandID is retained so the derived ID
// remains verifiable after restart and duplicate submissions remain idempotent.
type Request struct {
	ID                  RequestID           `json:"id"`
	CommandID           string              `json:"command_id"`
	Airport             string              `json:"airport"`
	FlightID            FlightID            `json:"flight_id"`
	RecipientController ControllerID        `json:"recipient_controller"`
	RecipientStatus     RecipientStatus     `json:"recipient_status,omitempty"`
	SubmittedBy         string              `json:"submitted_by"`
	SubmittedRole       string              `json:"submitted_role"`
	Kind                Kind                `json:"kind"`
	State               State               `json:"state"`
	Payload             Payload             `json:"payload"`
	CreatedAt           time.Time           `json:"created_at"`
	UpdatedAt           time.Time           `json:"updated_at"`
	ResolvedAt          *time.Time          `json:"resolved_at,omitempty"`
	Supersedes          *RequestID          `json:"supersedes,omitempty"`
	SupersededBy        *RequestID          `json:"superseded_by,omitempty"`
	Decision            *Decision           `json:"decision,omitempty"`
	RecipientTransfers  []RecipientTransfer `json:"recipient_transfers,omitempty"`
	Expiry              *Expiry             `json:"expiry,omitempty"`
	Clearance           *ClearanceAudit     `json:"clearance,omitempty"`
}

func IDForCommand(commandID string) RequestID {
	return RequestID("coordination-request/" + commandID)
}

func New(commandID, airport string, flightID FlightID, recipient ControllerID, actor, role string, kind Kind, payload Payload, at time.Time) (Request, error) {
	status := RecipientAssigned
	if recipient == "" {
		status = RecipientUnassigned
	}
	r := Request{ID: IDForCommand(commandID), CommandID: commandID, Airport: airport, FlightID: flightID,
		RecipientController: recipient, RecipientStatus: status, SubmittedBy: actor, SubmittedRole: role,
		Kind: kind, State: StatePending, Payload: payload, CreatedAt: at, UpdatedAt: at}
	return r, r.Validate()
}

func (r Request) Validate() error {
	if !present(r.CommandID) || r.ID != IDForCommand(r.CommandID) || !present(r.Airport) ||
		!present(string(r.FlightID)) || !present(r.SubmittedBy) || !present(r.SubmittedRole) {
		return errors.New("coordination request identity and ownership must be complete and trimmed")
	}
	// An omitted status is the rolling-upgrade representation written before
	// unassigned recipients were supported.
	if (r.RecipientStatus == "" || r.RecipientStatus == RecipientAssigned) && !present(string(r.RecipientController)) ||
		r.RecipientStatus == RecipientUnassigned && r.RecipientController != "" ||
		r.RecipientStatus != "" && r.RecipientStatus != RecipientAssigned && r.RecipientStatus != RecipientUnassigned {
		return errors.New("coordination request recipient assignment is invalid")
	}
	if !utc(r.CreatedAt) || !utc(r.UpdatedAt) || r.UpdatedAt.Before(r.CreatedAt) {
		return errors.New("coordination request timestamps must be chronological UTC values")
	}
	for index, transfer := range r.RecipientTransfers {
		if !present(transfer.OwnershipFact) || transfer.OwnershipRevision == 0 || !utc(transfer.TransferredAt) ||
			transfer.TransferredAt.Before(r.CreatedAt) || transfer.PreviousRecipient == transfer.NewRecipient ||
			(index > 0 && transfer.OwnershipRevision <= r.RecipientTransfers[index-1].OwnershipRevision) {
			return errors.New("coordination request recipient transfer audit is invalid")
		}
	}
	if err := r.Payload.validate(r.Kind); err != nil {
		return err
	}
	if r.State == StatePending {
		if r.ResolvedAt != nil || r.SupersededBy != nil || r.Decision != nil || r.Expiry != nil {
			return errors.New("pending coordination request cannot be resolved")
		}
		return nil
	}
	if !terminal(r.State) || r.ResolvedAt == nil || !utc(*r.ResolvedAt) ||
		r.ResolvedAt.Before(r.CreatedAt) || !r.UpdatedAt.Equal(*r.ResolvedAt) {
		return errors.New("terminal coordination request requires one chronological resolution timestamp")
	}
	if (r.State == StateSuperseded) != (r.SupersededBy != nil) || r.SupersededBy != nil && *r.SupersededBy == r.ID || r.Supersedes != nil && *r.Supersedes == r.ID {
		return errors.New("coordination request supersede audit links are invalid")
	}
	if r.State == StateAccepted || r.State == StateRejected {
		if r.Decision == nil || !present(r.Decision.CommandID) || r.Decision.Airport != r.Airport || !present(r.Decision.Actor) || !present(r.Decision.Role) ||
			r.Decision.AuthoritativeRecipient != r.RecipientController || r.Decision.RequestID != r.ID || r.Decision.RequestKind != r.Kind ||
			r.Decision.BeforeState != StatePending || r.Decision.AfterState != r.State || !r.Decision.ReceivedAt.Equal(*r.ResolvedAt) ||
			(r.State == StateRejected && !present(r.Decision.Reason)) || (r.State == StateAccepted && r.Decision.Reason != "") {
			return errors.New("coordination request decision audit is invalid")
		}
	} else if r.Decision != nil {
		return errors.New("only accepted or rejected requests may contain a decision audit")
	}
	if r.Clearance != nil && (r.State != StateAccepted || r.Clearance.Kind != r.Kind ||
		!present(r.Clearance.FactID) || !present(r.Clearance.Value) || !present(r.Clearance.Issuer) ||
		!utc(r.Clearance.ObservedAt) || r.Clearance.ObservedAt.Before(*r.ResolvedAt)) {
		return errors.New("coordination clearance correlation is invalid")
	}
	if r.State == StateExpired {
		if r.Expiry == nil || !present(r.Expiry.FactID) || r.Expiry.FactRevision == 0 || !r.Expiry.Reason.valid() ||
			!r.Expiry.ExpiredAt.Equal(*r.ResolvedAt) {
			return errors.New("expired coordination request requires an authoritative expiry audit")
		}
	} else if r.Expiry != nil {
		return errors.New("only expired requests may contain an expiry audit")
	}
	return nil
}

func (r Request) Correlate(fact ClearanceAudit) (Request, error) {
	if err := r.Validate(); err != nil || r.State != StateAccepted || r.Clearance != nil {
		return Request{}, errors.New("coordination request cannot be correlated")
	}
	r.Clearance = &fact
	return r, r.Validate()
}

func (r Request) effectiveRecipientStatus() RecipientStatus {
	if r.RecipientStatus == "" {
		return RecipientAssigned
	}
	return r.RecipientStatus
}

func (r Request) Supersede(next RequestID, at time.Time) (Request, error) {
	if !present(string(next)) || next == r.ID {
		return Request{}, errors.New("superseding coordination request identity is required")
	}
	if err := r.Validate(); err != nil || r.State != StatePending || !utc(at) || at.Before(r.UpdatedAt) {
		return Request{}, errors.New("coordination request supersede transition is invalid")
	}
	r.State, r.UpdatedAt, r.ResolvedAt, r.SupersededBy = StateSuperseded, at, &at, &next
	return r, r.Validate()
}

func (r Request) Expire(fact Expiry) (Request, error) {
	if err := r.Validate(); err != nil {
		return Request{}, err
	}
	if r.State != StatePending || !present(fact.FactID) || fact.FactRevision == 0 || !fact.Reason.valid() ||
		!utc(fact.ExpiredAt) || fact.ExpiredAt.Before(r.UpdatedAt) {
		return Request{}, errors.New("coordination request transition is invalid")
	}
	r.State, r.UpdatedAt, r.ResolvedAt, r.Expiry = StateExpired, fact.ExpiredAt, &fact.ExpiredAt, &fact
	return r, r.Validate()
}

func (r ExpiryReason) valid() bool {
	return r == ExpiryFlightCompleted || r == ExpiryGoAroundConfirmed || r == ExpiryAuthoritativeRemoval || r == ExpiryDesequenced
}

func (r Request) Decide(commandID, actor, role string, recipient ControllerID, next State, reason string, at time.Time) (Request, error) {
	if next != StateAccepted && next != StateRejected {
		return Request{}, errors.New("coordination request decision must accept or reject")
	}
	if !present(commandID) || !present(actor) || !present(role) || recipient == "" ||
		(next == StateRejected && !present(reason)) || (next == StateAccepted && reason != "") {
		return Request{}, errors.New("coordination request decision identity and reason are invalid")
	}
	if err := r.Validate(); err != nil || r.State != StatePending || !utc(at) || at.Before(r.UpdatedAt) {
		return Request{}, errors.New("coordination request decision transition is invalid")
	}
	resolved := r
	resolved.State, resolved.UpdatedAt, resolved.ResolvedAt = next, at, &at
	resolved.Decision = &Decision{CommandID: commandID, Actor: actor, Role: role, AuthoritativeRecipient: recipient,
		Airport: r.Airport, RequestID: r.ID, RequestKind: r.Kind, BeforeState: r.State, AfterState: next, Reason: reason, ReceivedAt: at}
	return resolved, resolved.Validate()
}

func (r Request) TransferRecipient(fact string, revision uint64, recipient ControllerID, at time.Time) (Request, error) {
	if err := r.Validate(); err != nil || r.State != StatePending || !present(fact) || revision == 0 || !utc(at) || at.Before(r.UpdatedAt) ||
		r.RecipientController == recipient {
		return Request{}, errors.New("coordination request recipient transfer is invalid")
	}
	transfer := RecipientTransfer{OwnershipFact: fact, OwnershipRevision: revision, PreviousRecipient: r.RecipientController,
		NewRecipient: recipient, TransferredAt: at}
	r.RecipientController, r.UpdatedAt = recipient, at
	r.RecipientStatus = RecipientAssigned
	if recipient == "" {
		r.RecipientStatus = RecipientUnassigned
	}
	r.RecipientTransfers = append(r.RecipientTransfers, transfer)
	return r, r.Validate()
}

func (p Payload) validate(kind Kind) error {
	switch kind {
	case KindRouteDirect:
		if p.RouteDirect == nil || p.Speed != nil || !optional(p.RouteDirect.Route) || !optional(p.RouteDirect.DirectTo) ||
			(p.RouteDirect.Route == "" && p.RouteDirect.DirectTo == "") {
			return fmt.Errorf("route/direct request requires only route/direct payload")
		}
	case KindSpeed:
		if p.Speed == nil || p.RouteDirect != nil || !present(p.Speed.Requested) {
			return fmt.Errorf("speed request requires only speed payload")
		}
	default:
		return fmt.Errorf("unknown coordination request kind %q", kind)
	}
	return nil
}

func terminal(state State) bool {
	return state == StateAccepted || state == StateRejected || state == StateSuperseded || state == StateExpired
}

func present(value string) bool  { return value != "" && value == strings.TrimSpace(value) }
func optional(value string) bool { return value == "" || present(value) }
func utc(value time.Time) bool   { return !value.IsZero() && value.Location() == time.UTC }
