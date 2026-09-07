package postgres

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"testing"
)

type ownerWriteRecorder struct {
	arguments [][]any
}

func (r *ownerWriteRecorder) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	r.arguments = append(r.arguments, args)
	return pgconn.CommandTag{}, nil
}

func (*ownerWriteRecorder) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("unexpected Query call")
}

func (*ownerWriteRecorder) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("unexpected QueryRow call")
}

func (*ownerWriteRecorder) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	panic("unexpected CopyFrom call")
}

func TestMarshalOwners_NormalizesNilToEmptyJSONArray(t *testing.T) {
	owners, err := marshalOwners(nil)
	require.NoError(t, err)
	require.NotNil(t, owners)
	require.JSONEq(t, `[]`, string(owners))
}

func TestOwnerWritesNormalizeNilBeforeExecutingQuery(t *testing.T) {
	recorder := &ownerWriteRecorder{}
	repo := &stripRepository{queries: database.New(recorder)}
	ctx := context.Background()

	require.NoError(t, repo.SetRouteState(ctx, 7, "SAS123", nil, nil))
	require.NoError(t, repo.SetNextAndPreviousOwners(ctx, 7, "SAS123", nil, nil))
	require.NoError(t, repo.SetPreviousOwners(ctx, 7, "SAS123", nil))
	require.Len(t, recorder.arguments, 3)

	assertEmptyOwnersArgument(t, recorder.arguments[0][0])
	assertEmptyOwnersArgument(t, recorder.arguments[1][0])
	assertEmptyOwnersArgument(t, recorder.arguments[1][1])
	assertEmptyOwnersArgument(t, recorder.arguments[2][0])
}

func TestOwnerWritesPersistNilAsEmptyArrays(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	sessionID := testdata.SeedTestSessionNamedWithSectors(t, queries, "OWNER-WRITES", nil)
	testdata.SeedTestStrip(t, queries, sessionID, "SAS123")
	repo := NewStripRepository(pool)
	ctx := context.Background()

	require.NoError(t, repo.SetRouteState(ctx, sessionID, "SAS123", nil, nil))
	require.NoError(t, repo.SetNextAndPreviousOwners(ctx, sessionID, "SAS123", nil, nil))
	require.NoError(t, repo.SetPreviousOwners(ctx, sessionID, "SAS123", nil))

	strip, err := queries.GetStrip(ctx, database.GetStripParams{Session: sessionID, Callsign: "SAS123"})
	require.NoError(t, err)
	require.NotNil(t, strip.NextOwners)
	require.Empty(t, strip.NextOwners)
	require.NotNil(t, strip.PreviousOwners)
	require.Empty(t, strip.PreviousOwners)

	created := &models.Strip{
		Callsign:    "SAS456",
		Session:     sessionID,
		Origin:      "EKCH",
		Destination: "ESSA",
		Bay:         "NOT_CLEARED",
	}
	require.NoError(t, repo.Create(ctx, created))
	created, err = repo.GetByCallsign(ctx, sessionID, created.Callsign)
	require.NoError(t, err)
	require.NotNil(t, created.NextOwners)
	require.Empty(t, created.NextOwners)
	require.NotNil(t, created.PreviousOwners)
	require.Empty(t, created.PreviousOwners)

	created.NextOwners = nil
	created.PreviousOwners = nil
	rows, err := repo.Update(ctx, created)
	require.NoError(t, err)
	require.Equal(t, int64(1), rows)
	updated, err := repo.GetByCallsign(ctx, sessionID, created.Callsign)
	require.NoError(t, err)
	require.NotNil(t, updated.NextOwners)
	require.Empty(t, updated.NextOwners)
	require.NotNil(t, updated.PreviousOwners)
	require.Empty(t, updated.PreviousOwners)
}

func assertEmptyOwnersArgument(t *testing.T, argument any) {
	t.Helper()
	owners, ok := argument.([]byte)
	require.True(t, ok)
	require.NotNil(t, owners)
	require.JSONEq(t, `[]`, string(owners))
}

func TestStripToModel_MapsEmbeddedManualAndValidationFields(t *testing.T) {
	validationStatus := &models.ValidationStatus{
		IssueType:      "PDC INVALID",
		Message:        "Missing runway",
		OwningPosition: "EKCH_DEL",
		Active:         true,
		ActivationKey:  "abc-123",
	}
	rawValidationStatus, err := json.Marshal(validationStatus)
	require.NoError(t, err)

	personsOnBoard := int32(123)
	fplType := "I"
	language := "EN"
	nextDisplayLabel := "AD"
	nextDisplayFrequency := "121.730"

	strip, err := stripToModel(database.Strip{
		ID:                   42,
		Callsign:             "SAS123",
		Session:              7,
		Origin:               "EKCH",
		Destination:          "ESSA",
		Bay:                  "CLEARED",
		TrackingController:   "119.805",
		EngineType:           "JET",
		IsManual:             true,
		PersonsOnBoard:       &personsOnBoard,
		FplType:              &fplType,
		Language:             &language,
		HasFp:                true,
		ValidationStatus:     rawValidationStatus,
		NextDisplayLabel:     &nextDisplayLabel,
		NextDisplayFrequency: &nextDisplayFrequency,
	})
	require.NoError(t, err)

	require.True(t, strip.IsManual)
	require.Equal(t, &personsOnBoard, strip.PersonsOnBoard)
	require.Equal(t, &fplType, strip.FplType)
	require.Equal(t, &language, strip.Language)
	require.True(t, strip.HasFP)
	require.NotNil(t, strip.ValidationStatus)
	require.Equal(t, *validationStatus, *strip.ValidationStatus)
	require.Equal(t, &models.NextDisplay{Label: nextDisplayLabel, Frequency: nextDisplayFrequency}, strip.NextDisplay)
}

func TestStripToModel_MapsPersistedArrivalETAInputs(t *testing.T) {
	eta := models.ArrivalETA{Source: "LIVE", EOBT: "0800", EnrouteDuration: "0245"}
	distance := 34.5
	groundspeed := int32(130)
	eta.DistanceNM = &distance
	eta.Groundspeed = &groundspeed
	rawETA, err := json.Marshal(eta)
	require.NoError(t, err)

	strip, err := stripToModel(database.Strip{
		Callsign:    "SAS123",
		Session:     7,
		Origin:      "EGLL",
		Destination: "EKCH",
		ArrivalEta:  rawETA,
	})
	require.NoError(t, err)
	require.NotNil(t, strip.ArrivalETA)
	require.Equal(t, eta, *strip.ArrivalETA)
}
