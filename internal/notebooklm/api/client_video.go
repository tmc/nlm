package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	intmethod "github.com/tmc/nlm/internal/method"
	"github.com/tmc/nlm/internal/notebooklm/rpc"
	"google.golang.org/protobuf/proto"
)

// Video operations

// VideoOverviewResult describes a generated video overview and its readiness.
type VideoOverviewResult struct {
	ProjectID string
	VideoID   string
	Title     string
	VideoData string // Base64 encoded or URL
	IsReady   bool
}

func videoOverviewResultFromArtifactData(projectID string, artifactData []interface{}) *VideoOverviewResult {
	if len(artifactData) == 0 {
		return &VideoOverviewResult{ProjectID: projectID}
	}

	result := &VideoOverviewResult{
		ProjectID: projectID,
		VideoID:   stringAt(artifactData, 0),
		Title:     stringAt(artifactData, 1),
	}
	if stateCode, ok := int32At(artifactData, 4); ok {
		result.IsReady = pb.ArtifactState(stateCode) == pb.ArtifactState_ARTIFACT_STATE_READY
	}
	return result
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

// ListVideoOverviews returns video overviews for a notebook
func (c *Client) ListVideoOverviews(ctx context.Context, projectID string) ([]*VideoOverviewResult, error) {
	// Since there's no GetVideoOverview RPC endpoint, we need to use a different approach
	// We can try to get the project and see if it has video overview metadata
	project, err := c.GetProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get project for video list: %w", err)
	}

	// NotebookLM typically stores at most one video overview per notebook
	// Since we don't have a direct way to get video overviews yet,
	// we'll return empty for now but this can be enhanced when we discover the proper method
	results := []*VideoOverviewResult{}

	// Check if project has any metadata that might indicate video overviews
	if project != nil && project.Metadata != nil {
		// Look for video-related metadata (this is speculative)
		// Will need to be updated when we discover the actual structure
		if c.config.Debug {
			fmt.Printf("Project %s metadata: %+v\n", projectID, project.Metadata)
		}
	}

	return results, nil
}

// GetVideoOverview attempts to get a video overview for a notebook
// Since there's no official GetVideoOverview RPC endpoint, we try alternative approaches
func (c *Client) GetVideoOverview(ctx context.Context, projectID string) (*VideoOverviewResult, error) {
	if !c.config.UseDirectRPC {
		return nil, fmt.Errorf("video overview requires --direct-rpc flag")
	}

	// Try using RPCGetAudioOverview with video-specific parameters
	// or see if we can get video data another way
	return c.getVideoOverviewAlternative(ctx, projectID)
}

// getVideoOverviewAlternative tries alternative methods to get video data
func (c *Client) getVideoOverviewAlternative(ctx context.Context, projectID string) (*VideoOverviewResult, error) {
	// First, try to get the project to see if it has video metadata
	project, err := c.GetProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get project for video overview: %w", err)
	}

	// Try different approaches to get video data
	approaches := []func(context.Context, string) (*VideoOverviewResult, error){
		c.tryVideoOverviewDirectRPC,
		c.tryVideoFromCreateResponse,
	}

	for i, approach := range approaches {
		if c.config.Debug {
			fmt.Printf("Trying video overview approach %d...\n", i+1)
		}

		result, err := approach(ctx, projectID)
		if err == nil && result != nil {
			if c.config.Debug {
				fmt.Printf("Video overview approach %d succeeded\n", i+1)
			}
			return result, nil
		}

		if c.config.Debug {
			fmt.Printf("Video overview approach %d failed: %v\n", i+1, err)
		}
	}

	_ = project // Use project to avoid unused variable warning
	return nil, fmt.Errorf("no method found to retrieve video overview data")
}

