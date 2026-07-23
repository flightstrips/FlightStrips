// Package prediction owns the deterministic conversion of physical AMAN
// predictions into the operational TETA used by lifecycle and sequencing.
package prediction

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"FlightStrips/internal/aman"
)

const smoothingWindowSize = 3

// Config is the operational smoothing policy for one airport. Durations are
// absolute TETA movements, not wall-clock intervals. This makes an accepted
// persisted window replay identically after a restart.
type Config struct {
	Deadband           time.Duration
	MaximumRoutineMove time.Duration
	SuperstableHorizon time.Duration
	ExcessiveDrift     time.Duration
}

// DefaultConfig is deliberately explicit so configuration can persist its
// version alongside Prediction without inheriting process-local defaults.
func DefaultConfig() Config {
	return Config{
		Deadband:           30 * time.Second,
		MaximumRoutineMove: time.Minute,
		SuperstableHorizon: 10 * time.Minute,
		ExcessiveDrift:     2 * time.Minute,
	}
}

func (c Config) Validate() error {
	if c.Deadband < 0 {
		return fmt.Errorf("prediction deadband cannot be negative")
	}
	if c.MaximumRoutineMove <= 0 {
		return fmt.Errorf("prediction maximum routine movement must be greater than zero")
	}
	if c.SuperstableHorizon <= 0 {
		return fmt.Errorf("prediction Superstable horizon must be greater than zero")
	}
	if c.ExcessiveDrift <= 0 {
		return fmt.Errorf("prediction excessive drift threshold must be greater than zero")
	}
	return nil
}

// Input records the facts that may legitimately bypass routine smoothing.
// The state engine is responsible for authorizing manual/release flags before
// it calls this pure reducer.
type Input struct {
	Raw                aman.Prediction
	State              aman.FlightState
	RouteRevision      bool
	RunwayGroupChanged bool
	ManualOverride     *time.Time
	ReleaseFreeze      bool
	ConfirmedGoAround  bool
	Slot               *aman.Slot
}

// Result contains the replacement aggregate and the observable drift while a
// flight is Superstable. Excessive drift never resequences or releases a slot.
type Result struct {
	Flight         aman.AMANFlight
	RawDrift       time.Duration
	ExcessiveDrift bool
}

// Reduce accepts one raw prediction and returns a copy of the flight with its
// persisted three-sample smoothing window and operational TETA updated.
func Reduce(config Config, flight aman.AMANFlight, input Input) (Result, error) {
	if err := config.Validate(); err != nil {
		return Result{}, err
	}
	if !input.State.Valid() {
		return Result{}, fmt.Errorf("prediction state is invalid")
	}
	if err := validateRaw(input.Raw); err != nil {
		return Result{}, err
	}
	if input.ManualOverride != nil && (input.ManualOverride.IsZero() || input.ManualOverride.Location() != time.UTC) {
		return Result{}, fmt.Errorf("manual operational TETA must be UTC")
	}
	if staleRaw(flight.RawTETASamples, input.Raw) {
		// Reconciliation may deliver an already-applied or delayed predictor
		// result. It must not regress prediction provenance or UpdatedAt.
		return result(config, flight), nil
	}
	if input.Slot != nil && (flight.FreezeReason != aman.FreezeSuperstable || input.ReleaseFreeze || input.ManualOverride != nil || input.ConfirmedGoAround) {
		copy := cloneSlot(input.Slot)
		flight.Slot = &copy
	}

	previousState := flight.State
	flight.State = input.State
	flight.RawTETASamples = acceptRaw(flight.RawTETASamples, input.Raw)
	flight.UpdatedAt = input.Raw.GeneratedAt

	// A confirmed go-around is the only automatic release. It intentionally
	// bypasses every normal filter and leaves the sequence owner to assign a
	// new slot later.
	if input.ConfirmedGoAround {
		clearFreeze(&flight)
		flight.Slot = nil
		setOperational(&flight, input.Raw, input.Raw.RawTETA, aman.OperationalReasonGoAround)
		return result(config, flight), nil
	}

	if input.ManualOverride != nil {
		manual := *input.ManualOverride
		freezeAt := input.Raw.GeneratedAt
		flight.FreezeReason = aman.FreezeManual
		flight.FrozenAt = &freezeAt
		flight.FrozenOperationalTETA = &manual
		flight.FrozenSlot = nil
		setOperational(&flight, input.Raw, manual, aman.OperationalReasonManualOverride)
		return result(config, flight), nil
	}

	if input.ReleaseFreeze {
		clearFreeze(&flight)
	}

	// Raw TETA continues to update during a freeze so the caller can surface
	// drift, but normal surveillance, wind, direct-to and route changes cannot
	// move a Superstable operational value.
	if flight.FreezeReason == aman.FreezeSuperstable {
		setOperational(&flight, input.Raw, *flight.FrozenOperationalTETA, aman.OperationalReasonSuperstableFreeze)
		return result(config, flight), nil
	}
	if flight.FreezeReason == aman.FreezeManual {
		setOperational(&flight, input.Raw, *flight.FrozenOperationalTETA, aman.OperationalReasonManualOverride)
		return result(config, flight), nil
	}

	candidate, reason := routineOperational(config, flight, previousState, input)
	setOperational(&flight, input.Raw, candidate, reason)

	// Superstable begins at or inside the configured holding-fix horizon and
	// never until a complete slot exists.
	if atSuperstableBoundary(input.Raw.GeneratedAt, input.Raw.HoldingFixETA, config.SuperstableHorizon) &&
		flight.SelectedHolding != nil && strings.TrimSpace(*flight.SelectedHolding) != "" && flight.Slot != nil {
		freezeAt := input.Raw.GeneratedAt
		frozenTETA := candidate
		frozenSlot := cloneSlot(flight.Slot)
		flight.FreezeReason = aman.FreezeSuperstable
		flight.FrozenAt = &freezeAt
		flight.FrozenOperationalTETA = &frozenTETA
		flight.FrozenSlot = &frozenSlot
		flight.Prediction.OperationalReason = aman.OperationalReasonSuperstableFreeze
	}
	return result(config, flight), nil
}

