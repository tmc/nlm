package beprotojson

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

type marshaler struct{}

// Marshal writes the given proto.Message in batchexecute JSON format.
func Marshal(m proto.Message) ([]byte, error) {
	return marshaler{}.marshal(m)
}

func (o marshaler) marshal(m proto.Message) ([]byte, error) {
	if m == nil || !m.ProtoReflect().IsValid() {
		return []byte("null"), nil
	}

	// Get message descriptor
	md := m.ProtoReflect()
	fields := md.Descriptor().Fields()

	// Shape unions marshal to the bare value of whichever case is set, at the
	// enclosing field's position — not to a positional array padded to the
	// case's field number. This mirrors setShapeUnion on the decode side.
	if oneof := shapeUnion(md.Descriptor()); oneof != nil {
		if which := md.WhichOneof(oneof); which != nil {
			return json.Marshal(o.marshalValue(which, md.Get(which)))
		}
		return []byte("null"), nil
	}

	// Object-encoded messages marshal to a sparse JSON object keyed by field
	// number, e.g. {"101": "•", "102": 1}. This mirrors setObjectMessage.
	if messageBoolOption(md.Descriptor(), extObjectEncoded) {
		obj := make(map[string]interface{})
		for i := 0; i < fields.Len(); i++ {
			field := fields.Get(i)
			if md.Has(field) {
				obj[strconv.Itoa(int(field.Number()))] = o.marshalValue(field, md.Get(field))
			}
		}
		return json.Marshal(obj)
	}

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
		}
		// Unset fields remain nil (JSON null) to match batchexecute protocol
	}
	if messageBoolOption(md.Descriptor(), extOmitTrailingNulls) {
		for len(result) > 0 && result[len(result)-1] == nil {
			result = result[:len(result)-1]
		}
	}

	return json.Marshal(result)
}

