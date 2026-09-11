package coordinationrequest

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type recordingSubmitter struct {
	request  Request
	decision Decision
	fact     OwnershipFact
}

func (r *recordingSubmitter) Submit(_ context.Context, request Request, revision uint64) (CommitResult, error) {
	r.request = request
	return CommitResult{Request: request, Revision: revision + 1}, nil
}

func (r *recordingSubmitter) Get(_ context.Context, _ string, _ RequestID) (Request, error) {
	return r.request, nil
}

func (r *recordingSubmitter) Decide(_ context.Context, _ RequestID, decision Decision, revision uint64) (CommitResult, error) {
	r.decision = decision
	return CommitResult{Request: r.request, Revision: revision + 1}, nil
}

func (r *recordingSubmitter) TransferPending(_ context.Context, fact OwnershipFact) (TransferResult, error) {
	r.fact = fact
	return TransferResult{Revision: 4}, nil
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

func TestServiceDecisionUsesAuthoritativeRecipientAndTrustedContext(t *testing.T) {
	request := routeRequest(t, "submit-1", testTime)
	repository := &recordingSubmitter{request: request}
	owners := &trackingControllerResolver{controller: "EKCH_APP"}
	service := NewService(repository, owners, []string{"EKCH_FMH"})
	auth := CommandContext{Airport: "EKCH", Actor: "7654321", Role: "EKCH_APP", ReceivedAt: testTime.Add(time.Minute)}

	_, err := service.Reject(context.Background(), auth, DecisionCommand{
		CommandID: "reject-1", ExpectedRevision: 1, RequestID: request.ID, Reason: "unable",
	})
	require.NoError(t, err)
	require.Equal(t, "EKCH", repository.decision.Airport)
	require.Equal(t, "7654321", repository.decision.Actor)
	require.Equal(t, "EKCH_APP", repository.decision.Role)
	require.Equal(t, ControllerID("EKCH_APP"), repository.decision.AuthoritativeRecipient)
	require.Equal(t, StateRejected, repository.decision.AfterState)

	auth.Role = "EKCH_DEP"
	_, err = service.Accept(context.Background(), auth, DecisionCommand{CommandID: "spoof", RequestID: request.ID})
	require.ErrorIs(t, err, ErrUnauthorized)

	auth.Role, owners.controller = "EKCH_DEP", "EKCH_DEP"
	_, err = service.Accept(context.Background(), auth, DecisionCommand{CommandID: "wrong-recipient", RequestID: request.ID})
	require.ErrorIs(t, err, ErrWrongRecipient)
}

func TestDecisionPayloadCannotSpoofAuthorityOrAudit(t *testing.T) {
	typeOf := reflect.TypeFor[DecisionCommand]()
	for _, forbidden := range []string{"Airport", "Actor", "Role", "Recipient", "ReceivedAt", "BeforeState", "AfterState", "Kind"} {
		_, present := typeOf.FieldByName(forbidden)
		require.Falsef(t, present, "decision payload must not accept server-owned %s", forbidden)
	}
}

func TestServiceObservesTrustedOwnershipFact(t *testing.T) {
	repository := &recordingSubmitter{}
	service := NewService(repository, &trackingControllerResolver{}, nil)
	fact := OwnershipFact{Airport: "EKCH", FlightID: "flight-1", FactID: "es/42", Revision: 42,
		Owner: "EKCH_DEP", ObservedAt: testTime}
	result, err := service.ObserveOwnership(context.Background(), fact)
	require.NoError(t, err)
	require.Equal(t, uint64(4), result.Revision)
	require.Equal(t, fact, repository.fact)
}
