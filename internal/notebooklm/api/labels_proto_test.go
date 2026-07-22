package api

import (
	"reflect"
	"testing"

	method "github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
	"google.golang.org/protobuf/proto"
)

func TestGetLabelsProtoAdapterMatchesLegacyParser(t *testing.T) {
	raw := []byte(`[[["Generated Code",[["src-1"],["src-2"]],"label-1",""]]]`)
	legacy, err := parseLabelsResponse(raw)
	if err != nil {
		t.Fatalf("legacy parser: %v", err)
	}
	var response pb.GetLabelsResponse
	if err := beprotojson.Unmarshal(raw, &response); err != nil {
		t.Fatalf("proto decoder: %v", err)
	}
	got := labelsFromProtoResponse(&response)
	assertEquivalent(t, "labels adaptation", legacy, got)
}

func TestGetLabelsProtoAdapterEmptyResponse(t *testing.T) {
	var response pb.GetLabelsResponse
	if err := beprotojson.Unmarshal([]byte(`[]`), &response); err != nil {
		t.Fatalf("proto decoder: %v", err)
	}
	got := labelsFromProtoResponse(&response)
	if got == nil || len(got) != 0 {
		t.Fatalf("empty labels = %#v, want non-nil empty slice", got)
	}
}

func TestGetLabelsRequestEncoder(t *testing.T) {
	got := method.EncodeGetLabelsArgs(&pb.GetLabelsRequest{
		Context:   &pb.RequestContext{Version: proto.Int32(2)},
		ProjectId: "project-1",
	})
	want := []interface{}{[]interface{}{float64(2)}, "project-1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("encoded args = %#v (%T/%T), want %#v (%T/%T)", got, got[0].([]interface{})[0], got[1], want, want[0].([]interface{})[0], want[1])
	}
}
