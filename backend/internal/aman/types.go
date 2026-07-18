package aman

import (
	"fmt"
	"strings"
	"time"
)

// FlightID identifies one active AMAN flight. It is generated once and is not
// derived from a mutable callsign.
type FlightID string

// RunwayGroupID identifies an airport-specific AMAN runway group.
type RunwayGroupID string

// SequenceRevision is the monotonically increasing committed revision for an
// airport AMAN state.
type SequenceRevision uint64

type FlightState string

const (
	StatePlanned  FlightState = "planned"
	StateAirborne FlightState = "airborne"
	StateUnstable FlightState = "unstable"
	StateStable   FlightState = "stable"
	StateLanded   FlightState = "landed"
	StateGoAround FlightState = "go_around"
	StateRemoved  FlightState = "removed"
)

func (s FlightState) Valid() bool {
	switch s {
	case StatePlanned, StateAirborne, StateUnstable, StateStable, StateLanded, StateGoAround, StateRemoved:
		return true
	default:
		return false
	}
}

type DataStatus string

const (
	DataFresh        DataStatus = "fresh"
	DataStale        DataStatus = "stale"
	DataDisconnected DataStatus = "disconnected"
)

func (s DataStatus) Valid() bool {
	return s == DataFresh || s == DataStale || s == DataDisconnected
}

type RolloutMode string

const (
	ModeDisabled      RolloutMode = "disabled"
	ModeShadow        RolloutMode = "shadow"
	ModeReadOnly      RolloutMode = "read_only"
	ModeAuthoritative RolloutMode = "authoritative"
)

func (m RolloutMode) Valid() bool {
	switch m {
	case ModeDisabled, ModeShadow, ModeReadOnly, ModeAuthoritative:
		return true
	default:
		return false
	}
}

type Confidence string

const (
	ConfidenceUnknown Confidence = "unknown"
	ConfidenceLow     Confidence = "low"
	ConfidenceMedium  Confidence = "medium"
	ConfidenceHigh    Confidence = "high"
)

func (c Confidence) Valid() bool {
	switch c {
	case ConfidenceUnknown, ConfidenceLow, ConfidenceMedium, ConfidenceHigh:
		return true
	default:
		return false
	}
}

// BaselineSource identifies the deterministic input used before the
// route-aware predictor can calculate a replacement raw prediction.
//
// The source is retained with a held first-airborne baseline so a restored
// aggregate retains the exact provenance of its initial prediction.
type BaselineSource string

const (
	BaselineSourceNone                                            BaselineSource = "none"
	BaselineSourcePlannedEOBTFiledEET                             BaselineSource = "planned_eobt_filed_eet"
	BaselineSourcePlannedEstimatedDepartureFiledEET               BaselineSource = "planned_estimated_departure_filed_eet"
	BaselineSourcePlannedEOBTAPIEstimatedFlightTime               BaselineSource = "planned_eobt_api_estimated_flight_time"
	BaselineSourcePlannedEstimatedDepartureAPIEstimatedFlightTime BaselineSource = "planned_estimated_departure_api_estimated_flight_time"
	BaselineSourceAirborneFiledEET                                BaselineSource = "airborne_filed_eet"
	BaselineSourceAirborneAPIEstimatedFlightTime                  BaselineSource = "airborne_api_estimated_flight_time"
	BaselineSourceAirborneGreatCircle                             BaselineSource = "airborne_great_circle"
	BaselineSourceRouteAware                                      BaselineSource = "route_aware"
)

func (s BaselineSource) Valid() bool {
	switch s {
	case BaselineSourceNone,
		BaselineSourcePlannedEOBTFiledEET,
		BaselineSourcePlannedEstimatedDepartureFiledEET,
		BaselineSourcePlannedEOBTAPIEstimatedFlightTime,
		BaselineSourcePlannedEstimatedDepartureAPIEstimatedFlightTime,
		BaselineSourceAirborneFiledEET,
		BaselineSourceAirborneAPIEstimatedFlightTime,
		BaselineSourceAirborneGreatCircle,
		BaselineSourceRouteAware:
		return true
	default:
		return false
	}
}

