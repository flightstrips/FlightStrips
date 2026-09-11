package frontend

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestAMANHandlersMapEveryTypedCommandWithServerDerivedContext(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		eventType frontendEvents.EventType
		payload   string
		operation string
	}{
		{"move", frontendEvents.AMANMoveFlightType, `{"type":"aman.move_flight","version":1,"data":{"command_id":"command-1","expected_revision":7,"flight_id":"flight-1","runway_group_id":"A","before_flight_id":"flight-2"}}`, "move"},
		{"lock", frontendEvents.AMANLockFlightType, `{"type":"aman.lock_flight","version":1,"data":{"command_id":"command-1","expected_revision":7,"flight_id":"flight-1"}}`, "lock"},
		{"unlock", frontendEvents.AMANUnlockFlightType, `{"type":"aman.unlock_flight","version":1,"data":{"command_id":"command-1","expected_revision":7,"flight_id":"flight-1"}}`, "unlock"},
		{"rate", frontendEvents.AMANSetRateType, `{"type":"aman.set_rate","version":1,"data":{"command_id":"command-1","expected_revision":7,"runway_group_id":"A","arrivals_per_hour":30,"effective_at":"2026-07-22T12:05:00Z"}}`, "rate"},
		{"runway selection", frontendEvents.AMANSelectRunwayGroupType, `{"type":"aman.select_runway_group","version":1,"data":{"command_id":"command-1","expected_revision":7,"runway_group_id":"A","effective_at":"2026-07-22T12:05:00Z"}}`, "runway_selection"},
		{"accept", frontendEvents.AMANAcceptTETAType, `{"type":"aman.accept_teta","version":1,"data":{"command_id":"command-1","expected_revision":7,"flight_id":"flight-1"}}`, "accept"},
		{"keep", frontendEvents.AMANKeepFPLETAType, `{"type":"aman.keep_fpl_eta","version":1,"data":{"command_id":"command-1","expected_revision":7,"flight_id":"flight-1"}}`, "keep"},
		{"manual", frontendEvents.AMANSetManualETAType, `{"type":"aman.set_manual_eta","version":1,"data":{"command_id":"command-1","expected_revision":7,"flight_id":"flight-1","manual_eta":"2026-07-22T12:10:00Z"}}`, "manual"},
		{"reset", frontendEvents.AMANResetTETAOverrideType, `{"type":"aman.reset_teta_override","version":1,"data":{"command_id":"command-1","expected_revision":7,"flight_id":"flight-1"}}`, "reset"},
		{"manual feeder", frontendEvents.AMANSetManualFeederETAType, `{"type":"aman.set_manual_feeder_eta","version":1,"data":{"command_id":"command-1","expected_revision":7,"flight_id":"flight-1","feeder_eta":"2026-07-22T12:10:00Z"}}`, "manual_feeder"},
		{"reset manual feeder", frontendEvents.AMANResetManualFeederETAType, `{"type":"aman.reset_manual_feeder_eta","version":1,"data":{"command_id":"command-1","expected_revision":7,"flight_id":"flight-1"}}`, "reset_manual_feeder"},
		{"recompute", frontendEvents.AMANRecomputeFlightType, `{"type":"aman.recompute_flight","version":1,"data":{"command_id":"command-1","expected_revision":7,"flight_id":"flight-1"}}`, "recompute"},
		{"change runway", frontendEvents.AMANChangeRunwayType, `{"type":"aman.change_runway","version":1,"data":{"command_id":"command-1","expected_revision":7,"flight_id":"flight-1","runway_group_id":"B"}}`, "change_runway"},
		{"go around", frontendEvents.AMANReportGoAroundType, `{"type":"aman.report_go_around","version":1,"data":{"command_id":"command-1","expected_revision":7,"flight_id":"flight-1","detected_at":"2026-07-22T11:59:00Z"}}`, "go_around"},
		{"confirm go around", frontendEvents.AMANConfirmGoAroundType, `{"type":"aman.confirm_go_around","version":1,"data":{"command_id":"command-1","expected_revision":7,"flight_id":"flight-1","episode_id":"flight-1/go-around/1"}}`, "confirm_go_around"},
		{"reject go around", frontendEvents.AMANRejectGoAroundType, `{"type":"aman.reject_go_around","version":1,"data":{"command_id":"command-1","expected_revision":7,"flight_id":"flight-1","episode_id":"flight-1/go-around/1"}}`, "reject_go_around"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &recordingAMANCommandService{}
			hub, client := newAMANCommandTestClient(service, now)
			err := hub.handlers.Handle(context.Background(), client, Message{Type: test.eventType, Message: []byte(test.payload)})
			require.NoError(t, err)
			require.Equal(t, test.operation, service.operation)
			require.Equal(t, aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKCH_FMH", ReceivedAt: now}, service.auth)
			require.Equal(t, "command-1", service.metadata.CommandID)
			require.Equal(t, aman.SequenceRevision(7), service.metadata.ExpectedRevision)
			require.Empty(t, client.send)
		})
	}
}

