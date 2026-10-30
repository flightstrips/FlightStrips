package pdc

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// These tests are deliberately sequential because they replace the default logger.
func capturePdcLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &logs
}

func findPdcLog(t *testing.T, logs *bytes.Buffer, message string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		if record["msg"] == message {
			return record
		}
	}
	t.Fatalf("missing log %q in %s", message, logs.String())
	return nil
}

type loggingSessionRepository struct{ testutil.MockSessionRepository }

func (*loggingSessionRepository) IncrementPdcMessageSequence(context.Context, int32) (int32, error) {
	return 7, nil
}

func TestPdcRejectionLogsFlightPlanAndRequestedIdentity(t *testing.T) {
	for _, cleared := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing flight plan", true: "already cleared"}[cleared], func(t *testing.T) {
			logs := capturePdcLogs(t)
			client := new(mockHoppieClient)
			client.On("SendCPDLC", mock.Anything, "EKCH", "SAS1TB", mock.Anything).Return(nil)
			service := &Service{
				client:      &hoppieClientAdapter{HoppieClient: client},
				sessionRepo: &loggingSessionRepository{},
				stripRepo: &testutil.MockStripRepository{GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
					return &models.Strip{Origin: "NOFP", Destination: "NOFP", Cleared: cleared}, nil
				}},
			}
			err := service.ProcessPDCRequest(context.Background(), &IncomingMessage{
				From: "SAS1TB", Payload: "REQUEST PREDEP CLEARANCE SAS1TB A321 TO ESSA AT EKCH STAND B10 ATIS D",
			}, sessionInformation{id: 476, airport: "EKCH", callsign: "EKCH"})
			require.Error(t, err)
			record := findPdcLog(t, logs, "PDC Service: Request processing failed")
			require.Equal(t, "SAS1TB", record["callsign"])
			require.Equal(t, "CPDLC", record["channel"])
			require.Equal(t, float64(476), record["session"])
			require.Equal(t, "SAS1TB", record["from"])
			require.Equal(t, "EKCH", record["requested_origin"])
			require.Equal(t, "ESSA", record["requested_destination"])
			require.Equal(t, "NOFP", record["expected_origin"])
			require.Equal(t, "NOFP", record["expected_destination"])
			require.Equal(t, false, record["has_fp"])
			require.Equal(t, cleared, record["cleared"])
			client.AssertExpectations(t)
		})
	}
}

func TestWebPdcLogsPendingFaults(t *testing.T) {
	logs := capturePdcLogs(t)
	strip := &models.Strip{Callsign: "NSZ3210", Origin: "EKCH", Destination: "ESSA", AircraftType: stringPtr("A320")}
	session := &models.Session{ID: 476, Name: "LIVE", Airport: "EKCH"}
	sessions := &testutil.MockSessionRepository{
		ListFn:    func(context.Context) ([]*models.Session, error) { return []*models.Session{session}, nil },
		GetByIDFn: func(context.Context, int32) (*models.Session, error) { return session, nil },
	}
	strips := &testutil.MockStripRepository{
		GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return strip, nil },
		SetPdcDataFn: func(_ context.Context, _ int32, _ string, data *models.PdcData) error {
			strip.PdcData = data
			return nil
		},
		SetPdcRequestedFn: func(_ context.Context, _ int32, _ string, state string, _ *time.Time, _ *string) error {
			strip.PdcState = state
			return nil
		},
	}
	frontend := new(mockPdcFrontendHub)
	frontend.On("SendPdcStateChange", int32(476), "NSZ3210", string(StateRequestedWithFaults), "").Return()
	stripService := new(mockPdcStripService)
	stripService.On("ReevaluatePdcRequestValidations", mock.Anything, int32(476), "NSZ3210", true, true).Return(nil)
	service := &Service{sessionRepo: sessions, stripRepo: strips, frontendHub: frontend, euroscopeHub: testPdcEuroscope{}, stripService: stripService}
	require.NoError(t, service.SubmitWebPDCRequest(context.Background(), "nsz3210", "D", "A4", "", "A320"))
	findPdcLog(t, logs, "PDC Service: Request received")
	record := findPdcLog(t, logs, "PDC Service: Request pending manual review")
	require.Equal(t, "NSZ3210", record["callsign"])
	require.Equal(t, "WEB", record["channel"])
	require.Equal(t, float64(476), record["session"])
	require.Equal(t, "requested_with_faults", record["outcome"])
	require.Equal(t, "REQUESTED_WITH_FAULTS", record["pending_state"])
	require.Contains(t, record["faults"], "No valid assigned squawk")
	frontend.AssertExpectations(t)
	stripService.AssertExpectations(t)
}

func TestWebPdcLogsInvalidSubmission(t *testing.T) {
	logs := capturePdcLogs(t)
	err := (&Service{}).SubmitWebPDCRequest(context.Background(), "nsz3210", "INVALID", "A4", "", "A320")
	require.ErrorIs(t, err, ErrWebInvalidAtis)
	record := findPdcLog(t, logs, "PDC Service: Request processing failed")
	require.Equal(t, "NSZ3210", record["callsign"])
	require.Equal(t, "WEB", record["channel"])
	require.Equal(t, ErrWebInvalidAtis.Error(), record["error"])
}

func TestHoppieLogsAcceptanceAndFailureWithoutCredentials(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		t.Run(map[bool]string{false: "rejected", true: "accepted"}[accepted], func(t *testing.T) {
			logs := capturePdcLogs(t)
			client := NewClient("secret-logon")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if accepted {
					_, _ = io.WriteString(w, "ok")
				} else {
					_, _ = io.WriteString(w, "error {invalid logon secret-logon}")
				}
			}))
			t.Cleanup(server.Close)
			client.baseURL = server.URL
			err := client.SendCPDLC(context.Background(), "EKCH", "SAS123", "/data2/7//WU/clearance")
			var record map[string]any
			if accepted {
				require.NoError(t, err)
				record = findPdcLog(t, logs, "PDC Hoppie: Message accepted")
				require.Equal(t, "accepted", record["outcome"])
			} else {
				require.Error(t, err)
				record = findPdcLog(t, logs, "PDC Hoppie: Message send failed")
				require.Equal(t, "failed", record["outcome"])
				require.NotContains(t, logs.String(), client.logon)
			}
			require.Equal(t, "EKCH", record["from"])
			require.Equal(t, "SAS123", record["to"])
			require.Equal(t, "7", record["message_sequence"])
		})
	}
}
