package euroscope

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestMarshalEnvelopeUsesOneofFieldAsEventDiscriminator(t *testing.T) {
	encoded, err := MarshalEnvelope(&LoginEvent{
		Airport:  "EKCH",
		Callsign: "EKCH_TWR",
		Range:    100,
	}, Login)
	require.NoError(t, err)

	eventType, payload, err := UnmarshalEnvelope(encoded)
	require.NoError(t, err)
	require.Equal(t, Login, eventType)

	var login LoginEvent
	require.NoError(t, UnmarshalEvent(encoded, Login, &login))
	require.Equal(t, "EKCH", login.Airport)
	require.Equal(t, "EKCH_TWR", login.Callsign)
	require.EqualValues(t, 100, login.Range)
	require.NotEmpty(t, payload)
}

func TestCommandEnvelopeMetadataAndResultCases(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	data, err := MarshalCommandEnvelope(&SendPrivateMessageEvent{Callsign: "SAS123", Message: "hello"},
		SendPrivateMessage, id, 42, 9, 3)
	require.NoError(t, err)
	var envelope Envelope
	require.NoError(t, proto.Unmarshal(data, &envelope))
	require.Equal(t, id, envelope.CommandId)
	require.EqualValues(t, 42, envelope.SessionId)
	require.EqualValues(t, 9, envelope.OwnerEpoch)
	require.EqualValues(t, 3, envelope.MasterEpoch)
	require.Equal(t, "hello", envelope.GetSendPrivateMessage().Message)
	_, err = MarshalCommandEnvelope(&SendPrivateMessageEvent{}, SendPrivateMessage,
		"11111111-1111-4111-8111-11111111111A", 42, 9, 3)
	require.ErrorContains(t, err, "requires command ID")

	result := &Envelope{CommandId: id, Event: &Envelope_CommandResult{CommandResult: &CommandResultEvent{
		CommandId: id, Status: CommandResultEvent_FAILED, Reason: CommandResultEvent_UI_UNAVAILABLE,
	}}}
	data, err = proto.Marshal(result)
	require.NoError(t, err)
	caseType, _, err := UnmarshalEnvelope(data)
	require.NoError(t, err)
	require.Equal(t, CommandResult, caseType)

	data, err = MarshalResultRecorded(id)
	require.NoError(t, err)
	caseType, _, err = UnmarshalEnvelope(data)
	require.NoError(t, err)
	require.Equal(t, ResultRecorded, caseType)
}

func TestOutgoingMessageMarshalWrapsPayloadInEnvelope(t *testing.T) {
	encoded, err := (SessionInfoEvent{Role: SessionInfoMaster}).Marshal()
	require.NoError(t, err)

	eventType, _, err := UnmarshalEnvelope(encoded)
	require.NoError(t, err)
	require.Equal(t, SessionInfo, eventType)
}

func TestHoldEventCanBeSentToEuroScope(t *testing.T) {
	encoded, err := (HoldEvent{Callsign: "SAS123", Hold: "OLPIB", HoldType: "enroute", HoldEat: "1422"}).Marshal()
	require.NoError(t, err)

	var hold HoldEvent
	require.NoError(t, UnmarshalEvent(encoded, Hold, &hold))
	require.Equal(t, "SAS123", hold.Callsign)
	require.Equal(t, "OLPIB", hold.Hold)
	require.Equal(t, "1422", hold.HoldEat)
}

func TestMarshalEnvelopeRejectsMismatchedOneofField(t *testing.T) {
	_, err := MarshalEnvelope(&LoginEvent{}, Authentication)
	require.ErrorContains(t, err, "does not match envelope field")
}

func TestUnmarshalEnvelopeRejectsMissingEvent(t *testing.T) {
	_, _, err := UnmarshalEnvelope(nil)
	require.ErrorContains(t, err, "contains no event")
}

func TestDecodeEnvelopeRejectsUnknownAndMismatchedResult(t *testing.T) {
	data, err := MarshalEnvelope(&TokenEvent{ProtocolRevision: 2}, Authentication)
	require.NoError(t, err)
	// Field 99, varint 1, appended to an otherwise valid envelope.
	data = append(data, 0x98, 0x06, 0x01)
	_, err = DecodeEnvelope(data)
	require.ErrorContains(t, err, "unknown EuroScope protobuf field")

	data, err = proto.Marshal(&Envelope{CommandId: "outer", Event: &Envelope_CommandResult{
		CommandResult: &CommandResultEvent{CommandId: "inner", Status: CommandResultEvent_EXECUTED, Reason: CommandResultEvent_OK},
	}})
	require.NoError(t, err)
	_, err = DecodeEnvelope(data)
	require.ErrorContains(t, err, "invalid EuroScope command result")
}
