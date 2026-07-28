package api

import (
	"context"
	"fmt"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	intmethod "github.com/tmc/nlm/internal/method"
	"github.com/tmc/nlm/internal/notebooklm/rpc"
	"google.golang.org/protobuf/proto"
)

// VideoOverviewResult describes a generated video overview and its readiness.
type VideoOverviewResult struct {
	ProjectID string
	VideoID   string
	Title     string
	IsReady   bool
}

// CreateVideoOverview starts a video overview with default generation options.
func (c *Client) CreateVideoOverview(ctx context.Context, projectID string, instructions string) (*VideoOverviewResult, error) {
	return c.CreateVideoOverviewWithOptions(ctx, projectID, CreateVideoOverviewOptions{
		Instructions: instructions,
	})
}

// CreateVideoOverviewWithOptions starts a video overview with explicit options.
func (c *Client) CreateVideoOverviewWithOptions(ctx context.Context, projectID string, opts CreateVideoOverviewOptions) (*VideoOverviewResult, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project ID required")
	}
	opts = opts.withDefaults()
	if opts.Instructions == "" {
		return nil, fmt.Errorf("instructions required")
	}

	sourceIDs, err := c.createArtifactSourceIDs(ctx, projectID, opts.SourceIDs)
	if err != nil {
		return nil, err
	}
	if len(sourceIDs) == 0 {
		return nil, fmt.Errorf("project has no sources - add sources before creating video overview")
	}

	req := &pb.CreateUniversalArtifactRequest{
		Context:   universalArtifactRequestContext(),
		ProjectId: projectID,
		Options: &pb.UniversalArtifactOptions{
			Kind:         3,
			SourceGroups: universalArtifactSourceGroups(sourceIDs),
			Video: &pb.UniversalVideoOptions{Details: &pb.UniversalVideoDetails{
				Sources: universalVideoSources(sourceIDs),
				Prompt:  proto.String(opts.Instructions),
				Style:   int32(opts.VideoStyle),
			}},
		},
	}

	artifact, err := c.orchestrationService.CreateUniversalArtifact(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create video overview: %w", err)
	}
	return videoOverviewResultFromProto(projectID, artifact), nil
}

func universalVideoSources(sourceIDs []string) []*pb.UniversalArtifactSources {
	sources := make([]*pb.UniversalArtifactSources, 0, len(sourceIDs))
	for _, sourceID := range sourceIDs {
		sources = append(sources, &pb.UniversalArtifactSources{SourceId: sourceID})
	}
	return sources
}

func videoOverviewResultFromProto(projectID string, artifact *pb.Artifact) *VideoOverviewResult {
	result := &VideoOverviewResult{ProjectID: projectID}
	if artifact == nil {
		return result
	}
	result.VideoID = artifact.GetArtifactId()
	result.Title = artifact.GetTitle()
	result.IsReady = artifact.GetState() == pb.ArtifactState_ARTIFACT_STATE_READY
	return result
}

// CreateAppArtifact starts generation of an interactive app artifact.
func (c *Client) CreateAppArtifact(ctx context.Context, projectID string, kind AppArtifactKind, instructions string, sourceIDs []string) (string, error) {
	if projectID == "" {
		return "", fmt.Errorf("project ID required")
	}
	if !kind.valid() {
		return "", fmt.Errorf("invalid app artifact type %q", kind.String())
	}
	if instructions == "" {
		return "", fmt.Errorf("instructions required")
	}
	resolvedSourceIDs, err := c.createArtifactSourceIDs(ctx, projectID, sourceIDs)
	if err != nil {
		return "", err
	}
	if len(resolvedSourceIDs) == 0 {
		return "", fmt.Errorf("notebook has no sources")
	}

	args := intmethod.EncodeCreateAppArtifactArgs(projectID, resolvedSourceIDs, int32(kind), instructions)
	resp, err := c.rpc.Do(ctx, rpc.Call{
		ID:         rpc.RPCCreateVideoOverview,
		NotebookID: projectID,
		Args:       args,
	})
	if err != nil {
		return "", fmt.Errorf("create %s app artifact: %w", kind.String(), err)
	}
	return createdArtifactIDFromProtoWithOptions(resp, c.unmarshalOptions())
}

func (c *Client) createArtifactSourceIDs(ctx context.Context, projectID string, sourceIDs []string) ([]string, error) {
	if len(sourceIDs) > 0 {
		return sourceIDs, nil
	}
	project, err := c.GetProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get project sources: %w", err)
	}
	for _, src := range project.Sources {
		if src.SourceId != nil {
			sourceIDs = append(sourceIDs, src.SourceId.SourceId)
		}
	}
	return sourceIDs, nil
}