// FreezeReason is the only representation of an operational TETA/slot freeze.
// A flight must not add a parallel state or locked boolean.
type FreezeReason string

const (
	FreezeNone        FreezeReason = "none"
	FreezeSuperstable FreezeReason = "superstable"
	FreezeManual      FreezeReason = "manual"
)

func (r FreezeReason) Valid() bool {
	switch r {
	case FreezeNone, FreezeSuperstable, FreezeManual:
		return true
	default:
		return false
	}
}

// FlightObservation is the provider-neutral reconciliation input. Adapters
// map their vendor data to this value before it reaches AMAN.
type FlightObservation struct {
	FlightID        FlightID
	VATSIMCID       string
	Callsign        string
	Origin          string
	Destination     string
	AircraftType    *string
	WakeCategory    *string
	FiledRoute      *string
	RequestedLevel  *int
	PlannedTiming   *PlannedTiming
	FlightPlan      FlightPlanFact
	Surveillance    *SurveillanceFact
	TakeoffDetected *time.Time
	ReconciledAt    time.Time
	SourceStatus    DataStatus
}

// PlannedTiming contains provider-neutral planned times. Durations are domain
// time.Duration values; owning wire packages serialize them as whole seconds.
type PlannedTiming struct {
	EstimatedOffBlockTime *time.Time
	EstimatedEnrouteTime  *time.Duration
}

// FlightPlanFact records the source ordering facts for a flight plan.
type FlightPlanFact struct {
	Revision   *uint64
	ObservedAt *time.Time
}

// SurveillanceFact is an optional positional observation. Coordinates are
// WGS84 degrees, altitude is feet, ground speed is knots, and track is true
// degrees in [0,360).
type SurveillanceFact struct {
	LatitudeDegrees  float64
	LongitudeDegrees float64
	AltitudeFeet     *int
	GroundspeedKnots *float64
	TrackTrueDegrees *float64
	Sequence         *uint64
	ObservedAt       *time.Time
}

// Prediction separates a physical/model result from the backend-owned value
// used for lifecycle and sequencing. RawTETA continues to move during a
// freeze; OperationalTETA is the only value consumers sequence.
type Prediction struct {
	RawTETA           time.Time
	OperationalTETA   time.Time
	OperationalReason string

	GeneratedAt       time.Time
	InputObservedAt   time.Time
	Confidence        Confidence
	Publishable       bool
	DegradationReason *string

	DatasetVersion string
	GeometryDigest string
	DistanceToGoNM *float64
	HoldingFixETA  *time.Time

	ModelVersion         string
	ConfigVersion        string
	PerformanceProfileID *string
	WeatherSource        *string
	Sources              []string
}

// BaselineState is the narrow persisted result of the first normal airborne
// calculation. It is held unchanged until the owning state engine supplies an
// explicit Unstable-review reset. The baseline predictor owns how this value
// is created; the aggregate owns its persistence through StateCommit.
type BaselineState struct {
	ArrivalAt            time.Time
	AirborneSensedAt     time.Time
	Source               BaselineSource
	Confidence           Confidence
	DegradationReason    *string
	FlightPlanRevision   *uint64
	FlightPlanObservedAt time.Time
	ModelVersion         string
	ConfigVersion        string
}

// Slot is a committed sequencing result. It intentionally has no locked
// field: freezes are represented exclusively by AMANFlight.FreezeReason.
type Slot struct {
	Time          time.Time
	RunwayGroupID RunwayGroupID
	Sequence      int
	Revision      SequenceRevision
	Reason        string
}

// RouteFact is the currently active operational route fact. Transport and
// tracking-authority details belong to the route-fact owner, not this core
// contract.
type RouteFact struct {
	ID         string
	Fix        string
	ObservedAt time.Time
}

