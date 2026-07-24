// Package argbuilder provides a generalized argument encoder for RPC methods
package argbuilder

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/tmc/nlm/internal/beprotojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var (
	// Pattern to match %field_name% placeholders
	fieldPattern = regexp.MustCompile(`%([a-z0-9_]+)%`)
)

// ArgumentEncoder handles generic encoding of protobuf messages to RPC arguments
type ArgumentEncoder struct {
	// Cache of field accessors for performance
	fieldCache map[string]map[string]protoreflect.FieldDescriptor
}

// NewArgumentEncoder creates a new argument encoder
func NewArgumentEncoder() *ArgumentEncoder {
	return &ArgumentEncoder{
		fieldCache: make(map[string]map[string]protoreflect.FieldDescriptor),
	}
}

// EncodeArgs takes a protobuf message and an arg_format string and returns encoded arguments
func (e *ArgumentEncoder) EncodeArgs(msg proto.Message, argFormat string) ([]interface{}, error) {
	if argFormat == "" || argFormat == "[]" {
		return []interface{}{}, nil
	}

	// Parse the format string into tokens
	tokens, err := e.parseFormat(argFormat)
	if err != nil {
		return nil, fmt.Errorf("parse format: %w", err)
	}

	// Build the argument array
	return e.buildArgs(msg.ProtoReflect(), tokens)
}

// Token represents a parsed element from the arg_format string
type Token struct {
	Type  TokenType
	Value string
	// Sub holds the per-element subformat for a TokenLoop (the bracketed
	// projection applied to each element of the repeated field named in Value).
	Sub string
}

type TokenType int

const (
	TokenField   TokenType = iota // %field_name%
	TokenNull                     // null
	TokenLiteral                  // literal value like 1, 2, "string"
	TokenArray                    // [...] nested array
	TokenLoop                     // %field:[subformat]% repeated-message projection
)

// parseFormat parses an arg_format string into tokens
func (e *ArgumentEncoder) parseFormat(format string) ([]Token, error) {
	// Remove outer brackets if present
	format = strings.TrimSpace(format)
	if strings.HasPrefix(format, "[") && strings.HasSuffix(format, "]") {
		format = format[1 : len(format)-1]
	}

	var tokens []Token
	parts := e.splitFormat(format)

	for _, part := range parts {
		part = strings.TrimSpace(part)

		// Check for a loop reference %field:[subformat]% first: a projection of
		// a repeated message field, applying the bracketed subformat to each
		// element. Must precede the array and field checks, which would each
		// mis-handle the embedded %...% and brackets.
		if field, sub, ok := parseLoop(part); ok {
			tokens = append(tokens, Token{Type: TokenLoop, Value: field, Sub: sub})
			continue
		}

		// Check for a nested array [...] next. A bracketed part is always an
		// array, even when it contains a field reference (e.g. "[%source_id%]"
		// wraps the field in a one-element array). This must precede the field
		// check, which would otherwise match the inner %field% and drop the
		// surrounding brackets.
		if strings.HasPrefix(part, "[") && strings.HasSuffix(part, "]") {
			innerFormat := part[1 : len(part)-1]
			tokens = append(tokens, Token{Type: TokenArray, Value: innerFormat})
			continue
		}

		// Check for a bare field reference %field_name%.
		if matches := fieldPattern.FindStringSubmatch(part); len(matches) > 1 {
			tokens = append(tokens, Token{Type: TokenField, Value: matches[1]})
			continue
		}

		// Check for null
		if part == "null" {
			tokens = append(tokens, Token{Type: TokenNull})
			continue
		}

		// Otherwise it's a literal
		tokens = append(tokens, Token{Type: TokenLiteral, Value: part})
	}

	return tokens, nil
}