// ApplyManualOperationalTETA applies an authorized controller selection
// without accepting a new physical prediction. It deliberately leaves RawTETA
// and the persisted smoothing window unchanged.
func ApplyManualOperationalTETA(flight aman.AMANFlight, operationalTETA, actionAt time.Time) (aman.AMANFlight, error) {
	if flight.Prediction == nil {
		return aman.AMANFlight{}, invalidArgument("manual operational TETA requires a current prediction")
	}
	if operationalTETA.IsZero() || operationalTETA.Location() != time.UTC || !operationalTETA.After(actionAt) {
		return aman.AMANFlight{}, invalidArgument("manual operational TETA must be a future UTC value")
	}
	if actionAt.IsZero() || actionAt.Location() != time.UTC || actionAt.Before(flight.UpdatedAt) {
		return aman.AMANFlight{}, invalidArgument("manual operational TETA action time is invalid")
	}

	freezeAt := actionAt
	frozenTETA := operationalTETA
	flight.FreezeReason = aman.FreezeManual
	flight.FrozenAt = &freezeAt
	flight.FrozenOperationalTETA = &frozenTETA
	flight.FrozenSlot = nil
	prediction := *flight.Prediction
	setOperational(&flight, prediction, operationalTETA, aman.OperationalReasonManualOverride)
	flight.UpdatedAt = actionAt
	return flight, nil
}

// ReleaseManualOperationalTETA returns a controller-selected value to the
// normal #314 smoothing policy without accepting or synthesizing a raw sample.
func ReleaseManualOperationalTETA(config Config, flight aman.AMANFlight, actionAt time.Time) (aman.AMANFlight, error) {
	if err := config.Validate(); err != nil {
		return aman.AMANFlight{}, err
	}
	if flight.FreezeReason != aman.FreezeManual || flight.Prediction == nil || len(flight.RawTETASamples) == 0 {
		return aman.AMANFlight{}, invalidTransition("manual operational TETA is not active")
	}
	if actionAt.IsZero() || actionAt.Location() != time.UTC || actionAt.Before(flight.UpdatedAt) {
		return aman.AMANFlight{}, invalidArgument("manual operational TETA action time is invalid")
	}

	prediction := *flight.Prediction
	clearFreeze(&flight)
	candidate, reason := routineOperational(config, flight, flight.State, Input{Raw: prediction, State: flight.State})
	setOperational(&flight, prediction, candidate, reason)
	flight.UpdatedAt = actionAt
	return flight, nil
}

