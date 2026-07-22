// Package compatibility contains one-way projections from AMAN-owned state to
// legacy FlightStrips fields. It never feeds a legacy value back into AMAN.
package compatibility

import (
	"context"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/models"
)

const ArrivalETASource = "aman"

// ArrivalETAStore is the narrow strip persistence capability required by the
// AMAN compatibility adapter. A nil projection is written as SQL NULL rather
// than leaving a legacy estimate visible in an operational AMAN mode.
type ArrivalETAStore interface {
	UpdateArrivalETA(context.Context, int32, string, models.ArrivalETA) (int64, error)
	ClearArrivalETA(context.Context, int32, string) (int64, error)
}

// Writer is the sole adapter permitted to write AMAN's one-way ArrivalETA
// compatibility projection. It has no prediction or sequencing inputs.
type Writer struct {
	mode   aman.RolloutMode
	maxAge time.Duration
	store  ArrivalETAStore
}

func NewWriter(mode aman.RolloutMode, maxAge time.Duration, store ArrivalETAStore) Writer {
	return Writer{mode: mode, maxAge: maxAge, store: store}
}

// Apply writes the current operational projection, or explicitly clears it
// when AMAN is unavailable/expired. Disabled and shadow modes do not mutate
// the compatibility field because the legacy reconciler owns it there.
func (w Writer) Apply(ctx context.Context, session int32, callsign string, prediction *aman.Prediction, now time.Time) (bool, error) {
	eta := ProjectArrivalETA(w.mode, prediction, now, w.maxAge)
	if w.mode != aman.ModeReadOnly && w.mode != aman.ModeAuthoritative {
		return false, nil
	}
	if eta == nil {
		updated, err := w.store.ClearArrivalETA(ctx, session, callsign)
		return updated != 0, err
	}
	updated, err := w.store.UpdateArrivalETA(ctx, session, callsign, *eta)
	return updated != 0, err
}

// ProjectArrivalETA returns the only legacy-compatible representation of an
// operational AMAN TETA. A nil result means the compatibility field must be
// unavailable/null; callers must not substitute the legacy estimator.
//
// Only read-only and authoritative modes own this projection. Disabled and
// shadow modes deliberately return nil because their legacy writer remains
// operational. A zero maxAge disables expiry checks.
func ProjectArrivalETA(mode aman.RolloutMode, prediction *aman.Prediction, now time.Time, maxAge time.Duration) *models.ArrivalETA {
	if mode != aman.ModeReadOnly && mode != aman.ModeAuthoritative {
		return nil
	}
	if prediction == nil || !prediction.Publishable || prediction.OperationalTETA.IsZero() || prediction.GeneratedAt.IsZero() {
		return nil
	}
	if maxAge > 0 && !now.IsZero() && prediction.GeneratedAt.Before(now.Add(-maxAge)) {
		return nil
	}
	return &models.ArrivalETA{
		Time:         prediction.OperationalTETA,
		Source:       ArrivalETASource,
		CalculatedAt: prediction.GeneratedAt,
	}
}
