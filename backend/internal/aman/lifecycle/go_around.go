package lifecycle

import (
	"fmt"
	"math"
	"strings"
	"time"

	"FlightStrips/internal/aman"
)

const maximumGoAroundEvidence = 64

// GoAroundReason identifies the independent multi-sample evidence that
// confirmed one go-around episode.
type GoAroundReason string

const (
	GoAroundReasonClimb      GoAroundReason = "climb"
	GoAroundReasonTrackAway  GoAroundReason = "track_away"
	GoAroundReasonRunwayExit GoAroundReason = "runway_exit_without_landing"
	GoAroundReasonController GoAroundReason = "controller_marked"
)

func (r GoAroundReason) Valid() bool {
	return r == GoAroundReasonClimb || r == GoAroundReasonTrackAway || r == GoAroundReasonRunwayExit || r == GoAroundReasonController
}

// GoAroundConfig contains detector policy rather than airport geometry. The
// owning application persists its version in AirportState and supplies the
// canonical final-path corridor selected for the flight.
type GoAroundConfig struct {
	EvidenceLimit                   int
	ArmSamples                      int
	ConfirmSamples                  int
	ArmBelowAltitudeFeet            int
	InboundToleranceDegrees         float64
	MinimumClimbFeet                int
	TrackAwayDegrees                float64
	RunwayExitAfterThresholdNM      float64
	LandingAltitudeToleranceFeet    int
	MinimumAirborneGroundspeedKnots float64
}

func (c GoAroundConfig) Validate() error {
	if c.EvidenceLimit < 2 || c.EvidenceLimit > maximumGoAroundEvidence || c.ArmSamples < 2 || c.ConfirmSamples < 2 || c.EvidenceLimit < c.ArmSamples || c.EvidenceLimit < c.ConfirmSamples {
		return invalidArgument("go-around sample counts or evidence limit are invalid")
	}
	if c.ArmBelowAltitudeFeet <= 0 || c.MinimumClimbFeet <= 0 || c.LandingAltitudeToleranceFeet < 0 {
		return invalidArgument("go-around altitude policy is invalid")
	}
	if !finiteNumber(c.InboundToleranceDegrees) || c.InboundToleranceDegrees <= 0 || c.InboundToleranceDegrees >= 90 || !finiteNumber(c.TrackAwayDegrees) || c.TrackAwayDegrees <= c.InboundToleranceDegrees || c.TrackAwayDegrees > 180 {
		return invalidArgument("go-around track policy is invalid")
	}
	if !finiteNumber(c.RunwayExitAfterThresholdNM) || c.RunwayExitAfterThresholdNM < 0 || !finiteNumber(c.MinimumAirborneGroundspeedKnots) || c.MinimumAirborneGroundspeedKnots < 0 {
		return invalidArgument("go-around runway-exit policy is invalid")
	}
	return nil
}

// FinalPathCorridor is the canonical, provider-neutral final geometry used by
// the detector. An adapter maps the selected #309 runway geometry into this
// type; the detector never calls a navigation provider or EuroScope.
type FinalPathCorridor struct {
	ID                     string
	ThresholdLatitude      float64
	ThresholdLongitude     float64
	ThresholdElevationFeet int
	InboundCourseDegrees   float64
	LengthNM               float64
	HalfWidthNM            float64
}

func (c FinalPathCorridor) Validate() error {
	if c.ID == "" || strings.TrimSpace(c.ID) != c.ID {
		return invalidArgument("final-path corridor ID is required")
	}
	if !finiteNumber(c.ThresholdLatitude) || !finiteNumber(c.ThresholdLongitude) || c.ThresholdLatitude < -90 || c.ThresholdLatitude > 90 || c.ThresholdLongitude < -180 || c.ThresholdLongitude > 180 {
		return invalidArgument("final-path threshold coordinate is invalid")
	}
	if !finiteNumber(c.InboundCourseDegrees) || c.InboundCourseDegrees < 0 || c.InboundCourseDegrees >= 360 || !finiteNumber(c.LengthNM) || c.LengthNM <= 0 || !finiteNumber(c.HalfWidthNM) || c.HalfWidthNM <= 0 {
		return invalidArgument("final-path corridor geometry is invalid")
	}
	return nil
}

