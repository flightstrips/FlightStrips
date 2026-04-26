package services

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *StripService) maybeDropArrivalTrackingInEuroscope(ctx context.Context, session int32, callsign string, ownerPosition string) {
	if s.esCommander == nil {
		return
	}

	controllerRepo := s.getControllerRepository()
	if controllerRepo == nil {
		return
	}

	if ownerPosition == "" {
		return
	}

	controllers, err := controllerRepo.ListBySession(ctx, session)
	if err != nil {
		slog.WarnContext(ctx, "Failed to list controllers for ALDT EuroScope drop",
			slog.String("callsign", callsign),
			slog.Any("error", err))
		return
	}

	sent := false
	for _, controller := range controllers {
		if controller.Position != ownerPosition || controller.Cid == nil || *controller.Cid == "" {
			continue
		}
		s.esCommander.SendDropTracking(session, *controller.Cid, callsign)
		sent = true
	}

	if !sent {
		slog.DebugContext(ctx, "No matching EuroScope client found for ALDT drop",
			slog.String("callsign", callsign),
			slog.String("owner", ownerPosition))
	}
}

// UpdateAircraftPosition updates the aircraft position and moves the strip to a new bay if needed.
func (s *StripService) UpdateAircraftPosition(ctx context.Context, session int32, callsign string, lat, lon float64, altitude int32, airport string) error {
	existingStrip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.DebugContext(ctx, "Strip being updated does not exist in database", slog.String("callsign", callsign), slog.String("event", "FlightStripOffline"))
			return nil
		}
		return err
	}

	dbStrip := database.Strip{
		Origin:      existingStrip.Origin,
		Destination: existingStrip.Destination,
		Cleared:     existingStrip.Cleared,
		Bay:         existingStrip.Bay,
		State:       existingStrip.State,
	}
	bay := shared.GetDepartureBayFromPosition(lat, lon, int64(altitude), dbStrip, config.GetAirborneAltitudeAGL(), airport)

	existingState := "<nil>"
	if existingStrip.State != nil {
		existingState = *existingStrip.State
	}
	slog.DebugContext(ctx, "UpdateAircraftPosition",
		slog.String("callsign", callsign),
		slog.String("current_bay", existingStrip.Bay),
		slog.String("current_state", existingState),
		slog.Int("altitude", int(altitude)),
		slog.Int64("airborne_threshold_agl", config.GetAirborneAltitudeAGL()),
		slog.String("computed_bay", bay),
	)

	_, err = s.stripRepo.UpdateAircraftPosition(ctx, session, callsign, &lat, &lon, &altitude, bay, nil)
	if err != nil {
		return err
	}

	if existingStrip.Bay != bay {
		slog.DebugContext(ctx, "UpdateAircraftPosition: bay changed, moving strip",
			slog.String("callsign", callsign),
			slog.String("from_bay", existingStrip.Bay),
			slog.String("to_bay", bay),
		)
		if err := s.MoveToBay(context.Background(), session, callsign, bay, true); err != nil {
			return err
		}
		if existingStrip.Bay == shared.BAY_DEPART && bay == shared.BAY_AIRBORNE {
			return s.AutoTransferAirborneStrip(ctx, session, callsign)
		}
	}

	if existingStrip.Destination == airport {
		s.handleArrivalPositionUpdate(ctx, session, callsign, lat, lon, int64(altitude), existingStrip)
	}

	return nil
}

