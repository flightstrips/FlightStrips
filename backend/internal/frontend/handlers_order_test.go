package frontend

import (
	"context"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	events "FlightStrips/pkg/events/frontend"
	"github.com/stretchr/testify/require"
)

type orderSpy struct {
	noOpStripService
	calls int
}

func (s *orderSpy) MoveStripBetween(context.Context, int32, string, *events.StripRef, string) error {
	s.calls++
	return nil
}

func TestHandleUpdateOrderOwnership(t *testing.T) {
	for _, tc := range []struct {
		name, owner string
		allowed     bool
	}{
		{"owner", "118.105", true},
		{"unowned", "", true},
		{"other controller", "118.580", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &orderSpy{}
			repo := &testutil.MockStripRepository{GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
				return &models.Strip{Callsign: "SAS404", Bay: "TAXI_LWR", Owner: &tc.owner}, nil
			}}
			client := buildFrontendTestClient(buildFrontendTestHub(mockServerWithStripRepo(repo), service), 1, "EKCH")
			client.position = "118.105"
			err := handleUpdateOrder(context.Background(), client, marshalMessage(t, events.UpdateOrderEvent{Callsign: "SAS404"}))
			if tc.allowed {
				require.NoError(t, err)
				require.Equal(t, 1, service.calls)
			} else {
				require.ErrorContains(t, err, "owned by another controller")
				require.Zero(t, service.calls)
			}
		})
	}
}