func TestAMANChangeRunwayRejectsSpoofedServerContextStrictly(t *testing.T) {
	service := &recordingAMANCommandService{}
	hub, client := newAMANCommandTestClient(service, time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC))
	payload := `{"type":"aman.change_runway","version":1,"data":{"command_id":"command-1","expected_revision":7,"flight_id":"flight-1","runway_group_id":"B","airport":"ZZZZ","actor":"spoof","role":"ADMIN","received_at":"2026-07-22T12:00:00Z"}}`
	require.NoError(t, hub.handlers.Handle(context.Background(), client, Message{Type: frontendEvents.AMANChangeRunwayType, Message: []byte(payload)}))
	require.Empty(t, service.operation)
	rejection := (<-client.send).(frontendEvents.AMANCommandRejectedEvent)
	require.Equal(t, string(aman.ErrorInvalidArgument), rejection.Data.Code)
}

func TestAMANRecomputeRejectsSpoofedContextAndClientResultsStrictly(t *testing.T) {
	for _, extra := range []string{
		`,"airport":"ZZZZ","actor":"spoof","role":"ADMIN","received_at":"2026-07-22T12:00:00Z"`,
		`,"teta":"2026-07-22T12:10:00Z"`, `,"result":{"raw_teta":"2026-07-22T12:10:00Z"}`,
		`,"before_flight_id":"flight-2"`,
	} {
		service := &recordingAMANCommandService{}
		hub, client := newAMANCommandTestClient(service, time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC))
		payload := `{"type":"aman.recompute_flight","version":1,"data":{"command_id":"command-1","expected_revision":7,"flight_id":"flight-1"` + extra + `}}`
		require.NoError(t, hub.handlers.Handle(context.Background(), client, Message{Type: frontendEvents.AMANRecomputeFlightType, Message: []byte(payload)}))
		require.Empty(t, service.operation)
		rejection := (<-client.send).(frontendEvents.AMANCommandRejectedEvent)
		require.Equal(t, string(aman.ErrorInvalidArgument), rejection.Data.Code)
		require.Equal(t, uint64(7), rejection.Data.CurrentRevision)
		require.False(t, rejection.Data.Retryable)
	}
}

func TestAMANSetManualFeederETARejectsNonUTCTimestamp(t *testing.T) {
	service := &recordingAMANCommandService{}
	hub, client := newAMANCommandTestClient(service, time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC))
	payload := `{"type":"aman.set_manual_feeder_eta","version":1,"data":{"command_id":"command-1","expected_revision":7,"flight_id":"flight-1","feeder_eta":"2026-07-22T14:10:00+02:00"}}`

	require.NoError(t, hub.handlers.Handle(context.Background(), client, Message{Type: frontendEvents.AMANSetManualFeederETAType, Message: []byte(payload)}))
	require.Empty(t, service.operation)
	rejection := (<-client.send).(frontendEvents.AMANCommandRejectedEvent)
	require.Equal(t, string(aman.ErrorInvalidArgument), rejection.Data.Code)
	require.Contains(t, rejection.Data.Message, "RFC3339 UTC")
}

