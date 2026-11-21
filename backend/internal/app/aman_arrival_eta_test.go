package app

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/compatibility"
	"FlightStrips/internal/models"
	"github.com/stretchr/testify/require"
)

type fakeArrivalETAStore struct {
	strips  []*models.Strip
	updates map[string]models.ArrivalETA
	clears  []string
}

func (s *fakeArrivalETAStore) List(context.Context, int32) ([]*models.Strip, error) {
	return s.strips, nil
}

func (s *fakeArrivalETAStore) UpdateArrivalETA(_ context.Context, _ int32, callsign string, eta models.ArrivalETA) (int64, error) {
	if s.updates == nil {
		s.updates = map[string]models.ArrivalETA{}
	}
	s.updates[callsign] = eta
	for _, strip := range s.strips {
		if strip.Callsign == callsign {
			stored := eta
			strip.ArrivalETA = &stored
		}
	}
	return 1, nil
}

func (s *fakeArrivalETAStore) ClearArrivalETA(_ context.Context, _ int32, callsign string) (int64, error) {
	s.clears = append(s.clears, callsign)
	return 1, nil
}

func amanETAPrediction(teta, generated time.Time) *aman.Prediction {
	return &aman.Prediction{OperationalTETA: teta, GeneratedAt: generated, Publishable: true}
}

func TestAMANArrivalETAProjectionWritesMissingAndChangedETAs(t *testing.T) {
	now := time.Date(2026, time.November, 19, 12, 0, 0, 0, time.UTC)
	teta := now.Add(40 * time.Minute)
	seen := now
	store := &fakeArrivalETAStore{strips: []*models.Strip{
		{Callsign: "SAS123", EuroscopeSeenAt: &seen},
		{Callsign: "DLH4", ArrivalETA: &models.ArrivalETA{Time: teta, Source: compatibility.ArrivalETASource, CalculatedAt: now.Add(-time.Minute)}},
		{Callsign: "BAW9", ArrivalETA: &models.ArrivalETA{Time: teta, Source: "LIVE"}},
	}}
	var notified []string
	transport := &amanTransport{now: func() time.Time { return now }}
	transport.setArrivalETAProjection(newAMANArrivalETAProjection(aman.ModeAuthoritative, store, func(_ int32, callsign string) {
		notified = append(notified, callsign)
	}))

	transport.projectArrivalETAs(context.Background(), aman.AirportState{SessionID: 7, Airport: "EKCH", Flights: []aman.AMANFlight{
		{Callsign: "SAS123", Prediction: amanETAPrediction(teta, now)},
		// Same time and source with a newer GeneratedAt must not rewrite.
		{Callsign: "DLH4", Prediction: amanETAPrediction(teta, now)},
		// A legacy ETA is replaced by the AMAN projection.
		{Callsign: "BAW9", Prediction: amanETAPrediction(teta, now)},
		// Flights without a strip are ignored.
		{Callsign: "KLM1", Prediction: amanETAPrediction(teta, now)},
	}}, aman.TechnicalHealth{AuthorityAllowed: true})

	require.Len(t, store.updates, 2)
	require.True(t, store.updates["SAS123"].Time.Equal(teta))
	require.Equal(t, compatibility.ArrivalETASource, store.updates["SAS123"].Source)
	require.Contains(t, store.updates, "BAW9")
	require.Equal(t, []string{"SAS123"}, notified)
	require.Empty(t, store.clears)
}

func TestAMANArrivalETAProjectionLeavesFlightsWithoutPredictionToFallback(t *testing.T) {
	now := time.Date(2026, time.November, 19, 12, 0, 0, 0, time.UTC)
	teta := now.Add(20 * time.Minute)
	store := &fakeArrivalETAStore{strips: []*models.Strip{
		{Callsign: "SAS1", ArrivalETA: &models.ArrivalETA{Time: teta, Source: compatibility.ArrivalETASource, CalculatedAt: now.Add(-time.Hour)}},
		{Callsign: "SAS2", ArrivalETA: &models.ArrivalETA{Time: teta, Source: "LIVE", CalculatedAt: now}},
	}}
	projection := newAMANArrivalETAProjection(aman.ModeAuthoritative, store, nil)

	projection.project(context.Background(), aman.AirportState{SessionID: 7, Flights: []aman.AMANFlight{
		{Callsign: "SAS1", Prediction: amanETAPrediction(teta, now.Add(-compatibility.FreshFor-time.Minute))},
		{Callsign: "SAS2"},
	}}, aman.TechnicalHealth{AuthorityAllowed: true}, now)

	require.Empty(t, store.clears, "a missing AMAN ETA must not erase the VATSIM fallback")
	require.Empty(t, store.updates)
}

