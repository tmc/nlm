package api

import (
	"encoding/json"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
)

func TestAnalyticsProtoModelIsNotSeriesModel(t *testing.T) {
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
	if generated.GetSourceCount() == nil && generated.GetNoteCount() == nil && generated.GetAudioOverviewCount() == nil {
		t.Fatal("fixture no longer demonstrates the generated scalar model consuming series positions")
	}
	// The generated type has no series field. Keep this assertion explicit so a
	// future proto correction updates this test and the migration ledger together.
	if len(legacy.Series) < 1 {
		t.Fatal("want at least one legacy metric series")
	}
}
