package frontend

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type coordinationSpyStripService struct {
	noOpStripService
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

func TestHandleCoordinationForceAssumeRequest_ReturnsRecalculatedRoute(t *testing.T) {
	const (
		session   = int32(42)
		callsign  = "SAS123"
		position  = "EKCH_A_TWR"
		requestID = "SAS123-1"
	)
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, gotCallsign string) (*models.Strip, error) {
			assert.Equal(t, callsign, gotCallsign)
			return &models.Strip{
				Callsign:   callsign,
				Owner:      ptr(position),
				NextOwners: []string{"EKCH_B_GND"},
			}, nil
		},
	}
	spy := &coordinationSpyStripService{}
	hub := buildFrontendTestHub(mockServerWithStripRepo(stripRepo), spy)
	client := buildFrontendTestClient(hub, session, "EKCH")
	client.position = position
	client.send = make(chan events.OutgoingMessage, 1)

	err := handleCoordinationForceAssumeRequest(context.Background(), client, marshalMessage(t, frontendEvents.CoordinationForceAssumeRequestEvent{
		Callsign:  callsign,
		RequestID: requestID,
	}))
	require.NoError(t, err)

	result := <-client.send
	forceAssumeResult, ok := result.(frontendEvents.CoordinationForceAssumeResultEvent)
	require.True(t, ok)
	assert.Equal(t, callsign, forceAssumeResult.Callsign)
	assert.Equal(t, requestID, forceAssumeResult.RequestID)
	assert.Equal(t, position, forceAssumeResult.Owner)
	assert.Equal(t, []string{"EKCH_B_GND"}, forceAssumeResult.NextOwners)
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
