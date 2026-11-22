package vatsim

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman/compatibility"
	"FlightStrips/internal/models"

	"github.com/stretchr/testify/require"
)

func TestLegacyArrivalETAWriterDefaultsOnAndCanBeDisabled(t *testing.T) {
	base := newReconciler(nil, nil, nil, nil, nil, nil, nil, time.Second)
	require.True(t, base.legacyArrivalETAWriter)

	disabled := newReconciler(nil, nil, nil, nil, nil, nil, nil, time.Second, WithLegacyArrivalETAWriter(false))
	require.False(t, disabled.legacyArrivalETAWriter)
}

// With AMAN owning the ETA, the VATSIM estimate is the fallback for any strip
// that has no fresh AMAN projection.
func TestVATSIMETAFallsBackWhenAMANHasNoFreshETA(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	amanETA := now.Add(30 * time.Minute)
	for name, test := range map[string]struct {
		existing   *models.ArrivalETA
		wantSource string
	}{
		"no ETA":         {existing: nil, wantSource: ETAFiled},
		"stale AMAN ETA": {existing: &models.ArrivalETA{Time: amanETA, Source: compatibility.ArrivalETASource, CalculatedAt: now.Add(-compatibility.FreshFor - time.Minute)}, wantSource: ETAFiled},
		"fresh AMAN ETA": {existing: &models.ArrivalETA{Time: amanETA, Source: compatibility.ArrivalETASource, CalculatedAt: now.Add(-time.Minute)}, wantSource: compatibility.ArrivalETASource},
	} {
		t.Run(name, func(t *testing.T) {
			flight := Flight{
				CID: "1", Callsign: "SAS101", State: FlightStatePrefile, LastUpdated: now,
				FlightPlan: FlightPlan{Origin: "EGLL", Destination: "EKCH", EOBT: "0800", EnrouteDuration: "0245", Revision: 1},
			}
			strips := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{7: {{
				Callsign: "SAS101", Session: 7, Origin: "EGLL", Destination: "EKCH", Bay: hiddenArrivalBay, ArrivalETA: test.existing,
			}}}}
			reconciler := newTestReconciler(newReconciliationTestCache(now, flight), reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}},
				strips, reconciliationTestAssignments{}, nil, time.Second, WithClock(func() time.Time { return now }), WithLegacyArrivalETAWriter(false))

			require.NoError(t, reconciler.Reconcile(context.Background()))
			eta := strips.bySession[7][0].ArrivalETA
			require.NotNil(t, eta)
			require.Equal(t, test.wantSource, eta.Source)
		})
	}
}

func TestVATSIMFallbackReloadsETAWhenAMANRefreshedAfterSnapshot(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	amanETA := &models.ArrivalETA{Time: now.Add(30 * time.Minute), Source: compatibility.ArrivalETASource, CalculatedAt: now}
	strip := &models.Strip{Callsign: "SAS101", Session: 7}
	persisted := *strip
	persisted.ArrivalETA = amanETA
	store := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{7: {&persisted}}}
	reconciler := &Reconciler{strips: store, now: func() time.Time { return now }}

	changed, err := reconciler.updateArrivalETA(context.Background(), 7, strip, Flight{
		State: FlightStatePrefile, FlightPlan: FlightPlan{Destination: "EKCH", EOBT: "0800", EnrouteDuration: "0245"},
	}, false)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, amanETA, strip.ArrivalETA)
	require.Empty(t, store.updated)
}

type stripDeletedBeforeFallback struct {
	*reconciliationTestStrips
	callsign string
}

func (s *stripDeletedBeforeFallback) UpdateFallbackArrivalETA(ctx context.Context, session int32, callsign string, eta models.ArrivalETA, now time.Time) (int64, error) {
	if callsign == s.callsign {
		delete(s.bySession, session)
	}
	return s.reconciliationTestStrips.UpdateFallbackArrivalETA(ctx, session, callsign, eta, now)
}

func TestVATSIMFallbackContinuesAfterConcurrentStripDeletion(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	first := Flight{
		CID: "1", Callsign: "SAS101", State: FlightStatePrefile, LastUpdated: now,
		FlightPlan: FlightPlan{Origin: "EGLL", Destination: "EKCH", EOBT: "0800", EnrouteDuration: "0245", Revision: 1},
	}
	second := first
	second.CID, second.Callsign = "2", "SAS202"
	store := &stripDeletedBeforeFallback{
		reconciliationTestStrips: stripStoreWithSingleSession(7, &models.Strip{
			Callsign: first.Callsign, Session: 7, Origin: "EGLL", Destination: "EKCH", Bay: hiddenArrivalBay,
		}),
		callsign: first.Callsign,
	}
	ctx := context.Background()
	cache := newReconciliationTestCache(now, first, second)
	reconciler := newTestReconciler(cache, reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}},
		store, reconciliationTestAssignments{}, nil, time.Second, WithClock(func() time.Time { return now }), WithLegacyArrivalETAWriter(false))
	lifecycle := &reconciliationTestArrivalLifecycle{}
	reconciler.arrivalLifecycle = lifecycle

	require.NoError(t, reconciler.Reconcile(ctx))
	require.Equal(t, []string{second.Callsign}, lifecycle.processed)
	require.Empty(t, lifecycle.cancelled)
	strip, err := store.GetByCallsign(ctx, 7, second.Callsign)
	require.NoError(t, err)
	require.NotNil(t, strip.ArrivalETA)
	require.Equal(t, ETAFiled, strip.ArrivalETA.Source)
}
