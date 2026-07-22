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

func TestCreateLabelResponseProtoAdapterMatchesLegacyParser(t *testing.T) {
	raw := []byte(`[null,[["Generated Code",[["src-1"]],"label-1",""]]]`)
	legacy, err := parseLabelsResponse(raw)
	if err != nil {
		t.Fatalf("legacy parser: %v", err)
	}
	var response pb.CreateLabelResponse
	if err := beprotojson.Unmarshal(raw, &response); err != nil {
		t.Fatalf("proto decoder: %v", err)
	}
	got := labelsFromProtoResponse(&pb.GetLabelsResponse{Labels: response.GetLabels()})
	assertEquivalent(t, "label mutation adaptation", legacy, got)
}

func TestLabelMutationEncoders(t *testing.T) {
	create := method.EncodeCreateLabelArgs(&pb.CreateLabelRequest{
		Context:   &pb.RequestContext{Version: proto.Int32(2)},
		ProjectId: "project-1",
		Labels:    []*pb.LabelCreation{{Name: "Generated Code", Emoji: proto.String("📚")}},
	})
	wantCreate := []interface{}{[]interface{}{float64(2)}, "project-1", nil, nil, nil, []interface{}{[]interface{}{"Generated Code", "📚"}}}
	if !reflect.DeepEqual(create, wantCreate) {
		t.Fatalf("create args = %#v, want %#v", create, wantCreate)
	}
	mode := method.EncodeMutateLabelsModeArgs(&pb.MutateLabelsModeRequest{
		Context:   &pb.RequestContext{Version: proto.Int32(2)},
		ProjectId: "project-1",
		Mode:      &pb.LabelMode{Value: proto.Int32(1)},
	})
	wantMode := []interface{}{[]interface{}{float64(2)}, "project-1", nil, nil, []interface{}{float64(1)}}
	if !reflect.DeepEqual(mode, wantMode) {
		t.Fatalf("mode args = %#v, want %#v", mode, wantMode)
	}
	emptyMode := method.EncodeMutateLabelsModeArgs(&pb.MutateLabelsModeRequest{
		Context:   &pb.RequestContext{Version: proto.Int32(2)},
		ProjectId: "project-1",
		Mode:      &pb.LabelMode{},
	})
	wantEmptyMode := []interface{}{[]interface{}{float64(2)}, "project-1", nil, nil, []interface{}{}}
	if !reflect.DeepEqual(emptyMode, wantEmptyMode) {
		t.Fatalf("empty mode args = %#v, want %#v", emptyMode, wantEmptyMode)
	}
}

func TestDeleteLabelsRequestEncoder(t *testing.T) {
	got := method.EncodeDeleteLabelsArgs(&pb.DeleteLabelsRequest{
		ProjectId: "project-1",
		LabelIds:  []string{"label-1", "label-2"},
	})
	want := []interface{}{[]interface{}{2}, "project-1", []string{"label-1", "label-2"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("delete args = %#v, want %#v", got, want)
	}
}

func TestMutateLabelEncoders(t *testing.T) {
	rename := method.EncodeMutateLabelArgs(&pb.MutateLabelRequest{
		Context:   &pb.RequestContext{Version: proto.Int32(2)},
		ProjectId: "project-1",
		LabelId:   "label-1",
		Mutation:  &pb.MutateLabelMutation{Entry: &pb.MutateLabelEntry{Name: &pb.LabelNameChange{Name: "Renamed"}}},
	})
	wantRename := []interface{}{[]interface{}{float64(2)}, "project-1", "label-1", []interface{}{[]interface{}{[]interface{}{"Renamed"}}}}
	if !reflect.DeepEqual(rename, wantRename) {
		t.Fatalf("rename args = %#v, want %#v", rename, wantRename)
	}
	attach := method.EncodeMutateLabelArgs(&pb.MutateLabelRequest{
		Context:   &pb.RequestContext{Version: proto.Int32(2)},
		ProjectId: "project-1",
		LabelId:   "label-1",
		Mutation:  &pb.MutateLabelMutation{Entry: &pb.MutateLabelEntry{Sources: []*pb.SourceIdList{{SourceId: "src-1"}}}},
	})
	wantAttach := []interface{}{[]interface{}{float64(2)}, "project-1", "label-1", []interface{}{[]interface{}{nil, []interface{}{[]interface{}{"src-1"}}}}}
	if !reflect.DeepEqual(attach, wantAttach) {
		t.Fatalf("attach args = %#v, want %#v", attach, wantAttach)
	}
}