// ETAReview and GoAroundDetectionState reserve the aggregate's owned state
// without defining command, persistence, or detector implementation details.
// Their full workflows are owned by their respective components.
type ETAReview struct {
	Status string
}

type GoAroundDetectionState struct {
	EpisodeID *string
}

// RunwayGroupPolicy is the airport-state identity for a runway group. The
// sequence component owns the policy's rate and spacing declarations.
type RunwayGroupPolicy struct {
	ID RunwayGroupID
}

// AMANFlight is the persisted aggregate shape. All operational TETA, state,
// freeze, slot, and order changes are backend-owned.
type AMANFlight struct {
	ID FlightID
	// VATSIMCID remains bound to the aggregate while CurrentCallsign may be
	// corrected without rekeying FlightID.
	VATSIMCID             string
	CurrentCallsign       string
	State                 FlightState
	DataStatus            DataStatus
	Prediction            *Prediction
	ArrivalBaseline       *BaselineState
	SelectedRunwayGroup   *RunwayGroupID
	SelectedFeeder        *string
	SelectedHolding       *string
	ActiveRouteFact       *RouteFact
	FreezeReason          FreezeReason
	FrozenAt              *time.Time
	FrozenOperationalTETA *time.Time
	Slot                  *Slot
	Order                 *int
	ETAReview             *ETAReview
	GoAroundDetection     *GoAroundDetectionState
	UpdatedAt             time.Time
}

// AirportState is the sole source for one coherent AMAN replacement state.
// Revisions are allocated only when a committed domain result changes it.
type AirportState struct {
	Airport       string
	Revision      SequenceRevision
	GeneratedAt   time.Time
	PolicyVersion string
	Mode          RolloutMode
	Authoritative bool
	Flights       []AMANFlight
	RunwayGroups  []RunwayGroupPolicy
}

// CommandMetadata is shared by typed command values. It deliberately does not
// use a kind plus nullable fields; each command owner defines a separate type.
type CommandMetadata struct {
	CommandID        string
	ExpectedRevision SequenceRevision
}

type ErrorClass string

const (
	ErrorInvalidArgument              ErrorClass = "invalid_argument"
	ErrorNotFound                     ErrorClass = "not_found"
	ErrorRevisionConflict             ErrorClass = "revision_conflict"
	ErrorUnauthorized                 ErrorClass = "unauthorized"
	ErrorInvalidTransition            ErrorClass = "invalid_transition"
	ErrorDependencyUnavailable        ErrorClass = "dependency_unavailable"
	ErrorDegradedOrIncompleteGeometry ErrorClass = "degraded_or_incomplete_geometry"
	ErrorUnsupportedLeg               ErrorClass = "unsupported_leg"
	ErrorDatasetMismatch              ErrorClass = "dataset_mismatch"
	ErrorCorruptData                  ErrorClass = "corrupt_data"
	ErrorActiveFlightConflict         ErrorClass = "active_flight_conflict"
)

func (c ErrorClass) Valid() bool {
	switch c {
	case ErrorInvalidArgument, ErrorNotFound, ErrorRevisionConflict, ErrorUnauthorized,
		ErrorInvalidTransition, ErrorDependencyUnavailable, ErrorDegradedOrIncompleteGeometry,
		ErrorUnsupportedLeg, ErrorDatasetMismatch, ErrorCorruptData, ErrorActiveFlightConflict:
		return true
	default:
		return false
	}
}

// DomainError provides a stable, provider-independent error class for a
// transport adapter to map once at its boundary.
type DomainError struct {
	Class   ErrorClass
	Message string
}

func (e *DomainError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Message == "" {
		return string(e.Class)
	}
	return fmt.Sprintf("%s: %s", e.Class, e.Message)
}

