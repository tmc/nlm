package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	intmethod "github.com/tmc/nlm/internal/method"
	"github.com/tmc/nlm/internal/notebooklm/rpc"
	"google.golang.org/protobuf/proto"
)

// GetProjectDetails returns the public details for a share ID.
func (c *Client) GetProjectDetails(ctx context.Context, shareID string) (*pb.ProjectDetails, error) {
	resp, err := c.sharingService.GetProjectDetails(ctx, &pb.GetProjectDetailsRequest{
		ShareId: shareID,
		Context: &pb.RequestContext{Version: proto.Int32(2)},
	})
	if err != nil {
		return nil, fmt.Errorf("get project details: %w", err)
	}
	return projectDetailsFromProto(resp), nil
}

// projectDetailsFromProto preserves the public ProjectDetails projection while
// letting the generated message own wire decoding and presence handling.
func projectDetailsFromProto(details *pb.ProjectDetails) *pb.ProjectDetails {
	if details == nil {
		return nil
	}
	out := &pb.ProjectDetails{}
	if collaborators := details.GetCollaborators(); len(collaborators) > 0 {
		if profile := collaborators[0].GetProfile(); profile != nil {
			out.OwnerName = profile.GetDisplayName()
		}
	}
	if flags := details.GetFlags(); flags != nil {
		if flags.Flag_1 != nil {
			out.IsPublic = flags.GetFlag_1()
		} else if flags.Flag_0 != nil {
			out.IsPublic = flags.GetFlag_0()
		}
	}
	return out
}

// Sharing operations

// ShareOption represents audio sharing visibility options
type ShareOption int

const (
	// SharePrivate restricts an audio overview to the current account.
	SharePrivate ShareOption = 0
	// SharePublic makes an audio overview available through its share link.
	SharePublic ShareOption = 1
)

// ShareAudioResult represents the response from sharing audio
type ShareAudioResult struct {
	ShareURL string
	ShareID  string
	IsPublic bool
}

// ShareAudio publishes an audio overview's share link by dispatching
// the RGP97b RPC on the LabsTailwindSharingService. arg_format =
// "[%share_options%, %project_id%]" per proto; share_options is
// [0] for private, [1] for public.
//
// Earlier this method delegated to shareProjectDirect (the ShareProject
// path), which actually shared the entire notebook rather than just
// the audio. This implementation routes
// the call to the correct LabsTailwindSharingService.ShareAudio
// endpoint. The ShareOption argument is preserved for back-compat.
func (c *Client) ShareAudio(ctx context.Context, projectID string, shareOption ShareOption) (*ShareAudioResult, error) {
	options := []int32{0}
	if shareOption == SharePublic {
		options[0] = 1
	}
	req := &pb.ShareAudioRequest{
		ProjectId:    projectID,
		ShareOptions: options,
	}
	resp, err := c.sharingService.ShareAudio(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("share audio: %w", err)
	}
	// ShareAudioResponse.share_info is [share_url, share_id] (per proto).
	info := resp.GetShareInfo()
	out := &ShareAudioResult{
		IsPublic: shareOption == SharePublic,
	}
	if len(info) >= 1 {
		out.ShareURL = info[0]
	}
	if len(info) >= 2 {
		out.ShareID = info[1]
	}
	return out, nil
}

// ShareProject shares a project with specified settings
func (c *Client) ShareProject(ctx context.Context, projectID string, settings *pb.ShareSettings) (*pb.ShareProjectResponse, error) {
	if settings == nil {
		settings = &pb.ShareSettings{}
	}
	return c.shareProjectDirect(ctx, projectID, settings.GetIsPublic())
}

func (c *Client) shareProjectDirect(ctx context.Context, projectID string, isPublic bool) (*pb.ShareProjectResponse, error) {
	req := &pb.ShareProjectRequest{
		ProjectId: projectID,
		Settings:  &pb.ShareSettings{IsPublic: isPublic},
	}
	resp, err := c.rpc.Do(ctx, rpc.Call{
		ID:         rpc.RPCShareProject,
		NotebookID: projectID,
		Args:       intmethod.EncodeShareProjectArgs(req),
	})
	if err != nil {
		return nil, fmt.Errorf("share project: %w", err)
	}
	return parseShareProjectResponse(projectID, isPublic, resp)
}

func parseShareProjectResponse(projectID string, isPublic bool, resp []byte) (*pb.ShareProjectResponse, error) {
	var responseData []interface{}
	if err := json.Unmarshal(resp, &responseData); err != nil {
		return nil, fmt.Errorf("parse share response: %w", err)
	}

	result := &pb.ShareProjectResponse{
		Settings: &pb.ShareSettings{IsPublic: isPublic},
	}
	if url := findStringMatching(responseData, func(s string) bool {
		return strings.HasPrefix(s, "http") &&
			(strings.Contains(s, "notebook.google.com") || strings.Contains(s, "notebooklm.google.com"))
	}); url != "" {
		result.ShareUrl = url
	}
	if result.ShareUrl == "" && isPublic {
		result.ShareUrl = fmt.Sprintf("https://notebook.google.com/notebook/%s", projectID)
	}
	if shareID := findStringMatching(responseData, isUUID); shareID != "" {
		result.ShareId = shareID
	}
	return result, nil
}

func findStringMatching(v interface{}, match func(string) bool) string {
	switch val := v.(type) {
	case string:
		if match(val) {
			return val
		}
	case []interface{}:
		for _, item := range val {
			if found := findStringMatching(item, match); found != "" {
				return found
			}
		}
	}
	return ""
}