// GoAroundInput supplies one normalized observation and all explicit facts
// that can disarm a detector. Previous is copied before reduction.
type GoAroundInput struct {
	FlightID           aman.FlightID
	Observation        aman.FlightObservation
	Corridor           FinalPathCorridor
	Previous           aman.GoAroundDetectionState
	PolicyVersion      string
	Now                time.Time
	InScope            bool
	LandingConfirmed   bool
	RouteChanged       bool
	RunwayGroupChanged bool
	ControllerMarked   *ControllerGoAround
}

// ControllerGoAround is the authorized controller fact that a strip has gone
// around. Command idempotency and authorization remain at the command boundary;
// the detector persists the last accepted command so replay remains stable.
type ControllerGoAround struct {
	CommandID string
	Actor     string
}

// GoAroundConfirmed is the typed operational event emitted once per arming
// episode. SupportingObservationTimes are oldest-first audit evidence.
type GoAroundConfirmed struct {
	ID                         string
	EpisodeID                  string
	FlightID                   aman.FlightID
	Reason                     GoAroundReason
	ConfirmedAt                time.Time
	SupportingObservationTimes []time.Time
	ControllerActor            *string
}

// LifecycleEvent maps the detector result into the state-machine event owned
// by #319 without allowing the detector to mutate lifecycle or slots itself.
func (e GoAroundConfirmed) LifecycleEvent() Event {
	return Event{ID: e.ID, Kind: EventGoAroundConfirmed, OccurredAt: e.ConfirmedAt}
}

type GoAroundResult struct {
	State     aman.GoAroundDetectionState
	Confirmed *GoAroundConfirmed
	Duplicate bool
}

// GoAroundDetector is the lifecycle-owned stateful detector boundary.
type GoAroundDetector interface {
	Detect(GoAroundInput) (GoAroundResult, error)
}

type goAroundDetector struct{ config GoAroundConfig }

func NewGoAroundDetector(config GoAroundConfig) (GoAroundDetector, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &goAroundDetector{config: config}, nil
}

