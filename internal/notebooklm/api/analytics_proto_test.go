package api

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	method "github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestGetProjectAnalyticsRequestEncoder(t *testing.T) {
	got := method.EncodeGetProjectAnalyticsArgs(&pb.GetProjectAnalyticsRequest{
		ProjectId:   "project-1",
		RequestedAt: timestamppb.New(time.Unix(1776236400, 0)),
		Mode:        &pb.Int32List{Value: 2},
	})
	want := []interface{}{"project-1", nil, []interface{}{float64(1776236400)}, []interface{}{2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("analytics args = %#v, want %#v", got, want)
	}
}

func TestProjectAnalyticsProtoAdapterMatchesLegacyParser(t *testing.T) {
	raw := json.RawMessage(mustReadAPIFixture(t, "testdata/AUrzMb_analytics_response.json"))
	legacy, err := parseProjectAnalytics(raw)
	if err != nil {
		t.Fatalf("legacy parser: %v", err)
	}
	if len(legacy.Series) == 0 || len(legacy.Series[0].Points) == 0 {
		t.Fatal("fixture no longer exercises metric-series behavior")
	}
	var generated pb.ProjectAnalytics
	if err := beprotojson.Unmarshal(raw, &generated); err != nil {
		t.Fatalf("generated decoder: %v", err)
	}
	got := projectAnalyticsFromProto(&generated)
	if !reflect.DeepEqual(got, legacy) {
		t.Fatalf("projectAnalyticsFromProto() = %#v, want %#v", got, legacy)
	}
}