func TestAMANArrivalETAProjectionRefreshesUnchangedETABeforeItGoesStale(t *testing.T) {
	now := time.Date(2026, time.November, 19, 12, 0, 0, 0, time.UTC)
	teta := now.Add(20 * time.Minute)
	store := &fakeArrivalETAStore{strips: []*models.Strip{
		{Callsign: "SAS1", ArrivalETA: &models.ArrivalETA{Time: teta, Source: compatibility.ArrivalETASource, CalculatedAt: now.Add(-amanArrivalETARefreshAfter - time.Second)}},
	}}
	projection := newAMANArrivalETAProjection(aman.ModeAuthoritative, store, nil)

	projection.project(context.Background(), aman.AirportState{SessionID: 7, Flights: []aman.AMANFlight{
		{Callsign: "SAS1", Prediction: amanETAPrediction(teta, now)},
	}}, aman.TechnicalHealth{AuthorityAllowed: true}, now)

	require.Contains(t, store.updates, "SAS1")
	require.True(t, compatibility.IsFresh(&models.ArrivalETA{Time: store.updates["SAS1"].Time, Source: store.updates["SAS1"].Source, CalculatedAt: store.updates["SAS1"].CalculatedAt}, now))
}

func TestAMANArrivalETAProjectionRejectsOlderPublications(t *testing.T) {
	now := time.Date(2026, time.November, 19, 12, 0, 0, 0, time.UTC)
	newer, older := now.Add(30*time.Minute), now.Add(10*time.Minute)
	store := &fakeArrivalETAStore{strips: []*models.Strip{{Callsign: "SAS1"}}}
	projection := newAMANArrivalETAProjection(aman.ModeAuthoritative, store, nil)
	healthy := aman.TechnicalHealth{AuthorityAllowed: true}

	projection.project(context.Background(), aman.AirportState{SessionID: 7, Revision: 5, Flights: []aman.AMANFlight{
		{Callsign: "SAS1", Prediction: amanETAPrediction(newer, now)},
	}}, healthy, now)
	require.True(t, store.strips[0].ArrivalETA.Time.Equal(newer))

	// An older revision resuming after the newer one has published.
	projection.project(context.Background(), aman.AirportState{SessionID: 7, Revision: 4, Flights: []aman.AMANFlight{
		{Callsign: "SAS1", Prediction: amanETAPrediction(older, now.Add(-time.Minute))},
	}}, healthy, now)
	require.True(t, store.strips[0].ArrivalETA.Time.Equal(newer), "an older revision must not overwrite the newer ETA")

	// The same revision republished with an older prediction is also ignored.
	projection.project(context.Background(), aman.AirportState{SessionID: 7, Revision: 5, Flights: []aman.AMANFlight{
		{Callsign: "SAS1", Prediction: amanETAPrediction(older, now.Add(-time.Minute))},
	}}, healthy, now)
	require.True(t, store.strips[0].ArrivalETA.Time.Equal(newer))
}

func TestAMANArrivalETAProjectionDoesNotWriteWhenAuthorityIsBlocked(t *testing.T) {
	now := time.Date(2026, time.November, 19, 12, 0, 0, 0, time.UTC)
	store := &fakeArrivalETAStore{strips: []*models.Strip{{Callsign: "SAS1"}}}
	projection := newAMANArrivalETAProjection(aman.ModeAuthoritative, store, nil)

	projection.project(context.Background(), aman.AirportState{SessionID: 7, Flights: []aman.AMANFlight{
		{Callsign: "SAS1", Prediction: amanETAPrediction(now.Add(time.Hour), now)},
	}}, aman.TechnicalHealth{AuthorityAllowed: false}, now)

	require.Empty(t, store.updates)
	require.Empty(t, store.clears)
}
