package betool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestBetoolInferProtoStaticMerge(t *testing.T) {
	raw, err := os.ReadFile("../batchexecute/testdata/list_notebooks.txt")
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	out, err := runBetoolCapture(t, []string{"--json", "infer-proto", "--rpc-id=wXbhsf", "../batchexecute/testdata/list_notebooks.txt"}, string(raw))
	if err != nil {
		t.Fatalf("infer-proto: %v", err)
	}
	var fd descriptorpb.FileDescriptorProto
	if err := json.Unmarshal([]byte(out), &fd); err != nil {
		t.Fatalf("infer-proto output is not JSON: %v\n%s", err, out)
	}
	if fd.GetPackage() != "notebooklm.v1alpha1" {
		t.Fatalf("package = %q, want static package", fd.GetPackage())
	}
	metadata := findMessageProto(&fd, "notebooklm.v1alpha1.SourceMetadata")
	if metadata == nil {
		t.Fatal("merged descriptor lost SourceMetadata")
	}
	field4 := fieldByNumber(metadata, 4)
	if field4 == nil || field4.GetName() != "revision_data" || field4.GetType() != descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
		t.Fatalf("field 4 = %v, want static message revision_data", field4)
	}
	hasMetadataOneof := false
	for _, oneof := range metadata.GetOneofDecl() {
		if oneof.GetName() == "metadata_type" {
			hasMetadataOneof = true
		}
	}
	if !hasMetadataOneof {
		t.Fatalf("oneof declarations = %v, want static metadata_type preserved", metadata.GetOneofDecl())
	}
	if fieldByNumber(metadata, 5).GetType() != descriptorpb.FieldDescriptorProto_TYPE_ENUM {
		t.Fatal("static enum field source_type was not preserved")
	}
	if _, err := protodesc.NewFile(&fd, protoregistry.GlobalFiles); err != nil {
		t.Fatalf("merged descriptor is invalid: %v", err)
	}
}

func TestBetoolInferProtoSamplesWidenSingleMessageToRepeated(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"b.json": `[[["second"]]]`,
		"a.json": `[[["first"]]]`,
		"c.json": `[[["first","second"]]]`,
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	out, err := runBetoolCapture(t, []string{"--json", "infer-proto", "--rpc-id=unbound", "--samples", dir}, "")
	if err != nil {
		t.Fatalf("infer-proto samples: %v", err)
	}
	var fd descriptorpb.FileDescriptorProto
	if err := protojson.Unmarshal([]byte(out), &fd); err != nil {
		t.Fatalf("samples output is not protojson: %v\n%s", err, out)
	}
	root := fd.GetMessageType()[0]
	field := fieldByNumber(root, 1)
	if field == nil || field.GetLabel() != descriptorpb.FieldDescriptorProto_LABEL_REPEATED || field.GetType() != descriptorpb.FieldDescriptorProto_TYPE_STRING {
		t.Fatalf("inferred field 1 = %v, want repeated string", field)
	}
	if _, err := protodesc.NewFile(&fd, protoregistry.GlobalFiles); err != nil {
		t.Fatalf("inferred descriptor is invalid: %v", err)
	}
}

func TestBetoolInferProtoExternalDescriptor(t *testing.T) {
	set := &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{{
		Name:    proto.String("external.proto"),
		Package: proto.String("external"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Response"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:   proto.String("name"),
				Number: proto.Int32(1),
				Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			}},
		}},
	}}}
	data, err := proto.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "schema.pb")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(t.TempDir(), "augmented.pb")

	out, err := runBetoolCapture(t, []string{
		"--json", "infer-proto",
		"--rpc-id=unbound",
		"--descriptor=" + path,
		"--message=external.Response",
		"--output=" + outputPath,
	}, `[["known",7]]`)
	if err != nil {
		t.Fatalf("infer-proto: %v", err)
	}
	var gotSet descriptorpb.FileDescriptorSet
	if err := protojson.Unmarshal([]byte(out), &gotSet); err != nil {
		t.Fatalf("output is not protojson: %v\n%s", err, out)
	}
	if len(gotSet.GetFile()) != 1 {
		t.Fatalf("output has %d files, want 1", len(gotSet.GetFile()))
	}
	fd := gotSet.GetFile()[0]
	root := findMessageProto(fd, "external.Response")
	if root == nil {
		t.Fatal("output lost external.Response")
	}
	if got := fieldByNumber(root, 1); got == nil || got.GetName() != "name" {
		t.Fatalf("field 1 = %v, want preserved name field", got)
	}
	if got := fieldByNumber(root, 2); got == nil || got.GetName() != "unknown_2" || got.GetType() != descriptorpb.FieldDescriptorProto_TYPE_INT64 {
		t.Fatalf("field 2 = %v, want inferred int64 unknown_2", got)
	}
	binary, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var binarySet descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(binary, &binarySet); err != nil {
		t.Fatalf("binary output is not a descriptor set: %v", err)
	}
	if !proto.Equal(&gotSet, &binarySet) {
		t.Fatal("binary and JSON descriptor sets differ")
	}
}

