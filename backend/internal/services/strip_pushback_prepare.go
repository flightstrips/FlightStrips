package services

import (
	"FlightStrips/internal/shared"
	"context"
	"errors"
	"strings"
)

// PrepareFrontendPushback checks the same authorization and timing as a move,
// without moving the strip or assigning a release point before text is sent.
func (s *StripService) PrepareFrontendPushback(ctx context.Context, session int32, callsign, cid, airport, position string) error {
	strip, err := s.stripReader.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}
	if strip.IsValidationLocked() && strip.ValidationStatus.IssueType != pushbackTsatValidationIssueType {
		return errors.New("strip is locked by an active validation")
	}
	if err := validateFrontendMoveBayTransition(strip, airport, shared.BAY_PUSH, false); err != nil {
		return err
	}
	if err := s.authorizeFrontendMove(ctx, session, strip, callsign, airport, shared.BAY_PUSH, position, false); err != nil {
		return err
	}
	if strip.Bay != shared.BAY_PUSH && strings.EqualFold(strip.Origin, airport) {
		if err := s.validatePushbackTiming(ctx, session, strip, position); err != nil {
			return err
		}
	}
	strip, err = s.stripReader.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}
	if strip.IsValidationLocked() {
		return errors.New("strip is locked by an active validation")
	}
	return nil
}
