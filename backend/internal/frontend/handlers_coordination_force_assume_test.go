package frontend

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type coordinationSpyStripService struct {
	testutil.NoOpStripService
	assumeCalled      bool
	forceAssumeCalled bool
	forceAssumeArg    struct {
		session  int32
		callsign string
		position string
	}
}

func (s *coordinationSpyStripService) AssumeStripCoordination(_ context.Context, _ int32, _ string, _ string) error {
	s.assumeCalled = true
	return nil
}

func (s *coordinationSpyStripService) ForceAssumeStrip(_ context.Context, session int32, callsign string, position string) error {
	s.forceAssumeCalled = true
	s.forceAssumeArg.session = session
	s.forceAssumeArg.callsign = callsign
	s.forceAssumeArg.position = position
	return nil
}

func TestHandleCoordinationForceAssumeRequest_ValidationLocked_Allowed(t *testing.T) {
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, callsign string) (*models.Strip, error) {
			return &models.Strip{
				Callsign: callsign,
				ValidationStatus: &models.ValidationStatus{
					Active: true,
				},
			}, nil
		},
	}

	spy := &coordinationSpyStripService{}
	hub := buildFrontendTestHub(mockServerWithStripRepo(stripRepo), spy)
	client := buildFrontendTestClient(hub, 42, "EKCH")
	client.position = "EKCH_A_TWR"

	msg := marshalMessage(t, frontendEvents.CoordinationForceAssumeRequestEvent{
		Callsign: "SAS123",
	})

	err := handleCoordinationForceAssumeRequest(context.Background(), client, msg)
	require.NoError(t, err)
	assert.True(t, spy.forceAssumeCalled, "force assume must still be allowed while validation is active")
	assert.Equal(t, int32(42), spy.forceAssumeArg.session)
	assert.Equal(t, "SAS123", spy.forceAssumeArg.callsign)
	assert.Equal(t, "EKCH_A_TWR", spy.forceAssumeArg.position)
}

func TestHandleCoordinationAssumeRequest_ValidationLocked_Rejected(t *testing.T) {
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, callsign string) (*models.Strip, error) {
			return &models.Strip{
				Callsign: callsign,
				ValidationStatus: &models.ValidationStatus{
					Active: true,
				},
			}, nil
		},
	}

	spy := &coordinationSpyStripService{}
	hub := buildFrontendTestHub(mockServerWithStripRepo(stripRepo), spy)
	client := buildFrontendTestClient(hub, 42, "EKCH")
	client.position = "EKCH_A_TWR"

	msg := marshalMessage(t, frontendEvents.CoordinationAssumeRequestEvent{
		Callsign: "SAS123",
	})

	err := handleCoordinationAssumeRequest(context.Background(), client, msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "strip is locked by an active validation")
	assert.False(t, spy.assumeCalled, "normal assume must still be blocked while validation is active")
}
