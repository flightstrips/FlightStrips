package efb

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/repository"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// FlightQuery resolves an EFB flight without depending on the optional PDC
// subsystem. In non-live environments a unique LIVE match remains preferred,
// matching the existing pilot/PDC development lookup behaviour.
type FlightQuery struct {
	sessions repository.SessionRepository
	strips   repository.StripRepository
	liveOnly bool
}

func NewFlightQuery(sessions repository.SessionRepository, strips repository.StripRepository, liveOnly bool) *FlightQuery {
	return &FlightQuery{sessions: sessions, strips: strips, liveOnly: liveOnly}
}

func (q *FlightQuery) FindWebStripByCallsign(ctx context.Context, callsign string) (pdc.WebStripMatch, error) {
	normalized := strings.ToUpper(strings.TrimSpace(callsign))
	if normalized == "" {
		return pdc.WebStripMatch{}, pdc.ErrWebStripNotFound
	}

	var (
		sessions []*models.Session
		err      error
	)
	if q.liveOnly {
		sessions, err = q.sessions.GetByNames(ctx, "LIVE")
	} else {
		sessions, err = q.sessions.List(ctx)
	}
	if err != nil {
		return pdc.WebStripMatch{}, fmt.Errorf("list EFB sessions: %w", err)
	}

	matches := make([]pdc.WebStripMatch, 0, 1)
	liveMatches := make([]pdc.WebStripMatch, 0, 1)
	for _, session := range sessions {
		strip, lookupErr := q.strips.GetByCallsign(ctx, session.ID, normalized)
		if lookupErr != nil {
			if errors.Is(lookupErr, sql.ErrNoRows) || errors.Is(lookupErr, pgx.ErrNoRows) {
				continue
			}
			return pdc.WebStripMatch{}, fmt.Errorf("get EFB strip: %w", lookupErr)
		}
		match := pdc.WebStripMatch{SessionID: session.ID, Strip: strip}
		matches = append(matches, match)
		if strings.EqualFold(session.Name, "LIVE") {
			liveMatches = append(liveMatches, match)
		}
	}

	if len(matches) == 0 {
		return pdc.WebStripMatch{}, pdc.ErrWebStripNotFound
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if !q.liveOnly && len(liveMatches) == 1 {
		return liveMatches[0], nil
	}
	return pdc.WebStripMatch{}, pdc.ErrWebAmbiguousCallsign
}
