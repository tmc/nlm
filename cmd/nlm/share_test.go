package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestPrintShareDetails(t *testing.T) {
	details := &pb.ProjectDetails{
		ProjectId: "project-123",
		Title:     "Shared Notebook",
		Emoji:     "📚",
		OwnerName: "Owner",
		IsPublic:  true,
		SharedAt:  timestamppb.New(time.Date(2026, 4, 7, 12, 30, 0, 0, time.UTC)),
		Sources: []*pb.SourceSummary{
			{
				SourceId:   "source-1",
				Title:      "Source One",
				SourceType: pb.SourceType_SOURCE_TYPE_TEXT,
			},
		},
	}

	var buf bytes.Buffer
	printShareDetails(&buf, "share-123", details)
	out := buf.String()

	for _, want := range []string{
		"Share ID: share-123",
		"Project ID: project-123",
		"Title: 📚 Shared Notebook",
		"Owner: Owner",
		"Visibility: public",
		"Shared At: 2026-04-07T12:30:00Z",
		"Sources: 1",
		"Source One (SOURCE_TYPE_TEXT)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q\n%s", want, out)
		}
	}
}

func TestPrintShareDetailsNil(t *testing.T) {
	var buf bytes.Buffer
	printShareDetails(&buf, "share-123", nil)

	out := buf.String()
	if !strings.Contains(out, "No details available for this share ID.") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}
