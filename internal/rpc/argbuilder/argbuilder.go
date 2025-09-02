// Package argbuilder provides a generalized argument encoder for RPC methods
package argbuilder

import (
	"fmt"
	"regexp"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var (
	// Pattern to match %field_name% placeholders
	fieldPattern = regexp.MustCompile(`%([a-z_]+)%`)
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
}

type TokenType int

const (
	TokenField   TokenType = iota // %field_name%
	TokenNull                      // null
	TokenLiteral                   // literal value like 1, 2, "string"
	TokenArray                     // [...] nested array
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
		
		// Check for field reference %field_name%
		if matches := fieldPattern.FindStringSubmatch(part); len(matches) > 1 {
			tokens = append(tokens, Token{Type: TokenField, Value: matches[1]})
			continue
		}

		// Check for null
		if part == "null" {
			tokens = append(tokens, Token{Type: TokenNull})
			continue
		}

		// Check for nested array [[...]]
		if strings.HasPrefix(part, "[") && strings.HasSuffix(part, "]") {
			// This is a nested array
			innerFormat := part[1 : len(part)-1]
			// For simplicity, store the inner format as a literal
			// In practice, we'd want a more sophisticated representation
			tokens = append(tokens, Token{Type: TokenArray, Value: innerFormat})
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

	for _, char := range format {
		switch char {
		case '[':
			depth++
			current.WriteRune(char)
		case ']':
			depth--
			current.WriteRune(char)
		case ',':
			if depth == 0 {
				parts = append(parts, current.String())
				current.Reset()
			} else {
				current.WriteRune(char)
			}
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
			// Handle nested array
			innerTokens, err := e.parseFormat(token.Value)
			if err != nil {
				return nil, err
			}
			innerArgs, err := e.buildArgs(msg, innerTokens)
			if err != nil {
				return nil, err
			}
			// For nested arrays like [[%field%]], wrap the result 
			// If there's only one element in innerArgs, wrap it in an array
			if len(innerArgs) == 1 {
				args = append(args, []interface{}{innerArgs[0]})
			} else {
				args = append(args, innerArgs)
			}
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

	// Handle repeated fields first
	if field.Cardinality() == protoreflect.Repeated {
		list := value.List()
		result := make([]interface{}, 0, list.Len())
		for i := 0; i < list.Len(); i++ {
			// For repeated string fields, directly append the string value
			if field.Kind() == protoreflect.StringKind {
				result = append(result, list.Get(i).String())
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
		return e.convertMessage(value.Message()), nil
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

// convertMessage converts a protoreflect.Message to a map or appropriate structure
func (e *ArgumentEncoder) convertMessage(msg protoreflect.Message) interface{} {
	// For now, return as a map
	// Could be enhanced to handle specific message types
	result := make(map[string]interface{})
	msg.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		result[fd.JSONName()] = v.Interface()
		return true
	})
	return result
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