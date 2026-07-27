package api

import (
	"context"
	"fmt"
	"time"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ProjectAnalytics is the AUrzMb response: a set of metric time series.
type ProjectAnalytics struct {
	Series []AnalyticsSeries `json:"series"`
}

// AnalyticsSeries contains all buckets for one metric id.
type AnalyticsSeries struct {
	MetricID int              `json:"metric_id"`
	Points   []AnalyticsPoint `json:"points"`
}

// AnalyticsPoint is one dated metric bucket.
type AnalyticsPoint struct {
	Time  time.Time `json:"time"`
	Value int       `json:"value"`
}

// GetProjectAnalytics returns the AUrzMb time-series analytics for projectID.
func (c *Client) GetProjectAnalytics(projectID string) (*ProjectAnalytics, error) {
	req := &pb.GetProjectAnalyticsRequest{
		ProjectId:   projectID,
		RequestedAt: timestamppb.Now(),
		Mode:        &pb.Int32List{Value: 2},
	}
	resp, err := c.orchestrationService.GetProjectAnalytics(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("get project analytics: %w", err)
	}
	return projectAnalyticsFromProto(resp), nil
}

// projectAnalyticsFromProto preserves the public metric-series projection.
func projectAnalyticsFromProto(in *pb.ProjectAnalytics) *ProjectAnalytics {
	if in == nil {
		return nil
	}
	out := &ProjectAnalytics{}
	for _, series := range in.GetSeries() {
		if series == nil {
			continue
		}
		item := AnalyticsSeries{MetricID: int(series.GetMetricId())}
		for _, point := range series.GetBuckets().GetPoints().GetPoints() {
			if point == nil || point.GetTime() == nil {
				continue
			}
			item.Points = append(item.Points, AnalyticsPoint{
				Time:  point.GetTime().AsTime().UTC(),
				Value: int(point.GetValue()),
			})
		}
		out.Series = append(out.Series, item)
	}
	return out
}
