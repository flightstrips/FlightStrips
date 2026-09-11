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

const (
	KindRouteDirect Kind = "route_direct"
	KindSpeed       Kind = "speed"

	StatePending    State = "pending"
	StateAccepted   State = "accepted"
	StateRejected   State = "rejected"
	StateSuperseded State = "superseded"
	StateExpired    State = "expired"
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

// Request is the durable aggregate. CommandID is retained so the derived ID
// remains verifiable after restart and duplicate submissions remain idempotent.
type Request struct {
	ID                  RequestID    `json:"id"`
	CommandID           string       `json:"command_id"`
	Airport             string       `json:"airport"`
	FlightID            FlightID     `json:"flight_id"`
	RecipientController ControllerID `json:"recipient_controller"`
	Kind                Kind         `json:"kind"`
	State               State        `json:"state"`
	Payload             Payload      `json:"payload"`
	CreatedAt           time.Time    `json:"created_at"`
	UpdatedAt           time.Time    `json:"updated_at"`
	ResolvedAt          *time.Time   `json:"resolved_at,omitempty"`
}

func IDForCommand(commandID string) RequestID {
	return RequestID("coordination-request/" + commandID)
}

func New(commandID, airport string, flightID FlightID, recipient ControllerID, kind Kind, payload Payload, at time.Time) (Request, error) {
	r := Request{ID: IDForCommand(commandID), CommandID: commandID, Airport: airport, FlightID: flightID,
		RecipientController: recipient, Kind: kind, State: StatePending, Payload: payload, CreatedAt: at, UpdatedAt: at}
	return r, r.Validate()
}

func (r Request) Validate() error {
	if !present(r.CommandID) || r.ID != IDForCommand(r.CommandID) || !present(r.Airport) ||
		!present(string(r.FlightID)) || !present(string(r.RecipientController)) {
		return errors.New("coordination request identity and ownership must be complete and trimmed")
	}
	if !utc(r.CreatedAt) || !utc(r.UpdatedAt) || r.UpdatedAt.Before(r.CreatedAt) {
		return errors.New("coordination request timestamps must be chronological UTC values")
	}
	if err := r.Payload.validate(r.Kind); err != nil {
		return err
	}
	if r.State == StatePending {
		if r.ResolvedAt != nil {
			return errors.New("pending coordination request cannot be resolved")
		}
		return nil
	}
	if !terminal(r.State) || r.ResolvedAt == nil || !utc(*r.ResolvedAt) ||
		r.ResolvedAt.Before(r.CreatedAt) || !r.UpdatedAt.Equal(*r.ResolvedAt) {
		return errors.New("terminal coordination request requires one chronological resolution timestamp")
	}
	return nil
}

func (r Request) Transition(next State, at time.Time) (Request, error) {
	if err := r.Validate(); err != nil {
		return Request{}, err
	}
	if r.State != StatePending || !terminal(next) || !utc(at) || at.Before(r.UpdatedAt) {
		return Request{}, errors.New("coordination request transition is invalid")
	}
	r.State, r.UpdatedAt, r.ResolvedAt = next, at, &at
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