func validateRaw(raw aman.Prediction) error {
	// Prediction.Validate deliberately validates the operational fields too.
	// The physical predictor supplies only RawTETA; set temporary valid values
	// here so the shared provenance and confidence validation remains central.
	copy := raw
	copy.OperationalTETA = raw.RawTETA
	copy.OperationalReason = aman.OperationalReasonPredicted
	if err := copy.Validate(); err != nil {
		return fmt.Errorf("raw prediction: %w", err)
	}
	return nil
}

func routineOperational(config Config, flight aman.AMANFlight, previousState aman.FlightState, input Input) (time.Time, aman.OperationalReason) {
	if input.RouteRevision {
		return input.Raw.RawTETA, aman.OperationalReasonRouteRevision
	}
	if input.RunwayGroupChanged {
		return input.Raw.RawTETA, aman.OperationalReasonRunwayGroupChanged
	}
	if input.State == aman.StateUnstable && previousState != aman.StateUnstable {
		return input.Raw.RawTETA, aman.OperationalReasonFirstUnstable
	}

	median := median(inputSamples(flight.RawTETASamples))
	if flight.Prediction == nil {
		return median, aman.OperationalReasonPredicted
	}
	previous := flight.Prediction.OperationalTETA
	delta := median.Sub(previous)
	if absolute(delta) <= config.Deadband {
		return previous, aman.OperationalReasonDeadband
	}
	if absolute(delta) > config.MaximumRoutineMove {
		if delta < 0 {
			return previous.Add(-config.MaximumRoutineMove), aman.OperationalReasonRateLimited
		}
		return previous.Add(config.MaximumRoutineMove), aman.OperationalReasonRateLimited
	}
	return median, aman.OperationalReasonSmoothed
}

func result(config Config, flight aman.AMANFlight) Result {
	result := Result{Flight: flight}
	if flight.Prediction == nil || flight.FreezeReason != aman.FreezeSuperstable || flight.FrozenOperationalTETA == nil {
		return result
	}
	result.RawDrift = absolute(flight.Prediction.RawTETA.Sub(*flight.FrozenOperationalTETA))
	result.ExcessiveDrift = result.RawDrift > config.ExcessiveDrift
	return result
}

func setOperational(flight *aman.AMANFlight, raw aman.Prediction, operational time.Time, reason aman.OperationalReason) {
	raw.OperationalTETA = operational
	raw.OperationalReason = reason
	flight.Prediction = &raw
}

func acceptRaw(history []aman.RawTETASample, raw aman.Prediction) []aman.RawTETASample {
	accepted := slices.Clone(history)
	if staleRaw(accepted, raw) {
		return accepted
	}
	accepted = append(accepted, aman.RawTETASample{TETA: raw.RawTETA, GeneratedAt: raw.GeneratedAt})
	if len(accepted) > smoothingWindowSize {
		accepted = accepted[len(accepted)-smoothingWindowSize:]
	}
	return accepted
}

func staleRaw(history []aman.RawTETASample, raw aman.Prediction) bool {
	return len(history) != 0 && !raw.GeneratedAt.After(history[len(history)-1].GeneratedAt)
}

func inputSamples(history []aman.RawTETASample) []time.Time {
	values := make([]time.Time, len(history))
	for index, sample := range history {
		values[index] = sample.TETA
	}
	return values
}

func median(values []time.Time) time.Time {
	values = slices.Clone(values)
	sort.Slice(values, func(i, j int) bool { return values[i].Before(values[j]) })
	middle := len(values) / 2
	if len(values)%2 != 0 {
		return values[middle]
	}
	return values[middle-1].Add(values[middle].Sub(values[middle-1]) / 2)
}

func atSuperstableBoundary(now time.Time, holdingFixETA *time.Time, horizon time.Duration) bool {
	return holdingFixETA != nil && !holdingFixETA.After(now.Add(horizon)) && holdingFixETA.After(now)
}

func clearFreeze(flight *aman.AMANFlight) {
	flight.FreezeReason = aman.FreezeNone
	flight.FrozenAt = nil
	flight.FrozenOperationalTETA = nil
	flight.FrozenSlot = nil
}

func cloneSlot(slot *aman.Slot) aman.Slot {
	return *slot
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
	return &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: message}
}
