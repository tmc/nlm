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

func TestPrintShareDetailsPartial(t *testing.T) {
	details := &pb.ProjectDetails{
		OwnerName: "Owner",
		IsPublic:  false,
	}

	var buf bytes.Buffer
	printShareDetails(&buf, "share-123", details)
	out := buf.String()

	for _, want := range []string{
		"Share ID: share-123",
		"Owner: Owner",
		"Visibility: private",
		"Note: current share-details responses only include owner/visibility metadata.",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "Sources: 0") {
		t.Fatalf("unexpected zero-source line:\n%s", out)
	}
}

func TestPrintPrivateShareResult(t *testing.T) {
	tests := []struct {
		name string
		resp *pb.ShareProjectResponse
		want []string
	}{
		{
			name: "url returned",
			resp: &pb.ShareProjectResponse{ShareUrl: "https://notebooklm.google.com/share/abc"},
			want: []string{"Private Share URL: https://notebooklm.google.com/share/abc"},
		},
		{
			name: "share id only",
			resp: &pb.ShareProjectResponse{ShareId: "share-123"},
			want: []string{
				"Private Share ID: share-123",
				"Open https://notebooklm.google.com/notebook/notebook-123 in the browser to copy the invite link.",
			},
		},
		{
			name: "no metadata",
			resp: &pb.ShareProjectResponse{},
			want: []string{
				"Project shared privately, but the server returned no share URL or share ID.",
			},
		},
		{
			name: "nil response",
			resp: nil,
			want: []string{
				"Project shared privately, but the server returned no share metadata.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			printPrivateShareResult(&buf, "notebook-123", tt.resp)
			out := buf.String()
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Fatalf("output missing %q\n%s", want, out)
				}
			}
		})
	}
}
