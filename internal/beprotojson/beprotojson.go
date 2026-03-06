package beprotojson

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

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
	if m == nil {
		return []byte("null"), nil
	}

	// Get message descriptor
	md := m.ProtoReflect()
	fields := md.Descriptor().Fields()

	// Find max field number to size our array
	maxFieldNum := 0
	for i := 0; i < fields.Len(); i++ {
		if num := int(fields.Get(i).Number()); num > maxFieldNum {
			maxFieldNum = num
		}
	}

	// Build array representation - batchexecute uses positional arrays
	result := make([]interface{}, maxFieldNum)

	// Iterate through all fields
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		fieldNum := int(field.Number()) - 1 // Convert to 0-indexed

		if md.Has(field) {
			value := md.Get(field)
			result[fieldNum] = o.marshalValue(field, value)
		} else {
			// Set appropriate defaults for unset fields
			if field.IsList() {
				result[fieldNum] = []interface{}{}
			} else if field.Kind() == protoreflect.MessageKind {
				result[fieldNum] = []interface{}{}
			}
		}
	}

	return json.Marshal(result)
}

// marshalValue converts a protobuf value to its batchexecute JSON representation
func (o MarshalOptions) marshalValue(fd protoreflect.FieldDescriptor, v protoreflect.Value) interface{} {
	if fd.IsList() {
		list := v.List()
		result := make([]interface{}, list.Len())
		for i := 0; i < list.Len(); i++ {
			result[i] = o.marshalSingleValue(fd, list.Get(i))
		}
		return result
	}
	return o.marshalSingleValue(fd, v)
}

// marshalSingleValue converts a single protobuf value
func (o MarshalOptions) marshalSingleValue(fd protoreflect.FieldDescriptor, v protoreflect.Value) interface{} {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		if v.Bool() {
			return 1
		}
		return 0
	case protoreflect.Int32Kind, protoreflect.Int64Kind,
		protoreflect.Sint32Kind, protoreflect.Sint64Kind,
		protoreflect.Sfixed32Kind, protoreflect.Sfixed64Kind:
		return v.Int()
	case protoreflect.Uint32Kind, protoreflect.Uint64Kind,
		protoreflect.Fixed32Kind, protoreflect.Fixed64Kind:
		return v.Uint()
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return v.Float()
	case protoreflect.StringKind:
		return v.String()
	case protoreflect.BytesKind:
		return base64.StdEncoding.EncodeToString(v.Bytes())
	case protoreflect.EnumKind:
		return int(v.Enum())
	case protoreflect.MessageKind:
		msg := v.Message()
		if !msg.IsValid() {
			return nil
		}
		// Handle well-known types specially
		switch msg.Descriptor().FullName() {
		case "google.protobuf.StringValue":
			if msg.Has(msg.Descriptor().Fields().ByNumber(1)) {
				return msg.Get(msg.Descriptor().Fields().ByNumber(1)).String()
			}
		case "google.protobuf.Int32Value":
			if msg.Has(msg.Descriptor().Fields().ByNumber(1)) {
				return int(msg.Get(msg.Descriptor().Fields().ByNumber(1)).Int())
			}
		case "google.protobuf.Timestamp":
			var seconds, nanos int64
			if f := msg.Descriptor().Fields().ByNumber(1); msg.Has(f) {
				seconds = msg.Get(f).Int()
			}
			if f := msg.Descriptor().Fields().ByNumber(2); msg.Has(f) {
				nanos = msg.Get(f).Int()
			}
			return []interface{}{seconds, nanos}
		default:
			// Recursively marshal nested message
			if nestedBytes, err := o.Marshal(msg.Interface()); err == nil {
				var result interface{}
				if err := json.Unmarshal(nestedBytes, &result); err == nil {
					return result
				}
			}
			return []interface{}{}
		}
	}
	return nil
}

// UnmarshalOptions is a configurable JSON format parser.
type UnmarshalOptions struct {
	// DiscardUnknown indicates whether to discard unknown fields during parsing. (default: true)
	DiscardUnknown bool

	// AllowPartial indicates whether to allow partial messages during parsing.
	AllowPartial bool

	// DebugParsing enables detailed parsing debug output showing field mappings
	DebugParsing bool

	// DebugFieldMapping shows how JSON array positions map to protobuf fields
	DebugFieldMapping bool
}

