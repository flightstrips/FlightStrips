// Package etareview owns the deterministic first-Unstable TETA discrepancy
// policy. It changes only review state and delegates operational-TETA overrides
// to the prediction package; it never writes raw prediction values.
package etareview

import (
	"fmt"
	"strings"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/prediction"
)

type Config struct {
	DiscrepancyThreshold time.Duration
	ReviewDeadline       time.Duration
}

func (c Config) Validate() error {
	if c.DiscrepancyThreshold < 0 {
		return invalidArgument("ETA review discrepancy threshold cannot be negative")
	}
	if c.ReviewDeadline <= 0 {
		return invalidArgument("ETA review deadline must be greater than zero")
	}
	return nil
}

type Result struct {
	Flight  aman.AMANFlight
	Changed bool
}

// Open evaluates only the first route-aware Unstable prediction. A difference
// equal to the configured threshold does not open a review; the policy says
// beyond the threshold, making the boundary explicit.
func Open(config Config, flight aman.AMANFlight, createdAt time.Time) (Result, error) {
	if err := config.Validate(); err != nil {
		return Result{}, err
	}
	if err := validActionTime(createdAt, flight.UpdatedAt); err != nil {
		return Result{}, err
	}
	if flight.ETAReview != nil && flight.ETAReview.Status != aman.ReviewNone {
		return Result{Flight: flight}, nil
	}
	if flight.State != aman.StateUnstable || flight.DataStatus != aman.DataFresh || flight.ArrivalBaseline == nil || flight.Prediction == nil ||
		flight.Prediction.OperationalReason != aman.OperationalReasonFirstUnstable {
		return Result{Flight: flight}, nil
	}
	if absolute(flight.Prediction.OperationalTETA.Sub(flight.ArrivalBaseline.ArrivalAt)) <= config.DiscrepancyThreshold {
		return Result{Flight: flight}, nil
	}

	deadline := createdAt.Add(config.ReviewDeadline)
	flight.ETAReview = &aman.ETAReview{
		Status:                    aman.ReviewPending,
		CreatedAt:                 createdAt,
		DeadlineAt:                deadline,
		InitialBaselineTETA:       flight.ArrivalBaseline.ArrivalAt,
		CalculatedOperationalTETA: flight.Prediction.OperationalTETA,
		SelectedTETA:              flight.Prediction.OperationalTETA,
	}
	flight.UpdatedAt = createdAt
	return Result{Flight: flight, Changed: true}, nil
}

type AcceptCalculated struct {
	At    time.Time
	Actor string
	Note  *string
}

func ResolveAcceptCalculated(flight aman.AMANFlight, command AcceptCalculated) (Result, error) {
	review, err := pendingReview(flight, command.At, command.Actor, command.Note)
	if err != nil {
		return Result{}, err
	}
	review.Status = aman.ReviewAcceptedCalculatedTETA
	review.SelectedTETA = review.CalculatedOperationalTETA
	resolveByActor(review, command.At, command.Actor, command.Note)
	flight.ETAReview = review
	flight.UpdatedAt = command.At
	return Result{Flight: flight, Changed: true}, nil
}

type KeepInitial struct {
	At    time.Time
	Actor string
	Note  *string
}

func ResolveKeepInitial(flight aman.AMANFlight, command KeepInitial) (Result, error) {
	review, err := pendingReview(flight, command.At, command.Actor, command.Note)
	if err != nil {
		return Result{}, err
	}
	updated, err := prediction.ApplyManualOperationalTETA(flight, review.InitialBaselineTETA, command.At)
	if err != nil {
		return Result{}, err
	}
	review.Status = aman.ReviewKeptInitialFPLETA
	review.SelectedTETA = review.InitialBaselineTETA
	resolveByActor(review, command.At, command.Actor, command.Note)
	updated.ETAReview = review
	return Result{Flight: updated, Changed: true}, nil
}

type SetManual struct {
	At         time.Time
	Actor      string
	Note       *string
	ManualTETA time.Time
}

func ResolveSetManual(flight aman.AMANFlight, command SetManual) (Result, error) {
	review, err := pendingReview(flight, command.At, command.Actor, command.Note)
	if err != nil {
		return Result{}, err
	}
	updated, err := prediction.ApplyManualOperationalTETA(flight, command.ManualTETA, command.At)
	if err != nil {
		return Result{}, err
	}
	manual := command.ManualTETA
	review.Status = aman.ReviewManualETA
	review.SelectedTETA = manual
	review.ManualTETA = &manual
	resolveByActor(review, command.At, command.Actor, command.Note)
	updated.ETAReview = review
	return Result{Flight: updated, Changed: true}, nil
}

