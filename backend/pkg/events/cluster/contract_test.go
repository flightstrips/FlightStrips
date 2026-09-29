package cluster

import (
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// Every non-synthetic case gets a binary fixture. Reflecting over descriptors
// makes adding a new case without coverage impossible.
func TestEveryOneofCaseBinaryRoundTrip(t *testing.T) {
	for _, file := range []protoreflect.FileDescriptor{File_storage_proto, File_wire_proto} {
		walkMessages(file.Messages(), func(message protoreflect.MessageDescriptor) {
			for i := 0; i < message.Oneofs().Len(); i++ {
				oneof := message.Oneofs().Get(i)
				if oneof.IsSynthetic() {
					continue
				}
				for j := 0; j < oneof.Fields().Len(); j++ {
					field := oneof.Fields().Get(j)
					t.Run(string(field.FullName()), func(t *testing.T) {
						value := dynamicpb.NewMessage(message)
						value.Set(field, fixtureValue(field))
						encoded, err := proto.Marshal(value)
						if err != nil {
							t.Fatal(err)
						}
						decoded := dynamicpb.NewMessage(message)
						if err := UnmarshalStrict(encoded, decoded); err != nil {
							t.Fatal(err)
						}
						if decoded.WhichOneof(oneof) != field || !proto.Equal(value, decoded) {
							t.Fatalf("case %s did not round-trip", field.FullName())
						}
					})
				}
			}
		})
	}
}

func walkMessages(messages protoreflect.MessageDescriptors, visit func(protoreflect.MessageDescriptor)) {
	for i := 0; i < messages.Len(); i++ {
		message := messages.Get(i)
		visit(message)
		walkMessages(message.Messages(), visit)
	}
}

func fixtureValue(field protoreflect.FieldDescriptor) protoreflect.Value {
	switch field.Kind() {
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return protoreflect.ValueOfMessage(dynamicpb.NewMessage(field.Message()))
	case protoreflect.StringKind:
		return protoreflect.ValueOfString("fixture")
	case protoreflect.BoolKind:
		return protoreflect.ValueOfBool(true)
	case protoreflect.DoubleKind, protoreflect.FloatKind:
		return protoreflect.ValueOfFloat64(1)
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return protoreflect.ValueOfInt32(1)
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return protoreflect.ValueOfInt64(1)
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return protoreflect.ValueOfUint32(1)
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return protoreflect.ValueOfUint64(1)
	case protoreflect.EnumKind:
		return protoreflect.ValueOfEnum(field.Enum().Values().Get(0).Number())
	default:
		panic("unsupported fixture field kind")
	}
}

func TestOptionalZeroAndStrictDecode(t *testing.T) {
	message := &CommandRequest{}
	field := message.ProtoReflect().Descriptor().Fields().ByName("expected_entity_revision")
	if message.ProtoReflect().Has(field) {
		t.Fatal("optional zero starts absent")
	}
	message.ProtoReflect().Set(field, protoreflect.ValueOfUint64(0))
	encoded, err := proto.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	decoded := &CommandRequest{}
	if err := UnmarshalStrict(encoded, decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.ProtoReflect().Has(field) || decoded.GetExpectedEntityRevision() != 0 {
		t.Fatal("present zero was lost")
	}
	unknown := protowire.AppendTag(encoded, 9999, protowire.VarintType)
	unknown = protowire.AppendVarint(unknown, 1)
	if err := UnmarshalStrict(unknown, &CommandRequest{}); err == nil {
		t.Fatal("unknown field accepted")
	}
	if err := UnmarshalStrict([]byte{0xff}, &CommandRequest{}); err == nil {
		t.Fatal("malformed bytes accepted")
	}
	if err := UnmarshalStrict([]byte{0x18, 0xe7, 0x07}, &CommandReply{}); err == nil {
		t.Fatal("unknown enum number accepted")
	}
	nested := &CommandRequest{Aggregate: &AggregateRef{}}
	nested.Aggregate.ProtoReflect().SetUnknown(protowire.AppendVarint(
		protowire.AppendTag(nil, 9999, protowire.VarintType), 1))
	nestedBytes, err := proto.Marshal(nested)
	if err != nil {
		t.Fatal(err)
	}
	if err := UnmarshalStrict(nestedBytes, &CommandRequest{}); err == nil {
		t.Fatal("nested unknown field accepted")
	}
}
