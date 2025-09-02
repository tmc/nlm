# Proposal: Improve arg_format Type Safety

## Current Problem

The `arg_format` field in protobuf extensions is stringly-typed with patterns like:
- `"[%project_id%, %source_ids%]"`
- `"[null, 1, null, [2]]"`
- `"[[%sources%], %project_id%]"`

This approach has several issues:
1. **No compile-time validation** - Typos in field names aren't caught
2. **Template complexity** - The template has hardcoded logic for each pattern
3. **Error-prone** - Easy to mismatch field names or structure
4. **Poor documentation** - Format isn't self-documenting

## Proposed Solution

### Option 1: Structured Argument Definition (Recommended)

Replace string-based `arg_format` with a structured message:

```protobuf
// In rpc_extensions.proto
message ArgumentDefinition {
  message Argument {
    oneof value {
      string field_ref = 1;      // Reference to request field
      bool null_value = 2;        // Literal null
      int32 int_value = 3;        // Literal integer
      string string_value = 4;    // Literal string
      ArgumentList nested = 5;    // Nested array
    }
  }
  
  message ArgumentList {
    repeated Argument args = 1;
  }
  
  ArgumentList root = 1;
}

extend google.protobuf.MethodOptions {
  string rpc_id = 51000;
  ArgumentDefinition args = 51001;  // Replaces arg_format
  bool chunked_response = 51002;
}
```

Usage in proto:
```protobuf
rpc CreateProject(CreateProjectRequest) returns (CreateProjectResponse) {
  option (rpc_id) = "CCqFvf";
  option (args) = {
    root: {
      args: [
        { field_ref: "title" },
        { field_ref: "emoji" }
      ]
    }
  };
}

rpc DeleteSources(DeleteSourcesRequest) returns (DeleteSourcesResponse) {
  option (rpc_id) = "tGMBJ";
  option (args) = {
    root: {
      args: [
        { nested: { args: [{ field_ref: "source_ids" }] } }
      ]
    }
  };
}
```

### Option 2: Field Annotations

Use field-level options to specify argument positions:

```protobuf
message CreateProjectRequest {
  string title = 1 [(arg_position) = 0];
  string emoji = 2 [(arg_position) = 1];
}

message DeleteSourcesRequest {
  repeated string source_ids = 1 [
    (arg_position) = 0,
    (arg_encoding) = "nested_array"  // [[source_ids]]
  ];
}
```

### Option 3: Argument Builder DSL

Create a type-safe builder in Go:

```go
// In internal/rpc/args.go
type ArgBuilder struct {
  args []interface{}
}

func NewArgBuilder() *ArgBuilder {
  return &ArgBuilder{args: []interface{}{}}
}

func (b *ArgBuilder) AddField(fieldName string, value interface{}) *ArgBuilder {
  b.args = append(b.args, value)
  return b
}

func (b *ArgBuilder) AddNull() *ArgBuilder {
  b.args = append(b.args, nil)
  return b
}

func (b *ArgBuilder) AddNested(values ...interface{}) *ArgBuilder {
  b.args = append(b.args, values)
  return b
}

func (b *ArgBuilder) Build() []interface{} {
  return b.args
}
```

Generated encoder would use:
```go
func EncodeCreateProjectArgs(req *CreateProjectRequest) []interface{} {
  return NewArgBuilder().
    AddField("title", req.GetTitle()).
    AddField("emoji", req.GetEmoji()).
    Build()
}
```

## Benefits of Structured Approach

1. **Compile-time safety** - Proto compiler validates field references
2. **Self-documenting** - Structure is clear from proto definition
3. **Simpler templates** - Template just iterates over argument definitions
4. **Better tooling** - IDE support, auto-completion
5. **Easier testing** - Can unit test argument encoding separately

## Migration Path

1. Add new structured `args` option alongside existing `arg_format`
2. Update template to prefer `args` when present, fallback to `arg_format`
3. Gradually migrate proto definitions to use structured format
4. Eventually deprecate `arg_format`

## Example Implementation

For the template, processing becomes much simpler:

```go
// In template
{{- $args := methodExtension .Method "notebooklm.v1alpha1.args" }}
{{- if $args }}
func Encode{{.Method.GoName}}Args(req *{{.Input.GoName}}) []interface{} {
  return buildArgs(req, {{$args | toJSON}})
}
{{- end }}
```

With a helper function:
```go
func buildArgs(req interface{}, argDef *ArgumentDefinition) []interface{} {
  result := make([]interface{}, 0, len(argDef.Root.Args))
  for _, arg := range argDef.Root.Args {
    switch v := arg.Value.(type) {
    case *Argument_FieldRef:
      result = append(result, getFieldValue(req, v.FieldRef))
    case *Argument_NullValue:
      result = append(result, nil)
    case *Argument_IntValue:
      result = append(result, v.IntValue)
    case *Argument_Nested:
      result = append(result, buildArgs(req, v.Nested))
    }
  }
  return result
}
```

## Conclusion

The current stringly-typed `arg_format` is error-prone and hard to maintain. Moving to a structured definition would provide:
- Type safety
- Better documentation
- Simpler implementation
- Easier testing
- More maintainable code

The structured approach (Option 1) is recommended as it provides the best balance of flexibility and type safety while being backward compatible during migration.