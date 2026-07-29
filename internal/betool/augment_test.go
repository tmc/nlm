package betool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestBetoolAugmentCorpus(t *testing.T) {
	set := augmentTestDescriptor()
	descriptorPath := filepath.Join(t.TempDir(), "input.pb")
	writeDescriptorSet(t, descriptorPath, set)
	loaded, files, err := readDescriptorSet(descriptorPath)
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := descriptorRPCBindings(loaded, files, "test.rpc_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings["rpc"]) != 1 {
		options := loaded.GetFile()[1].GetService()[0].GetMethod()[0].GetOptions()
		t.Fatalf("bindings = %v, method option unknown = %x", bindings, options.ProtoReflect().GetUnknown())
	}

	var lines []byte
	for _, payload := range []string{`["known",7,true,false,0,"once"]`, `["known",8,1,false,0]`} {
		entry := corpusTrafficEntry{}
		entry.Request.URL = "https://example.test/_/data/batchexecute?rpcids=rpc"
		entry.Response.Status = 200
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		entry.Response.Content.Text = `)]}'` + "\n\n" +
			`[["wrb.fr","rpc",` + string(encoded) + `,null,null,null,"generic"]]`
		line, err := json.Marshal(entry)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, line...)
		lines = append(lines, '\n')
	}
	corpusPath := filepath.Join(t.TempDir(), "traffic.jsonl")
	if err := os.WriteFile(corpusPath, lines, 0600); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(t.TempDir(), "output.pb")
	out, err := runBetoolCapture(t, []string{
		"--json", "augment-corpus",
		"--descriptor=" + descriptorPath,
		"--rpc-option=test.rpc_id",
		"--boolean-option=test.json_bool",
		"--output=" + outputPath,
		corpusPath,
	}, "")
	if err != nil {
		t.Fatalf("augment-corpus: %v", err)
	}
	var report augmentReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("report is not JSON: %v\n%s", err, out)
	}
	if report.Records != 2 || report.MatchedPayloads != 2 || report.MinimumObservations != 2 || len(report.Added) != 2 || len(report.Annotated) != 1 || len(report.Presence) != 2 || len(report.Conflicts) != 1 || len(report.Insufficient) != 1 {
		t.Fatalf("report = %+v", report)
	}
	if got := report.Added[0]; got.Message != "test.Response" || got.Field != 2 || got.Observations != 2 {
		t.Fatalf("added = %+v, want test.Response field 2 with 2 observations", got)
	}
	if got := report.Conflicts[0]; got.Message != "test.Response" || got.Field != 3 || got.Observations != 2 {
		t.Fatalf("conflict = %+v, want test.Response field 3 with 2 observations", got)
	}
	if got := report.Annotated[0]; got.Message != "test.Response" || got.Field != 4 || got.Observations != 2 {
		t.Fatalf("annotation = %+v, want test.Response field 4 with 2 observations", got)
	}
	if got := report.Presence[0]; got.Message != "test.Response" || got.Field != 4 || got.Observations != 2 {
		t.Fatalf("presence = %+v, want test.Response field 4 with 2 explicit defaults", got)
	}
	if got := report.Presence[1]; got.Message != "test.Response" || got.Field != 5 || got.Observations != 2 {
		t.Fatalf("presence = %+v, want test.Response field 5 with 2 explicit defaults", got)
	}
	if got := report.Insufficient[0]; got.Message != "test.Response" || got.Field != 6 || got.Observations != 1 {
		t.Fatalf("insufficient = %+v, want test.Response field 6 with 1 observation", got)
	}

	output := readDescriptorSetForTest(t, outputPath)
	if _, err := protodesc.NewFiles(output); err != nil {
		t.Fatalf("augmented descriptor is invalid: %v", err)
	}
	response := findMessageProto(output.GetFile()[1], "test.Response")
	if response == nil {
		t.Fatal("output lost test.Response")
	}
	if got := protoFieldByNumber(response, 2); got == nil || got.GetType() != descriptorpb.FieldDescriptorProto_TYPE_INT64 {
		t.Fatalf("field 2 = %v, want inferred int64", got)
	}
	if got := protoFieldByNumber(response, 3); got != nil {
		t.Fatalf("conflicting field 3 = %v, want absent", got)
	}
	if got := protoFieldByNumber(response, 4); got == nil || !wireFieldPresent(mustMarshal(t, got.GetOptions()), 50002) || !got.GetProto3Optional() || got.OneofIndex == nil {
		t.Fatalf("field 4 = %v, want boolean option and proto3 presence", got)
	}
	if got := protoFieldByNumber(response, 5); got == nil || !got.GetProto3Optional() || got.OneofIndex == nil {
		t.Fatalf("field 5 = %v, want inferred field with proto3 presence", got)
	}
	if got := protoFieldByNumber(response, 6); got != nil {
		t.Fatalf("field 6 = %v, want insufficient field absent", got)
	}
}

func augmentTestDescriptor() *descriptorpb.FileDescriptorSet {
	options := new(descriptorpb.MethodOptions)
	unknown := protowire.AppendTag(nil, 50001, protowire.BytesType)
	unknown = protowire.AppendString(unknown, "rpc")
	options.ProtoReflect().SetUnknown(unknown)

	extensionFile := &descriptorpb.FileDescriptorProto{
		Name:       proto.String("rpc.proto"),
		Package:    proto.String("test"),
		Syntax:     proto.String("proto2"),
		Dependency: []string{"google/protobuf/descriptor.proto"},
		Extension: []*descriptorpb.FieldDescriptorProto{{
			Name:     proto.String("rpc_id"),
			Number:   proto.Int32(50001),
			Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			Extendee: proto.String(".google.protobuf.MethodOptions"),
		}, {
			Name:     proto.String("json_bool"),
			Number:   proto.Int32(50002),
			Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			Type:     descriptorpb.FieldDescriptorProto_TYPE_BOOL.Enum(),
			Extendee: proto.String(".google.protobuf.FieldOptions"),
		}},
	}
	schemaFile := &descriptorpb.FileDescriptorProto{
		Name:       proto.String("schema.proto"),
		Package:    proto.String("test"),
		Syntax:     proto.String("proto3"),
		Dependency: []string{"rpc.proto"},
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: proto.String("Request")},
			{
				Name: proto.String("Response"),
				Field: []*descriptorpb.FieldDescriptorProto{{
					Name:   proto.String("name"),
					Number: proto.Int32(1),
					Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
				}, {
					Name:   proto.String("flag"),
					Number: proto.Int32(4),
					Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					Type:   descriptorpb.FieldDescriptorProto_TYPE_BOOL.Enum(),
				}},
			},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{{
			Name: proto.String("Service"),
			Method: []*descriptorpb.MethodDescriptorProto{{
				Name:       proto.String("Call"),
				InputType:  proto.String(".test.Request"),
				OutputType: proto.String(".test.Response"),
				Options:    options,
			}},
		}},
	}
	return &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{
		protodesc.ToFileDescriptorProto(descriptorpb.File_google_protobuf_descriptor_proto),
		schemaFile,
		extensionFile,
	}}
}

func writeDescriptorSet(t *testing.T, path string, set *descriptorpb.FileDescriptorSet) {
	t.Helper()
	data, err := proto.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func readDescriptorSetForTest(t *testing.T, path string) *descriptorpb.FileDescriptorSet {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(data, &set); err != nil {
		t.Fatal(err)
	}
	return &set
}

func mustMarshal(t *testing.T, message proto.Message) []byte {
	t.Helper()
	data, err := proto.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