// handleArrivalPositionUpdate detects landing and runway-vacated transitions for
// arrival strips using S2 runway polygon containment + altitude threshold.
func (s *StripService) handleArrivalPositionUpdate(ctx context.Context, session int32, callsign string, lat, lon float64, altitude int64, strip *internalModels.Strip) {
	const logPrefix = "handleArrivalPositionUpdate"

	landingThreshold := int64(shared.AirportElevation) + config.GetLandingAltitudeAGL()
	onGround := altitude < landingThreshold

	runwayRegion, inRunway := config.GetRunwayRegionForPosition(lat, lon)

	// If the strip has an assigned runway, only recognise the polygon for that runway.
	if inRunway && strip.Runway != nil && *strip.Runway != "" {
		if !slices.Contains(runwayRegion.Runways, strings.ToUpper(*strip.Runway)) {
			inRunway = false
		}
	}

	alreadyLanded := strip.CdmData != nil && strip.CdmData.Aldt != nil

	if !alreadyLanded && strip.Runway != nil && *strip.Runway != "" {
		switch strip.Bay {
		case shared.BAY_ARR_HIDDEN, shared.BAY_AIRBORNE:
			if finalRegion, inFinal := config.GetFinalApproachRegionForRunway(*strip.Runway, lat, lon); inFinal {
				distanceToThresholdNM := shared.GetDistance(lat, lon, finalRegion.ThresholdLat, finalRegion.ThresholdLon)
				altitudeCeiling := finalRegion.FinalApproachAltitudeCeiling(distanceToThresholdNM, int64(shared.AirportElevation))
				if altitude > altitudeCeiling {
					slog.DebugContext(ctx, "Arrival in final approach zone but above glideslope ceiling",
						slog.String("callsign", callsign),
						slog.String("runway", *strip.Runway),
						slog.Int64("altitude", altitude),
						slog.Int64("ceiling", altitudeCeiling),
						slog.Float64("distance_nm", distanceToThresholdNM),
					)
					break
				}
				slog.InfoContext(ctx, "Arrival entered final approach zone, moving to FINAL",
					slog.String("callsign", callsign),
					slog.String("runway", *strip.Runway),
				)
				if err := s.MoveToBay(ctx, session, callsign, shared.BAY_FINAL, true); err != nil {
					slog.ErrorContext(ctx, logPrefix+": failed to move to FINAL", slog.String("callsign", callsign), slog.Any("error", err))
				} else {
					strip.Bay = shared.BAY_FINAL
				}
			}
		}
	}

	// Phase 1: aircraft enters runway polygon at landing altitude → touchdown.
	if inRunway && onGround && !alreadyLanded {
		aldt := time.Now().UTC().Format("1504")
		newCdm := strip.CdmData.Clone()
		newCdm.Aldt = &aldt
		if _, err := s.stripRepo.SetCdmData(ctx, session, callsign, newCdm); err != nil {
			slog.ErrorContext(ctx, logPrefix+": failed to set ALDT", slog.String("callsign", callsign), slog.Any("error", err))
		} else {
			slog.InfoContext(ctx, "ALDT recorded", slog.String("callsign", callsign), slog.String("aldt", aldt), slog.String("runway", runwayRegion.Name))
			s.notifyStripUpdate(session, callsign)
			if strip.Owner != nil {
				s.maybeDropArrivalTrackingInEuroscope(ctx, session, callsign, *strip.Owner)
			}
		}
		s.autoAcceptPendingCoordination(ctx, session, strip)
		return
	}

	// Phase 2: aircraft exits runway polygon on the ground → strip vacated runway.
	if !inRunway && onGround && alreadyLanded {
		switch strip.Bay {
		case shared.BAY_FINAL, shared.BAY_RWY_ARR, shared.BAY_ARR_HIDDEN:
			slog.InfoContext(ctx, "Arrival vacated runway, moving to TWY_ARR", slog.String("callsign", callsign))
			if err := s.MoveToBay(ctx, session, callsign, shared.BAY_TWY_ARR, true); err != nil {
				slog.ErrorContext(ctx, logPrefix+": failed to move to TWY_ARR", slog.String("callsign", callsign), slog.Any("error", err))
			}
		}
	}
}

