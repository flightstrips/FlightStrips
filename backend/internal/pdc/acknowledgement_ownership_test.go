package pdc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestAcknowledgementOwnership(t *testing.T) {
	for _, channel := range []string{"CPDLC", "WEB"} {
		for _, tc := range []struct {
			name         string
			cleared      bool
			owner        *string
			assumeMethod string
		}{
			{name: "uncleared departure", assumeMethod: "AutoAssumeForClearedStripByCid"},
			{name: "already cleared and unowned", cleared: true, assumeMethod: "AutoAssumeUnownedClearedStripByCid"},
			{name: "already cleared with empty owner", cleared: true, owner: stringPtr(""), assumeMethod: "AutoAssumeUnownedClearedStripByCid"},
			{name: "already transferred", cleared: true, owner: stringPtr("121.730")},
		} {
			t.Run(channel+"/"+tc.name, func(t *testing.T) {
				sequence := int32(7)
				strip := &models.Strip{
					Callsign: "SAS123", Bay: shared.BAY_CLEARED, Cleared: tc.cleared,
					Owner: tc.owner, PdcState: string(StateCleared), PdcMessageSequence: &sequence,
					PdcData: &models.PdcData{State: string(StateCleared)},
				}
				if channel == "WEB" {
					strip.PdcData.RequestChannel = stringPtr(models.PdcChannelWeb)
					strip.PdcData.Web = &models.PdcWebData{}
				}
				session := &models.Session{ID: 42, Name: "LIVE", Airport: "EKCH"}
				sessions := &testutil.MockSessionRepository{
					GetByIDFn: func(context.Context, int32) (*models.Session, error) { return session, nil },
					ListFn:    func(context.Context) ([]*models.Session, error) { return []*models.Session{session}, nil },
				}
				strips := &testutil.MockStripRepository{
					GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return strip, nil },
					SetPdcDataFn: func(_ context.Context, _ int32, _ string, data *models.PdcData) error {
						require.Equal(t, string(StateConfirmed), data.State)
						return nil
					},
				}
				stripService := new(mockPdcStripService)
				stripService.On("ConfirmPdcClearance", mock.Anything, int32(42), "SAS123", shared.BAY_CLEARED, "").Return(nil).Once()
				if tc.assumeMethod != "" {
					stripService.On(tc.assumeMethod, mock.Anything, int32(42), "SAS123", "").Return(nil).Once()
				}
				stripService.On("ReevaluatePdcInvalidValidation", mock.Anything, int32(42), "SAS123", true, false).Return(nil).Once()
				frontend := new(mockPdcFrontendHub)
				frontend.On("SendStripUpdate", int32(42), "SAS123").Return().Once()
				frontend.On("SendPdcStateChange", int32(42), "SAS123", string(StateConfirmed), "").Return().Once()
				service := &Service{
					sessionRepo: sessions, stripRepo: strips, stripService: stripService,
					frontendHub: frontend, euroscopeHub: testPdcEuroscope{},
				}
				if channel == "CPDLC" {
					err := service.HandleWilco(context.Background(), &IncomingMessage{
						From: "SAS123", Payload: "/data2/1/7/N/WILCO",
					}, sessionInformation{id: 42, name: "LIVE", airport: "EKCH", callsign: "EKCH"})
					require.NoError(t, err)
				} else {
					api := NewWebAPI(pdcAuthStub{}, service, nil, false)
					recorder := httptest.NewRecorder()
					api.handleAcknowledge(recorder, acknowledgeRequest("SAS123"))
					require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
				}
				stripService.AssertExpectations(t)
				frontend.AssertExpectations(t)
				if tc.assumeMethod != "AutoAssumeForClearedStripByCid" {
					stripService.AssertNotCalled(t, "AutoAssumeForClearedStripByCid", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				}
				if tc.assumeMethod != "AutoAssumeUnownedClearedStripByCid" {
					stripService.AssertNotCalled(t, "AutoAssumeUnownedClearedStripByCid", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				}
			})
		}
	}
}
