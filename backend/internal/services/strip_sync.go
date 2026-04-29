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
	"reflect"
	"regexp"
	"slices"
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

type syncRouteComputer interface {
	ComputeNextOwnersForStripContext(ctx context.Context, strip *internalModels.Strip, sessionId int32) ([]string, bool, error)
}

func (s *StripService) syncEuroscopeStrip(ctx context.Context, session int32, cid string, strip euroscope.Strip, airport string) error {
	server := s.publisher.GetServer()
	if server == nil {
		return errors.New("server not configured")
	}
	routeComputer, _ := server.(syncRouteComputer)

	syncState := shared.GetSyncState(ctx)

	// Fetch the session so we can read ActiveRunways for runway auto-assignment.
	sessionObj := (*internalModels.Session)(nil)
	if syncState != nil {
		sessionObj = syncState.Session
	}
	if sessionObj == nil {
		var err error
		sessionObj, err = s.getCachedSession(ctx, session)
		if err != nil {
			return err
		}
	}

	var (
		existingStrip *internalModels.Strip
		err           error
		ok            bool
	)
	if syncState != nil && syncState.ExistingStrips != nil {
		existingStrip, ok = syncState.ExistingStrips[strip.Callsign]
		if !ok {
			err = pgx.ErrNoRows
		}
	} else {
		existingStrip, ok, err = s.getCachedStrip(ctx, session, strip.Callsign)
		if !ok {
			err = pgx.ErrNoRows
		}
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	var bay string
	gndOnline := s.isGndOnline(ctx, session)
	if syncState != nil {
		gndOnline = syncState.GndOnline
	}

	var validationStrip *internalModels.Strip
	createdStrip := false
	routeNeedsUpdate := false
	needsStripBroadcast := false
	needsPdcValidation := false

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
			Version:            1,
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
			HasFP:              strip.HasFP,
		}
		validationStrip = newStrip
		sequence, err := s.nextSequenceAtEndOfBay(ctx, session, bay)
		if err != nil {
			return err
		}
		newStrip.Sequence = &sequence
		reg := ParseRegistration(strip.Callsign, strip.Remarks)
		newStrip.Registration = &reg
		if routeComputer != nil {
			nextOwners, handled, err := routeComputer.ComputeNextOwnersForStripContext(ctx, newStrip, session)
			if err != nil {
				return err
			}
			if handled {
				newStrip.NextOwners = nextOwners
			}
		} else {
			routeNeedsUpdate = true
		}
		if err = s.stripRepo.Create(ctx, newStrip); err != nil {
			return err
		}
		shared.AddDBOperations(ctx, 1)
		s.cacheStrip(ctx, newStrip)
		if syncState != nil {
			syncState.ChangedStrips++
			if syncState.ExistingStrips != nil {
				syncState.ExistingStrips[strip.Callsign] = newStrip
			}
		}
		if err := s.applyBayChangeEffects(ctx, session, strip.Callsign, "", bay, false); err != nil {
			return err
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
		createdStrip = true
		needsStripBroadcast = true
		needsPdcValidation = true
	} else {
		// Strip exists, update it
		effectiveGroundState := strip.GroundState
		if (strip.Origin == "" || strip.Destination == "") &&
			strip.GroundState == euroscope.GroundStateUnknown &&
			existingStrip.State != nil &&
			*existingStrip.State != euroscope.GroundStateUnknown &&
			shared.GetGroundState(existingStrip.Bay) != euroscope.GroundStateUnknown {
			effectiveGroundState = *existingStrip.State
		}
		if existingStrip.Bay == shared.BAY_DEPART && strip.GroundState == euroscope.GroundStateTaxi {
			effectiveGroundState = shared.GetGroundState(existingStrip.Bay)
			if existingStrip.State != nil && *existingStrip.State != euroscope.GroundStateUnknown {
				effectiveGroundState = *existingStrip.State
			}
		}
		if existingStrip.Bay == shared.BAY_AIRBORNE && strip.GroundState == euroscope.GroundStateTaxi {
			effectiveGroundState = euroscope.GroundStateUnknown
			if existingStrip.State != nil &&
				*existingStrip.State != euroscope.GroundStateUnknown &&
				*existingStrip.State != euroscope.GroundStateTaxi {
				effectiveGroundState = *existingStrip.State
			}
		}
		dbExistingStrip := database.Strip{
			Origin:      existingStrip.Origin,
			Destination: existingStrip.Destination,
			Cleared:     existingStrip.Cleared,
			Bay:         existingStrip.Bay,
			State:       existingStrip.State,
			Stand:       existingStrip.Stand,
		}
		bayStrip := strip
		bayStrip.GroundState = effectiveGroundState
		bay = shared.GetDepartureBay(bayStrip, &dbExistingStrip, config.GetAirborneAltitudeAGL(), airport, gndOnline)
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

		origin := strip.Origin
		if origin == "" {
			origin = existingStrip.Origin
		}

		destination := strip.Destination
		if destination == "" {
			destination = existingStrip.Destination
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
			Origin:            origin,
			Destination:       destination,
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
			State:             &effectiveGroundState,
			PositionLatitude:  &strip.Position.Lat,
			PositionLongitude: &strip.Position.Lon,
			PositionAltitude:  &strip.Position.Altitude,
			Sequence:          existingStrip.Sequence,
			Bay:               bay,
			CdmData: func() *internalModels.CdmData {
				cdmData := existingStrip.CdmData.Clone()
				if strip.Eobt != "" {
					cdmData.Tobt = &strip.Eobt
					cdmData.Eobt = &strip.Eobt
				}
				return cdmData
			}(),
			NextOwners:             slices.Clone(existingStrip.NextOwners),
			PreviousOwners:         slices.Clone(existingStrip.PreviousOwners),
			Registration:           existingStrip.Registration,
			Owner:                  existingStrip.Owner,
			PdcState:               existingStrip.PdcState,
			PdcRequestRemarks:      existingStrip.PdcRequestRemarks,
			TrackingController:     strip.TrackingController,
			EngineType:             strip.EngineType,
			UnexpectedChangeFields: slices.Clone(existingStrip.UnexpectedChangeFields),
			ValidationStatus:       existingStrip.ValidationStatus,
			HasFP:                  strip.HasFP,
		}
		validationStrip = updateStrip
		registrationNeedsUpdate := false
		registrationValue := ""
		if existingStrip.Registration == nil || remarksContainsRegService(strip.Remarks) {
			registrationValue = ParseRegistration(strip.Callsign, strip.Remarks)
			registrationNeedsUpdate = existingStrip.Registration == nil || registrationValue != *existingStrip.Registration
		}
		if registrationNeedsUpdate {
			updateStrip.Registration = &registrationValue
		}
		if shouldClearOwnerForNotCleared {
			updateStrip.Owner = nil
			updateStrip.PreviousOwners = []string{}
		}

		unexpectedStandChange := strip.Stand != "" && existingStrip.Stand != nil && *existingStrip.Stand != "" && *existingStrip.Stand != strip.Stand
		unexpectedRunwayChange := strip.Runway != "" && existingStrip.Runway != nil && *existingStrip.Runway != "" && *existingStrip.Runway != strip.Runway && isApronBay(bay)
		if unexpectedStandChange {
			updateStrip.UnexpectedChangeFields = appendUnexpectedChangeField(updateStrip.UnexpectedChangeFields, "stand")
		}
		if unexpectedRunwayChange {
			updateStrip.UnexpectedChangeFields = appendUnexpectedChangeField(updateStrip.UnexpectedChangeFields, "runway")
		}
		bayChanged := existingStrip.Bay != updateStrip.Bay
		if bayChanged {
			sequence, err := s.nextSequenceAtEndOfBay(ctx, session, bay)
			if err != nil {
				return err
			}
			updateStrip.Sequence = &sequence
		}
		routeNeedsUpdate = syncStripRouteChanged(existingStrip, updateStrip) || shouldClearOwnerForNotCleared
		if routeNeedsUpdate && routeComputer != nil {
			nextOwners, handled, err := routeComputer.ComputeNextOwnersForStripContext(ctx, updateStrip, session)
			if err != nil {
				return err
			}
			if handled {
				updateStrip.NextOwners = nextOwners
			}
			routeNeedsUpdate = false
		}
		primaryChange := syncStripChanged(existingStrip, updateStrip)

		if !primaryChange {
			return nil
		}

		if primaryChange {
			if _, err = s.stripRepo.Update(ctx, updateStrip); err != nil {
				return err
			}
			shared.AddDBOperations(ctx, 1)
			if syncState != nil {
				syncState.ChangedStrips++
				applySyncStripUpdate(existingStrip, updateStrip)
			}
			s.cacheStrip(ctx, updateStrip)
		}
		if bayChanged {
			if err := s.applyBayChangeEffects(ctx, session, strip.Callsign, existingStrip.Bay, bay, false); err != nil {
				return err
			}
		}
		slog.DebugContext(ctx, "Updated strip", slog.String("callsign", strip.Callsign))
		if primaryChange && updateClearedAlt != strip.ClearedAltitude && s.esCommander != nil {
			s.esCommander.SendClearedAltitude(session, cid, strip.Callsign, updateClearedAlt)
		}

		needsStripBroadcast = true
		needsPdcValidation = true
	}

	if syncState != nil {
		if routeNeedsUpdate {
			syncState.MarkRouteRecalc(strip.Callsign)
		}
		if needsPdcValidation {
			syncState.MarkPdcValidation(strip.Callsign)
		}
		if createdStrip || validationStrip != nil {
			syncState.SquawkValidation = true
		}
		if s.cdmService != nil && validationStrip != nil && strings.EqualFold(strings.TrimSpace(validationStrip.Origin), strings.TrimSpace(airport)) {
			syncState.CdmRecalculation = true
		}
		if needsStripBroadcast {
			syncState.MarkStripUpdate(strip.Callsign)
		}
		return nil
	}

	if routeNeedsUpdate {
		if err := server.UpdateRouteForStripContext(ctx, strip.Callsign, session, false); err != nil {
			slog.ErrorContext(ctx, "Error updating route for strip", slog.String("callsign", strip.Callsign), slog.Any("error", err))
		}
	}

	if needsPdcValidation {
		if err := s.ReevaluatePdcRequestValidationsForStrip(ctx, session, validationStrip, sessionObj.ActiveRunways.DepartureRunways, true, false); err != nil {
			return err
		}
	}

	if createdStrip || validationStrip != nil {
		if err := s.reevaluateSquawkValidationsForSession(ctx, session, true); err != nil {
			return err
		}
	}

	if s.cdmService != nil && validationStrip != nil && strings.EqualFold(strings.TrimSpace(validationStrip.Origin), strings.TrimSpace(airport)) {
		s.cdmService.TriggerRecalculate(ctx, session, airport)
	}

	if needsStripBroadcast {
		s.publisher.SendStripUpdate(session, strip.Callsign)
	}

	return nil
}

