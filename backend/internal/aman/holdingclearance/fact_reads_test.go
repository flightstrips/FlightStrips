package holdingclearance

import (
	"FlightStrips/internal/aman"
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type factReadRepository struct {
	*memoryRepository
	reads int
}

func (r *factReadRepository) LoadAirportState(ctx context.Context, airport string) (aman.AirportState, error) {
	r.reads++
	return r.memoryRepository.LoadAirportState(ctx, airport)
}
func (r *factReadRepository) LoadHoldingFactSnapshots(context.Context, string, []aman.Callsign) ([]aman.HoldingFactSnapshot, error) {
	f := r.state.Flights[0]
	return []aman.HoldingFactSnapshot{{Callsign: f.Callsign, Clearance: f.HoldingClearance}}, nil
}

func TestHoldingFactPreflightOnlySkipsUnchangedAggregate(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	repo := &factReadRepository{memoryRepository: &memoryRepository{state: airportState(now)}}
	pub := &publisher{}
	svc, err := New(Dependencies{Repository: repo, Publisher: pub})
	require.NoError(t, err)
	fact := aman.HoldingClearanceFact{Callsign: repo.state.Flights[0].Callsign, Destination: repo.state.Airport, Hold: "OLPIB", HoldType: aman.HoldingClearanceEnroute, ObservedAt: now}
	require.NoError(t, svc.ObserveHoldingClearance(context.Background(), fact))
	require.Equal(t, 1, repo.reads)
	require.Equal(t, 1, repo.commits)
	fact.ObservedAt = now.Add(time.Second)
	require.NoError(t, svc.ObserveHoldingClearance(context.Background(), fact))
	require.Equal(t, 1, repo.reads, "unchanged fact must not load the aggregate")
	fact.Hold = ""
	require.NoError(t, svc.ObserveHoldingClearance(context.Background(), fact))
	require.Equal(t, 2, repo.reads, "clearing the hold still uses revision-checked aggregate persistence")
	require.Equal(t, 2, repo.commits)
	require.Equal(t, 2, pub.calls)
	fact.Hold = "OLPIB"
	fact.ObservedAt = now
	require.NoError(t, svc.ObserveHoldingClearance(context.Background(), fact))
	require.Equal(t, 2, repo.reads, "stale fact cannot overwrite the clearance")
}
