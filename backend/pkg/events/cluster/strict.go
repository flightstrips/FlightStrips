package cluster

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// UnmarshalStrict rejects unknown fields and enum numbers at every depth.
// Domain-specific required fields and identifiers are validated by the owner.
func UnmarshalStrict(data []byte, message proto.Message) error {
	if err := (proto.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(data, message); err != nil {
		return err
	}
	return validateFields(message.ProtoReflect())
}

func validateFields(message protoreflect.Message) error {
	if len(message.GetUnknown()) != 0 {
		return fmt.Errorf("unknown fields in %s", message.Descriptor().FullName())
	}
	var failure error
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		check := func(item protoreflect.Value) error {
			switch field.Kind() {
			case protoreflect.EnumKind:
				if field.Enum().Values().ByNumber(item.Enum()) == nil {
					return fmt.Errorf("unknown enum %d in %s", item.Enum(), field.FullName())
				}
			case protoreflect.MessageKind, protoreflect.GroupKind:
				return validateFields(item.Message())
			}
			return nil
		}
		if field.IsList() {
			list := value.List()
			for i := 0; i < list.Len(); i++ {
				if failure = check(list.Get(i)); failure != nil {
					return false
				}
			}
		} else if field.IsMap() {
			value.Map().Range(func(_ protoreflect.MapKey, item protoreflect.Value) bool {
				failure = check(item)
				return failure == nil
			})
		} else {
			failure = check(value)
		}
		return failure == nil
	})
	return failure
}
