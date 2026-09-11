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

func TestServiceAuthorizesFMPAndUsesOnlyTrustedContext(t *testing.T) {
	repository := &recordingSubmitter{}
	service := NewService(repository, []string{"EKCH_FMH"})
	command := SubmitCommand{CommandID: "submit-1", ExpectedRevision: 3, FlightID: "flight-1", Kind: KindSpeed,
		Payload: Payload{Speed: &SpeedPayload{Requested: "220 KT"}}}
	auth := CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKCH_FMH", RecipientController: "EKCH_APP", ReceivedAt: testTime}

	result, err := service.Submit(context.Background(), auth, command)
	require.NoError(t, err)
	require.Equal(t, uint64(4), result.Revision)
	require.Equal(t, "EKCH", repository.request.Airport)
	require.Equal(t, "1234567", repository.request.SubmittedBy)
	require.Equal(t, "EKCH_FMH", repository.request.SubmittedRole)
	require.Equal(t, ControllerID("EKCH_APP"), repository.request.RecipientController)
	require.Equal(t, testTime, repository.request.CreatedAt)

	auth.Role = "EKCH_APP"
	_, err = service.Submit(context.Background(), auth, command)
	require.ErrorIs(t, err, ErrUnauthorized)
	auth.Role = "EKCH_FMH"
	auth.RecipientController = " EKCH_APP"
	_, err = service.Submit(context.Background(), auth, command)
	require.ErrorContains(t, err, "server-derived")
}
