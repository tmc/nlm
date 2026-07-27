package main

import (
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

func TestReportSuggestionSourceIDs(t *testing.T) {
	suggestion := &pb.ReportSuggestion{
		SourceIds: []*pb.SourceIdList{
			{SourceId: "source-1"},
			nil,
			{SourceId: ""},
			{SourceId: "source-2"},
		},
	}
	got := reportSuggestionSourceIDs(suggestion)
	if len(got) != 2 || got[0] != "source-1" || got[1] != "source-2" {
		t.Fatalf("source IDs = %v, want [source-1 source-2]", got)
	}
	if got := reportSuggestionSourceIDs(nil); got != nil {
		t.Fatalf("nil suggestion = %v, want nil", got)
	}
}