func TestAMANHandlerRejectsObserverFMPAndReadOnlyBeforeCommandService(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*Hub, *Client)
		code      aman.ErrorClass
	}{
		{"unauthenticated", func(_ *Hub, client *Client) { client.user = shared.AuthenticatedUser{} }, aman.ErrorUnauthorized},
		{"observer", func(_ *Hub, client *Client) { client.readOnly = true }, aman.ErrorUnauthorized},
		{"non FMP", func(hub *Hub, _ *Client) { hub.amanFMPRoles = map[string]struct{}{} }, aman.ErrorUnauthorized},
		{"rollout read only", func(hub *Hub, _ *Client) { hub.amanMutations = false }, aman.ErrorReadOnly},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &recordingAMANCommandService{}
			hub, client := newAMANCommandTestClient(service, time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC))
			test.configure(hub, client)
			payload := `{"type":"aman.change_runway","version":1,"data":{"command_id":"command-1","expected_revision":7,"flight_id":"flight-1","runway_group_id":"B"}}`
			require.NoError(t, hub.handlers.Handle(context.Background(), client, Message{Type: frontendEvents.AMANChangeRunwayType, Message: []byte(payload)}))
			require.Empty(t, service.operation)
			rejection := (<-client.send).(frontendEvents.AMANCommandRejectedEvent)
			require.Equal(t, string(test.code), rejection.Data.Code)
		})
	}
}

func TestAMANRevisionConflictUsesCommandRejectionContract(t *testing.T) {
	service := &recordingAMANCommandService{execution: aman.CommandExecution{CurrentRevision: 12}, err: &aman.DomainError{Class: aman.ErrorRevisionConflict, Message: "revision changed"}}
	hub, client := newAMANCommandTestClient(service, time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC))
	payload := `{"type":"aman.set_manual_feeder_eta","version":1,"data":{"command_id":"command-1","expected_revision":7,"flight_id":"flight-1","feeder_eta":"2026-07-22T12:10:00Z"}}`

	require.NoError(t, hub.handlers.Handle(context.Background(), client, Message{Type: frontendEvents.AMANSetManualFeederETAType, Message: []byte(payload)}))
	rejection := (<-client.send).(frontendEvents.AMANCommandRejectedEvent)
	require.Equal(t, uint64(12), rejection.Data.CurrentRevision)
	require.Equal(t, string(aman.ErrorRevisionConflict), rejection.Data.Code)
	require.True(t, rejection.Data.Retryable)
}

func newAMANCommandTestClient(service aman.CommandService, now time.Time) (*Hub, *Client) {
	handlers := shared.NewMessageHandlers[frontendEvents.EventType, *Client]()
	registerAMANCommandHandlers(&handlers)
	hub := &Hub{
		handlers: handlers, amanCommandService: service, amanFMPRoles: map[string]struct{}{"EKCH_FMH": {}},
		amanMutations: true, amanNow: func() time.Time { return now }, amanRoleForPosition: func(string) string { return "EKCH_FMH" },
	}
	token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"exp": float64(time.Now().Add(time.Hour).Unix())})
	client := &Client{
		hub: hub, airport: "EKCH", position: "120.500", send: make(chan events.OutgoingMessage, 4),
		closed: make(chan struct{}), user: shared.NewAuthenticatedUser("1234567", 0, token),
	}
	return hub, client
}

type recordingAMANCommandService struct {
	operation string
	auth      aman.CommandContext
	metadata  aman.CommandMetadata
	execution aman.CommandExecution
	err       error
}

