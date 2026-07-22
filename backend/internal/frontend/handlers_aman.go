package frontend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/config"
	"FlightStrips/internal/shared"
	events "FlightStrips/pkg/events/frontend"
)

func registerAMANCommandHandlers(handlers *shared.MessageHandlers[events.EventType, *Client]) {
	handlers.Add(events.AMANMoveFlightType, handleAMANMoveFlight)
	handlers.Add(events.AMANLockFlightType, handleAMANLockFlight)
	handlers.Add(events.AMANUnlockFlightType, handleAMANUnlockFlight)
	handlers.Add(events.AMANSetRateType, handleAMANSetRate)
	handlers.Add(events.AMANAcceptTETAType, handleAMANAcceptTETA)
	handlers.Add(events.AMANKeepFPLETAType, handleAMANKeepFPLETA)
	handlers.Add(events.AMANSetManualETAType, handleAMANSetManualETA)
	handlers.Add(events.AMANResetTETAOverrideType, handleAMANResetTETAOverride)
	handlers.Add(events.AMANReportGoAroundType, handleAMANReportGoAround)
}

func handleAMANMoveFlight(ctx context.Context, client *Client, message Message) error {
	var wire events.AMANMoveFlightMessage
	if err := decodeAMANMessage(message, events.AMANMoveFlightType, &wire); err != nil {
		return rejectDecodedAMAN(ctx, client, commandIDFromMessage(message), err)
	}
	command := aman.MoveFlightCommand{
		Metadata: commandMetadata(wire.Data.AMANCommandMeta), FlightID: aman.FlightID(wire.Data.FlightID),
		RunwayGroupID:  aman.RunwayGroupID(wire.Data.RunwayGroupID),
		BeforeFlightID: flightIDPointer(wire.Data.BeforeFlightID), AfterFlightID: flightIDPointer(wire.Data.AfterFlightID),
	}
	return runAMANCommand(ctx, client, command.Metadata.CommandID, func(auth aman.CommandContext) (aman.CommandExecution, error) {
		return client.hub.amanCommandService.MoveFlight(ctx, auth, command)
	})
}

func handleAMANLockFlight(ctx context.Context, client *Client, message Message) error {
	return handleAMANFlightCommand(ctx, client, message, events.AMANLockFlightType, func(auth aman.CommandContext, data events.AMANFlightRequest) (aman.CommandExecution, error) {
		return client.hub.amanCommandService.LockFlight(ctx, auth, aman.LockFlightCommand{Metadata: commandMetadata(data.AMANCommandMeta), FlightID: aman.FlightID(data.FlightID)})
	})
}

func handleAMANUnlockFlight(ctx context.Context, client *Client, message Message) error {
	return handleAMANFlightCommand(ctx, client, message, events.AMANUnlockFlightType, func(auth aman.CommandContext, data events.AMANFlightRequest) (aman.CommandExecution, error) {
		return client.hub.amanCommandService.UnlockFlight(ctx, auth, aman.UnlockFlightCommand{Metadata: commandMetadata(data.AMANCommandMeta), FlightID: aman.FlightID(data.FlightID)})
	})
}

func handleAMANAcceptTETA(ctx context.Context, client *Client, message Message) error {
	return handleAMANFlightCommand(ctx, client, message, events.AMANAcceptTETAType, func(auth aman.CommandContext, data events.AMANFlightRequest) (aman.CommandExecution, error) {
		return client.hub.amanCommandService.AcceptTETA(ctx, auth, aman.AcceptTETACommand{Metadata: commandMetadata(data.AMANCommandMeta), FlightID: aman.FlightID(data.FlightID)})
	})
}

func handleAMANKeepFPLETA(ctx context.Context, client *Client, message Message) error {
	return handleAMANFlightCommand(ctx, client, message, events.AMANKeepFPLETAType, func(auth aman.CommandContext, data events.AMANFlightRequest) (aman.CommandExecution, error) {
		return client.hub.amanCommandService.KeepFPLETA(ctx, auth, aman.KeepFPLETACommand{Metadata: commandMetadata(data.AMANCommandMeta), FlightID: aman.FlightID(data.FlightID)})
	})
}

func handleAMANResetTETAOverride(ctx context.Context, client *Client, message Message) error {
	return handleAMANFlightCommand(ctx, client, message, events.AMANResetTETAOverrideType, func(auth aman.CommandContext, data events.AMANFlightRequest) (aman.CommandExecution, error) {
		return client.hub.amanCommandService.ResetTETAOverride(ctx, auth, aman.ResetTETAOverrideCommand{Metadata: commandMetadata(data.AMANCommandMeta), FlightID: aman.FlightID(data.FlightID)})
	})
}

