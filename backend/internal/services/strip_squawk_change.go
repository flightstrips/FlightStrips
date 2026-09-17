package services

import (
	"FlightStrips/internal/models"
	"context"
	"strings"
)

func (s *StripService) updateSquawkWithPrevious(ctx context.Context, session int32, callsign, code string, assigned bool) (*string, int64, bool, error) {
	if store, ok := s.fieldStore.(interface {
		UpdateSquawkWithPrevious(context.Context, int32, string, string, bool) (*string, int64, error)
	}); ok {
		previous, count, err := store.UpdateSquawkWithPrevious(ctx, session, callsign, code, assigned)
		return previous, count, true, err
	}
	if assigned {
		count, err := s.fieldStore.UpdateAssignedSquawk(ctx, session, callsign, &code, nil)
		return nil, count, false, err
	}
	count, err := s.fieldStore.UpdateSquawk(ctx, session, callsign, &code, nil)
	return nil, count, false, err
}

func (s *StripService) reevaluateChangedSquawk(ctx context.Context, session int32, callsign string, previous *string, code string, targeted bool) error {
	if !targeted {
		return s.reevaluateSquawkValidationsForSession(ctx, session, true)
	}
	oldCode, newCode := normalizeValidationSquawkCode(previous), normalizeValidationSquawkCode(&code)
	return s.reevaluateSquawkValidations(ctx, session, true, func(strip *models.Strip) bool {
		if strings.EqualFold(strip.Callsign, callsign) {
			return true
		}
		codes := duplicateSquawkCodesForStrip(strip)
		_, oldMatch := codes[oldCode]
		_, newMatch := codes[newCode]
		return oldMatch || newMatch
	})
}
