package pdc

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestIssuedClearancePersistsMessageAndIssuerTogether(t *testing.T) {
	for _, web := range []bool{false, true} {
		t.Run(map[bool]string{false: "cpdlc", true: "web"}[web], func(t *testing.T) {
			remarks := "request"
			prior := (&models.PdcData{RequestRemarks: &remarks}).Normalize()
			reads, writes := 0, 0
			var saved *models.PdcData
			repo := &testutil.MockStripRepository{
				GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
					reads++
					return &models.Strip{PdcData: prior}, nil
				},
				SetPdcDataFn: func(_ context.Context, _ int32, _ string, data *models.PdcData) error {
					writes++
					saved = data
					return nil
				},
			}
			svc := &Service{stripRepo: repo}
			require.NoError(t, svc.persistIssuedPdcClearance(context.Background(), 42, "SAS123", "1234567", ClearanceOptions{Sequence: 7}, web))
			require.Equal(t, 1, reads)
			require.Equal(t, 1, writes)
			require.Equal(t, string(StateCleared), saved.State)
			require.Equal(t, int32(7), *saved.MessageSequence)
			require.NotNil(t, saved.MessageSent)
			require.Equal(t, "1234567", *saved.IssuedByCid)
			require.Nil(t, saved.RequestRemarks)
			if web {
				require.NotNil(t, saved.Web.ClearanceText)
			}
			require.Equal(t, &remarks, prior.RequestRemarks, "do not mutate the read model before persistence")
		})
	}
}
