package api

import (
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// TestArtifactDownloadExtension verifies format detection from a download URL's
// filename query parameter — the key to letting deck download pick the pdf vs
// pptx rendering (issue #31).
func TestArtifactDownloadExtension(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			"pdf",
			"https://contribution.usercontent.google.com/download?c=abc&filename=Go-Pi_Architecture.pdf&opi=1",
			".pdf",
		},
		{
			"pptx",
			"https://contribution.usercontent.google.com/download?c=abc&filename=Go-Pi_Architecture.pptx&opi=1",
			".pptx",
		},
		{
			"uppercase extension is normalized",
			"https://contribution.usercontent.google.com/download?filename=Deck.PDF",
			".pdf",
		},
		{"no filename param", "https://example.com/download?c=abc", ""},
		{"filename without extension", "https://example.com/download?filename=deck", ""},
		{"not a url", "::::", ""},
	}
	for _, tt := range tests {
		if got := artifactDownloadExtension(tt.url); got != tt.want {
			t.Errorf("%s: artifactDownloadExtension(%q) = %q, want %q", tt.name, tt.url, got, tt.want)
		}
	}
}

func TestArtifactDownloadURLsFromArtifact(t *testing.T) {
	artifact := &pb.Artifact{SlideDeckPreview: &pb.ArtifactSlideDeckPreview{
		DownloadUrl:   artifactDownloadURLPrefix + "?filename=deck.pdf",
		DownloadUrl_2: artifactDownloadURLPrefix + "?filename=deck.pptx",
	}}
	got := artifactDownloadURLsFromArtifact(artifact)
	if len(got) != 2 {
		t.Fatalf("download URLs = %v, want PDF and PPTX", got)
	}
}
