package vatsim

import (
	"context"
	"errors"
	"testing"
	"time"

	"FlightStrips/internal/aman/compatibility"
	"FlightStrips/internal/models"

	"github.com/stretchr/testify/require"
)

func TestArrivalReloadNotifiesOnlyCommittedVisibleETAChanges(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	writeErr := errors.New("ETA write failed")
	for _, tc := range []struct {
		name                               string
		visible, fresh, refreshBeforeWrite bool
		writeErr                           error
		wantNotifications                  int
	}{
		{name: "visible fallback", visible: true, wantNotifications: 1},
		{name: "hidden fallback"},
		{name: "fresh AMAN is a no-op", visible: true, fresh: true},
		{name: "concurrent AMAN rejects fallback", visible: true, refreshBeforeWrite: true},
		{name: "failed write", visible: true, writeErr: writeErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			eta := &models.ArrivalETA{
				Time: now.Add(20 * time.Minute), Source: compatibility.ArrivalETASource,
				CalculatedAt: now.Add(-compatibility.FreshFor - time.Minute),
			}
			if tc.fresh {
				eta.CalculatedAt = now
			}
			persisted := &models.Strip{Session: 1, Callsign: "SAS101", ArrivalETA: eta}
			if tc.visible {
				persisted.EuroscopeSeenAt = &now
			}
			store := &arrivalReloadStore{
				reconciliationTestStrips: &reconciliationTestStrips{bySession: map[int32][]*models.Strip{1: {persisted}}},
				writeErr:                 tc.writeErr,
			}
			if tc.refreshBeforeWrite {
				store.beforeFallback = func() {
					persisted.ArrivalETA = &models.ArrivalETA{
						Time: now.Add(30 * time.Minute), Source: compatibility.ArrivalETASource, CalculatedAt: now,
					}
				}
			}
			notifications := 0
			reconciler := &Reconciler{
				strips: store, now: func() time.Time { return now },
				notifier: arrivalReloadNotifier(func(session int32, callsign string) {
					require.EqualValues(t, 1, session)
					require.Equal(t, persisted.Callsign, callsign)
					require.Equal(t, ETAFiled, persisted.ArrivalETA.Source, "notify only after the fallback commits")
					notifications++
				}),
			}
			flights := []Flight{{Callsign: persisted.Callsign, State: FlightStateOnline,
				FlightPlan: FlightPlan{EOBT: "0800", EnrouteDuration: "0300", Destination: "EKCH"}}}
			view := map[string]*models.Strip{}
			_, err := reconciler.reloadArrivalStrips(t.Context(), 1, view, flights)
			if tc.writeErr != nil {
				require.ErrorIs(t, err, writeErr)
			} else {
				require.NoError(t, err)
				_, err = reconciler.reloadArrivalStrips(t.Context(), 1, view, flights)
				require.NoError(t, err, "a revision retry must not duplicate the notification")
			}
			require.Equal(t, tc.wantNotifications, notifications)
		})
	}
}

func TestArrivalReloadRetainsAMANExpiryAcrossFreshnessCutoff(t *testing.T) {
	start := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	expires := start.Add(time.Nanosecond)
	strip := &models.Strip{Session: 1, Callsign: "SAS101", ArrivalETA: &models.ArrivalETA{
		Time: start.Add(20 * time.Minute), Source: compatibility.ArrivalETASource,
		CalculatedAt: expires.Add(-compatibility.FreshFor),
	}}
	store := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{1: {strip}}}
	reads := 0
	reconciler := &Reconciler{strips: store, now: func() time.Time {
		reads++
		if reads == 1 {
			return start
		}
		return start.Add(time.Minute)
	}}
	flights := []Flight{{Callsign: strip.Callsign, State: FlightStateOnline,
		FlightPlan: FlightPlan{EOBT: "0800", EnrouteDuration: "0300", Destination: "EKCH"}}}
	view := map[string]*models.Strip{}
	revision, err := reconciler.reloadArrivalStrips(t.Context(), 1, view, flights)
	require.NoError(t, err)
	require.NotNil(t, view[strip.Callsign].ArrivalETA)
	require.Equal(t, compatibility.ArrivalETASource, view[strip.Callsign].ArrivalETA.Source)
	ctx := withArrivalETASnapshot(t.Context(), 1, view, flights, revision)
	lock := func(context.Context, int32) (int64, error) { return revision, nil }
	require.ErrorIs(t, CheckArrivalETASnapshot(ctx, 1, lock, reconciler.now), ErrArrivalETAChanged,
		"expiry after the read must force a new pass instead of accepting a missing ETA")
	_, err = reconciler.reloadArrivalStrips(t.Context(), 1, view, flights)
	require.NoError(t, err)
	require.NotNil(t, view[strip.Callsign].ArrivalETA)
	require.Equal(t, ETAFiled, view[strip.Callsign].ArrivalETA.Source)
	require.Equal(t, start.Add(time.Hour), view[strip.Callsign].ArrivalETA.Time)
}

type arrivalReloadStore struct {
	*reconciliationTestStrips
	beforeFallback func()
	writeErr       error
}

func (s *arrivalReloadStore) List(ctx context.Context, session int32) ([]*models.Strip, error) {
	strips, err := s.reconciliationTestStrips.List(ctx, session)
	result := make([]*models.Strip, 0, len(strips))
	for _, strip := range strips {
		copy := *strip
		result = append(result, &copy)
	}
	return result, err
}

func (s *arrivalReloadStore) UpdateFallbackArrivalETA(ctx context.Context, session int32, callsign string, eta *models.ArrivalETA, now time.Time) (*models.ArrivalETA, bool, error) {
	if s.beforeFallback != nil {
		hook := s.beforeFallback
		s.beforeFallback = nil
		hook()
	}
	if s.writeErr != nil {
		return nil, false, s.writeErr
	}
	return s.reconciliationTestStrips.UpdateFallbackArrivalETA(ctx, session, callsign, eta, now)
}

type arrivalReloadNotifier func(int32, string)

func (n arrivalReloadNotifier) SendStripUpdate(session int32, callsign string) { n(session, callsign) }