// splitFormat splits the format string by commas, respecting brackets
func (e *ArgumentEncoder) splitFormat(format string) []string {
	var parts []string
	var current strings.Builder
	depth := 0
	inField := false // inside a %...% reference (which may itself hold brackets/commas)

	for _, char := range format {
		switch {
		case char == '%':
			inField = !inField
			current.WriteRune(char)
		case inField:
			// Everything between %…% is opaque: a loop subformat can contain
			// its own brackets and commas that must not split the outer array.
			current.WriteRune(char)
		case char == '[':
			depth++
			current.WriteRune(char)
		case char == ']':
			depth--
			current.WriteRune(char)
		case char == ',' && depth == 0:
			parts = append(parts, current.String())
			current.Reset()
		default:
			current.WriteRune(char)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// buildArgs builds the argument array from tokens
func (e *ArgumentEncoder) buildArgs(msg protoreflect.Message, tokens []Token) ([]interface{}, error) {
	args := make([]interface{}, 0, len(tokens))

	for _, token := range tokens {
		switch token.Type {
		case TokenNull:
			args = append(args, nil)

		case TokenField:
			value, err := e.getFieldValue(msg, token.Value)
			if err != nil {
				return nil, fmt.Errorf("get field %s: %w", token.Value, err)
			}
			args = append(args, value)

		case TokenLiteral:
			// Parse literal values (numbers, strings, etc)
			args = append(args, e.parseLiteral(token.Value))

		case TokenArray:
			// A nested array token contributes exactly one element: an array of
			// its inner tokens' values. A scalar field wraps
			// ("[%source_id%]" -> ["sid"]) and a literal wraps ("[2]" -> [2]).
			// A lone repeated field is splatted, since its value is already the
			// slice of elements ("[%source_ids%]" -> ["a","b"], not [["a","b"]]).
			innerTokens, err := e.parseFormat(token.Value)
			if err != nil {
				return nil, err
			}
			innerArgs, err := e.buildArgs(msg, innerTokens)
			if err != nil {
				return nil, err
			}
			if len(innerArgs) == 1 {
				if splat, ok := asSlice(innerArgs[0]); ok {
					args = append(args, splat)
					continue
				}
			}
			args = append(args, innerArgs)

		case TokenLoop:
			// Project a repeated message field: apply the subformat to each
			// element, yielding one array per element. The whole projection is a
			// single element (the list of per-element arrays); unlike a plain
			// repeated field it is never splatted by an enclosing array.
			list, err := e.loopValues(msg, token.Value, token.Sub)
			if err != nil {
				return nil, err
			}
			args = append(args, list)
		}
	}

	return args, nil
}

// getFieldValue extracts a field value from a protobuf message
func (e *ArgumentEncoder) getFieldValue(msg protoreflect.Message, fieldName string) (interface{}, error) {
	descriptor := msg.Descriptor()

	// Cache field descriptors for performance
	msgName := string(descriptor.FullName())
	if e.fieldCache[msgName] == nil {
		e.fieldCache[msgName] = make(map[string]protoreflect.FieldDescriptor)
		fields := descriptor.Fields()
		for i := 0; i < fields.Len(); i++ {
			field := fields.Get(i)
			// Store by both JSON name and proto name
			e.fieldCache[msgName][field.JSONName()] = field
			e.fieldCache[msgName][string(field.Name())] = field
		}
	}

	// Try exact match first (proto field name)
	field, ok := e.fieldCache[msgName][fieldName]
	if !ok {
		// Try converting to camelCase for JSON name
		camelName := snakeToCamel(fieldName)
		field, ok = e.fieldCache[msgName][camelName]
		if !ok {
			return nil, fmt.Errorf("field %s not found in %s", fieldName, msgName)
		}
	}

	value := msg.Get(field)
	if field.HasPresence() && !msg.Has(field) {
		return nil, nil
	}

	// Handle repeated fields first
	if field.Cardinality() == protoreflect.Repeated {
		list := value.List()
		result := make([]interface{}, 0, list.Len())
		for i := 0; i < list.Len(); i++ {
			// For repeated string fields, directly append the string value
			if field.Kind() == protoreflect.StringKind {
				result = append(result, list.Get(i).String())
			} else if field.Kind() == protoreflect.MessageKind {
				encoded, err := encodeMessage(list.Get(i).Message())
				if err != nil {
					return nil, err
				}
				result = append(result, encoded)
			} else {
				result = append(result, e.convertValue(list.Get(i), field.Kind()))
			}
		}
		// For repeated string fields, return as []string
		if field.Kind() == protoreflect.StringKind {
			strResult := make([]string, len(result))
			for i, v := range result {
				strResult[i] = v.(string)
			}
			return strResult, nil
		}
		return result, nil
	}

	// Convert protoreflect.Value to interface{}
	switch field.Kind() {
	case protoreflect.StringKind:
		return value.String(), nil
	case protoreflect.Int32Kind, protoreflect.Int64Kind:
		return value.Int(), nil
	case protoreflect.BoolKind:
		return value.Bool(), nil
	case protoreflect.BytesKind:
		return value.Bytes(), nil
	case protoreflect.MessageKind:
		return encodeMessage(value.Message())
	default:
		return value.Interface(), nil
	}
}

// convertValue converts a protoreflect.Value to a Go interface{}
func (e *ArgumentEncoder) convertValue(v protoreflect.Value, kind protoreflect.Kind) interface{} {
	switch kind {
	case protoreflect.StringKind:
		return v.String()
	case protoreflect.Int32Kind, protoreflect.Int64Kind:
		return v.Int()
	case protoreflect.BoolKind:
		return v.Bool()
	case protoreflect.BytesKind:
		return v.Bytes()
	default:
		return v.Interface()
	}
}

// encodeMessage converts a protobuf message to its positional batchexecute
// representation. Using the same encoder as response/request verification
// keeps nested messages, present empty wrappers, and custom field options
// consistent across both paths.
func encodeMessage(msg protoreflect.Message) (interface{}, error) {
	b, err := beprotojson.Marshal(msg.Interface())
	if err != nil {
		return nil, fmt.Errorf("marshal %s: %w", msg.Descriptor().FullName(), err)
	}
	var value interface{}
	if err := json.Unmarshal(b, &value); err != nil {
		return nil, fmt.Errorf("decode %s wire JSON: %w", msg.Descriptor().FullName(), err)
	}
	return trimTrailingNulls(value), nil
}

func trimTrailingNulls(value interface{}) interface{} {
	list, ok := value.([]interface{})
	if !ok {
		return value
	}
	for i := range list {
		list[i] = trimTrailingNulls(list[i])
	}
	for len(list) > 0 && list[len(list)-1] == nil {
		list = list[:len(list)-1]
	}
	return list
}

// parseLiteral parses a literal value from the format string
func (e *ArgumentEncoder) parseLiteral(s string) interface{} {
	// Try to parse as number
	if n := strings.TrimSpace(s); n != "" {
		// Check if it's a number
		if n[0] >= '0' && n[0] <= '9' {
			// Simple integer parsing for now
			var val int
			fmt.Sscanf(n, "%d", &val)
			return val
		}
	}
	// Return as string
	return strings.Trim(s, `"'`)
}

// loopValues projects a repeated message field named fieldName by applying the
// bracketed subformat to each element message, returning the list of per-element
// argument arrays.
func (e *ArgumentEncoder) loopValues(msg protoreflect.Message, fieldName, subformat string) (loopList, error) {
	field := msg.Descriptor().Fields().ByName(protoreflect.Name(fieldName))
	if field == nil {
		field = msg.Descriptor().Fields().ByJSONName(snakeToCamel(fieldName))
	}
	if field == nil {
		return nil, fmt.Errorf("loop field %s not found in %s", fieldName, msg.Descriptor().FullName())
	}
	if field.Cardinality() != protoreflect.Repeated || field.Message() == nil {
		return nil, fmt.Errorf("loop field %s must be a repeated message", fieldName)
	}
	subTokens, err := e.parseFormat(subformat)
	if err != nil {
		return nil, fmt.Errorf("loop %s subformat: %w", fieldName, err)
	}
	list := msg.Get(field).List()
	out := make(loopList, 0, list.Len())
	for i := 0; i < list.Len(); i++ {
		elemArgs, err := e.buildArgs(list.Get(i).Message(), subTokens)
		if err != nil {
			return nil, fmt.Errorf("loop %s element %d: %w", fieldName, i, err)
		}
		out = append(out, elemArgs)
	}
	return out, nil
}

// parseLoop recognizes a loop reference of the form "%field:[subformat]%" and
// returns the repeated field name and the per-element subformat (with its outer
// brackets kept). It reports ok=false for anything that is not a loop.
func parseLoop(part string) (field, sub string, ok bool) {
	if !strings.HasPrefix(part, "%") || !strings.HasSuffix(part, "%") {
		return "", "", false
	}
	inner := part[1 : len(part)-1]
	colon := strings.IndexByte(inner, ':')
	if colon < 0 {
		return "", "", false
	}
	field = strings.TrimSpace(inner[:colon])
	sub = strings.TrimSpace(inner[colon+1:])
	if field == "" || !strings.HasPrefix(sub, "[") || !strings.HasSuffix(sub, "]") {
		return "", "", false
	}
	return field, sub, true
}

// loopList is the value produced by a %field:[...]% loop projection: the list
// of per-element arrays. It is a distinct type from a plain repeated field's
// value so an enclosing array does not splat it (see asSlice). It marshals to
// JSON and encodes on the wire exactly like []interface{}.
type loopList []interface{}

// asSlice reports whether v is a plain repeated-field value (a slice) that a
// lone-element enclosing array should splat, so "[%source_ids%]" yields the
// elements directly rather than a slice nested one level too deep. A loopList
// is deliberately excluded: a loop projection is one element, not splatted.
// Scalars and []byte (a single bytes value) are also not slices.
func asSlice(v interface{}) (interface{}, bool) {
	switch v.(type) {
	case loopList, []byte:
		return nil, false
	case []interface{}, []string, []int, []int32, []int64, []bool, []float32, []float64:
		return v, true
	default:
		return nil, false
	}
}

// snakeToCamel converts snake_case to camelCase
func snakeToCamel(s string) string {
	parts := strings.Split(s, "_")
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// Helper function for use in generated code
var defaultEncoder = NewArgumentEncoder()

// EncodeRPCArgs is a convenience function for generated code
func EncodeRPCArgs(msg proto.Message, argFormat string) ([]interface{}, error) {
	return defaultEncoder.EncodeArgs(msg, argFormat)
}
