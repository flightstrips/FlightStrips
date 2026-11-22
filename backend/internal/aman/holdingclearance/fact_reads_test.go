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

func TestUnchangedFullSyncRetainsRevisionWithoutReplanning(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	repo := &factReadRepository{memoryRepository: &memoryRepository{state: airportState(now)}}
	pub := &publisher{}
	planner := &recordingPlanner{}
	svc, err := New(Dependencies{Repository: repo, Publisher: pub, Planner: planner})
	require.NoError(t, err)
	level := int32(9000)
	initial := aman.HoldingClearanceFact{
		Callsign: repo.state.Flights[0].Callsign, Destination: repo.state.Airport,
		Hold: "OLPIB", HoldType: aman.HoldingClearanceEnroute, ClearedAltitude: &level,
		StripRevision: aman.StripRevision{ID: 42, Version: 18}, ObservedAt: now,
	}
	require.NoError(t, svc.ObserveHoldingClearance(t.Context(), initial))
	newer := initial
	newer.StripRevision.Version = 20
	newer.ObservedAt = now.Add(time.Second)
	require.NoError(t, svc.ObserveHoldingClearance(t.Context(), newer))
	require.Equal(t, 2, repo.commits)
	require.Len(t, planner.batches, 1)
	require.Equal(t, now, repo.state.Flights[0].HoldingClearance.ObservedAt)
	require.Equal(t, newer.StripRevision, repo.state.Flights[0].HoldingClearance.StripRevision)

	previousLevel := int32(7000)
	delayed := initial
	delayed.ClearedAltitude = &previousLevel
	delayed.StripRevision.Version = 19
	delayed.ObservedAt = now.Add(2 * time.Second)
	require.NoError(t, svc.ObserveHoldingClearance(t.Context(), delayed))
	require.Equal(t, 2, repo.reads)
	require.Equal(t, 2, pub.calls)
	require.Equal(t, &level, repo.state.Flights[0].HoldingClearance.ClearedAltitude)
}

func TestHoldingFactPreflightKeepsStripVersionAcrossUnversionedSync(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	repo := &factReadRepository{memoryRepository: &memoryRepository{state: airportState(now)}}
	pub := &publisher{}
	svc, err := New(Dependencies{Repository: repo, Publisher: pub})
	require.NoError(t, err)
	level := int32(9000)
	fact := aman.HoldingClearanceFact{
		Callsign: repo.state.Flights[0].Callsign, Destination: repo.state.Airport,
		Hold: "OLPIB", HoldType: aman.HoldingClearanceEnroute, ClearedAltitude: &level, ObservedAt: now,
		StripRevision: aman.StripRevision{ID: 42, Version: 17}, DedicatedClearedAltitude: true,
	}
	require.NoError(t, svc.ObserveHoldingClearance(t.Context(), fact))
	sync := fact
	sync.DedicatedClearedAltitude = false
	sync.StripRevision = aman.StripRevision{}
	sync.HoldEAT = "1205"
	sync.ObservedAt = now.Add(time.Second)
	require.NoError(t, svc.ObserveHoldingClearance(t.Context(), sync))
	require.Equal(t, 2, repo.reads)

	stale := fact
	stale.StripRevision.Version--
	stale.ClearedAltitude = nil
	stale.ObservedAt = now.Add(2 * time.Second)
	require.NoError(t, svc.ObserveHoldingClearance(t.Context(), stale))
	require.Equal(t, 2, repo.reads, "the preflight rejects the delayed update after an unversioned sync")
	require.Equal(t, 2, pub.calls)
	require.Equal(t, "1205", repo.state.Flights[0].HoldingClearance.HoldEAT)

	replacement := stale
	replacement.StripRevision = aman.StripRevision{ID: 43, Version: 1}
	require.NoError(t, svc.ObserveHoldingClearance(t.Context(), replacement))
	require.Equal(t, 3, pub.calls)
	require.Equal(t, replacement.StripRevision, repo.state.Flights[0].HoldingClearance.StripRevision)

	newer := replacement
	newer.StripRevision.Version++
	newer.ClearedAltitude = &level
	newer.ObservedAt = now.Add(time.Second)
	require.NoError(t, svc.ObserveHoldingClearance(t.Context(), newer))
	require.Equal(t, 4, pub.calls, "a higher persisted version wins even with an earlier timestamp")
	require.Equal(t, newer.StripRevision, repo.state.Flights[0].HoldingClearance.StripRevision)
}