// HandleTrackingControllerChanged processes a tracking controller change event,
// potentially accepting a coordination and moving the strip if it becomes airborne.
func (s *StripService) HandleTrackingControllerChanged(ctx context.Context, session int32, callsign string, trackingController string) error {
	if s.controllerRepo == nil {
		return errors.New("controller repository not configured")
	}
	if s.coordRepo == nil {
		return errors.New("coordination repository not configured")
	}

	if _, err := s.stripRepo.UpdateTrackingController(ctx, session, callsign, trackingController); err != nil {
		return err
	}

	// Only act on assumption (non-empty tracking controller).
	if trackingController == "" {
		return nil
	}

	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}

	// Resolve the assuming controller's position.
	assumingController, err := s.controllerRepo.GetByCallsign(ctx, session, trackingController)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	assumingPosition := ""
	if err == nil {
		assumingPosition = assumingController.Position
	}

	// Check for a pending coordination on this strip.
	coordination, err := s.coordRepo.GetByStripID(ctx, session, strip.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	hasCoordination := err == nil

	if hasCoordination && assumingPosition == coordination.FromPosition {
		// The FROM controller assumed the tag to initiate the handover to the TO controller.
		// Don't move or accept yet — wait for the TO controller to assume.
		return nil
	}

	// Capture FromPosition before AcceptCoordination deletes the coordination record.
	coordFromPosition := ""
	if hasCoordination {
		coordFromPosition = coordination.FromPosition
	}

	// Accepting: either the TO controller assumed, or there is no coordination.
	if hasCoordination && assumingPosition != "" {
		if err := s.AcceptCoordination(ctx, session, callsign, assumingPosition); err != nil {
			slog.ErrorContext(ctx, "Failed to accept coordination on tracking controller change", slog.String("callsign", callsign), slog.Any("error", err))
		}
	}

	if strip.Bay != shared.BAY_AIRBORNE {
		return nil
	}

	// Arrivals that go AIRBORNE normally land in ARR_HIDDEN so the APP controller can
	// work them again. For missed-approach TWR->APP assumptions, restore the strip to
	// FINAL so TWR becomes the next controller again. Departures use the regular HIDDEN bay.
	targetBay := shared.BAY_HIDDEN
	if s.publisher != nil {
		if srv := s.publisher.GetServer(); srv != nil {
			if sess, sessErr := srv.GetSessionRepository().GetByID(ctx, session); sessErr == nil && strip.Destination == sess.Airport {
				targetBay = shared.BAY_ARR_HIDDEN
			}
		}
	}
	if targetBay == shared.BAY_ARR_HIDDEN && coordFromPosition != "" && isMissedApproachReturn(coordFromPosition, assumingPosition) {
		targetBay = shared.BAY_FINAL
	}

	count, err := s.stripRepo.UpdateBay(ctx, session, callsign, targetBay, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("failed to move airborne strip after tracking controller assumption")
	}

	if err := s.MoveToBay(ctx, session, callsign, targetBay, true); err != nil {
		return err
	}

	if targetBay == shared.BAY_FINAL && coordFromPosition != "" {
		s.applyMissedApproachOwnerFix(ctx, session, callsign, assumingPosition, coordFromPosition)
	}

	return nil
}

// isGndOnline returns true if at least one GND-section controller is active in the
// session, or if controllerRepo is not configured (safe default: no promotion).
func (s *StripService) isGndOnline(ctx context.Context, session int32) bool {
	if s.controllerRepo == nil {
		return true
	}
	controllers, err := s.controllerRepo.ListBySession(ctx, session)
	if err != nil {
		return true
	}
	for _, c := range controllers {
		pos, posErr := config.GetPositionBasedOnFrequency(c.Position)
		if posErr == nil && pos.Section == "GND" {
			return true
		}
	}
	return false
}

// maybeMoveToLowerTwyDepOnTowerTransfer moves a strip from TAXI (upper TWY DEP) to TAXI_LWR
// (lower TWY DEP) when a transfer to a tower position is started. This reflects the real-world
// handover flow where GND pushes a strip up to upper TWY DEP and TWR immediately pulls it down
// to the lower bay as part of the transfer.
func (s *StripService) maybeMoveToLowerTwyDepOnTowerTransfer(ctx context.Context, session int32, callsign string, stripBay string, targetPosition string) {
	if stripBay != shared.BAY_TAXI {
		return
	}

	pos, err := config.GetPositionBasedOnFrequency(targetPosition)
	if err != nil || pos.Section != "TWR" {
		return
	}

	if err := s.MoveToBay(ctx, session, callsign, shared.BAY_TAXI_LWR, true); err != nil {
		slog.ErrorContext(ctx, "Failed to move strip from TAXI to TAXI_LWR on tower transfer",
			slog.String("callsign", callsign), slog.Any("error", err))
	}
}
