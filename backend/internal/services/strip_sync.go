package services

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events/euroscope"
	"FlightStrips/pkg/helpers"
	"FlightStrips/pkg/models"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

// SyncStrip creates or updates a strip from an EuroScope sync/strip-update event.
// The strip parameter must be of type euroscope.Strip.
func (s *StripService) SyncStrip(ctx context.Context, session int32, cid string, strip interface{}, airport string) error {
	esStrip, ok := strip.(euroscope.Strip)
	if !ok {
		return fmt.Errorf("SyncStrip: unexpected strip type %T", strip)
	}
	return s.syncEuroscopeStrip(ctx, session, cid, esStrip, airport)
}

func (s *StripService) syncEuroscopeStrip(ctx context.Context, session int32, cid string, strip euroscope.Strip, airport string) error {
	server := s.publisher.GetServer()
	if server == nil {
		return errors.New("server not configured")
	}

	// Fetch the session so we can read ActiveRunways for runway auto-assignment.
	sessionObj, err := server.GetSessionRepository().GetByID(ctx, session)
	if err != nil {
		return err
	}

	existingStrip, err := s.stripRepo.GetByCallsign(ctx, session, strip.Callsign)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	var bay string
	gndOnline := s.isGndOnline(ctx, session)

	if errors.Is(err, pgx.ErrNoRows) {
		// Strip doesn't exist, so insert
		bay = shared.GetDepartureBay(strip, nil, config.GetAirborneAltitudeAGL(), airport, gndOnline)

		isArrival := strip.Destination == airport
		runwayForStrip := strip.Runway
		if runwayForStrip == "" {
			runwayForStrip = autoAssignRunway(isArrival, sessionObj.ActiveRunways)
		}

		newClearedAlt := strip.ClearedAltitude
		if newClearedAlt == 0 && bay == shared.BAY_NOT_CLEARED {
			if autoCfl, ok := config.GetInitialCFLForRunway(runwayForStrip); ok {
				newClearedAlt = int32(autoCfl)
			}
		}

		newStrip := &internalModels.Strip{
			Callsign:           strip.Callsign,
			Session:            session,
			Origin:             strip.Origin,
			Destination:        strip.Destination,
			Alternative:        &strip.Alternate,
			Route:              &strip.Route,
			Remarks:            &strip.Remarks,
			Runway:             &runwayForStrip,
			Squawk:             &strip.Squawk,
			AssignedSquawk:     &strip.AssignedSquawk,
			Sid:                &strip.Sid,
			Cleared:            strip.Cleared,
			State:              &strip.GroundState,
			ClearedAltitude:    &newClearedAlt,
			RequestedAltitude:  &strip.RequestedAltitude,
			Heading:            &strip.Heading,
			AircraftType:       &strip.AircraftType,
			AircraftCategory:   &strip.AircraftCategory,
			PositionLatitude:   &strip.Position.Lat,
			PositionLongitude:  &strip.Position.Lon,
			PositionAltitude:   &strip.Position.Altitude,
			Stand:              &strip.Stand,
			Capabilities:       &strip.Capabilities,
			CommunicationType:  &strip.CommunicationType,
			CdmData:            internalModels.NewLegacyCdmData(&strip.Eobt, nil, nil, nil, nil, nil, &strip.Eobt, nil),
			Bay:                bay,
			TrackingController: strip.TrackingController,
			EngineType:         strip.EngineType,
		}
		reg := ParseRegistration(strip.Callsign, strip.Remarks)
		newStrip.Registration = &reg
		if err = s.stripRepo.Create(ctx, newStrip); err != nil {
			return err
		}
		if strip.HasFP {
			if err = s.stripRepo.SetHasFP(ctx, session, strip.Callsign, true); err != nil {
				slog.WarnContext(ctx, "Failed to set has_fp on new strip", slog.String("callsign", strip.Callsign), slog.Any("error", err))
			}
		}
		slog.DebugContext(ctx, "Inserted strip",
			slog.String("callsign", strip.Callsign),
			slog.String("origin", strip.Origin),
			slog.String("destination", strip.Destination),
			slog.String("bay", bay),
		)
		if newClearedAlt != strip.ClearedAltitude && s.esCommander != nil {
			s.esCommander.SendClearedAltitude(session, cid, strip.Callsign, newClearedAlt)
		}
		if shouldGenerateDepartureSquawk(strip, airport, bay) && s.esCommander != nil {
			s.esCommander.SendGenerateSquawk(session, "", strip.Callsign)
		}
	} else {
		// Strip exists, update it
		dbExistingStrip := database.Strip{
			Origin:      existingStrip.Origin,
			Destination: existingStrip.Destination,
			Cleared:     existingStrip.Cleared,
			Bay:         existingStrip.Bay,
			State:       existingStrip.State,
			Stand:       existingStrip.Stand,
		}
		bay = shared.GetDepartureBay(strip, &dbExistingStrip, config.GetAirborneAltitudeAGL(), airport, gndOnline)
		effectiveCleared := strip.Cleared
		if shouldPreservePdcClearedFlag(existingStrip, strip) {
			effectiveCleared = existingStrip.Cleared
		}
		if shouldPreservePdcBay(existingStrip, strip, bay) {
			bay = existingStrip.Bay
		}
		shouldClearOwnerForNotCleared := bay == shared.BAY_NOT_CLEARED &&
			existingStrip.Bay != "" &&
			existingStrip.Bay != shared.BAY_NOT_CLEARED

		stand := existingStrip.Stand
		if strip.Stand != "" {
			stand = &strip.Stand
		}

		runway := existingStrip.Runway
		if strip.Runway != "" {
			runway = &strip.Runway
		} else if runway == nil || *runway == "" {
			isArrivalUpdate := strip.Destination == airport
			if assigned := autoAssignRunway(isArrivalUpdate, sessionObj.ActiveRunways); assigned != "" {
				runway = &assigned
			}
		}

		updateClearedAlt := strip.ClearedAltitude
		if updateClearedAlt == 0 && bay == shared.BAY_NOT_CLEARED {
			existingCfl := int32(0)
			if existingStrip.ClearedAltitude != nil {
				existingCfl = *existingStrip.ClearedAltitude
			}
			if existingCfl == 0 {
				runwayForAutoSet := ""
				if runway != nil {
					runwayForAutoSet = *runway
				}
				if autoCfl, ok := config.GetInitialCFLForRunway(runwayForAutoSet); ok {
					updateClearedAlt = int32(autoCfl)
				}
			}
		}

		updateStrip := &internalModels.Strip{
			Callsign:          strip.Callsign,
			Session:           session,
			Origin:            strip.Origin,
			Destination:       strip.Destination,
			Alternative:       &strip.Alternate,
			Route:             &strip.Route,
			Remarks:           &strip.Remarks,
			AssignedSquawk:    &strip.AssignedSquawk,
			Squawk:            &strip.Squawk,
			Sid:               &strip.Sid,
			ClearedAltitude:   &updateClearedAlt,
			Heading:           &strip.Heading,
			AircraftType:      &strip.AircraftType,
			Runway:            runway,
			RequestedAltitude: &strip.RequestedAltitude,
			Capabilities:      &strip.Capabilities,
			CommunicationType: &strip.CommunicationType,
			AircraftCategory:  &strip.AircraftCategory,
			Stand:             stand,
			Cleared:           effectiveCleared,
			State:             &strip.GroundState,
			PositionLatitude:  &strip.Position.Lat,
			PositionLongitude: &strip.Position.Lon,
			PositionAltitude:  &strip.Position.Altitude,
			Bay:               bay,
			CdmData: func() *internalModels.CdmData {
				cdmData := existingStrip.CdmData.Clone()
				if strip.Eobt != "" {
					cdmData.Tobt = &strip.Eobt
					cdmData.Eobt = &strip.Eobt
				}
				return cdmData
			}(),
			Registration:       existingStrip.Registration,
			Owner:              existingStrip.Owner,
			TrackingController: strip.TrackingController,
			EngineType:         strip.EngineType,
		}
		if _, err = s.stripRepo.Update(ctx, updateStrip); err != nil {
			return err
		}
		if shouldClearOwnerForNotCleared {
			if err := s.clearOwnerForNotCleared(ctx, session, strip.Callsign); err != nil {
				return err
			}
		}
		if strip.HasFP != existingStrip.HasFP {
			if err = s.stripRepo.SetHasFP(ctx, session, strip.Callsign, strip.HasFP); err != nil {
				slog.WarnContext(ctx, "Failed to update has_fp on strip", slog.String("callsign", strip.Callsign), slog.Any("error", err))
			}
		}
		slog.DebugContext(ctx, "Updated strip", slog.String("callsign", strip.Callsign))
		if updateClearedAlt != strip.ClearedAltitude && s.esCommander != nil {
			s.esCommander.SendClearedAltitude(session, cid, strip.Callsign, updateClearedAlt)
		}

		// Mark unexpected changes: stand is always unexpected when overwriting a non-empty value.
		// Runway is unexpected only for apron bays (not CLX/DEL/TWR).
		if strip.Stand != "" && existingStrip.Stand != nil && *existingStrip.Stand != "" && *existingStrip.Stand != strip.Stand {
			if err := s.stripRepo.AppendUnexpectedChangeField(ctx, session, strip.Callsign, "stand"); err != nil {
				slog.WarnContext(ctx, "Failed to mark stand as unexpected change", slog.String("callsign", strip.Callsign), slog.Any("error", err))
			}
		}
		if strip.Runway != "" && existingStrip.Runway != nil && *existingStrip.Runway != "" && *existingStrip.Runway != strip.Runway && isApronBay(bay) {
			if err := s.stripRepo.AppendUnexpectedChangeField(ctx, session, strip.Callsign, "runway"); err != nil {
				slog.WarnContext(ctx, "Failed to mark runway as unexpected change", slog.String("callsign", strip.Callsign), slog.Any("error", err))
			}
		}

		if existingStrip.Registration == nil || remarksContainsRegService(strip.Remarks) {
			newReg := ParseRegistration(strip.Callsign, strip.Remarks)
			if err := s.stripRepo.UpdateRegistration(ctx, session, strip.Callsign, newReg); err != nil {
				slog.ErrorContext(ctx, "Failed to update registration from remarks", slog.Any("error", err))
			}
		}
	}

	if err := server.UpdateRouteForStrip(strip.Callsign, session, false); err != nil {
		slog.ErrorContext(ctx, "Error updating route for strip", slog.String("callsign", strip.Callsign), slog.Any("error", err))
	}

	if err := s.MoveToBay(ctx, session, strip.Callsign, bay, false); err != nil {
		slog.ErrorContext(ctx, "Error moving bay for strip", slog.String("callsign", strip.Callsign), slog.Any("error", err))
	}

	if s.cdmService != nil && strings.EqualFold(strings.TrimSpace(strip.Origin), strings.TrimSpace(airport)) {
		s.cdmService.TriggerRecalculate(ctx, session, airport)
	}

	s.publisher.SendStripUpdate(session, strip.Callsign)

	return nil
}

