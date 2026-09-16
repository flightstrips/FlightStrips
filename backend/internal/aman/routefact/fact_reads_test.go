package routefact

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/models"
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type factIdentityRepository struct {
	*memoryRepository
	loads   int
	missing bool
}

func (r *factIdentityRepository) LoadAirportState(ctx context.Context, airport string) (aman.AirportState, error) {
	r.loads++
	return r.memoryRepository.LoadAirportState(ctx, airport)
}
func (r *factIdentityRepository) FindActiveFactFlight(context.Context, string, string) (aman.FlightID, error) {
	if r.missing {
		return "", &aman.DomainError{Class: aman.ErrorNotFound}
	}
	return "flight-1", nil
}
func TestRouteFactNarrowReadAvoidsAggregateForSpeedAndMissingFlight(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	repo := &factIdentityRepository{memoryRepository: &memoryRepository{}}
	correlator := &correlator{}
	svc, err := New(Dependencies{Repository: repo, Strips: &stripReader{strip: &models.Strip{Session: 42, Callsign: "SAS123", TrackingController: "EKCH_A_APP"}}, Geometry: geometry(now), Publisher: &publisher{}, Reconciler: &reconciler{}, Correlator: correlator, Now: func() time.Time { return now }})
	require.NoError(t, err)
	require.NoError(t, svc.ReportSpeed(context.Background(), 42, "EKCH", "SAS123", "EKCH_A_APP", "220 KT", now))
	require.EqualValues(t, "flight-1", correlator.fact.FlightID)
	require.Zero(t, repo.loads)
	repo.missing = true
	fix := "KEMAX"
	require.Error(t, svc.ReportDirectTo(context.Background(), 42, "EKCH", "SAS123", "EKCH_A_APP", &fix, now))
	require.Zero(t, repo.loads)
	require.Zero(t, repo.commits)
}
