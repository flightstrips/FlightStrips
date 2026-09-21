package postgres

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestStripPublicationSnapshot(t *testing.T) {
	pool, q := testdata.SetupTestDB(t)
	session := testdata.SeedTestSessionNamedWithSectors(t, q, "PUBLICATION", []database.InsertSectorOwnersParams{{Sector: []string{"GND"}, Position: "121.900", Identifier: "G"}})
	otherSession := testdata.SeedTestSessionNamedWithSectors(t, q, "OTHER_PUBLICATION", nil)
	ctx := context.Background()
	for _, name := range []string{"PUB1", "PUB2"} {
		testdata.SeedTestStrip(t, q, session, name)
	}
	testdata.SeedTestStrip(t, q, otherSession, "PUB1")
	_, err := pool.Exec(ctx, `INSERT INTO controllers (session, callsign, position, cid, last_seen_euroscope, observer) VALUES ($1, 'EKCH_GND', '121.900', '123', NOW(), false), ($1, 'EKCH_APP', '119.800', '456', NOW(), false)`, session)
	require.NoError(t, err)
	r := NewStripRepository(pool)
	strip, err := r.GetByCallsign(ctx, session, "PUB1")
	require.NoError(t, err)
	_, err = q.CreateCoordination(ctx, database.CreateCoordinationParams{Session: session, StripID: strip.ID, FromPosition: "121.900", ToPosition: "118.100"})
	require.NoError(t, err)
	now := time.Now().UTC().Truncate(time.Microsecond)
	for _, name := range []string{"PUB1", "PUB2"} {
		require.NoError(t, NewStandAssignmentRepository(pool).CreateAssignment(ctx, &models.StandAssignment{SessionID: session, Callsign: name, Stand: "A1", Direction: "DEPARTURE", Stage: "RESERVED", Source: "automatic", Version: 1, AssignedAt: &now}))
	}
	counter := &positionQueryCounter{}
	cfg := pool.Config()
	cfg.ConnConfig.Tracer = counter
	traced, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	defer traced.Close()
	snapshot, err := NewStripRepository(traced).GetStripPublicationSnapshot(ctx, session, "PUB1")
	require.NoError(t, err)
	require.Equal(t, int32(1), counter.n.Load(), "all publication inputs use one statement")
	require.Equal(t, strip, snapshot.Strip)
	wantSession, err := NewSessionRepository(pool).GetByID(ctx, session)
	require.NoError(t, err)
	require.Equal(t, wantSession, snapshot.Session)
	wantControllers, err := NewControllerRepository(pool).ListBySession(ctx, session)
	require.NoError(t, err)
	require.Equal(t, wantControllers, snapshot.Controllers, "preserve controller ordering when frequencies overlap")
	wantOwners, err := NewSectorOwnerRepository(pool).ListBySession(ctx, session)
	require.NoError(t, err)
	require.ElementsMatch(t, wantOwners, snapshot.SectorOwners)
	wantAssignments, err := NewStandAssignmentRepository(pool).ListAssignments(ctx, session)
	require.NoError(t, err)
	for _, assignment := range wantAssignments {
		assignment.CreatedAt = assignment.CreatedAt.UTC()
		assignment.UpdatedAt = assignment.UpdatedAt.UTC()
		at := assignment.AssignedAt.UTC()
		assignment.AssignedAt = &at
	}
	wantJSON, err := json.Marshal(wantAssignments)
	require.NoError(t, err)
	gotJSON, err := json.Marshal(snapshot.Assignments)
	require.NoError(t, err)
	require.JSONEq(t, string(wantJSON), string(gotJSON))
	require.True(t, snapshot.CoordinationPending)
	other, err := r.GetStripPublicationSnapshot(ctx, otherSession, "PUB1")
	require.NoError(t, err)
	require.Empty(t, other.Controllers)
	require.Empty(t, other.Assignments)
	require.False(t, other.CoordinationPending)
	_, err = r.GetStripPublicationSnapshot(ctx, session, "MISSING")
	require.ErrorIs(t, err, pgx.ErrNoRows)
	// A transaction-bound repository must see its own uncommitted field changes.
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)
	heading := int32(123)
	bound := r.WithTx(tx).(*stripRepository)
	_, err = bound.UpdateHeading(ctx, session, "PUB1", &heading, nil)
	require.NoError(t, err)
	updated, err := bound.GetStripPublicationSnapshot(ctx, session, "PUB1")
	require.NoError(t, err)
	require.Equal(t, &heading, updated.Strip.Heading)
	require.Equal(t, strip.Version+1, updated.Strip.Version)
}

func TestUpdateHeadingAndGetStrip(t *testing.T) {
	pool, q := testdata.SetupTestDB(t)
	session := testdata.SeedTestSessionNamedWithSectors(t, q, "HEADING_PUBLICATION", nil)
	testdata.SeedTestStrip(t, q, session, "PUB1")
	before, err := NewStripRepository(pool).GetByCallsign(context.Background(), session, "PUB1")
	require.NoError(t, err)

	counter := &positionQueryCounter{}
	cfg := pool.Config()
	cfg.ConnConfig.Tracer = counter
	traced, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	defer traced.Close()

	repo := NewStripRepository(traced)
	heading := int32(275)
	strip, count, err := repo.UpdateHeadingAndGetStrip(context.Background(), session, "PUB1", &heading, nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
	require.Equal(t, int32(1), counter.n.Load(), "heading persistence returns the updated strip in one statement")
	require.Equal(t, &heading, strip.Heading)
	require.Equal(t, before.Version+1, strip.Version)

	strip, count, err = repo.UpdateHeadingAndGetStrip(context.Background(), session, "MISSING", &heading, nil)
	require.NoError(t, err)
	require.Zero(t, count)
	require.Nil(t, strip)
}
