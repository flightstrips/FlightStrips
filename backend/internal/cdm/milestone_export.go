package cdm

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"FlightStrips/internal/models"
	"github.com/jackc/pgx/v5"
)

// Departure milestones must not be exported for an arrival or another airport.
func (c *ActionService) isLocalDeparture(ctx context.Context, session int32, strip *models.Strip) (bool, error) {
	if strip == nil || strings.TrimSpace(strip.Origin) == "" {
		return false, nil
	}
	sessionData, err := c.service.sessionRepo.GetByID(ctx, session)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load departure milestone session %d for %s: %w", session, strip.Callsign, err)
	}
	return sessionData != nil && strings.EqualFold(strings.TrimSpace(strip.Origin), strings.TrimSpace(sessionData.Airport)), nil
}

func (c *ActionService) sendPendingAobt(ctx context.Context, session int32, callsign string) error {
	return c.sendPendingMilestone(ctx, session, callsign, true)
}

func (c *ActionService) sendPendingMilestone(ctx context.Context, session int32, callsign string, offBlock bool) error {
	s := c.service
	if !s.client.isValid || !s.usesViffSession(session) || s.isSessionRemoved(session) {
		return nil
	}
	ctx, cancel := s.sessionContext(ctx, session)
	defer cancel()
	inFlight := &s.atotPushInFlight
	kind := "ATOT"
	if offBlock {
		inFlight, kind = &s.aobtPushInFlight, "AOBT"
	}
	key := viffPushKey(session, callsign)
	if _, busy := inFlight.LoadOrStore(key, struct{}{}); busy {
		return nil
	}
	defer inFlight.Delete(key)
	data, err := s.stripRepo.GetCdmDataForCallsign(ctx, session, callsign)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load pending %s for %s session %d: %w", kind, callsign, session, err)
	}
	value, pending := milestoneValue(data, offBlock)
	if !pending || value == "" {
		return nil
	}
	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load %s export target %s session %d: %w", kind, callsign, session, err)
	}
	departure, err := c.isLocalDeparture(ctx, session, strip)
	if err != nil || !departure {
		return err
	}
	if err := s.client.IFPSDpi(ctx, callsign, kind+"/"+value); err != nil {
		return fmt.Errorf("export %s/%s for %s session %d: %w", kind, value, callsign, session, err)
	}
	// Clear only the acknowledged flag, conditional on its timestamp. Replacing
	// the whole document here could overwrite concurrent timing or milestone updates.
	_, err = s.stripRepo.AcknowledgeCdmMilestone(ctx, session, callsign, kind, value)
	if err != nil {
		return fmt.Errorf("acknowledge %s/%s for %s session %d: %w", kind, value, callsign, session, err)
	}
	return nil
}

func milestoneValue(data *models.CdmData, offBlock bool) (string, bool) {
	if data == nil {
		return "", false
	}
	if offBlock {
		if data.Aobt == nil {
			return "", data.AobtViffPending
		}
		return *data.Aobt, data.AobtViffPending
	}
	if data.Atot == nil {
		return "", data.AtotViffPending
	}
	return *data.Atot, data.AtotViffPending
}
