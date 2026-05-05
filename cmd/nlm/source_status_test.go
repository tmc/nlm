package main

import (
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	wrapperspb "google.golang.org/protobuf/types/known/wrapperspb"
)

// TestFormatSourceStatus pins the precedence between the parsed-metadata
// "enabled" signal and the post-parse error / warnings signals. Sources that
// errored out late (PDF parse rejected after metadata extraction, text
// upload that landed but failed downstream processing) carry both
// Metadata.Status=1 and Settings.Status=3; the UI shows the red error chip
// in that case, and so should we.
func TestFormatSourceStatus(t *testing.T) {
	tests := []struct {
		name string
		src  *pb.Source
		want string
	}{
		{
			name: "healthy",
			src: &pb.Source{
				Metadata: &pb.SourceMetadata{Status: pb.SourceSettings_SOURCE_STATUS_ENABLED},
				Settings: &pb.SourceSettings{Status: pb.SourceSettings_SOURCE_STATUS_ENABLED},
			},
			want: "enabled",
		},
		{
			// Regression: previously masked as "enabled" because the
			// metadata-status branch ran first.
			name: "metadata enabled but settings errored",
			src: &pb.Source{
				Metadata: &pb.SourceMetadata{Status: pb.SourceSettings_SOURCE_STATUS_ENABLED},
				Settings: &pb.SourceSettings{Status: pb.SourceSettings_SOURCE_STATUS_ERROR},
			},
			want: "error",
		},
		{
			name: "settings only errored",
			src: &pb.Source{
				Settings: &pb.SourceSettings{Status: pb.SourceSettings_SOURCE_STATUS_ERROR},
			},
			want: "error",
		},
		{
			name: "metadata only errored",
			src: &pb.Source{
				Metadata: &pb.SourceMetadata{Status: pb.SourceSettings_SOURCE_STATUS_ERROR},
			},
			want: "error",
		},
		{
			name: "warnings beat enabled",
			src: &pb.Source{
				Metadata: &pb.SourceMetadata{Status: pb.SourceSettings_SOURCE_STATUS_ENABLED},
				Warnings: []*wrapperspb.Int32Value{wrapperspb.Int32(7)},
			},
			want: "warn:7",
		},
		{
			name: "multiple warnings",
			src: &pb.Source{
				Warnings: []*wrapperspb.Int32Value{wrapperspb.Int32(7), wrapperspb.Int32(9)},
			},
			want: "warn:7,warn:9",
		},
		{
			name: "no metadata no settings",
			src:  &pb.Source{},
			want: "ok",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatSourceStatus(tt.src); got != tt.want {
				t.Errorf("formatSourceStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}
