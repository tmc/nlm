package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestBetoolInferProtoStaticMerge(t *testing.T) {
	raw, err := os.ReadFile("../../internal/batchexecute/testdata/list_notebooks.txt")
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	out, err := runBetoolCapture(t, []string{"--json", "infer-proto", "--rpc-id=wXbhsf", "../../internal/batchexecute/testdata/list_notebooks.txt"}, string(raw))
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
