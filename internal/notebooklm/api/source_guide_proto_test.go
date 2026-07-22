package api

import (
	"encoding/json"
	"reflect"
	"testing"

	method "github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
)

func TestGenerateSourceGuideProtoAdapterMatchesLegacyParser(t *testing.T) {
	raw := []byte(`[[[null,["summary text"],[["topic one","topic two"]],[]]]]`)
	legacy, err := parseSourceGuideResponseLegacy(raw)
	if err != nil {
		t.Fatalf("legacy parser: %v", err)
	}
	var response pb.GenerateDocumentGuidesResponse
	if err := beprotojson.Unmarshal(raw, &response); err != nil {
		t.Fatalf("proto decoder: %v", err)
	}
	got := sourceGuideFromProto(&response)
	if !reflect.DeepEqual(legacy, got) {
		t.Fatalf("source guide mismatch\n legacy: %#v\n proto: %#v", legacy, got)
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

func parseSourceGuideResponseLegacy(raw []byte) (*SourceGuide, error) {
	var outer [][]interface{}
	if err := json.Unmarshal(raw, &outer); err != nil {
		return nil, err
	}
	g := &SourceGuide{}
	if len(outer) == 0 || len(outer[0]) == 0 {
		return g, nil
	}
	inner, _ := outer[0][0].([]interface{})
	if len(inner) >= 2 {
		if sumArr, ok := inner[1].([]interface{}); ok && len(sumArr) > 0 {
			g.Summary, _ = sumArr[0].(string)
		}
	}
	if len(inner) >= 3 {
		if topicOuter, ok := inner[2].([]interface{}); ok && len(topicOuter) > 0 {
			if topicArr, ok := topicOuter[0].([]interface{}); ok {
				for _, t := range topicArr {
					if s, ok := t.(string); ok {
						g.KeyTopics = append(g.KeyTopics, s)
					}
				}
			}
		}
	}
	return g, nil
}