// tryVideoOverviewDirectRPC attempts to use GetAudioOverview RPC but for video
func (c *Client) tryVideoOverviewDirectRPC(ctx context.Context, projectID string) (*VideoOverviewResult, error) {
	// Try using the audio RPC with different parameters that might work for video
	resp, err := c.rpc.Do(ctx, rpc.Call{
		ID: rpc.RPCGetAudioOverview, // Reuse audio RPC
		Args: []interface{}{
			projectID,
			2, // Different request type for video?
		},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("video overview direct RPC: %w", err)
	}

	// Try to parse as video data
	var data []interface{}
	if err := json.Unmarshal(resp, &data); err != nil {
		return nil, fmt.Errorf("parse video response: %w", err)
	}

	// Check if this looks like video data
	result := &VideoOverviewResult{
		ProjectID: projectID,
	}

	// Try to extract video information
	if len(data) > 0 {
		if videoData, ok := data[0].([]interface{}); ok {
			// Look for video-like data structure
			if len(videoData) > 0 {
				if id, ok := videoData[0].(string); ok {
					result.VideoID = id
				}
			}
			if len(videoData) > 1 {
				if content, ok := videoData[1].(string); ok {
					// This might be video data or URL
					result.VideoData = content
				}
			}
			if len(videoData) > 2 {
				if status, ok := videoData[2].(string); ok {
					result.IsReady = status != "CREATING"
				}
			}
		}
	}

	return result, nil
}

// tryVideoFromCreateResponse attempts to get video data by analyzing creation patterns
func (c *Client) tryVideoFromCreateResponse(ctx context.Context, projectID string) (*VideoOverviewResult, error) {
	// This is a speculative approach - try to create a "get" request
	// using the same structure as CreateVideoOverview but with different parameters

	// Get sources from the project first
	project, err := c.GetProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get sources for video: %w", err)
	}

	if len(project.Sources) == 0 {
		return nil, fmt.Errorf("no sources in project for video")
	}

	// Use first source ID
	sourceID := project.Sources[0].SourceId
	sourceIDs := []interface{}{[]interface{}{sourceID}}

	// Try a "get" version of the video args with mode 1 instead of 2
	videoArgs := []interface{}{
		[]interface{}{1}, // Mode 1 = get instead of create?
		projectID,        // Notebook ID
		[]interface{}{
			nil, nil, 3,
			[]interface{}{sourceIDs}, // Source IDs array
			nil, nil, nil, nil,
			[]interface{}{
				nil, nil,
				[]interface{}{
					sourceIDs, // Source IDs again
					"en",      // Language
					"",        // Empty instructions for get
				},
			},
		},
	}

	resp, err := c.rpc.Do(ctx, rpc.Call{
		ID:         rpc.RPCCreateVideoOverview, // Reuse create RPC with different args
		NotebookID: projectID,
		Args:       videoArgs,
	})
	if err != nil {
		return nil, fmt.Errorf("video get via create RPC: %w", err)
	}

	// Parse response similar to CreateVideoOverview
	var responseData []interface{}
	if err := json.Unmarshal(resp, &responseData); err != nil {
		return nil, fmt.Errorf("parse video get response: %w", err)
	}

	result := &VideoOverviewResult{
		ProjectID: projectID,
	}

	// Extract video details
	if len(responseData) > 0 {
		if videoData, ok := responseData[0].([]interface{}); ok && len(videoData) > 0 {
			if id, ok := videoData[0].(string); ok {
				result.VideoID = id
			}
			if len(videoData) > 1 {
				if title, ok := videoData[1].(string); ok {
					result.Title = title
				}
			}
			if len(videoData) > 2 {
				if status, ok := videoData[2].(float64); ok {
					result.IsReady = status == 2
				}
			}
			// Look for video data/URL in additional fields
			if len(videoData) > 3 {
				if videoUrl, ok := videoData[3].(string); ok {
					result.VideoData = videoUrl
				}
			}
		}
	}

	return result, nil
}

// DownloadVideoOverview attempts to download video overview data
func (c *Client) DownloadVideoOverview(ctx context.Context, projectID string) (*VideoOverviewResult, error) {
	if !c.config.UseDirectRPC {
		return nil, fmt.Errorf("video download requires --direct-rpc flag")
	}

	// Try to get video overview data
	result, err := c.GetVideoOverview(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get video overview: %w", err)
	}

	if result.VideoData == "" {
		return nil, manualVideoDownloadError(projectID)
	}

	return result, nil
}

