package beprotojson

import (
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// The json_bool and object_encoded field/message options change how a value is
// carried on the wire. These tests build dynamic messages that set the options
// and confirm both directions round-trip.

func strp(s string) *string { return &s }
func i32p(i int32) *int32   { return &i }

// buildOptionsFile constructs:
//
//	Holder { bool plain=1; bool jb [(json_bool)=true]=2; Style style=3 }
//	Style  [(object_encoded)=true] { string bullet=101; int64 level=103 }
func buildOptionsFile(t *testing.T, pkg string) (protoreflect.MessageDescriptor, protoreflect.MessageDescriptor) {
	t.Helper()

	jsonBoolOpts := &descriptorpb.FieldOptions{}
	proto.SetExtension(jsonBoolOpts, pb.E_JsonBool, true)

	styleMsgOpts := &descriptorpb.MessageOptions{}
	proto.SetExtension(styleMsgOpts, pb.E_ObjectEncoded, true)

	lbl := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	tBool := descriptorpb.FieldDescriptorProto_TYPE_BOOL
	tStr := descriptorpb.FieldDescriptorProto_TYPE_STRING
	tI64 := descriptorpb.FieldDescriptorProto_TYPE_INT64
	tMsg := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE

	fdp := &descriptorpb.FileDescriptorProto{
		Name:    strp("encoding_options_" + pkg + ".proto"),
		Package: strp(pkg),
		Syntax:  strp("proto3"),
		Dependency: []string{
			"notebooklm/v1alpha1/rpc_extensions.proto",
		},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strp("Holder"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strp("plain"), Number: i32p(1), Label: &lbl, Type: &tBool},
					{Name: strp("jb"), Number: i32p(2), Label: &lbl, Type: &tBool, Options: jsonBoolOpts},
					{Name: strp("style"), Number: i32p(3), Label: &lbl, Type: &tMsg, TypeName: strp("." + pkg + ".Style")},
				},
			},
			{
				Name:    strp("Style"),
				Options: styleMsgOpts,
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: strp("bullet"), Number: i32p(101), Label: &lbl, Type: &tStr},
					{Name: strp("level"), Number: i32p(103), Label: &lbl, Type: &tI64},
				},
			},
		},
	}

	fd, err := protodesc.NewFile(fdp, protoregistry.GlobalFiles)
	if err != nil {
		t.Fatalf("build file descriptor: %v", err)
	}
	for i := 0; i < fd.Messages().Len(); i++ {
		md := fd.Messages().Get(i)
		if _, err := protoregistry.GlobalTypes.FindMessageByName(md.FullName()); err != nil {
			if err := protoregistry.GlobalTypes.RegisterMessage(dynamicpb.NewMessageType(md)); err != nil {
				t.Fatalf("register %s: %v", md.FullName(), err)
			}
		}
	}
	return fd.Messages().ByName("Holder"), fd.Messages().ByName("Style")
}

func TestJSONBoolOption(t *testing.T) {
	holderDesc, _ := buildOptionsFile(t, "beprotojson.opt1")

	// plain=true (1), jb=true (JSON true): [1,true]
	wire := `[1,true]`
	msg := dynamicpb.NewMessage(holderDesc)
	if err := Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	fields := holderDesc.Fields()
	if !msg.Get(fields.ByName("plain")).Bool() {
		t.Error("plain not decoded as true")
	}
	if !msg.Get(fields.ByName("jb")).Bool() {
		t.Error("jb not decoded as true from JSON true")
	}
	out, err := Marshal(msg.Interface())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// style (field 3) is unset, so marshal pads a trailing null; batchexecute
	// trims trailing nulls, so compare on the trimmed prefix.
	if got := trimTrailingNull(string(out)); got != `[1,true]` {
		t.Errorf("marshal = %s (trimmed %s), want [1,true] (plain as 1, jb as true)", out, got)
	}
}

// trimTrailingNull removes a run of trailing ,null tokens from a JSON array
// string so an assertion can ignore wire-equivalent trailing-null padding.
func trimTrailingNull(s string) string {
	for {
		if len(s) >= 6 && s[len(s)-6:] == ",null]" {
			s = s[:len(s)-6] + "]"
			continue
		}
		return s
	}
}

func TestObjectEncodedOption(t *testing.T) {
	holderDesc, styleDesc := buildOptionsFile(t, "beprotojson.opt2")

	// A Holder whose style is object-encoded: style at position 3 arrives as
	// {"101":"•","103":2}.
	wire := `[null,null,{"101":"•","103":2}]`
	msg := dynamicpb.NewMessage(holderDesc)
	if err := Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	style := msg.Get(holderDesc.Fields().ByName("style")).Message()
	if got := style.Get(styleDesc.Fields().ByName("bullet")).String(); got != "•" {
		t.Errorf("bullet = %q, want •", got)
	}
	if got := style.Get(styleDesc.Fields().ByName("level")).Int(); got != 2 {
		t.Errorf("level = %d, want 2", got)
	}
	out, err := Marshal(msg.Interface())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !jsonEqual(t, out, []byte(wire)) {
		t.Errorf("round-trip mismatch:\n got %s\nwant %s", out, wire)
	}
}
