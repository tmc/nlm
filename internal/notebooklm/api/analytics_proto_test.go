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
	want := []interface{}{"project-1", nil, []interface{}{float64(1776236400)}, []interface{}{float64(2)}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("analytics args = %#v, want %#v", got, want)
	}
}

func TestProjectAnalyticsProtoAdapter(t *testing.T) {
	raw := json.RawMessage(mustReadAPIFixture(t, "testdata/AUrzMb_analytics_response.json"))
	var generated pb.ProjectAnalytics
	if err := beprotojson.Unmarshal(raw, &generated); err != nil {
		t.Fatalf("generated decoder: %v", err)
	}
	got := projectAnalyticsFromProto(&generated)
	want := &ProjectAnalytics{Series: []AnalyticsSeries{
		{MetricID: 1, Points: []AnalyticsPoint{{Time: time.Unix(1773730800, 0).UTC(), Value: 0}}},
		{MetricID: 2, Points: []AnalyticsPoint{{Time: time.Unix(1773730800, 0).UTC(), Value: 0}}},
	}}
	if len(got.Series) != 2 || len(got.Series[0].Points) != 30 || len(got.Series[1].Points) != 30 {
		t.Fatalf("projectAnalyticsFromProto() = %#v, want two 30-point series", got)
	}
	for i := range want.Series {
		if got.Series[i].MetricID != want.Series[i].MetricID || got.Series[i].Points[0] != want.Series[i].Points[0] {
			t.Fatalf("series[%d] = %#v, want %#v", i, got.Series[i], want.Series[i])
		}
	}
}