func handleAMANSetRate(ctx context.Context, client *Client, message Message) error {
	var wire events.AMANSetRateMessage
	if err := decodeAMANMessage(message, events.AMANSetRateType, &wire); err != nil {
		return rejectDecodedAMAN(ctx, client, commandIDFromMessage(message), err)
	}
	effectiveAt, err := parseAMANTime(wire.Data.EffectiveAt)
	if err != nil {
		return rejectDecodedAMAN(ctx, client, wire.Data.CommandID, err)
	}
	command := aman.SetRateCommand{
		Metadata: commandMetadata(wire.Data.AMANCommandMeta), RunwayGroupID: aman.RunwayGroupID(wire.Data.RunwayGroupID),
		ArrivalsPerHour: wire.Data.ArrivalsPerHour, EffectiveAt: effectiveAt,
	}
	return runAMANCommand(ctx, client, command.Metadata.CommandID, func(auth aman.CommandContext) (aman.CommandExecution, error) {
		return client.hub.amanCommandService.SetRate(ctx, auth, command)
	})
}

func handleAMANSetManualETA(ctx context.Context, client *Client, message Message) error {
	var wire events.AMANSetManualETAMessage
	if err := decodeAMANMessage(message, events.AMANSetManualETAType, &wire); err != nil {
		return rejectDecodedAMAN(ctx, client, commandIDFromMessage(message), err)
	}
	manualETA, err := parseAMANTime(wire.Data.ManualETA)
	if err != nil {
		return rejectDecodedAMAN(ctx, client, wire.Data.CommandID, err)
	}
	command := aman.SetManualETACommand{Metadata: commandMetadata(wire.Data.AMANCommandMeta), FlightID: aman.FlightID(wire.Data.FlightID), ManualETA: manualETA}
	return runAMANCommand(ctx, client, command.Metadata.CommandID, func(auth aman.CommandContext) (aman.CommandExecution, error) {
		return client.hub.amanCommandService.SetManualETA(ctx, auth, command)
	})
}

func handleAMANReportGoAround(ctx context.Context, client *Client, message Message) error {
	var wire events.AMANReportGoAroundMessage
	if err := decodeAMANMessage(message, events.AMANReportGoAroundType, &wire); err != nil {
		return rejectDecodedAMAN(ctx, client, commandIDFromMessage(message), err)
	}
	detectedAt, err := parseAMANTime(wire.Data.DetectedAt)
	if err != nil {
		return rejectDecodedAMAN(ctx, client, wire.Data.CommandID, err)
	}
	command := aman.ReportGoAroundCommand{Metadata: commandMetadata(wire.Data.AMANCommandMeta), FlightID: aman.FlightID(wire.Data.FlightID), DetectedAt: detectedAt}
	return runAMANCommand(ctx, client, command.Metadata.CommandID, func(auth aman.CommandContext) (aman.CommandExecution, error) {
		return client.hub.amanCommandService.ReportGoAround(ctx, auth, command)
	})
}

func handleAMANFlightCommand(ctx context.Context, client *Client, message Message, expected events.EventType, execute func(aman.CommandContext, events.AMANFlightRequest) (aman.CommandExecution, error)) error {
	var wire events.AMANFlightMessage
	if err := decodeAMANMessage(message, expected, &wire); err != nil {
		return rejectDecodedAMAN(ctx, client, commandIDFromMessage(message), err)
	}
	return runAMANCommand(ctx, client, wire.Data.CommandID, func(auth aman.CommandContext) (aman.CommandExecution, error) {
		return execute(auth, wire.Data)
	})
}

func runAMANCommand(ctx context.Context, client *Client, commandID string, execute func(aman.CommandContext) (aman.CommandExecution, error)) error {
	auth, err := client.hub.amanContext(client)
	if err != nil {
		return rejectDecodedAMAN(ctx, client, commandID, err)
	}
	execution, err := execute(auth)
	if err != nil {
		return rejectAMAN(ctx, client, commandID, execution.CurrentRevision, err)
	}
	slog.InfoContext(ctx, "AMAN command accepted", slog.String("command_id", commandID), slog.String("airport", auth.Airport), slog.String("actor", auth.Actor), slog.String("role", auth.Role), slog.Uint64("revision", uint64(execution.CurrentRevision)), slog.Bool("duplicate", execution.Duplicate))
	return nil
}

