package api

import (
    pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

type (
    Project = pb.Project
    Source  = pb.Source
)

// AudioOverview represents an audio overview response
type AudioOverview struct {
    Status       string
    Content      string
    Instructions string
}

