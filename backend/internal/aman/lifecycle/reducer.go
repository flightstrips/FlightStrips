// Package lifecycle owns deterministic AMAN flight-state transitions.
// Source freshness remains orthogonal to the lifecycle state, and prediction
// and freeze mechanics remain owned by the prediction reducer.
package lifecycle

import (
	"fmt"
	"strings"
	"time"

	"FlightStrips/internal/aman"
)

// Config contains the clock boundaries used by the lifecycle reducer.
// Callers persist the selected policy version with AirportState.
type Config struct {
	UnstableHorizon      time.Duration
	StableHorizon        time.Duration
	MinimumUnstableDwell time.Duration
}

func DefaultConfig() Config {
	return Config{
		UnstableHorizon:      45 * time.Minute,
		StableHorizon:        20 * time.Minute,
		MinimumUnstableDwell: 5 * time.Minute,
	}
}

func (c Config) Validate() error {
	if c.UnstableHorizon <= 0 {
		return invalidArgument("Unstable horizon must be greater than zero")
	}
	if c.StableHorizon <= 0 || c.StableHorizon >= c.UnstableHorizon {
		return invalidArgument("Stable horizon must be positive and shorter than the Unstable horizon")
	}
	if c.MinimumUnstableDwell < 0 {
		return invalidArgument("minimum Unstable dwell cannot be negative")
	}
	return nil
}

// EventKind identifies accepted lifecycle evidence. Each kind carries only
// the data it owns; prediction values are already-accepted operational TETA
// snapshots and never raw physical estimates.
type EventKind string

const (
	EventAirborneDetected    EventKind = "airborne_detected"
	EventPredictionAccepted  EventKind = "prediction_accepted"
	EventDataStatusChanged   EventKind = "data_status_changed"
	EventGoAroundConfirmed   EventKind = "go_around_confirmed"
	EventLandingConfirmed    EventKind = "landing_confirmed"
	EventManualRemoval       EventKind = "manual_removal"
	EventLandedTimeout       EventKind = "landed_timeout"
	EventPlannedCancellation EventKind = "planned_cancellation"
)

func (k EventKind) Valid() bool {
	switch k {
	case EventAirborneDetected,
		EventPredictionAccepted,
		EventDataStatusChanged,
		EventGoAroundConfirmed,
		EventLandingConfirmed,
		EventManualRemoval,
		EventLandedTimeout,
		EventPlannedCancellation:
		return true
	default:
		return false
	}
}

// Event is a clock-injected fact presented to the reducer. DataStatus is used
// only by EventDataStatusChanged. OperationalTETA is used only by
// EventPredictionAccepted.
type Event struct {
	ID              string
	Kind            EventKind
	OccurredAt      time.Time
	DataStatus      aman.DataStatus
	OperationalTETA *time.Time
}

// Transition is emitted once for a real state change. It can be recorded in
// the same StateCommit as the replacement aggregate.
type Transition struct {
	EventID string
	From    aman.FlightState
	To      aman.FlightState
	Reason  aman.LifecycleReason
	At      time.Time
}

type Result struct {
	Flight     aman.AMANFlight
	Transition *Transition
	Duplicate  bool
}

// Reduce applies one accepted event to a copy of flight. Exact retries are
// idempotent. A different event at or before the persisted cursor is rejected
// explicitly so delayed input cannot regress lifecycle or freshness.
func Reduce(config Config, flight aman.AMANFlight, event Event) (Result, error) {
	if err := config.Validate(); err != nil {
		return Result{}, err
	}
	if err := validateEvent(event); err != nil {
		return Result{}, err
	}
	if !flight.State.Valid() || !flight.DataStatus.Valid() {
		return Result{}, invalidArgument("flight lifecycle state is invalid")
	}

	if flight.Lifecycle != nil {
		if event.ID == flight.Lifecycle.LastEventID {
			if fingerprint(event) == flight.Lifecycle.LastEventFingerprint {
				return Result{Flight: flight, Duplicate: true}, nil
			}
			return Result{}, invalidTransition(flight.State, event.Kind, "event ID was reused with different content")
		}
		if !event.OccurredAt.After(flight.Lifecycle.LastEventAt) {
			return Result{}, invalidTransition(flight.State, event.Kind, "event is out of order")
		}
	} else if event.OccurredAt.Before(flight.UpdatedAt) {
		return Result{}, invalidTransition(flight.State, event.Kind, "event predates the aggregate")
	}

	enteredAt := flight.UpdatedAt
	reason := aman.LifecycleReasonInitial
	if flight.Lifecycle != nil {
		enteredAt = flight.Lifecycle.EnteredAt
		reason = flight.Lifecycle.Reason
	}

	from := flight.State
	to, transitionReason, err := nextState(config, flight, enteredAt, event)
	if err != nil {
		return Result{}, err
	}
	if to != from {
		enteredAt = event.OccurredAt
		reason = transitionReason
	}
	if event.Kind == EventDataStatusChanged {
		flight.DataStatus = event.DataStatus
	}

	flight.State = to
	flight.UpdatedAt = event.OccurredAt
	flight.Lifecycle = &aman.LifecycleState{
		EnteredAt:            enteredAt,
		Reason:               reason,
		LastEventID:          event.ID,
		LastEventFingerprint: fingerprint(event),
		LastEventAt:          event.OccurredAt,
	}

	result := Result{Flight: flight}
	if to != from {
		result.Transition = &Transition{EventID: event.ID, From: from, To: to, Reason: transitionReason, At: event.OccurredAt}
	}
	return result, nil
}