func manualVideoDownloadError(projectID string) error {
	return fmt.Errorf("direct video download URL is not exposed by the current API response; download manually from https://notebooklm.google.com/notebook/%s", projectID)
}

// SaveVideoToFile saves video data to a file
// Handles both base64 encoded data and URLs
// NOTE: For URL downloads, use client.DownloadVideoWithAuth() for proper authentication
func (r *VideoOverviewResult) SaveVideoToFile(ctx context.Context, filename string) error {
	if r.VideoData == "" {
		return fmt.Errorf("no video data to save")
	}

	// Check if VideoData is a URL or base64 data
	if strings.HasPrefix(r.VideoData, "http://") || strings.HasPrefix(r.VideoData, "https://") {
		// It's a URL - try basic download (may fail without auth)
		// For proper authentication, use client.DownloadVideoWithAuth()
		return r.downloadVideoFromURL(ctx, r.VideoData, filename)
	} else {
		// It's base64 encoded data
		return r.saveBase64VideoToFile(r.VideoData, filename)
	}
}

// downloadVideoFromURL downloads video from a URL with proper authentication
func (r *VideoOverviewResult) downloadVideoFromURL(ctx context.Context, url, filename string) error {
	// Create HTTP client with authentication
	client := httpClientWithTimeout(30 * time.Second)

	// Create request with proper headers
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Add headers similar to browser request
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.6")
	req.Header.Set("Range", "bytes=0-")
	req.Header.Set("Referer", "https://notebooklm.google.com/")
	req.Header.Set("Sec-Fetch-Dest", "video")
	req.Header.Set("Sec-Fetch-Mode", "no-cors")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36")

	// Note: This method is deprecated for authenticated downloads.
	// Use client.DownloadVideoWithAuth() instead for proper authentication.
	// This basic download method may fail for private videos that require cookies.

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download video from URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %s (may need authentication cookies)", resp.Status)
	}

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("create video file: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("write video file: %w", err)
	}

	return nil
}

// saveBase64VideoToFile saves base64 encoded video data to file
func (r *VideoOverviewResult) saveBase64VideoToFile(base64Data, filename string) error {
	videoBytes, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return fmt.Errorf("decode video data: %w", err)
	}

	if err := os.WriteFile(filename, videoBytes, 0644); err != nil {
		return fmt.Errorf("write video file: %w", err)
	}

	return nil
}

// DownloadVideoWithAuth downloads a video using the client's authentication
func (c *Client) DownloadVideoWithAuth(ctx context.Context, videoURL, filename string) error {
	// Create HTTP client with timeout
	client := httpClientWithTimeout(300 * time.Second)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", videoURL, nil)
	if err != nil {
		return fmt.Errorf("create video download request: %w", err)
	}

	// Add browser-like headers
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.6")
	req.Header.Set("Range", "bytes=0-")
	req.Header.Set("Referer", "https://notebooklm.google.com/")
	req.Header.Set("Sec-Fetch-Dest", "video")
	req.Header.Set("Sec-Fetch-Mode", "no-cors")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36")

	cookies := c.rpc.Config.Cookies
	if cookies != "" {
		req.Header.Set("Cookie", cookies)
	}

	authToken := c.rpc.Config.AuthToken
	if authToken != "" && !strings.Contains(videoURL, "authuser=") {
		separator := "?"
		if strings.Contains(videoURL, "?") {
			separator = "&"
		}
		req.URL, _ = url.Parse(videoURL + separator + "authuser=" + c.authUserOrDefault())
	}

	if c.config.Debug {
		fmt.Printf("Downloading video from: %s\n", req.URL.String())
		fmt.Printf("Using cookies: %v\n", cookies != "")
	}

	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download video: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("download failed with status: %s (check authentication)", resp.Status)
	}

	// Create output file
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("create video file: %w", err)
	}
	defer file.Close()

	// Copy with progress if debug enabled
	if c.config.Debug {
		// Get content length for progress
		contentLength := resp.ContentLength
		if contentLength > 0 {
			fmt.Printf("Video size: %.2f MB\n", float64(contentLength)/(1024*1024))
		}
	}

	// Copy the video data
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("write video file: %w", err)
	}

	return nil
}