// AutoAccept resolves at the exact persisted deadline even when the scheduler
// observes it later. Before the boundary it is a deterministic no-op.
func AutoAccept(flight aman.AMANFlight, observedAt time.Time) (Result, error) {
	if err := validActionTime(observedAt, flight.UpdatedAt); err != nil {
		return Result{}, err
	}
	if flight.ETAReview == nil || flight.ETAReview.Status != aman.ReviewPending {
		return Result{Flight: flight}, nil
	}
	if observedAt.Before(flight.ETAReview.DeadlineAt) {
		return Result{Flight: flight}, nil
	}
	review := cloneReview(flight.ETAReview)
	resolvedAt := review.DeadlineAt
	review.Status = aman.ReviewAutoAcceptedCalculatedTETA
	review.ResolvedAt = &resolvedAt
	review.SelectedTETA = review.CalculatedOperationalTETA
	flight.ETAReview = review
	flight.UpdatedAt = observedAt
	return Result{Flight: flight, Changed: true}, nil
}

type Reset struct {
	At    time.Time
	Actor string
	Note  *string
}

func ResolveReset(config prediction.Config, flight aman.AMANFlight, command Reset) (Result, error) {
	if err := validActorCommand(command.At, flight.UpdatedAt, command.Actor, command.Note); err != nil {
		return Result{}, err
	}
	if flight.ETAReview == nil || flight.ETAReview.Status == aman.ReviewNone || flight.ETAReview.Status == aman.ReviewPending {
		return Result{}, invalidTransition("ETA review reset requires a resolved review")
	}
	updated := flight
	var err error
	if flight.FreezeReason == aman.FreezeManual {
		updated, err = prediction.ReleaseManualOperationalTETA(config, flight, command.At)
		if err != nil {
			return Result{}, err
		}
	} else {
		updated.UpdatedAt = command.At
	}
	updated.ETAReview = nil
	return Result{Flight: updated, Changed: true}, nil
}

func pendingReview(flight aman.AMANFlight, at time.Time, actor string, note *string) (*aman.ETAReview, error) {
	if err := validActorCommand(at, flight.UpdatedAt, actor, note); err != nil {
		return nil, err
	}
	if flight.ETAReview == nil || flight.ETAReview.Status != aman.ReviewPending {
		return nil, invalidTransition("ETA review resolution requires a pending review")
	}
	if !at.Before(flight.ETAReview.DeadlineAt) {
		return nil, invalidTransition("ETA review deadline has elapsed")
	}
	return cloneReview(flight.ETAReview), nil
}

func validActorCommand(at, previous time.Time, actor string, note *string) error {
	if err := validActionTime(at, previous); err != nil {
		return err
	}
	if strings.TrimSpace(actor) == "" || actor != strings.TrimSpace(actor) {
		return invalidArgument("ETA review actor is required")
	}
	if note != nil && (strings.TrimSpace(*note) == "" || *note != strings.TrimSpace(*note)) {
		return invalidArgument("ETA review note must be trimmed and non-empty")
	}
	return nil
}

func validActionTime(at, previous time.Time) error {
	if at.IsZero() || at.Location() != time.UTC || at.Before(previous) {
		return invalidArgument("ETA review action time is invalid")
	}
	return nil
}

func resolveByActor(review *aman.ETAReview, at time.Time, actor string, note *string) {
	resolvedAt := at
	actorCopy := actor
	review.ResolvedAt = &resolvedAt
	review.Actor = &actorCopy
	review.Note = cloneString(note)
}

func cloneReview(review *aman.ETAReview) *aman.ETAReview {
	if review == nil {
		return nil
	}
	copy := *review
	copy.ResolvedAt = cloneTime(review.ResolvedAt)
	copy.Actor = cloneString(review.Actor)
	copy.Note = cloneString(review.Note)
	copy.ManualTETA = cloneTime(review.ManualTETA)
	return &copy
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func absolute(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

func invalidArgument(message string) error {
	return &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: message}
}

func invalidTransition(message string) error {
	return &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: fmt.Sprintf("ETA review: %s", message)}
}
