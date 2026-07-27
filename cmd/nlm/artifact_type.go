package main

import (
	"fmt"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

func artifactTypeName(artifactType pb.ArtifactType) string {
	if name, ok := pb.ArtifactType_name[int32(artifactType)]; ok {
		return name
	}
	return fmt.Sprintf("ARTIFACT_TYPE_%d", artifactType)
}