// Validate checks the unit and nullability rules that can be established at
// the neutral observation boundary without imposing a source implementation.
func (o FlightObservation) Validate() error {
	if strings.TrimSpace(string(o.FlightID)) == "" || strings.TrimSpace(o.VATSIMCID) == "" ||
		strings.TrimSpace(o.Callsign) == "" || strings.TrimSpace(o.Origin) == "" || strings.TrimSpace(o.Destination) == "" {
		return invalid("flight observation identity is incomplete")
	}
	if !o.SourceStatus.Valid() {
		return invalid("source status is invalid")
	}
	if err := requireUTCTime("reconciled at", o.ReconciledAt); err != nil {
		return err
	}
	if o.FlightPlan.ObservedAt != nil {
		if err := requireUTCTime("flight plan observed at", *o.FlightPlan.ObservedAt); err != nil {
			return err
		}
	}
	if o.TakeoffDetected != nil {
		if err := requireUTCTime("takeoff detected at", *o.TakeoffDetected); err != nil {
			return err
		}
	}
	if o.Surveillance != nil {
		return o.Surveillance.validate()
	}
	return nil
}

func (s SurveillanceFact) validate() error {
	if s.LatitudeDegrees < -90 || s.LatitudeDegrees > 90 || s.LongitudeDegrees < -180 || s.LongitudeDegrees > 180 {
		return invalid("surveillance coordinates are invalid")
	}
	if s.GroundspeedKnots != nil && *s.GroundspeedKnots < 0 {
		return invalid("ground speed cannot be negative")
	}
	if s.TrackTrueDegrees != nil && (*s.TrackTrueDegrees < 0 || *s.TrackTrueDegrees >= 360) {
		return invalid("track must be true degrees in [0,360)")
	}
	if s.ObservedAt == nil {
		return invalid("surveillance observed at is required")
	}
	return requireUTCTime("surveillance observed at", *s.ObservedAt)
}

func (p Prediction) Validate() error {
	if err := requireUTCTime("raw TETA", p.RawTETA); err != nil {
		return err
	}
	if err := requireUTCTime("operational TETA", p.OperationalTETA); err != nil {
		return err
	}
	if strings.TrimSpace(p.OperationalReason) == "" {
		return invalid("operational reason is required")
	}
	if err := requireUTCTime("generated at", p.GeneratedAt); err != nil {
		return err
	}
	if err := requireUTCTime("input observed at", p.InputObservedAt); err != nil {
		return err
	}
	if !p.Confidence.Valid() {
		return invalid("confidence is invalid")
	}
	if strings.TrimSpace(p.DatasetVersion) == "" || strings.TrimSpace(p.GeometryDigest) == "" ||
		strings.TrimSpace(p.ModelVersion) == "" || strings.TrimSpace(p.ConfigVersion) == "" {
		return invalid("prediction provenance is incomplete")
	}
	if p.DistanceToGoNM != nil && *p.DistanceToGoNM < 0 {
		return invalid("distance to go cannot be negative")
	}
	if p.HoldingFixETA != nil {
		if err := requireUTCTime("holding fix ETA", *p.HoldingFixETA); err != nil {
			return err
		}
	}
	if p.Sources == nil {
		return invalid("prediction sources must be explicit")
	}
	return nil
}

func (b BaselineState) Validate() error {
	if err := requireUTCTime("baseline arrival", b.ArrivalAt); err != nil {
		return err
	}
	if err := requireUTCTime("baseline airborne sensed at", b.AirborneSensedAt); err != nil {
		return err
	}
	if err := requireUTCTime("baseline flight plan observed at", b.FlightPlanObservedAt); err != nil {
		return err
	}
	if !b.Source.Valid() || !b.Source.holdsAirborneBaseline() || !b.Confidence.Valid() {
		return invalid("baseline provenance is invalid")
	}
	if strings.TrimSpace(b.ModelVersion) == "" || strings.TrimSpace(b.ConfigVersion) == "" {
		return invalid("baseline model and config versions are required")
	}
	return nil
}