func (d *goAroundDetector) Detect(input GoAroundInput) (GoAroundResult, error) {
	if err := validateGoAroundInput(input); err != nil {
		return GoAroundResult{}, err
	}
	if !zeroDetectionState(input.Previous) {
		if err := input.Previous.Validate(); err != nil {
			return GoAroundResult{}, err
		}
	}
	state := cloneDetectionState(input.Previous)
	if state.PolicyVersion != input.PolicyVersion {
		disarm(&state, true)
		state.PolicyVersion = input.PolicyVersion
	}
	if input.ControllerMarked != nil {
		return controllerConfirmed(state, input)
	}

	surveillance := input.Observation.Surveillance
	if input.LandingConfirmed || input.Observation.SourceStatus != aman.DataFresh {
		disarm(&state, true)
		return checkedGoAroundResult(state, nil, false)
	}
	if surveillance == nil {
		return checkedGoAroundResult(state, nil, false)
	}
	if duplicateOrOutOfOrder(state, *surveillance) {
		return checkedGoAroundResult(state, nil, true)
	}
	if input.RouteChanged || input.RunwayGroupChanged || (state.Armed && state.ArmedCorridorID != input.Corridor.ID) {
		disarm(&state, true)
	}
	if !input.InScope && !state.Armed {
		disarm(&state, true)
		acceptCursor(&state, *surveillance)
		return checkedGoAroundResult(state, nil, false)
	}

	evidence, complete, retain := d.evidence(input, state)
	acceptCursor(&state, *surveillance)
	if !complete {
		state.ArmCount, state.ClimbCount, state.TrackAwayCount, state.RunwayExitCount = 0, 0, 0, 0
		if retain {
			appendEvidence(&state, evidence, d.config.EvidenceLimit)
		}
		return checkedGoAroundResult(state, nil, false)
	}

	along, lateral := relativeNM(input.Corridor, evidence.LatitudeDegrees, evidence.LongitudeDegrees)
	insideFinal := along >= -input.Corridor.LengthNM && along <= 0 && math.Abs(lateral) <= input.Corridor.HalfWidthNM
	inbound := angularDifference(*evidence.TrackTrueDegrees, input.Corridor.InboundCourseDegrees) <= d.config.InboundToleranceDegrees
	armEvidence := insideFinal && inbound && evidence.AltitudeFeet <= d.config.ArmBelowAltitudeFeet
	if !state.Armed {
		if armEvidence {
			state.ArmCount++
		} else {
			state.ArmCount = 0
		}
	}

	previous, hasPrevious := lastEvidence(state.Evidence)
	justArmed := false
	if !state.Armed && state.ArmCount >= d.config.ArmSamples {
		state.Armed = true
		state.Episode++
		state.ArmedCorridorID = input.Corridor.ID
		state.ArmedAt = ptrTime(firstSupportingTime(state.Evidence, evidence, d.config.ArmSamples))
		state.ClimbCount, state.TrackAwayCount, state.RunwayExitCount = 0, 0, 0
		justArmed = true
	}

	if state.Armed && !justArmed {
		if along >= 0 {
			state.ThresholdCrossed = true
		}
		evidence.ClimbEvidence = hasPrevious && evidence.AltitudeFeet-previous.AltitudeFeet >= d.config.MinimumClimbFeet
		evidence.TrackAwayEvidence = angularDifference(*evidence.TrackTrueDegrees, input.Corridor.InboundCourseDegrees) >= d.config.TrackAwayDegrees
		evidence.RunwayExitEvidence = state.ThresholdCrossed && (along >= d.config.RunwayExitAfterThresholdNM || math.Abs(lateral) > input.Corridor.HalfWidthNM) && evidence.AltitudeFeet >= input.Corridor.ThresholdElevationFeet+d.config.LandingAltitudeToleranceFeet && evidence.GroundspeedKnots >= d.config.MinimumAirborneGroundspeedKnots
		state.ClimbCount = consecutive(state.ClimbCount, evidence.ClimbEvidence)
		state.TrackAwayCount = consecutive(state.TrackAwayCount, evidence.TrackAwayEvidence)
		state.RunwayExitCount = consecutive(state.RunwayExitCount, evidence.RunwayExitEvidence)
	}

	appendEvidence(&state, evidence, d.config.EvidenceLimit)
	if justArmed {
		return checkedGoAroundResult(state, nil, false)
	}
	reason := GoAroundReason("")
	switch {
	case state.ClimbCount >= d.config.ConfirmSamples:
		reason = GoAroundReasonClimb
	case state.TrackAwayCount >= d.config.ConfirmSamples:
		reason = GoAroundReasonTrackAway
	case state.RunwayExitCount >= d.config.ConfirmSamples:
		reason = GoAroundReasonRunwayExit
	}
	if reason == "" {
		return checkedGoAroundResult(state, nil, false)
	}

	episodeID := fmt.Sprintf("%s/go-around/%d", input.FlightID, state.Episode)
	confirmed := &GoAroundConfirmed{
		ID: episodeID + "/confirmed", EpisodeID: episodeID, FlightID: input.FlightID,
		Reason: reason, ConfirmedAt: input.Now, SupportingObservationTimes: supportingTimes(state.Evidence, reason, d.config.ConfirmSamples),
	}
	state.LastEmittedEpisode = state.Episode
	disarm(&state, false)
	return checkedGoAroundResult(state, confirmed, false)
}

func validateGoAroundInput(input GoAroundInput) error {
	if strings.TrimSpace(string(input.FlightID)) == "" || input.FlightID != input.Observation.FlightID {
		return invalidArgument("go-around input flight identity is invalid")
	}
	if err := input.Observation.Validate(); err != nil {
		return err
	}
	if err := input.Corridor.Validate(); err != nil {
		return err
	}
	if input.PolicyVersion == "" || strings.TrimSpace(input.PolicyVersion) != input.PolicyVersion {
		return invalidArgument("go-around policy version is required")
	}
	if input.Now.IsZero() || input.Now.Location() != time.UTC {
		return invalidArgument("go-around injected time must be UTC")
	}
	if input.Observation.Surveillance != nil && input.Observation.Surveillance.ObservedAt != nil && input.Observation.Surveillance.ObservedAt.After(input.Now) {
		return invalidArgument("go-around observation cannot be in the future")
	}
	if input.ControllerMarked != nil && (!isTrimmed(input.ControllerMarked.CommandID) || !isTrimmed(input.ControllerMarked.Actor)) {
		return invalidArgument("controller go-around command identity is required")
	}
	return nil
}