func TestBetoolInferProtoSkipsConflictingField(t *testing.T) {
	dir := t.TempDir()
	for name, contents := range map[string]string{
		"bool.json":   `[["known",true,7]]`,
		"number.json": `[["known",1,7]]`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}

	out, err := runBetoolCapture(t, []string{
		"--json", "infer-proto",
		"--rpc-id=unbound",
		"--samples=" + dir,
	}, "")
	if err != nil {
		t.Fatalf("infer-proto: %v", err)
	}
	var fd descriptorpb.FileDescriptorProto
	if err := protojson.Unmarshal([]byte(out), &fd); err != nil {
		t.Fatalf("output is not protojson: %v\n%s", err, out)
	}
	root := fd.GetMessageType()[0]
	if got := fieldByNumber(root, 2); got != nil {
		t.Fatalf("conflicting field 2 = %v, want absent", got)
	}
	if got := fieldByNumber(root, 3); got == nil || got.GetType() != descriptorpb.FieldDescriptorProto_TYPE_INT64 {
		t.Fatalf("unambiguous field 3 = %v, want int64", got)
	}
}

func TestBetoolInferProtoTextproto(t *testing.T) {
	input := `[[["value",null,4]]]`
	out, err := runBetoolCapture(t, []string{"infer-proto", "--rpc-id=unbound"}, input)
	if err != nil {
		t.Fatalf("infer-proto textproto: %v", err)
	}
	if len(out) == 0 || !containsAll(out, `syntax = "proto3";`, "package betool.inferred;", "message InferredMessage", "unknown_1") {
		t.Fatalf("unexpected proto output:\n%s", out)
	}
}

func TestInferredValueTypeDistinguishesMessageTuples(t *testing.T) {
	if got := inferredValueType([]byte(`["id","id",1820823566]`)); got != "message" {
		t.Fatalf("inferredValueType = %q, want message", got)
	}
	if got := inferredValueType([]byte(`["one","two"]`)); got != "repeated string" {
		t.Fatalf("inferredValueType = %q, want repeated string", got)
	}
}

func TestInferFileSamplesAcceptsHARAndHTTPRR(t *testing.T) {
	har := []byte(`{"log":{"entries":[{"response":{"content":{"text":"[[[\"har-value\"]]]"}}}]}}`)
	values, err := inferFileSamples(har, "unbound")
	if err != nil || len(values) != 1 {
		t.Fatalf("HAR samples = %v, %v; want one sample", values, err)
	}
	jsonl := []byte(`{"response":{"content":{"text":"[[[\"jsonl-value\"]]]"}}}` + "\n")
	values, err = inferFileSamples(jsonl, "unbound")
	if err != nil || len(values) != 1 {
		t.Fatalf("JSONL samples = %v, %v; want one sample", values, err)
	}

	request := "GET / HTTP/1.1\r\nHost: example.test\r\n\r\n"
	body := "[[[\"httprr-value\"]]]"
	response := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
	httprr := fmt.Sprintf("httprr trace v1\n%d %d\n%s%s", len(request), len(response), request, response)
	values, err = inferFileSamples([]byte(httprr), "unbound")
	if err != nil || len(values) != 1 {
		t.Fatalf("httprr samples = %v, %v; want one sample", values, err)
	}
}

func TestInferJSONLFiltersRPCID(t *testing.T) {
	data := strings.Join([]string{
		`{"request":{"url":"https://example.test/_/rpc?rpcids=other"},"response":{"content":{"text":"[[[\"wrong\"]]]"}}}`,
		`{"request":{"url":"https://example.test/_/rpc?rpcids=target"},"response":{"content":{"text":"[[[\"right\"]]]"}}}`,
	}, "\n")
	values, err := inferJSONL([]byte(data), "target")
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || fmt.Sprint(values[0]) != "[[right]]" {
		t.Fatalf("values = %v, want matching RPC only", values)
	}
}

func fieldByNumber(message *descriptorpb.DescriptorProto, number int32) *descriptorpb.FieldDescriptorProto {
	for _, field := range message.GetField() {
		if field.GetNumber() == number {
			return field
		}
	}
	return nil
}

func containsAll(s string, wants ...string) bool {
	for _, want := range wants {
		if !strings.Contains(s, want) {
			return false
		}
	}
	return true
}