var defaultUnmarshalOptions = UnmarshalOptions{
	DiscardUnknown: true,
}

// SetGlobalDebugOptions sets debug options for all beprotojson unmarshaling
func SetGlobalDebugOptions(debugParsing, debugFieldMapping bool) {
	defaultUnmarshalOptions.DebugParsing = debugParsing
	defaultUnmarshalOptions.DebugFieldMapping = debugFieldMapping
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

	// Handle double-wrapped arrays (common in batchexecute responses)
	// If the array has only one element and that element is also an array,
	// unwrap it once
	if len(arr) == 1 {
		if innerArr, ok := arr[0].([]interface{}); ok {
			arr = innerArr
		}
	}

	return o.populateMessage(arr, m)
}

func (o UnmarshalOptions) populateMessage(arr []interface{}, m proto.Message) error {
	msg := m.ProtoReflect()
	fields := msg.Descriptor().Fields()

	if o.DebugParsing {
		fmt.Fprintf(os.Stderr,"\n=== BEPROTOJSON PARSING ===\n")
		fmt.Fprintf(os.Stderr,"Message Type: %s\n", msg.Descriptor().FullName())
		fmt.Fprintf(os.Stderr,"Array Length: %d\n", len(arr))
		fmt.Fprintf(os.Stderr,"Available Fields: %d\n", fields.Len())
	}

	if o.DebugFieldMapping {
		fmt.Fprintf(os.Stderr,"\n=== FIELD MAPPING ===\n")
		for i := 0; i < fields.Len(); i++ {
			field := fields.Get(i)
			fmt.Fprintf(os.Stderr,"Field #%d: %s (%s)\n", field.Number(), field.Name(), field.Kind())
		}
		fmt.Fprintf(os.Stderr,"\n=== ARRAY MAPPING ===\n")
	}

	for i, value := range arr {
		if o.DebugFieldMapping {
			fmt.Fprintf(os.Stderr,"Position %d: ", i)
		}

		if value == nil {
			if o.DebugFieldMapping {
				fmt.Fprintf(os.Stderr,"null (skipped)\n")
			}
			continue
		}

		field := fields.ByNumber(protoreflect.FieldNumber(i + 1))
		if field == nil {
			if o.DebugFieldMapping {
				fmt.Fprintf(os.Stderr,"NO FIELD (position %d) -> value: %v\n", i+1, value)
			}
			if !o.DiscardUnknown {
				return fmt.Errorf("beprotojson: no field for position %d", i+1)
			}
			continue
		}

		if o.DebugFieldMapping {
			fmt.Fprintf(os.Stderr,"maps to field #%d %s (%s) -> value: %v\n",
				field.Number(), field.Name(), field.Kind(), value)
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
		// Handle special cases where API returns non-array values for repeated fields
		switch val.(type) {
		case nil:
			// Null value - leave the repeated field empty
			return nil
		case bool:
			// Boolean value - could indicate empty/disabled state, leave empty
			return nil
		case float64:
			// Handle special case where API returns a number instead of array for repeated fields
			// This typically represents an empty array or special condition
			// For now, treat any number as an indicator of empty array to be more forgiving
			return nil
		default:
			return fmt.Errorf("expected array for repeated field, got %T", val)
		}
	}

	// Special handling for repeated message fields
	if len(arr) > 0 && fd.Message() != nil {
		list := m.Mutable(fd).List()

		// Check if this is a double-nested array (like sources field)
		if _, isNestedArray := arr[0].([]interface{}); isNestedArray {
			// Pattern: [[[item1_data], [item2_data], ...]]
			// Each item in arr should be an array representing a message
			for _, item := range arr {
				if itemArr, ok := item.([]interface{}); ok {
					// Create a new message instance
					msgType, err := protoregistry.GlobalTypes.FindMessageByName(fd.Message().FullName())
					if err != nil {
						return fmt.Errorf("failed to find message type %q: %v", fd.Message().FullName(), err)
					}
					msg := msgType.New().Interface()

					// Populate the message from the array
					if err := o.populateMessage(itemArr, msg); err != nil {
						return fmt.Errorf("failed to populate message: %w", err)
					}

					list.Append(protoreflect.ValueOfMessage(msg.ProtoReflect()))
				}
			}
			return nil
		} else {
			// Pattern: [[item1_data, item2_data, item3_data, ...]]
			// The entire arr represents a list of messages, treating each as a message
			// This is for cases like ListRecentlyViewedProjects where projects are directly in sequence
			msgType, err := protoregistry.GlobalTypes.FindMessageByName(fd.Message().FullName())
			if err != nil {
				return fmt.Errorf("failed to find message type %q: %v", fd.Message().FullName(), err)
			}

			// Group consecutive elements that belong to the same message
			// For now, let's try parsing each individual element as a message
			for _, item := range arr {
				if itemArr, ok := item.([]interface{}); ok {
					// Each item is an array representing a Project message
					msg := msgType.New().Interface()
					if err := o.populateMessage(itemArr, msg); err != nil {
						return fmt.Errorf("failed to populate message: %w", err)
					}
					list.Append(protoreflect.ValueOfMessage(msg.ProtoReflect()))
				}
			}
			return nil
		}
	}

	list := m.Mutable(fd).List()
	for _, item := range arr {
		if err := o.appendToList(list, fd, item); err != nil {
			return err
		}
	}
	return nil
}

// isEmptyArrayCode checks if a value represents an empty array code from the NotebookLM API
func isEmptyArrayCode(val interface{}) bool {
	if num, isNum := val.(float64); isNum {
		// For NotebookLM API, certain numbers represent empty arrays
		// [16] represents an empty project list
		// Add other codes here as we discover them
		return num == 16
	}
	return false
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

// UnmarshalArray attempts to parse a JSON array string that may have trailing digits
// This is used specifically for handling the error: "cannot unmarshal object into Go value of type []interface {}"
func UnmarshalArray(data string) ([]interface{}, error) {
	// Clean the data by removing trailing digits
	data = cleanTrailingDigits(data)

	// Try standard unmarshaling first
	var result []interface{}
	err := json.Unmarshal([]byte(data), &result)
	if err == nil {
		return result, nil
	}

	// Try to extract just the array part if unmarshaling fails
	arrayPattern := regexp.MustCompile(`\[\[.*?\]\]`)
	matches := arrayPattern.FindString(data)
	if matches != "" {
		err = json.Unmarshal([]byte(matches), &result)
		if err == nil {
			return result, nil
		}
	}

	// Try to find a balanced bracket structure
	start := strings.Index(data, "[[")
	if start >= 0 {
		// Find matching end brackets
		bracketCount := 0
		end := start
		for i := start; i < len(data); i++ {
			if data[i] == '[' {
				bracketCount++
			} else if data[i] == ']' {
				bracketCount--
				if bracketCount == 0 {
					end = i + 1
					break
				}
			}
		}

		if end > start {
			extracted := data[start:end]
			err = json.Unmarshal([]byte(extracted), &result)
			if err == nil {
				return result, nil
			}
		}
	}

	return nil, fmt.Errorf("failed to unmarshal array: %w", err)
}

// cleanTrailingDigits removes any trailing digits that might appear after valid JSON
func cleanTrailingDigits(data string) string {
	// First check if the data ends with a closing bracket
	if len(data) > 0 && data[len(data)-1] == ']' {
		return data
	}

	// Find the last valid JSON character (closing bracket)
	for i := len(data) - 1; i >= 0; i-- {
		if data[i] == ']' {
			return data[:i+1]
		}
	}

	return data
}

func (o UnmarshalOptions) setMessageField(m protoreflect.Message, fd protoreflect.FieldDescriptor, val interface{}) error {
	if o.DebugParsing {
		fmt.Fprintf(os.Stderr,"  -> Parsing nested message: %s\n", fd.Message().FullName())
	}

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

		if o.DebugFieldMapping {
			fmt.Fprintf(os.Stderr,"    Nested message %s has %d array elements\n",
				fd.Message().FullName(), len(v))
		}

		// Populate fields from array
		fields := msgReflect.Descriptor().Fields()
		for i := 0; i < len(v); i++ {
			if v[i] == nil {
				if o.DebugFieldMapping {
					fmt.Fprintf(os.Stderr,"    Position %d: null (skipped)\n", i)
				}
				continue
			}

			fieldNum := protoreflect.FieldNumber(i + 1)
			field := fields.ByNumber(fieldNum)
			if field == nil {
				if o.DebugFieldMapping {
					fmt.Fprintf(os.Stderr,"    Position %d: NO FIELD -> value: %v\n", i, v[i])
				}
				if !o.DiscardUnknown {
					return fmt.Errorf("no field for position %d", i+1)
				}
				continue
			}

			if o.DebugFieldMapping {
				fmt.Fprintf(os.Stderr,"    Position %d: maps to field #%d %s (%s) -> value: %v\n",
					i, field.Number(), field.Name(), field.Kind(), v[i])
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

	case string:
		// Handle string values that might be intended for message fields
		// This can happen when the API returns a string ID instead of a nested object
		// Set the first field of the message to this string value
		fields := msgReflect.Descriptor().Fields()
		if fields.Len() > 0 {
			firstField := fields.Get(0)
			if firstField.Kind() == protoreflect.StringKind {
				msgReflect.Set(firstField, protoreflect.ValueOfString(v))
				m.Set(fd, protoreflect.ValueOfMessage(msgReflect))
				return nil
			}
		}
		// If we can't find a compatible field, just create an empty message
		// This handles cases where the API response format doesn't match the protobuf structure
		m.Set(fd, protoreflect.ValueOfMessage(msgReflect))
		return nil
	case float64:
		// Handle numeric values that might be intended for message fields
		// This can happen when the API returns a number instead of a nested object
		// Set the first field of the message to this numeric value
		fields := msgReflect.Descriptor().Fields()
		if fields.Len() > 0 {
			firstField := fields.Get(0)
			switch firstField.Kind() {
			case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
				msgReflect.Set(firstField, protoreflect.ValueOfInt32(int32(v)))
				m.Set(fd, protoreflect.ValueOfMessage(msgReflect))
				return nil
			case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
				msgReflect.Set(firstField, protoreflect.ValueOfInt64(int64(v)))
				m.Set(fd, protoreflect.ValueOfMessage(msgReflect))
				return nil
			case protoreflect.FloatKind:
				msgReflect.Set(firstField, protoreflect.ValueOfFloat32(float32(v)))
				m.Set(fd, protoreflect.ValueOfMessage(msgReflect))
				return nil
			case protoreflect.DoubleKind:
				msgReflect.Set(firstField, protoreflect.ValueOfFloat64(v))
				m.Set(fd, protoreflect.ValueOfMessage(msgReflect))
				return nil
			}
		}
		// If we can't find a compatible field, just create an empty message
		// This handles cases where the API response format doesn't match the protobuf structure
		m.Set(fd, protoreflect.ValueOfMessage(msgReflect))
		return nil
	default:
		// For any other scalar types passed to message fields, create an empty message
		// This is a fallback for API response format mismatches
		m.Set(fd, protoreflect.ValueOfMessage(msgReflect))
		return nil
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
		case float64:
			// Handle numeric values as strings (API might return numbers for some string fields)
			return protoreflect.ValueOfString(fmt.Sprintf("%v", v)), nil
		case bool:
			// Handle boolean values as strings
			return protoreflect.ValueOfString(fmt.Sprintf("%v", v)), nil
		case nil:
			// Handle null values as empty strings
			return protoreflect.ValueOfString(""), nil
		case []interface{}:
			// Handle nested arrays by recursively looking for a string
			if len(v) > 0 {
				switch first := v[0].(type) {
				case string:
					return protoreflect.ValueOfString(first), nil
				case float64:
					// Handle numbers in arrays as strings
					return protoreflect.ValueOfString(fmt.Sprintf("%v", first)), nil
				case bool:
					// Handle booleans in arrays as strings
					return protoreflect.ValueOfString(fmt.Sprintf("%v", first)), nil
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
		case float64:
			// Convert numbers to booleans (0 = false, non-zero = true)
			return protoreflect.ValueOfBool(v != 0), nil
		case int64:
			return protoreflect.ValueOfBool(v != 0), nil
		case string:
			// Convert string booleans
			switch v {
			case "true", "True", "TRUE", "1":
				return protoreflect.ValueOfBool(true), nil
			case "false", "False", "FALSE", "0":
				return protoreflect.ValueOfBool(false), nil
			default:
				return protoreflect.Value{}, fmt.Errorf("cannot convert string %q to bool", v)
			}
		case nil:
			// Null values become false
			return protoreflect.ValueOfBool(false), nil
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
		case []interface{}:
			// Handle arrays passed to enum fields - use first element or default to 0
			if len(v) > 0 {
				// Recursively try to convert the first element
				return o.convertValue(fd, v[0])
			}
			// Empty array defaults to 0
			return protoreflect.ValueOfEnum(0), nil
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