func controllerConfirmed(state aman.GoAroundDetectionState, input GoAroundInput) (GoAroundResult, error) {
	command := input.ControllerMarked
	if state.LastControllerCommandID == command.CommandID {
		return checkedGoAroundResult(state, nil, true)
	}
	state.LastControllerCommandID = command.CommandID
	disarm(&state, false)
	actor := command.Actor
	episodeID := fmt.Sprintf("%s/go-around/controller/%s", input.FlightID, command.CommandID)
	confirmed := &GoAroundConfirmed{
		ID: episodeID + "/confirmed", EpisodeID: episodeID, FlightID: input.FlightID,
		Reason: GoAroundReasonController, ConfirmedAt: input.Now, ControllerActor: &actor,
	}
	return checkedGoAroundResult(state, confirmed, false)
}

func (d *goAroundDetector) evidence(input GoAroundInput, state aman.GoAroundDetectionState) (aman.GoAroundEvidence, bool, bool) {
	surveillance := *input.Observation.Surveillance
	evidence := aman.GoAroundEvidence{
		ObservedAt: *surveillance.ObservedAt, Sequence: cloneUint64(surveillance.Sequence),
		LatitudeDegrees: surveillance.LatitudeDegrees, LongitudeDegrees: surveillance.LongitudeDegrees,
	}
	if surveillance.AltitudeFeet == nil || surveillance.GroundspeedKnots == nil {
		return evidence, false, false
	}
	evidence.AltitudeFeet = *surveillance.AltitudeFeet
	evidence.GroundspeedKnots = *surveillance.GroundspeedKnots
	if surveillance.TrackTrueDegrees != nil {
		evidence.TrackTrueDegrees = cloneFloat64(surveillance.TrackTrueDegrees)
	} else if previous, ok := lastEvidence(state.Evidence); ok {
		if track, ok := bearing(previous.LatitudeDegrees, previous.LongitudeDegrees, evidence.LatitudeDegrees, evidence.LongitudeDegrees); ok {
			evidence.TrackTrueDegrees = &track
		}
	}
	return evidence, evidence.TrackTrueDegrees != nil, true
}

func duplicateOrOutOfOrder(state aman.GoAroundDetectionState, observation aman.SurveillanceFact) bool {
	if state.LastProcessedSequence != nil && observation.Sequence != nil && *observation.Sequence <= *state.LastProcessedSequence {
		return true
	}
	return state.LastProcessedAt != nil && observation.ObservedAt != nil && !observation.ObservedAt.After(*state.LastProcessedAt)
}

func acceptCursor(state *aman.GoAroundDetectionState, observation aman.SurveillanceFact) {
	state.LastProcessedAt = ptrTime(*observation.ObservedAt)
	state.LastProcessedSequence = cloneUint64(observation.Sequence)
}

func appendEvidence(state *aman.GoAroundDetectionState, evidence aman.GoAroundEvidence, limit int) {
	state.Evidence = append(state.Evidence, evidence)
	if len(state.Evidence) > limit {
		state.Evidence = append([]aman.GoAroundEvidence(nil), state.Evidence[len(state.Evidence)-limit:]...)
	}
}

func disarm(state *aman.GoAroundDetectionState, clearEvidence bool) {
	state.ArmCount, state.ClimbCount, state.TrackAwayCount, state.RunwayExitCount = 0, 0, 0, 0
	state.Armed, state.ThresholdCrossed = false, false
	state.ArmedAt, state.ArmedCorridorID = nil, ""
	if clearEvidence {
		state.Evidence = nil
	}
}

func checkedGoAroundResult(state aman.GoAroundDetectionState, confirmed *GoAroundConfirmed, duplicate bool) (GoAroundResult, error) {
	if err := state.Validate(); err != nil {
		return GoAroundResult{}, err
	}
	return GoAroundResult{State: state, Confirmed: confirmed, Duplicate: duplicate}, nil
}