func fingerprint(event Event) string {
	operationalTETA := ""
	if event.OperationalTETA != nil {
		operationalTETA = event.OperationalTETA.Format(time.RFC3339Nano)
	}
	return fmt.Sprintf("%s|%s|%s|%s", event.Kind, event.OccurredAt.Format(time.RFC3339Nano), event.DataStatus, operationalTETA)
}

func nextState(config Config, flight aman.AMANFlight, enteredAt time.Time, event Event) (aman.FlightState, aman.LifecycleReason, error) {
	switch event.Kind {
	case EventDataStatusChanged:
		if flight.State == aman.StateRemoved {
			return "", "", invalidTransition(flight.State, event.Kind, "Removed is terminal")
		}
		return flight.State, "", nil
	case EventAirborneDetected:
		switch flight.State {
		case aman.StatePlanned:
			return aman.StateAirborne, aman.LifecycleReasonAirborneDetected, nil
		case aman.StateAirborne:
			return flight.State, "", nil
		default:
			return "", "", invalidTransition(flight.State, event.Kind, "airborne evidence cannot enter this state")
		}
	case EventPredictionAccepted:
		if flight.DataStatus != aman.DataFresh {
			return flight.State, "", nil
		}
		untilArrival := event.OperationalTETA.Sub(event.OccurredAt)
		switch flight.State {
		case aman.StateAirborne, aman.StateGoAround:
			if untilArrival <= config.UnstableHorizon {
				return aman.StateUnstable, aman.LifecycleReasonUnstableHorizon, nil
			}
		case aman.StateUnstable:
			if untilArrival <= config.StableHorizon && event.OccurredAt.Sub(enteredAt) >= config.MinimumUnstableDwell {
				return aman.StateStable, aman.LifecycleReasonStableHorizon, nil
			}
		case aman.StateStable:
			return flight.State, "", nil
		default:
			return "", "", invalidTransition(flight.State, event.Kind, "prediction cannot advance this state")
		}
		return flight.State, "", nil
	case EventGoAroundConfirmed:
		if flight.State != aman.StateUnstable && flight.State != aman.StateStable {
			return "", "", invalidTransition(flight.State, event.Kind, "go-around requires an arriving flight")
		}
		return aman.StateGoAround, aman.LifecycleReasonGoAroundConfirmed, nil
	case EventLandingConfirmed:
		switch flight.State {
		case aman.StateAirborne, aman.StateUnstable, aman.StateStable, aman.StateGoAround:
			return aman.StateLanded, aman.LifecycleReasonLandingConfirmed, nil
		default:
			return "", "", invalidTransition(flight.State, event.Kind, "landing requires an airborne flight")
		}
	case EventManualRemoval:
		if flight.State == aman.StateRemoved {
			return "", "", invalidTransition(flight.State, event.Kind, "Removed is terminal")
		}
		return aman.StateRemoved, aman.LifecycleReasonManualRemoval, nil
	case EventLandedTimeout:
		if flight.State != aman.StateLanded {
			return "", "", invalidTransition(flight.State, event.Kind, "landing timeout requires Landed")
		}
		return aman.StateRemoved, aman.LifecycleReasonLandedTimeout, nil
	case EventPlannedCancellation:
		if flight.State != aman.StatePlanned {
			return "", "", invalidTransition(flight.State, event.Kind, "planned cancellation requires Planned")
		}
		return aman.StateRemoved, aman.LifecycleReasonPlannedCancellation, nil
	default:
		return "", "", invalidArgument("lifecycle event kind is invalid")
	}
}

func validateEvent(event Event) error {
	if !event.Kind.Valid() {
		return invalidArgument("lifecycle event kind is invalid")
	}
	if event.ID == "" || strings.TrimSpace(event.ID) != event.ID {
		return invalidArgument("lifecycle event ID is required")
	}
	if event.OccurredAt.IsZero() || event.OccurredAt.Location() != time.UTC {
		return invalidArgument("lifecycle event time must be UTC")
	}
	if event.Kind == EventDataStatusChanged {
		if !event.DataStatus.Valid() || event.OperationalTETA != nil {
			return invalidArgument("data-status event payload is invalid")
		}
		return nil
	}
	if event.DataStatus != "" {
		return invalidArgument("only a data-status event may change DataStatus")
	}
	if event.Kind == EventPredictionAccepted {
		if event.OperationalTETA == nil || event.OperationalTETA.IsZero() || event.OperationalTETA.Location() != time.UTC {
			return invalidArgument("accepted prediction requires a UTC operational TETA")
		}
		return nil
	}
	if event.OperationalTETA != nil {
		return invalidArgument("only an accepted prediction may carry operational TETA")
	}
	return nil
}

func invalidArgument(message string) error {
	return &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: message}
}

func invalidTransition(state aman.FlightState, event EventKind, message string) error {
	return &aman.DomainError{
		Class:   aman.ErrorInvalidTransition,
		Message: fmt.Sprintf("%s from %s: %s", event, state, message),
	}
}
