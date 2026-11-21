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
	session   int32
	callsigns []string
	timings   map[string]arrivalETATiming
}

func withArrivalETASnapshot(ctx context.Context, session int32, strips map[string]*models.Strip, flights []Flight) context.Context {
	snapshot := arrivalETASnapshot{
		session: session, callsigns: make([]string, 0, len(flights)), timings: make(map[string]arrivalETATiming, len(flights)),
	}
	for _, flight := range flights {
		if strip := strips[flight.Callsign]; strip != nil {
			snapshot.callsigns = append(snapshot.callsigns, strip.Callsign)
			snapshot.timings[strip.Callsign] = arrivalTiming(strip.ArrivalETA)
		}
	}
	return context.WithValue(ctx, arrivalETAContextKey{}, snapshot)
}

// CheckArrivalETASnapshot locks and validates all timing inputs that determined
// the pass order. Those locks must be held until the stand transaction commits.
func CheckArrivalETASnapshot(ctx context.Context, session int32, lockETAs func(context.Context, int32, []string) (map[string]*models.ArrivalETA, error), clock func() time.Time) error {
	snapshot, ok := ctx.Value(arrivalETAContextKey{}).(arrivalETASnapshot)
	if !ok || snapshot.session != session || len(snapshot.callsigns) == 0 {
		return nil
	}
	etas, err := lockETAs(ctx, session, snapshot.callsigns)
	if err != nil {
		return err
	}
	if len(etas) != len(snapshot.timings) {
		return ErrArrivalETAChanged
	}
	now := clock()
	for callsign, expected := range snapshot.timings {
		eta, found := etas[callsign]
		if !found || arrivalTiming(effectiveArrivalETA(eta, now)) != expected {
			return ErrArrivalETAChanged
		}
	}
	return nil
}