func cloneDetectionState(state aman.GoAroundDetectionState) aman.GoAroundDetectionState {
	state.Evidence = append([]aman.GoAroundEvidence(nil), state.Evidence...)
	for index := range state.Evidence {
		state.Evidence[index].Sequence = cloneUint64(state.Evidence[index].Sequence)
		state.Evidence[index].TrackTrueDegrees = cloneFloat64(state.Evidence[index].TrackTrueDegrees)
	}
	state.ArmedAt = cloneTime(state.ArmedAt)
	state.LastProcessedAt = cloneTime(state.LastProcessedAt)
	state.LastProcessedSequence = cloneUint64(state.LastProcessedSequence)
	return state
}

func zeroDetectionState(state aman.GoAroundDetectionState) bool {
	return state.PolicyVersion == "" && len(state.Evidence) == 0 && state.ArmCount == 0 && state.ClimbCount == 0 && state.TrackAwayCount == 0 && state.RunwayExitCount == 0 && !state.Armed && state.ArmedAt == nil && state.ArmedCorridorID == "" && state.Episode == 0 && state.LastEmittedEpisode == 0 && state.LastProcessedAt == nil && state.LastProcessedSequence == nil && !state.ThresholdCrossed && state.LastControllerCommandID == ""
}

func lastEvidence(values []aman.GoAroundEvidence) (aman.GoAroundEvidence, bool) {
	if len(values) == 0 {
		return aman.GoAroundEvidence{}, false
	}
	return values[len(values)-1], true
}

func firstSupportingTime(existing []aman.GoAroundEvidence, current aman.GoAroundEvidence, count int) time.Time {
	all := append(append([]aman.GoAroundEvidence(nil), existing...), current)
	return all[len(all)-count].ObservedAt
}

func supportingTimes(evidence []aman.GoAroundEvidence, reason GoAroundReason, count int) []time.Time {
	result := make([]time.Time, 0, count)
	for index := len(evidence) - 1; index >= 0 && len(result) < count; index-- {
		value := evidence[index]
		matches := reason == GoAroundReasonClimb && value.ClimbEvidence || reason == GoAroundReasonTrackAway && value.TrackAwayEvidence || reason == GoAroundReasonRunwayExit && value.RunwayExitEvidence
		if !matches {
			break
		}
		result = append(result, value.ObservedAt)
	}
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

func consecutive(current int, matched bool) int {
	if !matched {
		return 0
	}
	return current + 1
}

func relativeNM(corridor FinalPathCorridor, latitude, longitude float64) (float64, float64) {
	north := (latitude - corridor.ThresholdLatitude) * 60
	east := (longitude - corridor.ThresholdLongitude) * 60 * math.Cos(corridor.ThresholdLatitude*math.Pi/180)
	course := corridor.InboundCourseDegrees * math.Pi / 180
	inboundEast, inboundNorth := math.Sin(course), math.Cos(course)
	return east*inboundEast + north*inboundNorth, east*inboundNorth - north*inboundEast
}

func bearing(fromLatitude, fromLongitude, toLatitude, toLongitude float64) (float64, bool) {
	lat1, lat2 := fromLatitude*math.Pi/180, toLatitude*math.Pi/180
	deltaLongitude := (toLongitude - fromLongitude) * math.Pi / 180
	y := math.Sin(deltaLongitude) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(deltaLongitude)
	if math.Hypot(x, y) < 1e-12 {
		return 0, false
	}
	return math.Mod(math.Atan2(y, x)*180/math.Pi+360, 360), true
}

func angularDifference(first, second float64) float64 {
	difference := math.Abs(first - second)
	if difference > 180 {
		return 360 - difference
	}
	return difference
}

func finiteNumber(value float64) bool    { return !math.IsNaN(value) && !math.IsInf(value, 0) }
func isTrimmed(value string) bool        { return value != "" && strings.TrimSpace(value) == value }
func ptrTime(value time.Time) *time.Time { return &value }
func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
func cloneUint64(value *uint64) *uint64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
