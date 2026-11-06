package services

import (
	"FlightStrips/internal/airportworkflow"
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	es "FlightStrips/pkg/events/euroscope"
	pkg "FlightStrips/pkg/models"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRADISRemovalRecoveryAndAutomaticLifecycleUseDurableAirportState(t *testing.T) {
	pool, q := testdata.SetupTestDB(t)
	ctx := t.Context()
	registry := radisWorkflowFixtures(t)
	ids := map[string]int32{}
	for _, a := range []string{"EKBI", "EKYT"} {
		require.NoError(t, q.InsertAirport(ctx, a))
		id, err := q.InsertSession(ctx, database.InsertSessionParams{Airport: a, Name: "LIVE"})
		require.NoError(t, err)
		ids[a] = id
	}
	repo := postgres.NewStripRepository(pool)
	sessions := postgres.NewSessionRepository(pool)
	pub := &productionWorkflowPublisher{}
	makeService := func() *StripService {
		return NewStripService(repo, WithAirportWorkflows(registry), WithSessionReader(sessions), WithStripEventPublisher(pub), WithControllerReader(postgres.NewControllerRepository(pool)), WithSectorOwnerRepository(postgres.NewSectorOwnerRepository(pool)), WithCoordinationStore(postgres.NewCoordinationRepository(pool)), WithEuroscopeCommander(&testutil.MockEuroscopeHub{}))
	}
	s := makeService()
	now := time.Now().UTC()
	s.SetConnectedFlightSource(nil, time.Minute, func() time.Time { return now })
	event := es.Strip{Callsign: "RECOVER1", Origin: "EKBI", Destination: "EKYT", HasFp: true, GroundState: es.GroundStateTaxi, Position: &es.Position{Lat: 0, Lon: 0, Altitude: 100}}
	for a, id := range ids {
		require.NoError(t, s.SyncStrip(ctx, id, "MASTER", event, a))
	}
	session := ids["EKBI"]
	b, _ := registry.Lookup("EKBI")
	read := func() *models.Strip {
		strip, err := repo.GetByCallsign(ctx, session, event.Callsign)
		require.NoError(t, err)
		return strip
	}
	rev := func() pkg.StripRevision {
		strip := read()
		return pkg.StripRevision{StripID: strip.ID, Version: strip.Version, Definition: b.Definition().Revision()}
	}
	one := &testutil.MockCommandBinding{SessionID: session, CIDValue: "OWN"}
	peer := &testutil.MockCommandBinding{SessionID: session, CIDValue: "PEER"}
	oneCtx, err := shared.WithHumanCommand(ctx, one)
	require.NoError(t, err)
	peerCtx, err := shared.WithHumanCommand(ctx, peer)
	require.NoError(t, err)
	require.NoError(t, s.AnnotateStrip(oneCtx, session, event.Callsign, "OWN", "EKBI", "", rev(), airportworkflow.StripAnnotation{Field: "notes", Value: "shared recovery note"}))
	before := read()
	stale := rev()
	require.NoError(t, s.RemoveStripBoardEntry(oneCtx, session, event.Callsign, "OWN", "EKBI", "", rev()))
	removed := read()
	require.NotNil(t, removed.BoardState.RemovedAt)
	require.Equal(t, "manual", removed.BoardState.RemovalReason)
	// Reconciliation and another browser refresh must not undo manual removal.
	require.NoError(t, s.SyncStrip(ctx, session, "MASTER", event, "EKBI"))
	require.NoError(t, s.UpdateAircraftPosition(ctx, session, event.Callsign, 0, 0, 100, "EKBI"))
	require.Equal(t, "manual", read().BoardState.RemovalReason)
	other, err := repo.GetByCallsign(ctx, ids["EKYT"], event.Callsign)
	require.NoError(t, err)
	require.Nil(t, other.BoardState.RemovedAt)
	require.Error(t, s.RestoreStripBoardEntry(peerCtx, session, event.Callsign, "PEER", "EKBI", "", stale))
	one.Unavailable.Store(true)
	require.Error(t, s.RestoreStripBoardEntry(oneCtx, session, event.Callsign, "OWN", "EKBI", "", rev()))
	handledHidden, err := s.HandleRetainedAircraftDisconnect(ctx, session, event.Callsign)
	require.NoError(t, err)
	require.True(t, handledHidden)
	require.Nil(t, read().EuroscopeSeenAt, "disconnect clears association even after manual removal")
	restarted := makeService()
	restarted.SetConnectedFlightSource(nil, time.Minute, func() time.Time { return now })
	require.NoError(t, restarted.RestoreStripBoardEntry(peerCtx, session, event.Callsign, "PEER", "EKBI", "", rev()))
	restored := read()
	require.Equal(t, before.ID, restored.ID)
	require.Equal(t, before.Bay, restored.Bay)
	require.Equal(t, before.Sequence, restored.Sequence)
	require.Equal(t, "shared recovery note", restored.BoardState.Annotations.Notes)
	require.True(t, restored.HasFP)
	require.Nil(t, restored.BoardState.RemovedAt)
	// Position events use configured completion bounds; eligible return restores
	// only automatic removal, retaining the exact strip identity and annotations.
	require.NoError(t, s.UpdateAircraftPosition(ctx, session, event.Callsign, 0, 0, 3200, "EKBI"))
	require.Equal(t, "departure-altitude", read().BoardState.RemovalReason)
	now = now.Add(time.Second)
	require.NoError(t, s.UpdateAircraftPosition(ctx, session, event.Callsign, 0, 0, 100, "EKBI"))
	require.Nil(t, read().BoardState.RemovedAt)
	require.Equal(t, before.ID, read().ID)
	delay, override, err := s.AircraftDisconnectDelay(ctx, session)
	require.NoError(t, err)
	require.True(t, override)
	require.Equal(t, 7*time.Second, delay)
	// Both retained-source and ordinary disconnects preserve recoverable state.
	handled, err := s.HandleRetainedAircraftDisconnect(ctx, session, event.Callsign)
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, "disconnect", read().BoardState.RemovalReason)
	require.Nil(t, read().EuroscopeSeenAt)
	require.NoError(t, s.RestoreStripBoardEntry(peerCtx, session, event.Callsign, "PEER", "EKBI", "", rev()))
	require.Nil(t, read().EuroscopeSeenAt)
	require.NoError(t, s.DeleteStrip(ctx, session, event.Callsign))
	require.NotNil(t, read().BoardState.RemovedAt)
	now = now.Add(124 * time.Second)
	require.ErrorContains(t, s.RestoreStripBoardEntry(peerCtx, session, event.Callsign, "PEER", "EKBI", "", rev()), "expired")
	require.NoError(t, s.SyncStrip(ctx, session, "MASTER", event, "EKBI"))
	require.NotNil(t, read().BoardState.RemovedAt, "expired recovery is not bypassed by source refresh")
	require.Equal(t, before.ID, read().ID)
	require.Equal(t, "shared recovery note", read().BoardState.Annotations.Notes)
	require.NoError(t, repo.Create(ctx, &models.Strip{Callsign: "FINDONLY", Session: session, Origin: "EKBI", Destination: "EKYT", Bay: "PASSIVE_TOP", HasFP: false, BoardState: &models.StripBoardState{}}))
	handled, err = s.HandleRetainedAircraftDisconnect(ctx, session, "FINDONLY")
	require.NoError(t, err)
	require.True(t, handled)
	require.NoError(t, s.DeleteStrip(ctx, session, "FINDONLY"))
	found, err := repo.GetByCallsign(ctx, session, "FINDONLY")
	require.NoError(t, err)
	require.Nil(t, found.BoardState.RemovedAt)
	require.Empty(t, one.Sent())
	require.Empty(t, peer.Sent(), "remove/restore never creates, deletes or replaces an ES FPL")
}
