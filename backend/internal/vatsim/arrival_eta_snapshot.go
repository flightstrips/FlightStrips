package vatsim

import (
	"context"
	"errors"
	"time"

	"FlightStrips/internal/aman/compatibility"
	"FlightStrips/internal/models"
)

// ErrArrivalETAChanged asks reconciliation to reload and reprioritize arrivals
// when the ETA used for a stand decision has changed before the write.
var ErrArrivalETAChanged = errors.New("arrival ETA changed during stand reconciliation")

type arrivalETATiming struct {
	time   time.Time
	source string
}

func effectiveArrivalETA(eta *models.ArrivalETA, now time.Time) *models.ArrivalETA {
	if eta == nil || eta.Time.IsZero() ||
		(eta.Source == compatibility.ArrivalETASource && !compatibility.IsFresh(eta, now)) {
		return nil
	}
	return eta
}

func arrivalTiming(eta *models.ArrivalETA) arrivalETATiming {
	if eta == nil {
		return arrivalETATiming{}
	}
	return arrivalETATiming{time: eta.Time.UTC(), source: eta.Source}
}

type arrivalETAContextKey struct{}

type arrivalETASnapshot struct {
	session  int32
	revision int64
	expires  time.Time
}

func withArrivalETASnapshot(ctx context.Context, session int32, strips map[string]*models.Strip, flights []Flight, revision int64) context.Context {
	snapshot := arrivalETASnapshot{session: session, revision: revision}
	for _, flight := range flights {
		strip := strips[flight.Callsign]
		if strip == nil || strip.ArrivalETA == nil || strip.ArrivalETA.Source != compatibility.ArrivalETASource {
			continue
		}
		expires := strip.ArrivalETA.CalculatedAt.Add(compatibility.FreshFor)
		if snapshot.expires.IsZero() || expires.Before(snapshot.expires) {
			snapshot.expires = expires
		}
	}
	return context.WithValue(ctx, arrivalETAContextKey{}, snapshot)
}

// CheckArrivalETASnapshot checks one transactionally maintained session revision.
// The shared revision lock protects all pass inputs through the stand commit.
func CheckArrivalETASnapshot(ctx context.Context, session int32, lockRevision func(context.Context, int32) (int64, error), clock func() time.Time) error {
	snapshot, ok := ctx.Value(arrivalETAContextKey{}).(arrivalETASnapshot)
	if !ok || snapshot.session != session {
		return nil
	}
	revision, err := lockRevision(ctx, session)
	if err != nil {
		return err
	}
	if revision != snapshot.revision || (!snapshot.expires.IsZero() && clock().After(snapshot.expires)) {
		return ErrArrivalETAChanged
	}
	return nil
}