func syncStripChanged(existingStrip, updateStrip *internalModels.Strip) bool {
	if existingStrip == nil || updateStrip == nil {
		return true
	}

	return existingStrip.Origin != updateStrip.Origin ||
		existingStrip.Destination != updateStrip.Destination ||
		!reflect.DeepEqual(existingStrip.Alternative, updateStrip.Alternative) ||
		!reflect.DeepEqual(existingStrip.Route, updateStrip.Route) ||
		!reflect.DeepEqual(existingStrip.Remarks, updateStrip.Remarks) ||
		!reflect.DeepEqual(existingStrip.AssignedSquawk, updateStrip.AssignedSquawk) ||
		!reflect.DeepEqual(existingStrip.Squawk, updateStrip.Squawk) ||
		!reflect.DeepEqual(existingStrip.Sid, updateStrip.Sid) ||
		!reflect.DeepEqual(existingStrip.ClearedAltitude, updateStrip.ClearedAltitude) ||
		!reflect.DeepEqual(existingStrip.Heading, updateStrip.Heading) ||
		!reflect.DeepEqual(existingStrip.AircraftType, updateStrip.AircraftType) ||
		!reflect.DeepEqual(existingStrip.Runway, updateStrip.Runway) ||
		!reflect.DeepEqual(existingStrip.RequestedAltitude, updateStrip.RequestedAltitude) ||
		!reflect.DeepEqual(existingStrip.Capabilities, updateStrip.Capabilities) ||
		!reflect.DeepEqual(existingStrip.CommunicationType, updateStrip.CommunicationType) ||
		!reflect.DeepEqual(existingStrip.AircraftCategory, updateStrip.AircraftCategory) ||
		!reflect.DeepEqual(existingStrip.Stand, updateStrip.Stand) ||
		!reflect.DeepEqual(existingStrip.Sequence, updateStrip.Sequence) ||
		existingStrip.Cleared != updateStrip.Cleared ||
		!reflect.DeepEqual(existingStrip.State, updateStrip.State) ||
		!reflect.DeepEqual(existingStrip.Owner, updateStrip.Owner) ||
		!reflect.DeepEqual(existingStrip.PositionLatitude, updateStrip.PositionLatitude) ||
		!reflect.DeepEqual(existingStrip.PositionLongitude, updateStrip.PositionLongitude) ||
		!reflect.DeepEqual(existingStrip.PositionAltitude, updateStrip.PositionAltitude) ||
		existingStrip.Bay != updateStrip.Bay ||
		!reflect.DeepEqual(existingStrip.CdmData, updateStrip.CdmData) ||
		!reflect.DeepEqual(existingStrip.NextOwners, updateStrip.NextOwners) ||
		!reflect.DeepEqual(existingStrip.PreviousOwners, updateStrip.PreviousOwners) ||
		!reflect.DeepEqual(existingStrip.Registration, updateStrip.Registration) ||
		existingStrip.TrackingController != updateStrip.TrackingController ||
		existingStrip.EngineType != updateStrip.EngineType ||
		!reflect.DeepEqual(existingStrip.UnexpectedChangeFields, updateStrip.UnexpectedChangeFields) ||
		existingStrip.HasFP != updateStrip.HasFP
}