func shouldGenerateDepartureSquawk(strip euroscope.Strip, airport string, bay string) bool {
	if !strings.EqualFold(strings.TrimSpace(strip.Origin), strings.TrimSpace(airport)) {
		return false
	}

	if bay != shared.BAY_NOT_CLEARED {
		return false
	}

	return !helpers.IsValidAssignedSquawk(strip.AssignedSquawk)
}

var remarksRegReService = regexp.MustCompile(`\bREG/([A-Z0-9-]+)`)

func remarksContainsRegService(remarks string) bool {
	return remarksRegReService.MatchString(strings.ToUpper(remarks))
}

// isApronBay returns true if the bay is managed by the apron controller
// (i.e., not CLX/DEL bays and not the TWR departure lineup bay).
// Runway unexpected-change marking is only applied for apron bays.
func isApronBay(bay string) bool {
	switch bay {
	case shared.BAY_PUSH, shared.BAY_TAXI, shared.BAY_TAXI_LWR, shared.BAY_TAXI_TWR,
		shared.BAY_TWY_ARR, shared.BAY_STAND:
		return true
	default:
		return false
	}
}

func shouldPreservePdcClearedFlag(existingStrip *internalModels.Strip, strip euroscope.Strip) bool {
	if existingStrip == nil || strip.Cleared || !existingStrip.Cleared {
		return false
	}

	return existingStrip.PdcState == "CLEARED" || existingStrip.PdcState == "CONFIRMED"
}

func shouldPreservePdcBay(existingStrip *internalModels.Strip, strip euroscope.Strip, bay string) bool {
	if existingStrip == nil || strip.Cleared || bay != shared.BAY_NOT_CLEARED {
		return false
	}

	if existingStrip.Bay == "" || existingStrip.Bay == shared.BAY_UNKNOWN || existingStrip.Bay == shared.BAY_NOT_CLEARED {
		return false
	}

	switch existingStrip.PdcState {
	case "CLEARED":
		return true
	case "CONFIRMED":
		return existingStrip.Cleared
	default:
		return false
	}
}

// autoAssignRunway returns the first active runway for the strip's direction,
// or "" if no active runways are configured.
func autoAssignRunway(isArrival bool, activeRunways models.ActiveRunways) string {
	if isArrival {
		if len(activeRunways.ArrivalRunways) > 0 {
			return activeRunways.ArrivalRunways[0]
		}
	} else {
		if len(activeRunways.DepartureRunways) > 0 {
			return activeRunways.DepartureRunways[0]
		}
	}
	return ""
}
