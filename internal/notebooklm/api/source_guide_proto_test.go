package api

import (
	"reflect"
	"testing"

	method "github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
)

func TestGenerateSourceGuideProtoAdapter(t *testing.T) {
	raw := []byte(`[[[null,["summary text"],[["topic one","topic two"]],[]]]]`)
	var response pb.GenerateDocumentGuidesResponse
	if err := beprotojson.Unmarshal(raw, &response); err != nil {
		t.Fatalf("proto decoder: %v", err)
	}
	got := sourceGuideFromProto(&response)
	want := &SourceGuide{Summary: "summary text", KeyTopics: []string{"topic one", "topic two"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("source guide = %#v, want %#v", got, want)
	}
}

func TestGenerateSourceGuideRequestEncoder(t *testing.T) {
	got := method.EncodeGenerateDocumentGuidesArgs(&pb.GenerateDocumentGuidesRequest{
		Sources: &pb.GenerateDocumentGuideSources{Source: &pb.GenerateDocumentGuideSource{
			Source: &pb.SourceIdList{SourceId: "source-1"},
		}},
	})
	want := []interface{}{
		[]interface{}{[]interface{}{[]interface{}{"source-1"}}},
		[]interface{}{2, nil, []interface{}{1}, []interface{}{1, nil, nil, nil, nil, nil, nil, nil, nil, nil, []interface{}{1, 3}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("generate source guide args = %#v, want %#v", got, want)
	}
}
