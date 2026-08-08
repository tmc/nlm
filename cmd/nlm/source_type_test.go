package main

import (
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

func TestFormatSourceType(t *testing.T) {
	tests := []struct {
		name     string
		metadata *pb.SourceMetadata
		want     string
	}{
		// The server files uploaded PDFs under the Slides source type while
		// reporting a PDF mime type. Reading the enum alone labelled every
		// PDF "gslides", which reads as a Slides conversion that never
		// happened.
		{
			name:     "pdf despite slides source type",
			metadata: &pb.SourceMetadata{SourceType: sourceType(pb.SourceType_SOURCE_TYPE_GOOGLE_SLIDES), MimeType: "application/pdf"},
			want:     "pdf",
		},
		{
			name:     "slides without a pdf mime type",
			metadata: &pb.SourceMetadata{SourceType: sourceType(pb.SourceType_SOURCE_TYPE_GOOGLE_SLIDES)},
			want:     "gslides",
		},
		{
			name:     "note keeps its own type",
			metadata: &pb.SourceMetadata{SourceType: sourceType(pb.SourceType_SOURCE_TYPE_SHARED_NOTE), MimeType: "text/markdown"},
			want:     "note",
		},
		{
			name:     "web page",
			metadata: &pb.SourceMetadata{SourceType: sourceType(pb.SourceType_SOURCE_TYPE_WEB_PAGE)},
			want:     "web",
		},
		{
			name: "no metadata",
			want: "-",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatSourceType(&pb.Source{Metadata: tt.metadata}); got != tt.want {
				t.Errorf("formatSourceType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func sourceType(t pb.SourceType) *pb.SourceType { return &t }
