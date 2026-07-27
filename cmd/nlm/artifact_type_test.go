package main

import (
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

func TestArtifactTypeName(t *testing.T) {
	tests := []struct {
		name         string
		artifactType pb.ArtifactType
		want         string
	}{
		{"known", pb.ArtifactType_ARTIFACT_TYPE_NOTE, "ARTIFACT_TYPE_NOTE"},
		{"placeholder", pb.ArtifactType(10), "ARTIFACT_TYPE_10"},
		{"unknown", pb.ArtifactType(42), "ARTIFACT_TYPE_42"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := artifactTypeName(test.artifactType); got != test.want {
				t.Fatalf("artifactTypeName(%d) = %q, want %q", test.artifactType, got, test.want)
			}
		})
	}
}
