package beprotojson

import (
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// MarshalOptions is a configurable JSON format marshaler.
type MarshalOptions struct {
}

// Marshal writes the given proto.Message in batchexecute JSON format.
func Marshal(m proto.Message) ([]byte, error) {
	return MarshalOptions{}.Marshal(m)
}

// Marshal writes the given proto.Message in batchexecute JSON format using options in MarshalOptions.
func (o MarshalOptions) Marshal(m proto.Message) ([]byte, error) {
	// TODO: implement
	return nil, fmt.Errorf("not implemented")
}

// UnmarshalOptions is a configurable JSON format parser.
type UnmarshalOptions struct {
	// DiscardUnknown indicates whether to discard unknown fields during parsing. (default: true)
	DiscardUnknown bool

	// AllowPartial indicates whether to allow partial messages during parsing.
	AllowPartial bool
}

var defaultUnmarshalOptions = UnmarshalOptions{
	DiscardUnknown: true,
}

// Unmarshal reads the given batchexecute JSON data into the given proto.Message.
func Unmarshal(b []byte, m proto.Message) error {
	return defaultUnmarshalOptions.Unmarshal(b, m)
}

// Unmarshal reads the given batchexecute JSON data into the given proto.Message using options in UnmarshalOptions.
func (o UnmarshalOptions) Unmarshal(b []byte, m proto.Message) error {
	var arr []interface{}
	if err := json.Unmarshal(b, &arr); err != nil {
		return fmt.Errorf("beprotojson: invalid JSON array: %w", err)
	}

	return o.populateMessage(arr, m)
}

func (o UnmarshalOptions) populateMessage(arr []interface{}, m proto.Message) error {
	msg := m.ProtoReflect()
	fields := msg.Descriptor().Fields()

	for i, value := range arr {
		if value == nil {
			continue
		}

		field := fields.ByNumber(protoreflect.FieldNumber(i + 1))
		if field == nil {
			if !o.DiscardUnknown {
				return fmt.Errorf("beprotojson: no field for position %d", i+1)
			}
			continue
		}

		if err := o.setField(msg, field, value); err != nil {
			return fmt.Errorf("beprotojson: field %s: %w", field.Name(), err)
		}
	}

	if !o.AllowPartial {
		if err := proto.CheckInitialized(m); err != nil {
			return fmt.Errorf("beprotojson: %v", err)
		}
	}

	return nil
}

func (o UnmarshalOptions) setField(m protoreflect.Message, fd protoreflect.FieldDescriptor, val interface{}) error {
	switch {
	case fd.IsList():
		return o.setRepeatedField(m, fd, val)
	case fd.Message() != nil:
		return o.setMessageField(m, fd, val)
	default:
		return o.setScalarField(m, fd, val)
	}
}

func (o UnmarshalOptions) setRepeatedField(m protoreflect.Message, fd protoreflect.FieldDescriptor, val interface{}) error {
	arr, ok := val.([]interface{})
	if !ok {
		return fmt.Errorf("expected array for repeated field, got %T", val)
	}

	list := m.Mutable(fd).List()
	for _, item := range arr {
		if err := o.appendToList(list, fd, item); err != nil {
			return err
		}
	}
	return nil
}

func (o UnmarshalOptions) appendToList(list protoreflect.List, fd protoreflect.FieldDescriptor, val interface{}) error {
	if fd.Message() != nil {
		// Get the concrete message type from the registry
		msgType, err := protoregistry.GlobalTypes.FindMessageByName(fd.Message().FullName())
		if err != nil {
			return fmt.Errorf("failed to find message type %q: %v", fd.Message().FullName(), err)
		}

		msg := msgType.New().Interface()
		msgReflect := msg.ProtoReflect()

		switch v := val.(type) {
		case []interface{}:
			// If this is a nested array structure representing a single value,
			// flatten it to get the actual value
			flatVal := flattenSingleValueArray(v)
			if !isArray(flatVal) {
				if err := o.setField(msgReflect, msgReflect.Descriptor().Fields().ByNumber(1), flatVal); err != nil {
					return err
				}
			} else if arr, ok := flatVal.([]interface{}); ok {
				if err := o.populateMessage(arr, msg); err != nil {
					return err
				}
			}
			list.Append(protoreflect.ValueOfMessage(msgReflect))
			return nil
		default:
			return fmt.Errorf("expected array for message field, got %T", val)
		}
	}

	v, err := o.convertValue(fd, val)
	if err != nil {
		return err
	}
	list.Append(v)
	return nil
}

// flattenSingleValueArray recursively flattens nested arrays that represent a single value
func flattenSingleValueArray(arr []interface{}) interface{} {
	if len(arr) != 1 {
		return arr
	}

	switch v := arr[0].(type) {
	case []interface{}:
		return flattenSingleValueArray(v)
	default:
		return v
	}
}

// isArray checks if an interface{} value is an array
func isArray(val interface{}) bool {
	_, ok := val.([]interface{})
	return ok
}

func (o UnmarshalOptions) setMessageField(m protoreflect.Message, fd protoreflect.FieldDescriptor, val interface{}) error {
	msgType, err := protoregistry.GlobalTypes.FindMessageByName(fd.Message().FullName())
	if err != nil {
		return fmt.Errorf("failed to find message type %q: %v", fd.Message().FullName(), err)
	}

	msg := msgType.New().Interface()
	msgReflect := msg.ProtoReflect()

	switch v := val.(type) {
	case []interface{}:
		// Handle nil or empty arrays
		if len(v) == 0 {
			m.Set(fd, protoreflect.ValueOfMessage(msgReflect))
			return nil
		}

		// Populate fields from array
		fields := msgReflect.Descriptor().Fields()
		for i := 0; i < len(v); i++ {
			if v[i] == nil {
				continue
			}

			fieldNum := protoreflect.FieldNumber(i + 1)
			field := fields.ByNumber(fieldNum)
			if field == nil {
				if !o.DiscardUnknown {
					return fmt.Errorf("no field for position %d", i+1)
				}
				continue
			}

			// For wrapper types, handle the value directly
			if field.Message() != nil && isWrapperType(field.Message().FullName()) {
				wrapperType, err := protoregistry.GlobalTypes.FindMessageByName(field.Message().FullName())
				if err != nil {
					return fmt.Errorf("failed to find wrapper type %q: %v", field.Message().FullName(), err)
				}

				wrapperMsg := wrapperType.New()
				valueField := field.Message().Fields().ByName("value")
				if valueField != nil {
					if val, err := o.convertValue(valueField, v[i]); err == nil {
						wrapperMsg.Set(valueField, val)
						msgReflect.Set(field, protoreflect.ValueOfMessage(wrapperMsg))
						continue
					}
				}
			}

			if err := o.setField(msgReflect, field, v[i]); err != nil {
				return fmt.Errorf("field %s: %w", field.FullName(), err)
			}
		}
		m.Set(fd, protoreflect.ValueOfMessage(msgReflect))
		return nil

	default:
		return fmt.Errorf("expected array or map for message field, got %T", val)
	}
}

func isWrapperType(name protoreflect.FullName) bool {
	switch name {
	case "google.protobuf.Int32Value",
		"google.protobuf.Int64Value",
		"google.protobuf.UInt32Value",
		"google.protobuf.UInt64Value",
		"google.protobuf.FloatValue",
		"google.protobuf.DoubleValue",
		"google.protobuf.BoolValue",
		"google.protobuf.StringValue",
		"google.protobuf.BytesValue":
		return true
	}
	return false
}

func (o UnmarshalOptions) setScalarField(m protoreflect.Message, fd protoreflect.FieldDescriptor, val interface{}) error {
	v, err := o.convertValue(fd, val)
	if err != nil {
		return err
	}
	m.Set(fd, v)
	return nil
}

func (o UnmarshalOptions) convertValue(fd protoreflect.FieldDescriptor, val interface{}) (protoreflect.Value, error) {
	switch fd.Kind() {
	case protoreflect.StringKind:
		switch v := val.(type) {
		case string:
			return protoreflect.ValueOfString(v), nil
		case []interface{}:
			// Handle nested arrays by recursively looking for a string
			if len(v) > 0 {
				switch first := v[0].(type) {
				case string:
					return protoreflect.ValueOfString(first), nil
				case []interface{}:
					// Recursively unwrap arrays until we find a string
					if converted, err := o.convertValue(fd, first); err == nil {
						return converted, nil
					}
				}
			}
			return protoreflect.Value{}, fmt.Errorf("expected string, got %T", val)
		default:
			return protoreflect.Value{}, fmt.Errorf("expected string, got %T", val)
		}

	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		switch v := val.(type) {
		case float64:
			return protoreflect.ValueOfInt32(int32(v)), nil
		case int64:
			return protoreflect.ValueOfInt32(int32(v)), nil
		case int32:
			return protoreflect.ValueOfInt32(v), nil
		default:
			return protoreflect.Value{}, fmt.Errorf("expected number, got %T", val)
		}

	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		switch v := val.(type) {
		case float64:
			return protoreflect.ValueOfInt64(int64(v)), nil
		case int64:
			return protoreflect.ValueOfInt64(v), nil
		case int32:
			return protoreflect.ValueOfInt64(int64(v)), nil
		default:
			return protoreflect.Value{}, fmt.Errorf("expected number, got %T", val)
		}

	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		switch v := val.(type) {
		case float64:
			return protoreflect.ValueOfUint32(uint32(v)), nil
		case int64:
			return protoreflect.ValueOfUint32(uint32(v)), nil
		case uint32:
			return protoreflect.ValueOfUint32(v), nil
		default:
			return protoreflect.Value{}, fmt.Errorf("expected number, got %T", val)
		}

	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		switch v := val.(type) {
		case float64:
			return protoreflect.ValueOfUint64(uint64(v)), nil
		case int64:
			return protoreflect.ValueOfUint64(uint64(v)), nil
		case uint64:
			return protoreflect.ValueOfUint64(v), nil
		default:
			return protoreflect.Value{}, fmt.Errorf("expected number, got %T", val)
		}

	case protoreflect.FloatKind:
		switch v := val.(type) {
		case float64:
			return protoreflect.ValueOfFloat32(float32(v)), nil
		case float32:
			return protoreflect.ValueOfFloat32(v), nil
		default:
			return protoreflect.Value{}, fmt.Errorf("expected float, got %T", val)
		}

	case protoreflect.DoubleKind:
		switch v := val.(type) {
		case float64:
			return protoreflect.ValueOfFloat64(v), nil
		case float32:
			return protoreflect.ValueOfFloat64(float64(v)), nil
		default:
			return protoreflect.Value{}, fmt.Errorf("expected float, got %T", val)
		}

	case protoreflect.BoolKind:
		switch v := val.(type) {
		case bool:
			return protoreflect.ValueOfBool(v), nil
		default:
			return protoreflect.Value{}, fmt.Errorf("expected bool, got %T", val)
		}

	case protoreflect.EnumKind:
		switch v := val.(type) {
		case float64:
			return protoreflect.ValueOfEnum(protoreflect.EnumNumber(v)), nil
		case int64:
			return protoreflect.ValueOfEnum(protoreflect.EnumNumber(v)), nil
		case int32:
			return protoreflect.ValueOfEnum(protoreflect.EnumNumber(v)), nil
		case string:
			// Look up enum value by name
			if enumVal := fd.Enum().Values().ByName(protoreflect.Name(v)); enumVal != nil {
				return protoreflect.ValueOfEnum(enumVal.Number()), nil
			}
			return protoreflect.Value{}, fmt.Errorf("unknown enum value %q", v)
		default:
			return protoreflect.Value{}, fmt.Errorf("expected number or string for enum, got %T", val)
		}

	case protoreflect.BytesKind:
		switch v := val.(type) {
		case string:
			return protoreflect.ValueOfBytes([]byte(v)), nil
		case []byte:
			return protoreflect.ValueOfBytes(v), nil
		default:
			return protoreflect.Value{}, fmt.Errorf("expected string or bytes, got %T", val)
		}

	default:
		return protoreflect.Value{}, fmt.Errorf("unsupported field kind %v", fd.Kind())
	}
}