func (s BaselineSource) holdsAirborneBaseline() bool {
	return s == BaselineSourceAirborneFiledEET ||
		s == BaselineSourceAirborneAPIEstimatedFlightTime ||
		s == BaselineSourceAirborneGreatCircle
}

func (f AMANFlight) Validate() error {
	if strings.TrimSpace(string(f.ID)) == "" {
		return invalid("flight ID is required")
	}
	if !f.State.Valid() || !f.DataStatus.Valid() || !f.FreezeReason.Valid() {
		return invalid("flight has an invalid state")
	}
	if f.State != StateRemoved && (!isTrimmedNonEmpty(f.VATSIMCID) || !isTrimmedNonEmpty(f.CurrentCallsign)) {
		return invalid("active flight requires VATSIM CID and current callsign")
	}
	if f.Prediction != nil {
		if err := f.Prediction.Validate(); err != nil {
			return err
		}
	}
	if f.ArrivalBaseline != nil {
		if err := f.ArrivalBaseline.Validate(); err != nil {
			return err
		}
	}
	if err := f.validateFreeze(); err != nil {
		return err
	}
	if f.Slot != nil {
		if err := f.Slot.validate(); err != nil {
			return err
		}
	}
	return requireUTCTime("updated at", f.UpdatedAt)
}

func (f AMANFlight) validateFreeze() error {
	if f.FreezeReason == FreezeNone {
		if f.FrozenAt != nil || f.FrozenOperationalTETA != nil {
			return invalid("unfrozen flight cannot retain freeze values")
		}
		return nil
	}
	if f.FrozenAt == nil || f.FrozenOperationalTETA == nil {
		return invalid("frozen flight requires timestamp and operational TETA")
	}
	if err := requireUTCTime("frozen at", *f.FrozenAt); err != nil {
		return err
	}
	return requireUTCTime("frozen operational TETA", *f.FrozenOperationalTETA)
}

func (s Slot) validate() error {
	if err := requireUTCTime("slot time", s.Time); err != nil {
		return err
	}
	if strings.TrimSpace(string(s.RunwayGroupID)) == "" || s.Sequence < 1 || strings.TrimSpace(s.Reason) == "" {
		return invalid("slot is incomplete")
	}
	return nil
}

func (s AirportState) Validate() error {
	if strings.TrimSpace(s.Airport) == "" || strings.TrimSpace(s.PolicyVersion) == "" || !s.Mode.Valid() {
		return invalid("airport state is incomplete")
	}
	if err := requireUTCTime("generated at", s.GeneratedAt); err != nil {
		return err
	}
	flightIDs := make(map[FlightID]struct{}, len(s.Flights))
	for _, flight := range s.Flights {
		if err := flight.Validate(); err != nil {
			return err
		}
		if _, exists := flightIDs[flight.ID]; exists {
			return invalid("airport state contains duplicate flight ID")
		}
		flightIDs[flight.ID] = struct{}{}
		if flight.Slot != nil && flight.Slot.Revision != s.Revision {
			return invalid("slot revision must match airport state revision")
		}
	}
	groupIDs := make(map[RunwayGroupID]struct{}, len(s.RunwayGroups))
	for _, group := range s.RunwayGroups {
		if strings.TrimSpace(string(group.ID)) == "" {
			return invalid("runway group ID is required")
		}
		if _, exists := groupIDs[group.ID]; exists {
			return invalid("airport state contains duplicate runway group")
		}
		groupIDs[group.ID] = struct{}{}
	}
	return nil
}

func requireUTCTime(name string, value time.Time) error {
	if value.IsZero() {
		return invalid(name + " is required")
	}
	if value.Location() != time.UTC {
		return invalid(name + " must be UTC")
	}
	return nil
}

func invalid(message string) error {
	return &DomainError{Class: ErrorInvalidArgument, Message: message}
}

func isTrimmedNonEmpty(value string) bool {
	return value != "" && strings.TrimSpace(value) == value
}