func (hub *Hub) amanContext(client *Client) (aman.CommandContext, error) {
	if client == nil || !client.IsAuthenticated() {
		return aman.CommandContext{}, &aman.DomainError{Class: aman.ErrorUnauthorized, Message: "AMAN command requires an authenticated session"}
	}
	if client.readOnly {
		return aman.CommandContext{}, &aman.DomainError{Class: aman.ErrorUnauthorized, Message: "observer clients cannot mutate AMAN state"}
	}
	if !hub.amanMutations {
		return aman.CommandContext{}, &aman.DomainError{Class: aman.ErrorReadOnly, Message: "AMAN controller mutations are read-only in the current rollout mode"}
	}
	roleForPosition := configuredAMANRole
	if hub.amanRoleForPosition != nil {
		roleForPosition = hub.amanRoleForPosition
	}
	role := roleForPosition(client.position)
	if _, authorized := hub.amanFMPRoles[strings.ToUpper(role)]; !authorized {
		return aman.CommandContext{}, &aman.DomainError{Class: aman.ErrorUnauthorized, Message: "AMAN command requires a configured FMP role"}
	}
	now := time.Now
	if hub.amanNow != nil {
		now = hub.amanNow
	}
	return aman.CommandContext{Airport: client.airport, Actor: client.GetCid(), Role: role, ReceivedAt: now().UTC()}, nil
}

func configuredAMANRole(position string) string {
	if value, err := config.GetPositionBasedOnFrequency(position); err == nil {
		return value.Name
	}
	if value, err := config.GetPositionByName(position); err == nil {
		return value.Name
	}
	return ""
}

func rejectDecodedAMAN(ctx context.Context, client *Client, commandID string, err error) error {
	if commandID == "" {
		return err
	}
	revision := aman.SequenceRevision(0)
	if client != nil && client.hub != nil && client.hub.amanCommandService != nil && client.airport != "" {
		if current, currentErr := client.hub.amanCommandService.CurrentRevision(ctx, client.airport); currentErr == nil {
			revision = current
		}
	}
	return rejectAMAN(ctx, client, commandID, revision, err)
}

func rejectAMAN(ctx context.Context, client *Client, commandID string, revision aman.SequenceRevision, err error) error {
	domain := stableAMANError(err)
	event, mapErr := events.NewAMANCommandRejectedEvent(commandID, revision, domain, retryableAMANError(domain.Class))
	if mapErr != nil {
		return mapErr
	}
	client.Enqueue(event)
	slog.WarnContext(ctx, "AMAN command rejected", slog.String("command_id", commandID), slog.String("airport", client.airport), slog.String("actor", client.GetCid()), slog.String("code", string(domain.Class)), slog.Uint64("revision", uint64(revision)))
	return nil
}

func stableAMANError(err error) *aman.DomainError {
	var domain *aman.DomainError
	if errors.As(err, &domain) && domain != nil && domain.Class.Valid() {
		return domain
	}
	return &aman.DomainError{Class: aman.ErrorDependencyUnavailable, Message: "AMAN command could not be completed"}
}

func retryableAMANError(class aman.ErrorClass) bool {
	return class == aman.ErrorRevisionConflict || class == aman.ErrorDependencyUnavailable
}

func decodeAMANMessage(message Message, expected events.EventType, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(message.Message))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return invalidAMANPayload(err)
	}
	if err := ensureAMANEOF(decoder); err != nil {
		return err
	}
	var header struct {
		Type    events.EventType `json:"type"`
		Version int              `json:"version"`
	}
	if err := json.Unmarshal(message.Message, &header); err != nil {
		return invalidAMANPayload(err)
	}
	if header.Type != expected || header.Version != events.AMANWireVersion {
		return invalidAMANPayload(fmt.Errorf("expected %s version %d", expected, events.AMANWireVersion))
	}
	return nil
}

func ensureAMANEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return invalidAMANPayload(errors.New("trailing JSON value"))
		}
		return invalidAMANPayload(err)
	}
	return nil
}

func invalidAMANPayload(err error) error {
	return &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: "invalid AMAN command payload: " + err.Error()}
}

func parseAMANTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil || parsed.Location() != time.UTC {
		return time.Time{}, invalidAMANPayload(errors.New("timestamp must be RFC3339 UTC"))
	}
	return parsed, nil
}

func commandMetadata(value events.AMANCommandMeta) aman.CommandMetadata {
	return aman.CommandMetadata{CommandID: value.CommandID, ExpectedRevision: aman.SequenceRevision(value.ExpectedRevision)}
}

func flightIDPointer(value *string) *aman.FlightID {
	if value == nil {
		return nil
	}
	converted := aman.FlightID(*value)
	return &converted
}

func commandIDFromMessage(message Message) string {
	var value struct {
		Data struct {
			CommandID string `json:"command_id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(message.Message, &value)
	return value.Data.CommandID
}