func (*recordingAMANCommandService) Name() string { return "recording AMAN command service" }
func (*recordingAMANCommandService) CurrentRevision(context.Context, string) (aman.SequenceRevision, error) {
	return 7, nil
}
func (s *recordingAMANCommandService) record(operation string, auth aman.CommandContext, metadata aman.CommandMetadata) (aman.CommandExecution, error) {
	s.operation, s.auth, s.metadata = operation, auth, metadata
	if s.execution.CurrentRevision == 0 && s.err == nil {
		s.execution.CurrentRevision = metadata.ExpectedRevision + 1
	}
	return s.execution, s.err
}
func (s *recordingAMANCommandService) MoveFlight(_ context.Context, auth aman.CommandContext, command aman.MoveFlightCommand) (aman.CommandExecution, error) {
	return s.record("move", auth, command.Metadata)
}
func (s *recordingAMANCommandService) LockFlight(_ context.Context, auth aman.CommandContext, command aman.LockFlightCommand) (aman.CommandExecution, error) {
	return s.record("lock", auth, command.Metadata)
}
func (s *recordingAMANCommandService) UnlockFlight(_ context.Context, auth aman.CommandContext, command aman.UnlockFlightCommand) (aman.CommandExecution, error) {
	return s.record("unlock", auth, command.Metadata)
}
func (s *recordingAMANCommandService) SetRate(_ context.Context, auth aman.CommandContext, command aman.SetRateCommand) (aman.CommandExecution, error) {
	return s.record("rate", auth, command.Metadata)
}
func (s *recordingAMANCommandService) SelectRunwayGroup(_ context.Context, auth aman.CommandContext, command aman.SelectRunwayGroupCommand) (aman.CommandExecution, error) {
	return s.record("runway_selection", auth, command.Metadata)
}
func (s *recordingAMANCommandService) SetActiveRunwayGroups(_ context.Context, auth aman.CommandContext, command aman.SetActiveRunwayGroupsCommand) (aman.CommandExecution, error) {
	return s.record("set_active_runway_groups", auth, command.Metadata)
}
func (s *recordingAMANCommandService) CreateRunwayGap(_ context.Context, auth aman.CommandContext, command aman.CreateRunwayGapCommand) (aman.CommandExecution, error) {
	return s.record("create_runway_gap", auth, command.Metadata)
}
func (s *recordingAMANCommandService) RemoveRunwayGap(_ context.Context, auth aman.CommandContext, command aman.RemoveRunwayGapCommand) (aman.CommandExecution, error) {
	return s.record("remove_runway_gap", auth, command.Metadata)
}
func (s *recordingAMANCommandService) AcceptTETA(_ context.Context, auth aman.CommandContext, command aman.AcceptTETACommand) (aman.CommandExecution, error) {
	return s.record("accept", auth, command.Metadata)
}
func (s *recordingAMANCommandService) KeepFPLETA(_ context.Context, auth aman.CommandContext, command aman.KeepFPLETACommand) (aman.CommandExecution, error) {
	return s.record("keep", auth, command.Metadata)
}
func (s *recordingAMANCommandService) SetManualETA(_ context.Context, auth aman.CommandContext, command aman.SetManualETACommand) (aman.CommandExecution, error) {
	return s.record("manual", auth, command.Metadata)
}
func (s *recordingAMANCommandService) ResetTETAOverride(_ context.Context, auth aman.CommandContext, command aman.ResetTETAOverrideCommand) (aman.CommandExecution, error) {
	return s.record("reset", auth, command.Metadata)
}
func (s *recordingAMANCommandService) SetManualFeederETA(_ context.Context, auth aman.CommandContext, command aman.SetManualFeederETACommand) (aman.CommandExecution, error) {
	return s.record("manual_feeder", auth, command.Metadata)
}
func (s *recordingAMANCommandService) ResetManualFeederETA(_ context.Context, auth aman.CommandContext, command aman.ResetManualFeederETACommand) (aman.CommandExecution, error) {
	return s.record("reset_manual_feeder", auth, command.Metadata)
}
func (s *recordingAMANCommandService) RecomputeFlight(_ context.Context, auth aman.CommandContext, command aman.RecomputeFlightCommand) (aman.CommandExecution, error) {
	return s.record("recompute", auth, command.Metadata)
}
func (s *recordingAMANCommandService) ChangeRunway(_ context.Context, auth aman.CommandContext, command aman.ChangeRunwayCommand) (aman.CommandExecution, error) {
	return s.record("change_runway", auth, command.Metadata)
}
func (s *recordingAMANCommandService) ReportGoAround(_ context.Context, auth aman.CommandContext, command aman.ReportGoAroundCommand) (aman.CommandExecution, error) {
	return s.record("go_around", auth, command.Metadata)
}
func (s *recordingAMANCommandService) ConfirmGoAround(_ context.Context, auth aman.CommandContext, command aman.ConfirmGoAroundCommand) (aman.CommandExecution, error) {
	return s.record("confirm_go_around", auth, command.Metadata)
}
func (s *recordingAMANCommandService) RejectGoAround(_ context.Context, auth aman.CommandContext, command aman.RejectGoAroundCommand) (aman.CommandExecution, error) {
	return s.record("reject_go_around", auth, command.Metadata)
}
