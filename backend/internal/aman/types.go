package aman

import (
	"fmt"
	"math"
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

// LifecycleReason records why the flight entered its current lifecycle state.
// It is persisted with the aggregate so restart and replay do not have to
// infer a transition from the current prediction or source status.
type LifecycleReason string

const (
	LifecycleReasonInitial             LifecycleReason = "initial"
	LifecycleReasonAirborneDetected    LifecycleReason = "airborne_detected"
	LifecycleReasonUnstableHorizon     LifecycleReason = "unstable_horizon"
	LifecycleReasonStableHorizon       LifecycleReason = "stable_horizon"
	LifecycleReasonGoAroundConfirmed   LifecycleReason = "go_around_confirmed"
	LifecycleReasonLandingConfirmed    LifecycleReason = "landing_confirmed"
	LifecycleReasonSuddenAppearance    LifecycleReason = "sudden_appearance"
	LifecycleReasonManualRemoval       LifecycleReason = "manual_removal"
	LifecycleReasonLandedTimeout       LifecycleReason = "landed_timeout"
	LifecycleReasonPlannedCancellation LifecycleReason = "planned_cancellation"
	LifecycleReasonSourceDisappearance LifecycleReason = "source_disappearance"
)

func (r LifecycleReason) Valid() bool {
	switch r {
	case LifecycleReasonInitial,
		LifecycleReasonAirborneDetected,
		LifecycleReasonUnstableHorizon,
		LifecycleReasonStableHorizon,
		LifecycleReasonGoAroundConfirmed,
		LifecycleReasonLandingConfirmed,
		LifecycleReasonSuddenAppearance,
		LifecycleReasonManualRemoval,
		LifecycleReasonLandedTimeout,
		LifecycleReasonPlannedCancellation,
		LifecycleReasonSourceDisappearance:
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

// OperationalReason explains why OperationalTETA differs from, or follows,
// the latest physical prediction. Consumers must use OperationalTETA rather
// than inferring policy from RawTETA or the flight state.
type OperationalReason string

const (
	OperationalReasonPredicted          OperationalReason = "predicted"
	OperationalReasonSmoothed           OperationalReason = "smoothed"
	OperationalReasonDeadband           OperationalReason = "deadband"
	OperationalReasonRateLimited        OperationalReason = "rate_limited"
	OperationalReasonRouteRevision      OperationalReason = "route_revision"
	OperationalReasonRunwayGroupChanged OperationalReason = "runway_group_changed"
	OperationalReasonFirstUnstable      OperationalReason = "first_unstable"
	OperationalReasonManualOverride     OperationalReason = "manual_override"
	OperationalReasonSuperstableFreeze  OperationalReason = "superstable_freeze"
	OperationalReasonGoAround           OperationalReason = "go_around"
)

func (r OperationalReason) Valid() bool {
	switch r {
	case OperationalReasonPredicted,
		OperationalReasonSmoothed,
		OperationalReasonDeadband,
		OperationalReasonRateLimited,
		OperationalReasonRouteRevision,
		OperationalReasonRunwayGroupChanged,
		OperationalReasonFirstUnstable,
		OperationalReasonManualOverride,
		OperationalReasonSuperstableFreeze,
		OperationalReasonGoAround:
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

// BaselineDegradationReason is persisted with a successful degraded baseline.
// It is deliberately separate from transient unavailable reasons so corrupt
// aggregate JSON cannot claim a successful fallback with arbitrary text.
type BaselineDegradationReason string

const (
	BaselineDegradationFiledEETMissingAPIUsed BaselineDegradationReason = "filed_eet_missing_api_estimated_flight_time_used"
	BaselineDegradationGreatCircleUsed        BaselineDegradationReason = "route_geometry_or_duration_unavailable_great_circle_used"
)

func (r BaselineDegradationReason) Valid() bool {
	return r == BaselineDegradationFiledEETMissingAPIUsed || r == BaselineDegradationGreatCircleUsed
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
	OperationalReason OperationalReason

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

// RawTETASample is one accepted physical/model output in the persisted
// smoothing window. The window is deliberately small and complete: retaining
// the TETA together with its generation time makes a restart produce the same
// median and rate-limited result without re-reading an external source.
type RawTETASample struct {
	TETA        time.Time
	GeneratedAt time.Time
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
	DegradationReason    *BaselineDegradationReason
	SpeedDefaultsVersion string
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
	State      RouteFactState
}

// RouteFactState is supplied by the direct-to fact owner. Trajectory consumes
// it and never infers expiry from wall-clock time.
type RouteFactState string

const (
	RouteFactActive  RouteFactState = "active"
	RouteFactExpired RouteFactState = "expired"
)

func (s RouteFactState) Valid() bool { return s == "" || s == RouteFactActive || s == RouteFactExpired }

// RouteProgress is task-owned projection state persisted inside AMANFlight's
// aggregate JSON. The compatibility identity intentionally includes exact
// manifest/terminal revisions so same-cycle terminal activation resets safely.
type RouteProgress struct {
	GeometryDigest     string
	ManifestRevision   int64
	TerminalDigest     string
	FlightPlanRevision uint64
	RouteFactID        string
	RunwayGroupID      RunwayGroupID
	LegIndex           int
	RejoinLegIndex     int
	AlongTrackNM       float64
}

// ETAReviewStatus is the durable state of the first-Unstable TETA review.
// A nil AMANFlight.ETAReview has the same meaning as ReviewNone; the explicit
// zero status is retained for wire mappings that cannot represent a nil value.
type ETAReviewStatus string

const (
	ReviewNone                       ETAReviewStatus = "none"
	ReviewPending                    ETAReviewStatus = "pending"
	ReviewAcceptedCalculatedTETA     ETAReviewStatus = "accepted_calculated_teta"
	ReviewKeptInitialFPLETA          ETAReviewStatus = "kept_initial_fpl_eta"
	ReviewManualETA                  ETAReviewStatus = "manual_eta"
	ReviewAutoAcceptedCalculatedTETA ETAReviewStatus = "auto_accepted_calculated_teta"
)

func (s ETAReviewStatus) Valid() bool {
	switch s {
	case ReviewNone, ReviewPending, ReviewAcceptedCalculatedTETA,
		ReviewKeptInitialFPLETA, ReviewManualETA, ReviewAutoAcceptedCalculatedTETA:
		return true
	default:
		return false
	}
}

// ETAReview records both alternatives and the selected operational value so a
// restart never has to reconstruct the controller decision from prediction
// history. Raw TETA remains exclusively inside Prediction.
type ETAReview struct {
	Status                    ETAReviewStatus
	CreatedAt                 time.Time
	DeadlineAt                time.Time
	ResolvedAt                *time.Time
	Actor                     *string
	Note                      *string
	InitialBaselineTETA       time.Time
	CalculatedOperationalTETA time.Time
	SelectedTETA              time.Time
	ManualTETA                *time.Time
}

// OperationalException records an explicit degraded path that is orthogonal
// to lifecycle and ETA discrepancy review state.
type OperationalException struct {
	Reason     OperationalExceptionReason
	DetectedAt time.Time
}

type OperationalExceptionReason string

const (
	OperationalExceptionSuddenInsideFreeze OperationalExceptionReason = "sudden_appearance_inside_freeze_horizon_manual_review"
)

func (r OperationalExceptionReason) Valid() bool {
	return r == OperationalExceptionSuddenInsideFreeze
}

// GoAroundEvidence is one accepted surveillance sample retained by the
// go-around detector. Evidence is persisted oldest-first so replay and restart
// do not need to recover provider history.
type GoAroundEvidence struct {
	ObservedAt         time.Time
	Sequence           *uint64
	LatitudeDegrees    float64
	LongitudeDegrees   float64
	AltitudeFeet       int
	GroundspeedKnots   float64
	TrackTrueDegrees   *float64
	ClimbEvidence      bool
	TrackAwayEvidence  bool
	RunwayExitEvidence bool
}

// GoAroundDetectionState is the complete persisted detector cursor. Episode
// is incremented only when an approach arms; LastEmittedEpisode makes an
// already-confirmed episode idempotent across commit retries and restart.
type GoAroundDetectionState struct {
	PolicyVersion           string
	Evidence                []GoAroundEvidence
	ArmCount                int
	ClimbCount              int
	TrackAwayCount          int
	RunwayExitCount         int
	Armed                   bool
	ArmedAt                 *time.Time
	ArmedCorridorID         string
	Episode                 uint64
	LastEmittedEpisode      uint64
	LastProcessedAt         *time.Time
	LastProcessedSequence   *uint64
	ThresholdCrossed        bool
	LastControllerCommandID string
}

// LifecycleState is the persisted ordering cursor and current-state entry
// metadata owned by the lifecycle reducer. The event fingerprint makes an
// exact retry idempotent; LastEventAt rejects delayed events that would
// otherwise regress the aggregate after a restart.
type LifecycleState struct {
	EnteredAt            time.Time
	Reason               LifecycleReason
	LastEventID          string
	LastEventFingerprint string
	LastEventAt          time.Time
	// ReconciliationPending is set across stale/disconnected periods and is
	// cleared only when a current fresh snapshot explicitly observes or omits
	// the flight. A fresh source-status transition alone cannot resume timers.
	ReconciliationPending bool
	Absence               *AbsenceState
}

// AbsenceState is the persisted disappearance timer. RemovalDueAt is nil
// while source data is unknown. Remaining preserves only the already-earned
// fresh-data delay, so outage time cannot advance removal after restart.
type AbsenceState struct {
	MissingSince time.Time
	RemovalDueAt *time.Time
	Remaining    time.Duration
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
	RawTETASamples        []RawTETASample
	ArrivalBaseline       *BaselineState
	SelectedRunwayGroup   *RunwayGroupID
	SelectedFeeder        *string
	SelectedHolding       *string
	ActiveRouteFact       *RouteFact
	RouteProgress         *RouteProgress
	FreezeReason          FreezeReason
	FrozenAt              *time.Time
	FrozenOperationalTETA *time.Time
	FrozenSlot            *Slot
	Slot                  *Slot
	Order                 *int
	ETAReview             *ETAReview
	OperationalException  *OperationalException
	GoAroundDetection     *GoAroundDetectionState
	Lifecycle             *LifecycleState
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
	if !p.OperationalReason.Valid() {
		return invalid("operational reason is invalid")
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
	if !b.ArrivalAt.After(b.AirborneSensedAt) {
		return invalid("baseline arrival must be after airborne sensed at")
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
	switch b.Source {
	case BaselineSourceAirborneFiledEET:
		if b.DegradationReason != nil || b.SpeedDefaultsVersion != "" {
			return invalid("filed airborne baseline must not claim degradation or speed defaults")
		}
	case BaselineSourceAirborneAPIEstimatedFlightTime:
		if b.DegradationReason == nil || *b.DegradationReason != BaselineDegradationFiledEETMissingAPIUsed || b.SpeedDefaultsVersion != "" {
			return invalid("API airborne baseline provenance is invalid")
		}
	case BaselineSourceAirborneGreatCircle:
		if b.DegradationReason == nil || *b.DegradationReason != BaselineDegradationGreatCircleUsed || strings.TrimSpace(b.SpeedDefaultsVersion) == "" {
			return invalid("great-circle airborne baseline provenance is invalid")
		}
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
	if len(f.RawTETASamples) > 3 {
		return invalid("raw TETA smoothing window exceeds three samples")
	}
	for index, sample := range f.RawTETASamples {
		if err := sample.Validate(); err != nil {
			return err
		}
		if index > 0 && !sample.GeneratedAt.After(f.RawTETASamples[index-1].GeneratedAt) {
			return invalid("raw TETA smoothing samples must be ordered by generation time")
		}
	}
	if f.ArrivalBaseline != nil {
		if err := f.ArrivalBaseline.Validate(); err != nil {
			return err
		}
	}
	if f.ETAReview != nil {
		if err := f.ETAReview.Validate(); err != nil {
			return err
		}
		if f.ETAReview.Status != ReviewNone && (f.ArrivalBaseline == nil || !f.ArrivalBaseline.ArrivalAt.Equal(f.ETAReview.InitialBaselineTETA)) {
			return invalid("ETA review initial baseline does not match the flight baseline")
		}
	}
	if f.ActiveRouteFact != nil && !f.ActiveRouteFact.State.Valid() {
		return invalid("route fact state is invalid")
	}
	if f.RouteProgress != nil {
		if strings.TrimSpace(f.RouteProgress.GeometryDigest) == "" || f.RouteProgress.ManifestRevision < 1 || f.RouteProgress.LegIndex < 0 || f.RouteProgress.RejoinLegIndex < 0 || f.RouteProgress.AlongTrackNM < 0 || math.IsNaN(f.RouteProgress.AlongTrackNM) || math.IsInf(f.RouteProgress.AlongTrackNM, 0) {
			return invalid("route progress is invalid")
		}
	}
	if f.Lifecycle != nil {
		if err := f.Lifecycle.Validate(); err != nil {
			return err
		}
		if f.Lifecycle.EnteredAt.After(f.Lifecycle.LastEventAt) {
			return invalid("lifecycle state entry cannot follow its event cursor")
		}
		if !f.lifecycleReasonMatchesState() {
			return invalid("lifecycle reason does not match flight state")
		}
		if f.Lifecycle.Absence != nil {
			if err := f.Lifecycle.Absence.Validate(); err != nil {
				return err
			}
			if f.Lifecycle.ReconciliationPending && f.Lifecycle.Absence.RemovalDueAt != nil {
				return invalid("pending source reconciliation cannot run an absence timer")
			}
		}
	}
	if f.OperationalException != nil {
		if !f.OperationalException.Reason.Valid() {
			return invalid("operational exception reason is invalid")
		}
		if err := requireUTCTime("operational exception detected at", f.OperationalException.DetectedAt); err != nil {
			return err
		}
	}
	if f.GoAroundDetection != nil {
		if err := f.GoAroundDetection.Validate(); err != nil {
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

func (s GoAroundDetectionState) Validate() error {
	if s.empty() {
		return nil
	}
	if !isTrimmedNonEmpty(s.PolicyVersion) {
		return invalid("go-around detector policy version is required")
	}
	if len(s.Evidence) > 64 {
		return invalid("go-around detector evidence exceeds the durable bound")
	}
	for index, evidence := range s.Evidence {
		if err := evidence.Validate(); err != nil {
			return err
		}
		if index > 0 && !evidence.ObservedAt.After(s.Evidence[index-1].ObservedAt) {
			return invalid("go-around detector evidence must be ordered oldest-first")
		}
		if index > 0 && evidence.Sequence != nil && s.Evidence[index-1].Sequence != nil && *evidence.Sequence <= *s.Evidence[index-1].Sequence {
			return invalid("go-around detector evidence sequence must increase")
		}
	}
	for _, count := range []int{s.ArmCount, s.ClimbCount, s.TrackAwayCount, s.RunwayExitCount} {
		if count < 0 || count > len(s.Evidence) {
			return invalid("go-around detector counter is invalid")
		}
	}
	if s.LastEmittedEpisode > s.Episode {
		return invalid("go-around emitted episode exceeds the current episode")
	}
	if s.LastProcessedAt != nil {
		if err := requireUTCTime("go-around last processed at", *s.LastProcessedAt); err != nil {
			return err
		}
	}
	if s.LastProcessedSequence != nil && s.LastProcessedAt == nil {
		return invalid("go-around sequence cursor requires an observation time")
	}
	if s.LastControllerCommandID != "" && !isTrimmedNonEmpty(s.LastControllerCommandID) {
		return invalid("go-around controller command identity is invalid")
	}
	if len(s.Evidence) > 0 && (s.LastProcessedAt == nil || s.Evidence[len(s.Evidence)-1].ObservedAt.After(*s.LastProcessedAt)) {
		return invalid("go-around evidence exceeds the processed observation cursor")
	}
	if len(s.Evidence) > 0 && s.Evidence[len(s.Evidence)-1].Sequence != nil && s.LastProcessedSequence != nil && *s.Evidence[len(s.Evidence)-1].Sequence > *s.LastProcessedSequence {
		return invalid("go-around evidence exceeds the processed sequence cursor")
	}
	if s.Armed {
		if s.ArmedAt == nil || !isTrimmedNonEmpty(s.ArmedCorridorID) || s.Episode == 0 || s.Episode <= s.LastEmittedEpisode {
			return invalid("armed go-around detector requires episode metadata")
		}
		if err := requireUTCTime("go-around armed at", *s.ArmedAt); err != nil {
			return err
		}
		if s.LastProcessedAt == nil || s.ArmedAt.After(*s.LastProcessedAt) {
			return invalid("go-around armed time exceeds the processed observation cursor")
		}
	} else if s.ArmedAt != nil || s.ArmedCorridorID != "" || s.ThresholdCrossed {
		return invalid("disarmed go-around detector retains arming state")
	}
	return nil
}

func (s GoAroundDetectionState) empty() bool {
	return s.PolicyVersion == "" && len(s.Evidence) == 0 && s.ArmCount == 0 && s.ClimbCount == 0 && s.TrackAwayCount == 0 && s.RunwayExitCount == 0 && !s.Armed && s.ArmedAt == nil && s.ArmedCorridorID == "" && s.Episode == 0 && s.LastEmittedEpisode == 0 && s.LastProcessedAt == nil && s.LastProcessedSequence == nil && !s.ThresholdCrossed && s.LastControllerCommandID == ""
}

func (e GoAroundEvidence) Validate() error {
	if err := requireUTCTime("go-around evidence observed at", e.ObservedAt); err != nil {
		return err
	}
	if !finite(e.LatitudeDegrees) || !finite(e.LongitudeDegrees) || e.LatitudeDegrees < -90 || e.LatitudeDegrees > 90 || e.LongitudeDegrees < -180 || e.LongitudeDegrees > 180 {
		return invalid("go-around evidence coordinate is invalid")
	}
	if e.AltitudeFeet < -2000 || !finite(e.GroundspeedKnots) || e.GroundspeedKnots < 0 {
		return invalid("go-around evidence surveillance values are invalid")
	}
	if e.TrackTrueDegrees != nil && (!finite(*e.TrackTrueDegrees) || *e.TrackTrueDegrees < 0 || *e.TrackTrueDegrees >= 360) {
		return invalid("go-around evidence track is invalid")
	}
	return nil
}

func (r ETAReview) Validate() error {
	if !r.Status.Valid() {
		return invalid("ETA review status is invalid")
	}
	if r.Status == ReviewNone {
		if !r.CreatedAt.IsZero() || !r.DeadlineAt.IsZero() || r.ResolvedAt != nil || r.Actor != nil || r.Note != nil ||
			!r.InitialBaselineTETA.IsZero() || !r.CalculatedOperationalTETA.IsZero() || !r.SelectedTETA.IsZero() || r.ManualTETA != nil {
			return invalid("empty ETA review cannot retain review values")
		}
		return nil
	}
	for _, field := range []struct {
		label string
		value time.Time
	}{
		{label: "ETA review created at", value: r.CreatedAt},
		{label: "ETA review deadline at", value: r.DeadlineAt},
		{label: "ETA review initial baseline", value: r.InitialBaselineTETA},
		{label: "ETA review calculated operational TETA", value: r.CalculatedOperationalTETA},
		{label: "ETA review selected TETA", value: r.SelectedTETA},
	} {
		if err := requireUTCTime(field.label, field.value); err != nil {
			return err
		}
	}
	if !r.DeadlineAt.After(r.CreatedAt) {
		return invalid("ETA review deadline must follow creation")
	}
	if r.Note != nil && !isTrimmedNonEmpty(*r.Note) {
		return invalid("ETA review note must be trimmed and non-empty")
	}
	if r.Status == ReviewPending {
		if r.ResolvedAt != nil || r.Actor != nil || r.Note != nil || r.ManualTETA != nil || !r.SelectedTETA.Equal(r.CalculatedOperationalTETA) {
			return invalid("pending ETA review has resolved values")
		}
		return nil
	}
	if r.ResolvedAt == nil {
		return invalid("resolved ETA review requires resolution time")
	}
	if err := requireUTCTime("ETA review resolved at", *r.ResolvedAt); err != nil {
		return err
	}
	if r.ResolvedAt.Before(r.CreatedAt) {
		return invalid("ETA review resolution cannot predate creation")
	}

	switch r.Status {
	case ReviewAcceptedCalculatedTETA:
		if !validReviewActor(r.Actor) || r.ManualTETA != nil || !r.SelectedTETA.Equal(r.CalculatedOperationalTETA) || !r.ResolvedAt.Before(r.DeadlineAt) {
			return invalid("accepted calculated ETA review is inconsistent")
		}
	case ReviewKeptInitialFPLETA:
		if !validReviewActor(r.Actor) || r.ManualTETA != nil || !r.SelectedTETA.Equal(r.InitialBaselineTETA) || !r.ResolvedAt.Before(r.DeadlineAt) {
			return invalid("kept initial ETA review is inconsistent")
		}
	case ReviewManualETA:
		if !validReviewActor(r.Actor) || r.ManualTETA == nil || !r.SelectedTETA.Equal(*r.ManualTETA) || !r.ResolvedAt.Before(r.DeadlineAt) {
			return invalid("manual ETA review is inconsistent")
		}
		if err := requireUTCTime("ETA review manual TETA", *r.ManualTETA); err != nil {
			return err
		}
	case ReviewAutoAcceptedCalculatedTETA:
		if r.Actor != nil || r.Note != nil || r.ManualTETA != nil || !r.SelectedTETA.Equal(r.CalculatedOperationalTETA) || !r.ResolvedAt.Equal(r.DeadlineAt) {
			return invalid("auto-accepted ETA review is inconsistent")
		}
	}
	return nil
}

func validReviewActor(actor *string) bool {
	return actor != nil && isTrimmedNonEmpty(*actor)
}
func (s LifecycleState) Validate() error {
	if !s.Reason.Valid() {
		return invalid("lifecycle reason is invalid")
	}
	if err := requireUTCTime("lifecycle entered at", s.EnteredAt); err != nil {
		return err
	}
	if err := requireUTCTime("lifecycle last event at", s.LastEventAt); err != nil {
		return err
	}
	if !isTrimmedNonEmpty(s.LastEventID) || !isTrimmedNonEmpty(s.LastEventFingerprint) {
		return invalid("lifecycle last event identity is required")
	}
	return nil
}

func (s AbsenceState) Validate() error {
	if err := requireUTCTime("absence missing since", s.MissingSince); err != nil {
		return err
	}
	if s.Remaining < 0 {
		return invalid("absence remaining duration cannot be negative")
	}
	if s.RemovalDueAt == nil {
		return nil
	}
	if s.Remaining != 0 {
		return invalid("running absence timer cannot retain a paused duration")
	}
	if err := requireUTCTime("absence removal due at", *s.RemovalDueAt); err != nil {
		return err
	}
	if s.RemovalDueAt.Before(s.MissingSince) {
		return invalid("absence removal deadline cannot predate disappearance")
	}
	return nil
}

func (f AMANFlight) lifecycleReasonMatchesState() bool {
	switch f.Lifecycle.Reason {
	case LifecycleReasonInitial:
		return true
	case LifecycleReasonAirborneDetected:
		return f.State == StateAirborne
	case LifecycleReasonUnstableHorizon:
		return f.State == StateUnstable
	case LifecycleReasonStableHorizon:
		return f.State == StateStable
	case LifecycleReasonGoAroundConfirmed:
		return f.State == StateGoAround
	case LifecycleReasonLandingConfirmed:
		return f.State == StateLanded
	case LifecycleReasonSuddenAppearance:
		return f.State == StateAirborne || f.State == StateUnstable || f.State == StateStable
	case LifecycleReasonManualRemoval, LifecycleReasonLandedTimeout, LifecycleReasonPlannedCancellation:
		return f.State == StateRemoved
	case LifecycleReasonSourceDisappearance:
		return f.State == StateRemoved
	default:
		return false
	}
}

func (f AMANFlight) validateFreeze() error {
	if f.FreezeReason == FreezeNone {
		if f.FrozenAt != nil || f.FrozenOperationalTETA != nil || f.FrozenSlot != nil {
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
	if err := requireUTCTime("frozen operational TETA", *f.FrozenOperationalTETA); err != nil {
		return err
	}
	if f.FreezeReason == FreezeSuperstable {
		if f.Slot == nil || f.FrozenSlot == nil {
			return invalid("Superstable freeze requires a captured slot")
		}
		if err := f.FrozenSlot.validate(); err != nil {
			return err
		}
	}
	return nil
}

func (s RawTETASample) Validate() error {
	if err := requireUTCTime("raw TETA sample", s.TETA); err != nil {
		return err
	}
	return requireUTCTime("raw TETA sample generated at", s.GeneratedAt)
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

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
