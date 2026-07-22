package beprotojson

import (
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// shapeUnionFile builds a self-contained descriptor exercising a positional
// shape union: a recursive Span whose content is either a text leaf or a
// nested container, both at the same wire position.
//
//	Span       { int64 start=1; int64 end=2; SpanContent content=3 }
//	SpanContent{ oneof value { TextLeaf leaf=1; SpanList container=2 } }
//	TextLeaf   { string text=1 }
//	SpanList   { repeated Span spans=1 }
//
// Both leaf and container cases are messages, so each round-trips to an
// array-wrapped wire form (["hi"] for the leaf, [[...]] for the container),
// matching the observed rich_content layout.
func shapeUnionFile(t *testing.T) *protoregistry.Files {
	t.Helper()
	s := func(x string) *string { return &x }
	i32 := func(x int32) *int32 { return &x }
	lbl := func(l descriptorpb.FieldDescriptorProto_Label) *descriptorpb.FieldDescriptorProto_Label { return &l }
	typ := func(x descriptorpb.FieldDescriptorProto_Type) *descriptorpb.FieldDescriptorProto_Type { return &x }

	fdp := &descriptorpb.FileDescriptorProto{
		Name:    s("shapeunion_test.proto"),
		Package: s("beprotojson.test"),
		Syntax:  s("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: s("Span"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: s("start"), Number: i32(1), Label: lbl(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL), Type: typ(descriptorpb.FieldDescriptorProto_TYPE_INT64)},
					{Name: s("end"), Number: i32(2), Label: lbl(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL), Type: typ(descriptorpb.FieldDescriptorProto_TYPE_INT64)},
					{Name: s("content"), Number: i32(3), Label: lbl(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL), Type: typ(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE), TypeName: s(".beprotojson.test.SpanContent")},
				},
			},
			{
				Name:      s("SpanContent"),
				OneofDecl: []*descriptorpb.OneofDescriptorProto{{Name: s("value")}},
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: s("leaf"), Number: i32(1), Label: lbl(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL), Type: typ(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE), TypeName: s(".beprotojson.test.TextLeaf"), OneofIndex: i32(0)},
					{Name: s("container"), Number: i32(2), Label: lbl(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL), Type: typ(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE), TypeName: s(".beprotojson.test.SpanList"), OneofIndex: i32(0)},
				},
			},
			{
				Name: s("TextLeaf"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: s("text"), Number: i32(1), Label: lbl(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL), Type: typ(descriptorpb.FieldDescriptorProto_TYPE_STRING)},
				},
			},
			{
				Name: s("SpanList"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: s("spans"), Number: i32(1), Label: lbl(descriptorpb.FieldDescriptorProto_LABEL_REPEATED), Type: typ(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE), TypeName: s(".beprotojson.test.Span")},
				},
			},
		},
	}
	fd, err := protodesc.NewFile(fdp, protoregistry.GlobalFiles)
	if err != nil {
		t.Fatalf("build file descriptor: %v", err)
	}
	files := &protoregistry.Files{}
	if err := files.RegisterFile(fd); err != nil {
		t.Fatalf("register file: %v", err)
	}
	// The codec resolves nested message types via GlobalTypes, so register each.
	for i := 0; i < fd.Messages().Len(); i++ {
		if err := protoregistry.GlobalTypes.RegisterMessage(dynamicpb.NewMessageType(fd.Messages().Get(i))); err != nil {
			t.Fatalf("register message: %v", err)
		}
	}
	return files
}

func newDyn(t *testing.T, files *protoregistry.Files, name protoreflect.FullName) *dynamicpb.Message {
	t.Helper()
	d, err := files.FindDescriptorByName(name)
	if err != nil {
		t.Fatalf("find %s: %v", name, err)
	}
	return dynamicpb.NewMessage(d.(protoreflect.MessageDescriptor))
}

func TestShapeUnionRoundTrip(t *testing.T) {
	files := shapeUnionFile(t)
	spanDesc := func() protoreflect.MessageDescriptor {
		d, _ := files.FindDescriptorByName("beprotojson.test.Span")
		return d.(protoreflect.MessageDescriptor)
	}()

	// A container span holding two text-leaf children:
	//   [1, 5, [[[1, 3, ["hi"]], [3, 5, ["yo"]]]]]
	// content of the outer span is a container (array-first); content of each
	// inner span is a text leaf (string-first). Same wire position, different
	// shapes — the union must route each correctly. Offsets are non-zero so
	// proto3 scalar presence does not turn them into nulls on marshal.
	wire := `[1,5,[[[1,3,["hi"]],[3,5,["yo"]]]]]`

	msg := dynamicpb.NewMessage(spanDesc)
	if err := Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Verify the decoded tree.
	get := func(m protoreflect.Message, num int) protoreflect.Value {
		return m.Get(m.Descriptor().Fields().ByNumber(protoreflect.FieldNumber(num)))
	}
	if got := get(msg, 1).Int(); got != 1 {
		t.Errorf("outer start = %d, want 1", got)
	}
	if got := get(msg, 2).Int(); got != 5 {
		t.Errorf("outer end = %d, want 5", got)
	}
	content := get(msg, 3).Message() // SpanContent
	container := content.Get(content.Descriptor().Fields().ByName("container")).Message()
	spans := container.Get(container.Descriptor().Fields().ByName("spans")).List()
	if spans.Len() != 2 {
		t.Fatalf("container spans = %d, want 2", spans.Len())
	}
	leafText := func(span protoreflect.Message) string {
		c := get(span, 3).Message()
		leaf := c.Get(c.Descriptor().Fields().ByName("leaf")).Message()
		return leaf.Get(leaf.Descriptor().Fields().ByName("text")).String()
	}
	if txt := leafText(spans.Get(0).Message()); txt != "hi" {
		t.Errorf("inner[0] text = %q, want %q", txt, "hi")
	}
	if txt := leafText(spans.Get(1).Message()); txt != "yo" {
		t.Errorf("inner[1] text = %q, want %q", txt, "yo")
	}

	// Round-trip back to wire and confirm byte-identical structure.
	out, err := Marshal(msg.Interface())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !jsonEqual(t, out, []byte(wire)) {
		t.Errorf("round-trip mismatch:\n got %s\nwant %s", out, wire)
	}
}

func jsonEqual(t *testing.T, a, b []byte) bool {
	t.Helper()
	var av, bv any
	if err := json.Unmarshal(a, &av); err != nil {
		t.Fatalf("unmarshal a: %v", err)
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		t.Fatalf("unmarshal b: %v", err)
	}
	aj, _ := json.Marshal(av)
	bj, _ := json.Marshal(bv)
	return string(aj) == string(bj)
}