func syncStripRouteChanged(existingStrip, updateStrip *internalModels.Strip) bool {
	if existingStrip == nil || updateStrip == nil {
		return true
	}

	return existingStrip.Origin != updateStrip.Origin ||
		existingStrip.Destination != updateStrip.Destination ||
		!reflect.DeepEqual(existingStrip.Sid, updateStrip.Sid) ||
		!reflect.DeepEqual(existingStrip.Runway, updateStrip.Runway) ||
		!reflect.DeepEqual(existingStrip.Stand, updateStrip.Stand) ||
		!reflect.DeepEqual(existingStrip.PositionLatitude, updateStrip.PositionLatitude) ||
		!reflect.DeepEqual(existingStrip.PositionLongitude, updateStrip.PositionLongitude)
}

func applySyncStripUpdate(existingStrip, updateStrip *internalModels.Strip) {
	if existingStrip == nil || updateStrip == nil {
		return
	}

	existingStrip.Version++
	existingStrip.Origin = updateStrip.Origin
	existingStrip.Destination = updateStrip.Destination
	existingStrip.Alternative = updateStrip.Alternative
	existingStrip.Route = updateStrip.Route
	existingStrip.Remarks = updateStrip.Remarks
	existingStrip.AssignedSquawk = updateStrip.AssignedSquawk
	existingStrip.Squawk = updateStrip.Squawk
	existingStrip.Sid = updateStrip.Sid
	existingStrip.ClearedAltitude = updateStrip.ClearedAltitude
	existingStrip.Heading = updateStrip.Heading
	existingStrip.AircraftType = updateStrip.AircraftType
	existingStrip.Runway = updateStrip.Runway
	existingStrip.RequestedAltitude = updateStrip.RequestedAltitude
	existingStrip.Capabilities = updateStrip.Capabilities
	existingStrip.CommunicationType = updateStrip.CommunicationType
	existingStrip.AircraftCategory = updateStrip.AircraftCategory
	existingStrip.Stand = updateStrip.Stand
	existingStrip.State = updateStrip.State
	existingStrip.Cleared = updateStrip.Cleared
	existingStrip.Bay = updateStrip.Bay
	existingStrip.Sequence = updateStrip.Sequence
	existingStrip.PositionLatitude = updateStrip.PositionLatitude
	existingStrip.PositionLongitude = updateStrip.PositionLongitude
	existingStrip.PositionAltitude = updateStrip.PositionAltitude
	existingStrip.CdmData = updateStrip.CdmData
	existingStrip.NextOwners = slices.Clone(updateStrip.NextOwners)
	existingStrip.PreviousOwners = slices.Clone(updateStrip.PreviousOwners)
	existingStrip.Registration = updateStrip.Registration
	existingStrip.Owner = updateStrip.Owner
	existingStrip.PdcState = updateStrip.PdcState
	existingStrip.PdcRequestRemarks = updateStrip.PdcRequestRemarks
	existingStrip.TrackingController = updateStrip.TrackingController
	existingStrip.EngineType = updateStrip.EngineType
	existingStrip.UnexpectedChangeFields = slices.Clone(updateStrip.UnexpectedChangeFields)
	existingStrip.HasFP = updateStrip.HasFP
	existingStrip.ValidationStatus = updateStrip.ValidationStatus
}

func appendUnexpectedChangeField(fields []string, field string) []string {
	if field == "" || slices.Contains(fields, field) {
		return fields
	}
	return append(fields, field)
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
