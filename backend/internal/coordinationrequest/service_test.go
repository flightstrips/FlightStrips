package coordinationrequest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type recordingSubmitter struct{ request Request }

func (r *recordingSubmitter) Submit(_ context.Context, request Request, revision uint64) (CommitResult, error) {
	r.request = request
	return CommitResult{Request: request, Revision: revision + 1}, nil
}

type trackingControllerResolver struct {
	controller ControllerID
	requests   []FlightID
}

func (r *trackingControllerResolver) TrackingController(_ context.Context, _ string, flightID FlightID) (ControllerID, error) {
	r.requests = append(r.requests, flightID)
	return r.controller, nil
}

func TestServiceAuthorizesFMPAndUsesOnlyTrustedContext(t *testing.T) {
	repository := &recordingSubmitter{}
	owners := &trackingControllerResolver{controller: "EKCH_APP"}
	service := NewService(repository, owners, []string{"EKCH_FMH"})
	command := SubmitCommand{CommandID: "submit-1", ExpectedRevision: 3, FlightID: "flight-1", Kind: KindSpeed,
		Payload: Payload{Speed: &SpeedPayload{Requested: "220 KT"}}}
	auth := CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKCH_FMH", ReceivedAt: testTime}

	result, err := service.Submit(context.Background(), auth, command)
	require.NoError(t, err)
	require.Equal(t, uint64(4), result.Revision)
	require.Equal(t, "EKCH", repository.request.Airport)
	require.Equal(t, "1234567", repository.request.SubmittedBy)
	require.Equal(t, "EKCH_FMH", repository.request.SubmittedRole)
	require.Equal(t, ControllerID("EKCH_APP"), repository.request.RecipientController)
	require.Equal(t, RecipientAssigned, repository.request.RecipientStatus)
	require.Equal(t, []FlightID{"flight-1"}, owners.requests)
	require.Equal(t, testTime, repository.request.CreatedAt)

	auth.Role = "EKCH_APP"
	_, err = service.Submit(context.Background(), auth, command)
	require.ErrorIs(t, err, ErrUnauthorized)
}

func TestServicePersistsVisibleUnassignedRecipient(t *testing.T) {
	repository := &recordingSubmitter{}
	service := NewService(repository, &trackingControllerResolver{}, []string{"EKCH_FMH"})
	command := SubmitCommand{CommandID: "submit-1", FlightID: "flight-1", Kind: KindSpeed,
		Payload: Payload{Speed: &SpeedPayload{Requested: "220 KT"}}}

	_, err := service.Submit(context.Background(), CommandContext{
		Airport: "EKCH", Actor: "1234567", Role: "EKCH_FMH", ReceivedAt: testTime,
	}, command)

	require.NoError(t, err)
	require.Empty(t, repository.request.RecipientController)
	require.Equal(t, RecipientUnassigned, repository.request.RecipientStatus)
}
