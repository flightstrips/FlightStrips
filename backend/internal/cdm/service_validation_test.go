package cdm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type spyValidationReevaluator struct {
	callsigns   []string
	sessions    []int32
	publish     []bool
	reactivate  []bool
	sessionOnly []int32
}

func (s *spyValidationReevaluator) ReevaluateCtotValidation(_ context.Context, session int32, callsign string, publish bool, forceReactivate bool) error {
	s.sessions = append(s.sessions, session)
	s.callsigns = append(s.callsigns, callsign)
	s.publish = append(s.publish, publish)
	s.reactivate = append(s.reactivate, forceReactivate)
	return nil
}

func (s *spyValidationReevaluator) ReevaluateCtotValidationsForSession(_ context.Context, session int32, publish bool) error {
	s.sessionOnly = append(s.sessionOnly, session)
	s.publish = append(s.publish, publish)
	return nil
}

func TestHandleManualCtot_ReevaluatesCtotValidation(t *testing.T) {
	const sessionID = int32(43)
	const callsign = "SAS321"

	stored := (&models.CdmData{}).Normalize()
	reevaluator := &spyValidationReevaluator{}
	service := NewCdmService(
		newTestClientWithAirportMasters(nil),
		&testutil.MockStripRepository{
			GetByCallsignFn: func(_ context.Context, session int32, cs string) (*models.Strip, error) {
				assert.Equal(t, sessionID, session)
				assert.Equal(t, callsign, cs)
				return &models.Strip{Callsign: cs, Origin: "EKCH"}, nil
			},
			GetCdmDataForCallsignFn: func(_ context.Context, session int32, cs string) (*models.CdmData, error) {
				assert.Equal(t, sessionID, session)
				assert.Equal(t, callsign, cs)
				return stored.Clone(), nil
			},
			SetCdmDataFn: func(_ context.Context, session int32, cs string, data *models.CdmData) (int64, error) {
				assert.Equal(t, sessionID, session)
				assert.Equal(t, callsign, cs)
				stored = data.Clone()
				return 1, nil
			},
		},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)
	service.SetValidationReevaluator(reevaluator)

	err := service.HandleManualCtot(context.Background(), sessionID, callsign, "1045")
	require.NoError(t, err)
	require.Equal(t, []string{callsign}, reevaluator.callsigns)
	assert.Equal(t, []int32{sessionID}, reevaluator.sessions)
	assert.Equal(t, []bool{true}, reevaluator.publish)
	assert.Equal(t, []bool{false}, reevaluator.reactivate)
}

func TestSyncCdmData_ReevaluatesCtotValidationWhenSyncedCtotChanges(t *testing.T) {
	const sessionID = int32(80)
	const callsign = "SAS126"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ifps/depAirport" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{
			"callsign":"SAS126",
			"departure":"EKCH",
			"eobt":"1000",
			"tobt":"1010",
			"ctot":"1040",
			"cdmSts":"REA",
			"cdmData":{"tsat":"101500","ttot":"102500"}
		}]`))
	}))
	defer server.Close()

	var persisted *models.CdmData
	reevaluator := &spyValidationReevaluator{}
	service := NewCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetCdmDataFn: func(context.Context, int32) ([]*models.CdmDataRow, error) {
				return []*models.CdmDataRow{{Callsign: callsign, Data: (&models.CdmData{}).Normalize()}}, nil
			},
			SetCdmDataFn: func(_ context.Context, session int32, gotCallsign string, data *models.CdmData) (int64, error) {
				assert.Equal(t, sessionID, session)
				assert.Equal(t, callsign, gotCallsign)
				persisted = data.Clone()
				return 1, nil
			},
			GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) {
				return persisted.Clone(), nil
			},
		},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)
	service.SetValidationReevaluator(reevaluator)

	err := service.syncCdmData(context.Background(), &models.Session{ID: sessionID, Airport: "EKCH"})
	require.NoError(t, err)
	require.Equal(t, []string{callsign}, reevaluator.callsigns)
	assert.Equal(t, []int32{sessionID}, reevaluator.sessions)
}

func TestSchedulePeriodicCtotValidationReevaluation_ReevaluatesAllSessions(t *testing.T) {
	service := NewCdmService(
		newTestClientWithAirportMasters(nil),
		&testutil.MockStripRepository{},
		&testutil.MockSessionRepository{
			ListFn: func(context.Context) ([]*models.Session, error) {
				return []*models.Session{
					{ID: 1, Airport: "EKCH"},
					{ID: 2, Airport: "ESSA"},
				}, nil
			},
		},
		&testutil.MockControllerRepository{},
	)
	reevaluator := &spyValidationReevaluator{}
	service.SetValidationReevaluator(reevaluator)

	require.NoError(t, service.schedulePeriodicCtotValidationReevaluation(context.Background()))
	assert.Equal(t, []int32{1, 2}, reevaluator.sessionOnly)
	assert.Equal(t, []bool{true, true}, reevaluator.publish)
}
