package operational

import (
	"fmt"
	"strings"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/lifecycle"
)

const liveGoAroundPolicyVersion = "aman-go-around-v1"

func liveGoAroundConfig() lifecycle.GoAroundConfig {
	return lifecycle.GoAroundConfig{
		EvidenceLimit: 16, ArmSamples: 2, ConfirmSamples: 2, ArmBelowAltitudeFeet: 2000,
		InboundToleranceDegrees: 20, MinimumClimbFeet: 100, TrackAwayDegrees: 60,
		RunwayExitAfterThresholdNM: 0.1, LandingAltitudeToleranceFeet: 200, MinimumAirborneGroundspeedKnots: 80,
	}
}

func (s *Service) detectLiveGoAround(flight *aman.AMANFlight, observation aman.FlightObservation, group aman.RunwayGroupID, routeChanged, runwayChanged bool, now time.Time) error {
	if observation.Surveillance == nil || flight.State == aman.StatePlanned || flight.State == aman.StateRemoved {
		return nil
	}
	corridor, configured, err := s.liveGoAroundCorridor(group)
	if err != nil || !configured {
		return err
	}
	previous := aman.GoAroundDetectionState{}
	if flight.GoAroundDetection != nil {
		previous = *flight.GoAroundDetection
	}
	result, err := s.goAroundDetector.Detect(lifecycle.GoAroundInput{
		FlightID: flight.ID, Observation: observation, Corridor: corridor, Previous: previous,
		PolicyVersion: liveGoAroundPolicyVersion + "/" + s.deps.Terminal.ConfigVersion,
		Now:           now, InScope: true, LandingConfirmed: flight.State == aman.StateLanded,
		RouteChanged: routeChanged, RunwayGroupChanged: runwayChanged,
	})
	if err != nil {
		return fmt.Errorf("detect live go-around: %w", err)
	}
	flight.GoAroundDetection = &result.State
	// A confirmation belongs only to the detector episode that produced it.
	// Once that episode is reset (departure from final, route/runway change,
	// stale source, or landing), the audit record remains durable but the live
	// prompt must no longer be actionable or suppress a later episode.
	if flight.GoAroundConfirmation != nil && !result.State.AwaitingReset {
		flight.GoAroundConfirmation = nil
	}
	if result.Confirmed != nil && (flight.GoAroundConfirmation == nil || flight.GoAroundConfirmation.EpisodeID != result.Confirmed.EpisodeID) {
		flight.GoAroundConfirmation = &aman.GoAroundConfirmation{
			EpisodeID: result.Confirmed.EpisodeID, Reason: string(result.Confirmed.Reason),
			DetectedAt:    result.Confirmed.ConfirmedAt,
			EvidenceTimes: append([]time.Time(nil), result.Confirmed.SupportingObservationTimes...),
			Status:        aman.GoAroundConfirmationPending,
		}
	}
	return nil
}

func invalidateLiveGoAroundEpisode(flight *aman.AMANFlight) {
	flight.GoAroundConfirmation = nil
	if flight.GoAroundDetection == nil {
		return
	}
	state := *flight.GoAroundDetection
	state.Evidence = nil
	state.ArmCount, state.ClimbCount, state.TrackAwayCount, state.RunwayExitCount = 0, 0, 0, 0
	state.Armed, state.ThresholdCrossed, state.AwaitingReset = false, false, false
	state.ArmedAt, state.ArmedCorridorID = nil, ""
	flight.GoAroundDetection = &state
}

func liveDetectorRouteChanged(flight aman.AMANFlight, previous *aman.FlightObservation, current aman.FlightObservation) bool {
	if previous != nil && normalizedRoute(previous.FiledRoute) != normalizedRoute(current.FiledRoute) {
		return true
	}
	return flight.ActiveRouteFact != nil && flight.GoAroundDetection != nil && flight.GoAroundDetection.LastProcessedAt != nil && flight.ActiveRouteFact.ObservedAt.After(*flight.GoAroundDetection.LastProcessedAt)
}

func normalizedRoute(route *string) string {
	if route == nil {
		return ""
	}
	return strings.Join(strings.Fields(strings.ToUpper(*route)), " ")
}

func (s *Service) liveGoAroundCorridor(selected aman.RunwayGroupID) (lifecycle.FinalPathCorridor, bool, error) {
	for _, group := range s.deps.Terminal.RunwayGroups {
		if group.ID != selected {
			continue
		}
		if len(group.FinalApproaches) != 1 {
			return lifecycle.FinalPathCorridor{}, false, nil
		}
		final := group.FinalApproaches[0]
		elevation := 0
		if final.Threshold.ElevationFt != nil {
			elevation = *final.Threshold.ElevationFt
		}
		return lifecycle.FinalPathCorridor{
			ID:                string(s.deps.Terminal.Airport) + "-" + string(final.Runway),
			ThresholdLatitude: final.Threshold.Position.LatitudeDeg, ThresholdLongitude: final.Threshold.Position.LongitudeDeg,
			ThresholdElevationFeet: elevation, InboundCourseDegrees: final.CourseTrueDeg,
			LengthNM: 6, HalfWidthNM: 0.75,
		}, true, nil
	}
	return lifecycle.FinalPathCorridor{}, false, fmt.Errorf("runway group %q is not configured for go-around detection", selected)
}

func pendingCreated(previous, next *aman.GoAroundConfirmation) bool {
	return next != nil && next.Status == aman.GoAroundConfirmationPending && (previous == nil || previous.EpisodeID != next.EpisodeID)
}
