package cdm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"

	"github.com/stretchr/testify/require"
)

func TestAtfcmCancellationDoesNotRestoreExportedCtot(t *testing.T) {
	flight := &models.CdmData{}
	service := newTestCdmService(NewClient(), &testutil.MockStripRepository{
		SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
			flight = data.Clone()
			return 1, nil
		},
		GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) {
			return flight.Clone(), nil
		},
	}, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	row := IFPSData{Callsign: "SAS123", CTOT: "124000", CDMData: CDMData{CTOT: "124000", Reason: "REG1"}}
	merge := func() bool {
		ctot, source := effectiveIfpsCtotAndSource(row, flight)
		updated, recalc, err := service.mergeMasterViffFlight(context.Background(), 1, row.Callsign, flight, row, ctot, source)
		require.NoError(t, err)
		flight = updated
		return recalc
	}
	require.True(t, merge())
	require.Equal(t, "1240", valueOrEmpty(flight.Ctot))
	require.Equal(t, models.CtotSourceATFCM, valueOrEmpty(flight.CtotSource))

	// The top-level slot is cancelled while setCdmData still contains its echo.
	row.CTOT = ""
	require.True(t, merge())
	require.Empty(t, valueOrEmpty(flight.Ctot))
	require.Empty(t, valueOrEmpty(flight.CtotSource))
	require.Empty(t, valueOrEmpty(flight.EcfmpID))

	// Provenance must survive persistence/restart and repeated airport syncs.
	encoded, err := json.Marshal(flight)
	require.NoError(t, err)
	flight = &models.CdmData{}
	require.NoError(t, json.Unmarshal(encoded, flight))
	merge()
	require.Empty(t, valueOrEmpty(flight.Ctot))

	// A distinct CDM event slot remains valid.
	row.CDMData.CTOT = "125000"
	require.True(t, merge())
	require.Equal(t, "1250", valueOrEmpty(flight.Ctot))
	require.Equal(t, models.CtotSourceEvent, valueOrEmpty(flight.CtotSource))

	// A new explicit ATFCM assignment may reuse the cancelled time.
	row.CTOT = "124000"
	require.True(t, merge())
	require.Equal(t, "1240", valueOrEmpty(flight.Ctot))
	require.Equal(t, models.CtotSourceATFCM, valueOrEmpty(flight.CtotSource))
}

func TestAtfcmCancellationRecognizesExistingFlightWithoutProvenance(t *testing.T) {
	flight := &models.CdmData{Ctot: testStringPtr("1240"), CtotSource: testStringPtr(models.CtotSourceATFCM)}
	ctot, source := effectiveIfpsCtotAndSource(IFPSData{CDMData: CDMData{CTOT: "124000"}}, flight)
	require.Empty(t, ctot)
	require.Empty(t, source)
}

func TestPushbackCtotReadIgnoresCancelledAtfcmEcho(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/ifps/callsign", r.URL.Path)
		_, _ = w.Write([]byte(`{"callsign":"SAS123","ctot":"","cdmData":{"ctot":"124000"}}`))
	}))
	defer server.Close()
	service := newTestCdmService(NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)), &testutil.MockStripRepository{
		GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) {
			return &models.CdmData{LastViffAtfcmCtot: "1240"}, nil
		},
	}, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	markSessionLive(service, 1)
	ctot, err := service.ReadPushbackCtot(context.Background(), 1, "SAS123")
	require.NoError(t, err)
	require.Empty(t, ctot)
}