// marshalValue converts a protobuf value to its batchexecute JSON representation
func (o marshaler) marshalValue(fd protoreflect.FieldDescriptor, v protoreflect.Value) interface{} {
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
func (o marshaler) marshalSingleValue(fd protoreflect.FieldDescriptor, v protoreflect.Value) interface{} {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		// A field tagged (json_bool) is carried as a JSON boolean; the default
		// batchexecute encoding is 1/0.
		if fieldBoolOption(fd, extJSONBool) {
			return v.Bool()
		}
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
		fields := msg.Descriptor().Fields()
		if flatListMessage(msg.Descriptor()) && fields.Len() == 1 {
			field := fields.Get(0)
			if field.IsList() {
				return o.marshalValue(field, msg.Get(field))
			}
		}
		// Handle well-known types specially. A wrapper's presence carries the
		// value, so a present wrapper emits its value even when that value is
		// the zero value (Get returns the zero value when the inner field is
		// unset) — gating on Has() would drop a legitimate 0/"" back to null.
		switch msg.Descriptor().FullName() {
		case "google.protobuf.StringValue":
			return msg.Get(msg.Descriptor().Fields().ByNumber(1)).String()
		case "google.protobuf.Int32Value":
			return int(msg.Get(msg.Descriptor().Fields().ByNumber(1)).Int())
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
			if nestedBytes, err := o.marshal(msg.Interface()); err == nil {
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

// Unmarshal reads the given batchexecute JSON data into the given proto.Message.
func Unmarshal(b []byte, m proto.Message) error {
	return (UnmarshalOptions{DiscardUnknown: true}).Unmarshal(b, m)
}

// Unmarshal reads the given batchexecute JSON data into the given proto.Message using options in UnmarshalOptions.
func (o UnmarshalOptions) Unmarshal(b []byte, m proto.Message) error {
	var arr []interface{}
	if err := json.Unmarshal(b, &arr); err != nil {
		return fmt.Errorf("beprotojson: invalid JSON array: %w", err)
	}

	// Handle response format detection.
	// Batchexecute responses come in two main formats:
	//
	// 1. Positional field array: [field1_val, field2_val, ...]
	//    Maps directly to proto field numbers: position 0 → field #1, etc.
	//
	// 2. Flat repeated array: [[item1], [item2], ...]
	//    When the message has a single repeated field and the top-level array
	//    contains multiple sub-arrays, the entire array is the repeated field value.
	//
	// Detect case 2: if the message has only one field, it's a repeated message field,
	// and all top-level elements are arrays, wrap into a positional array so that
	// position 0 maps to field #1.
	msg := m.ProtoReflect()
	fields := msg.Descriptor().Fields()

	// Handle response format detection for repeated-only messages.
	//
	// Batchexecute responses for messages with a single repeated message field
	// can come in two formats:
	//
	// 1. Positional: [[[item1], [item2], ...]]
	//    Outer [] = message, position 0 = field #1 (the repeated field)
	//
	// 2. Flat: [[item1], [item2], ...]
	//    No message wrapper — items are directly at the top level
	//
	// Both need the items to end up as the value for field #1.
	// The old unwrap logic would incorrectly strip the positional wrapper in
	// case 1, treating each item as a field position instead of a list element.
	if fields.Len() == 1 {
		fd := fields.Get(0)
		if fd.IsList() && fd.Message() != nil {
			if len(arr) == 1 {
				// Case 1: [[[item1], [item2], ...]]
				// Already in positional format — don't unwrap.
				// Fall through to populateMessage which handles it correctly.
			} else if len(arr) > 1 {
				// Case 2: [[item1], [item2], ...]
				// Flat format — wrap into positional format.
				allArrays := true
				for _, v := range arr {
					if _, ok := v.([]interface{}); !ok {
						allArrays = false
						break
					}
				}
				if allArrays {
					arr = []interface{}{arr}
				}
			}
			return o.populateMessage(arr, m)
		}
	}

	// Handle double-wrapped arrays (common in batchexecute responses).
	// If the array has only one element and that element is also an array,
	// unwrap it once — but only when the inner array is NOT a list of items
	// for a repeated field at position 0.
	//
	// Example: [[[p1],[p2],...]] where field 1 is repeated should NOT unwrap,
	// because the outer [] is the positional wrapper and position 0 holds the
	// repeated field value. Unwrapping would misinterpret each list element
	// as a separate field position.
	if len(arr) == 1 {
		if innerArr, ok := arr[0].([]interface{}); ok {
			fd := fields.ByNumber(1)
			if fd != nil && fd.IsList() && fd.Message() != nil {
				// Keep positional format: position 0 = repeated field value.
			} else if fd != nil && fd.Message() != nil && flatListMessage(fd.Message()) {
				// Keep the positional wrapper for a singular flat-list message.
				// The inner empty list is field 1's value, not an extra envelope.
			} else if fd != nil && fd.Message() != nil && len(innerArr) == 1 {
				inner := fd.Message().Fields().ByNumber(1)
				if inner != nil && inner.IsList() && inner.Message() != nil {
					// Keep the wrapper needed by a singular message whose first
					// field is a repeated message. The inner array is the singular
					// field value, not another response envelope.
				} else {
					arr = innerArr
				}
			} else {
				arr = innerArr
			}
		}
	}

	return o.populateMessage(arr, m)
}

func (o UnmarshalOptions) populateMessage(arr []interface{}, m proto.Message) error {
	msg := m.ProtoReflect()
	fields := msg.Descriptor().Fields()

	if o.DebugParsing {
		fmt.Printf("\n=== BEPROTOJSON PARSING ===\n")
		fmt.Printf("Message Type: %s\n", msg.Descriptor().FullName())
		fmt.Printf("Array Length: %d\n", len(arr))
		fmt.Printf("Available Fields: %d\n", fields.Len())
	}

	if o.DebugFieldMapping {
		fmt.Printf("\n=== FIELD MAPPING ===\n")
		for i := 0; i < fields.Len(); i++ {
			field := fields.Get(i)
			fmt.Printf("Field #%d: %s (%s)\n", field.Number(), field.Name(), field.Kind())
		}
		fmt.Printf("\n=== ARRAY MAPPING ===\n")
	}

	for i, value := range arr {
		if o.DebugFieldMapping {
			fmt.Printf("Position %d: ", i)
		}

		if value == nil {
			if o.DebugFieldMapping {
				fmt.Printf("null (skipped)\n")
			}
			continue
		}

		field := fields.ByNumber(protoreflect.FieldNumber(i + 1))
		if field == nil {
			if o.DebugFieldMapping {
				fmt.Printf("NO FIELD (position %d) -> value: %v\n", i+1, value)
			}
			if !o.DiscardUnknown {
				return fmt.Errorf("beprotojson: no field for position %d", i+1)
			}
			continue
		}

		if o.DebugFieldMapping {
			fmt.Printf("maps to field #%d %s (%s) -> value: %v\n",
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
		case string:
			// Server sometimes returns a string where a repeated field is expected.
			// Treat as a single-element array of the appropriate type.
			return nil
		default:
			return fmt.Errorf("expected array for repeated field, got %T", val)
		}
	}

	// Special handling for repeated message fields
	if len(arr) > 0 && fd.Message() != nil {
		list := m.Mutable(fd).List()

		// A repeated shape-union element (e.g. SpanGroup.spans) can itself be
		// single-element on the wire, with the wrapping array elided the same
		// way any other single-element repeated field is. That elision is
		// ambiguous exactly when the lone element's own case is a message
		// with a trailing array field (e.g. a Span's [start, end, content]):
		// arr's raw form [start, end, content] then looks exactly like "N
		// top-level items" to the isNestedArray check below, instead of one
		// element. Disambiguate by checking whether arr, taken as a whole,
		// fits one union case before assuming it is several. A genuine
		// 1-element wrapper ([span]) does not accidentally match here: its
		// lone element is itself an array, which a message case's shapeMatches
		// rejects when checked against arr's own leading position (arr[0]),
		// so this shortcut naturally no-ops and falls through to unwrap arr[0]
		// via the isNestedArray path below instead.
		if oneof := shapeUnion(fd.Message()); oneof != nil && containsNestedArray(arr) {
			if union, matched, err := o.buildShapeUnion(fd.Message(), oneof, arr); err == nil && matched {
				list.Append(protoreflect.ValueOfMessage(union))
				return nil
			}
		}

		// Check if this is a double-nested array (like sources field)
		if _, isNestedArray := arr[0].([]interface{}); isNestedArray {
			// Pattern: [[[item1_data], [item2_data], ...]]
			// Each item in arr should be an array representing a message.
			// Route through appendToList so wrapped item shapes get the
			// same handling as the generic repeated-message path.
			for _, item := range arr {
				if err := o.appendToList(list, fd, item); err != nil {
					if o.DebugParsing {
						fmt.Printf("beprotojson: skipping item: %v\n", err)
					}
					continue
				}
			}
			return nil
		} else {
			// Pattern: [[item1_data, item2_data, item3_data, ...]]
			// The entire arr represents a list of messages, treating each as a message
			// This is for cases like ListRecentlyViewedProjects where projects are directly in sequence
			// Group consecutive elements that belong to the same message.
			// Route through appendToList for consistency with wrapped items.
			for _, item := range arr {
				if err := o.appendToList(list, fd, item); err != nil {
					if o.DebugParsing {
						fmt.Printf("beprotojson: skipping item: %v\n", err)
					}
					continue
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

// containsNestedArray reports whether any element of arr is itself an array,
// the signal setRepeatedField uses to tell a single Span-shaped
// ([start, end, content]) value apart from a genuine run of independent
// scalar leaves when deciding whether arr as a whole is one shape-union
// element.
func containsNestedArray(arr []interface{}) bool {
	for _, v := range arr {
		if _, ok := v.([]interface{}); ok {
			return true
		}
	}
	return false
}

func (o UnmarshalOptions) appendToList(list protoreflect.List, fd protoreflect.FieldDescriptor, val interface{}) error {
	if fd.Message() != nil {
		// Shape unions: a repeated field whose element type is itself a shape
		// union (e.g. SpanGroup.spans, which holds either a bare scalar or a
		// nested Span at each position) selects the case by the element's
		// shape, the same way a singular shape-union field does in
		// setMessageField. Handling this here — rather than falling through
		// to the positional decode below — matters because the positional
		// decode would otherwise misinterpret the union's message-case shape
		// as numbered fields of some other message.
		if oneof := shapeUnion(fd.Message()); oneof != nil {
			union, _, err := o.buildShapeUnion(fd.Message(), oneof, val)
			if err != nil {
				return err
			}
			list.Append(protoreflect.ValueOfMessage(union))
			return nil
		}

		// Get the concrete message type from the registry
		msgType, err := protoregistry.GlobalTypes.FindMessageByName(fd.Message().FullName())
		if err != nil {
			return fmt.Errorf("failed to find message type %q: %v", fd.Message().FullName(), err)
		}

		msg := msgType.New().Interface()
		msgReflect := msg.ProtoReflect()

		switch v := val.(type) {
		case []interface{}:
			fieldTwo := msgReflect.Descriptor().Fields().ByNumber(2)
			if len(v) == 2 && (fieldTwo == nil || fieldTwo.Message() == nil) {
				if nested, ok := v[1].([]interface{}); ok && len(nested) > 0 {
					if outerID, ok := v[0].(string); ok {
						if innerID, ok := nested[0].(string); ok && outerID == innerID {
							if err := o.populateMessage(nested, msg); err == nil {
								list.Append(protoreflect.ValueOfMessage(msgReflect))
								return nil
							}
						}
					}
				}
			}
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
	if o.DebugParsing {
		fmt.Printf("  -> Parsing nested message: %s\n", fd.Message().FullName())
	}

	// Shape unions: a message whose only fields are the members of a single
	// oneof represents a wire position that holds one of several alternative
	// shapes (a text leaf vs. a nested container, say). Such a message is not
	// populated positionally; instead the whole wire value selects one case by
	// its shape. See setShapeUnion.
	if oneof := shapeUnion(fd.Message()); oneof != nil {
		return o.setShapeUnion(m, fd, oneof, val)
	}

	msgType, err := protoregistry.GlobalTypes.FindMessageByName(fd.Message().FullName())
	if err != nil {
		return fmt.Errorf("failed to find message type %q: %v", fd.Message().FullName(), err)
	}

	// Object-encoded messages arrive as a JSON object keyed by field number
	// ({"101": "•", ...}) rather than a positional array.
	if obj, ok := val.(map[string]interface{}); ok && messageBoolOption(fd.Message(), extObjectEncoded) {
		return o.setObjectMessage(m, fd, msgType, obj)
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
			fmt.Printf("    Nested message %s has %d array elements\n",
				fd.Message().FullName(), len(v))
		}

		// Populate fields from array
		fields := msgReflect.Descriptor().Fields()
		// Some one-field repeated-value messages are carried without their
		// usual positional wrapper. Treat the whole value as field 1 before
		// walking it positionally.
		if flatListMessage(msgReflect.Descriptor()) && len(v) > 0 && !isArray(v[0]) && fields.Len() == 1 {
			field := fields.Get(0)
			if field.IsList() {
				if err := o.setRepeatedField(msgReflect, field, v); err != nil {
					return fmt.Errorf("field %s: %w", field.FullName(), err)
				}
				m.Set(fd, protoreflect.ValueOfMessage(msgReflect))
				return nil
			}
		}
		for i := 0; i < len(v); i++ {
			if v[i] == nil {
				if o.DebugFieldMapping {
					fmt.Printf("    Position %d: null (skipped)\n", i)
				}
				continue
			}

			fieldNum := protoreflect.FieldNumber(i + 1)
			field := fields.ByNumber(fieldNum)
			if field == nil {
				if o.DebugFieldMapping {
					fmt.Printf("    Position %d: NO FIELD -> value: %v\n", i, v[i])
				}
				if !o.DiscardUnknown {
					return fmt.Errorf("no field for position %d", i+1)
				}
				continue
			}

			if o.DebugFieldMapping {
				fmt.Printf("    Position %d: maps to field #%d %s (%s) -> value: %v\n",
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

// flatListMessage reports messages whose sole repeated field is carried
// without the usual nested positional wrapper.
func flatListMessage(md protoreflect.MessageDescriptor) bool {
	switch md.FullName() {
	case "notebooklm.v1alpha1.AccessTokenScopes",
		"notebooklm.v1alpha1.SourceSettingsIntList",
		"notebooklm.v1alpha1.StringList":
		return true
	}
	return false
}

// setObjectMessage populates an object-encoded message field from a JSON object
// keyed by field number ({"101": "•", "102": 1}), the sparse form batchexecute
// uses for messages with large, sparse field numbers.
func (o UnmarshalOptions) setObjectMessage(m protoreflect.Message, fd protoreflect.FieldDescriptor, msgType protoreflect.MessageType, obj map[string]interface{}) error {
	msg := msgType.New()
	fields := fd.Message().Fields()
	for key, raw := range obj {
		if raw == nil {
			continue
		}
		num, err := strconv.Atoi(key)
		if err != nil {
			if o.DebugParsing {
				fmt.Printf("  -> object-encoded %s: non-numeric key %q skipped\n", fd.Message().FullName(), key)
			}
			continue
		}
		field := fields.ByNumber(protoreflect.FieldNumber(num))
		if field == nil {
			if !o.DiscardUnknown {
				return fmt.Errorf("object-encoded %s: no field %d", fd.Message().FullName(), num)
			}
			continue
		}
		if err := o.setField(msg, field, raw); err != nil {
			return fmt.Errorf("object-encoded %s field %d: %w", fd.Message().FullName(), num, err)
		}
	}
	m.Set(fd, protoreflect.ValueOfMessage(msg))
	return nil
}

// Extension full names for the batchexecute encoding options. They are read by
// name from the global registry so this codec stays decoupled from the
// concrete generated package that declares them.
const (
	extJSONBool          = "notebooklm.v1alpha1.json_bool"
	extObjectEncoded     = "notebooklm.v1alpha1.object_encoded"
	extOmitTrailingNulls = "notebooklm.v1alpha1.omit_trailing_nulls"
)

// fieldBoolOption reports whether the named bool extension is set to true on a
// field's options. It resolves the extension by name at call time; if the
// extension is not registered (its package was never loaded), it returns false.
func fieldBoolOption(fd protoreflect.FieldDescriptor, name protoreflect.FullName) bool {
	return boolExtension(fd.Options(), name)
}

// messageBoolOption reports whether the named bool extension is set to true on a
// message's options.
func messageBoolOption(md protoreflect.MessageDescriptor, name protoreflect.FullName) bool {
	return boolExtension(md.Options(), name)
}

func boolExtension(opts proto.Message, name protoreflect.FullName) bool {
	if opts == nil {
		return false
	}
	xt, err := protoregistry.GlobalTypes.FindExtensionByName(name)
	if err != nil {
		return false
	}
	if !proto.HasExtension(opts, xt) {
		return false
	}
	v, ok := proto.GetExtension(opts, xt).(bool)
	return ok && v
}

// shapeUnion reports whether md is a shape-union message: one whose fields are
// exactly the members of a single oneof (no other fields). Such a message maps
// a single wire position to one of several alternative shapes, chosen by the
// wire value's shape rather than by field position. It returns the oneof, or
// nil if md is an ordinary positional message.
func shapeUnion(md protoreflect.MessageDescriptor) protoreflect.OneofDescriptor {
	oneofs := md.Oneofs()
	if oneofs.Len() != 1 {
		return nil
	}
	oneof := oneofs.Get(0)
	if oneof.IsSynthetic() { // proto3 optional fields are synthetic oneofs
		return nil
	}
	if oneof.Fields().Len() != md.Fields().Len() {
		return nil // md has fields outside the oneof; treat positionally
	}
	return oneof
}

// setShapeUnion populates a shape-union message field by selecting the oneof
// case whose declared shape matches the wire value, then decoding val into that
// case. Cases are tried in declaration order; the first whose leading shape is
// compatible wins.
func (o UnmarshalOptions) setShapeUnion(m protoreflect.Message, fd protoreflect.FieldDescriptor, oneof protoreflect.OneofDescriptor, val interface{}) error {
	union, _, err := o.buildShapeUnion(fd.Message(), oneof, val)
	if err != nil {
		return err
	}
	m.Set(fd, protoreflect.ValueOfMessage(union))
	return nil
}

// buildShapeUnion decodes val into a new instance of a shape-union message,
// selecting the oneof case whose declared shape matches the wire value. Cases
// are tried in declaration order; the first whose leading shape is compatible
// wins. matched reports whether a case was actually selected, as opposed to
// the forgiving empty-union fallback. Shared by setShapeUnion (a singular
// shape-union field) and appendToList/setRepeatedField (a repeated field
// whose element type is itself a shape union, e.g. a list that holds either a
// bare scalar or a nested message at each position).
func (o UnmarshalOptions) buildShapeUnion(md protoreflect.MessageDescriptor, oneof protoreflect.OneofDescriptor, val interface{}) (union protoreflect.Message, matched bool, err error) {
	msgType, err := protoregistry.GlobalTypes.FindMessageByName(md.FullName())
	if err != nil {
		return protoreflect.Value{}.Message(), false, fmt.Errorf("failed to find message type %q: %v", md.FullName(), err)
	}
	u := msgType.New()

	cases := oneof.Fields()
	for i := 0; i < cases.Len(); i++ {
		c := cases.Get(i)
		if !shapeMatches(c, val) {
			continue
		}
		if err := o.setField(u, c, val); err != nil {
			if o.DebugParsing {
				fmt.Printf("  -> shape-union case %s rejected: %v\n", c.Name(), err)
			}
			continue
		}
		return u, true, nil
	}
	// No case matched: leave the union empty rather than error, matching the
	// forgiving behavior of the positional path.
	if o.DebugParsing {
		fmt.Printf("  -> shape-union %s: no case matched value %T\n", md.FullName(), val)
	}
	return u, false, nil
}

// shapeMatches reports whether a wire value is a plausible fit for a oneof case
// field. It steers a shape union to the right variant by comparing the wire
// value's leading shape against the case's leading field:
//
//   - a scalar case (string/number/bool) matches the matching scalar kind, or a
//     single-element array wrapping it;
//   - a message case matches an array whose first element is compatible with
//     that message's first field (so a text leaf and a span container, both
//     message cases, are told apart by whether element [0] is a string or an
//     array);
//   - a repeated case matches any array.
func shapeMatches(c protoreflect.FieldDescriptor, val interface{}) bool {
	switch v := val.(type) {
	case []interface{}:
		switch {
		case c.IsList():
			return true
		case c.Message() != nil:
			// Look one level in: does the wire's leading element fit the case
			// message's first field? This distinguishes message cases that
			// differ only by their contents' shape.
			lead := c.Message().Fields()
			if lead.Len() == 0 {
				return len(v) == 0
			}
			if len(v) == 0 {
				return true
			}
			return shapeMatches(lead.Get(0), v[0])
		case len(v) == 1:
			// A scalar case may be wrapped in a single-element array: [x].
			return shapeMatches(c, v[0])
		}
		return false
	case string:
		return c.Kind() == protoreflect.StringKind || c.Kind() == protoreflect.BytesKind
	case float64:
		switch c.Kind() {
		case protoreflect.Int32Kind, protoreflect.Int64Kind, protoreflect.Sint32Kind,
			protoreflect.Sint64Kind, protoreflect.Sfixed32Kind, protoreflect.Sfixed64Kind,
			protoreflect.Uint32Kind, protoreflect.Uint64Kind, protoreflect.Fixed32Kind,
			protoreflect.Fixed64Kind, protoreflect.FloatKind, protoreflect.DoubleKind,
			protoreflect.EnumKind:
			return true
		}
		return false
	case bool:
		return c.Kind() == protoreflect.BoolKind
	case map[string]interface{}:
		return c.Message() != nil && messageBoolOption(c.Message(), extObjectEncoded)
	case nil:
		return true
	}
	return false
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
		case string:
			n, err := strconv.ParseInt(v, 10, 32)
			if err != nil {
				return protoreflect.Value{}, fmt.Errorf("expected number, got string %q: %w", v, err)
			}
			return protoreflect.ValueOfInt32(int32(n)), nil
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
		case string:
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return protoreflect.Value{}, fmt.Errorf("expected number, got string %q: %w", v, err)
			}
			return protoreflect.ValueOfInt64(n), nil
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
		case string:
			n, err := strconv.ParseUint(v, 10, 32)
			if err != nil {
				return protoreflect.Value{}, fmt.Errorf("expected number, got string %q: %w", v, err)
			}
			return protoreflect.ValueOfUint32(uint32(n)), nil
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
		case string:
			n, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return protoreflect.Value{}, fmt.Errorf("expected number, got string %q: %w", v, err)
			}
			return protoreflect.ValueOfUint64(n), nil
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
