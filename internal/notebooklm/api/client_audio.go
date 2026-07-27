package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/beprotojson"
	"github.com/tmc/nlm/internal/notebooklm/rpc"
	"google.golang.org/protobuf/proto"
)

func (c *Client) CreateAudioOverview(ctx context.Context, projectID string, instructions string) (*AudioOverviewResult, error) {
	return c.CreateAudioOverviewWithOptions(ctx, projectID, CreateAudioOverviewOptions{
		Instructions: instructions,
	})
}

func (c *Client) CreateAudioOverviewWithOptions(ctx context.Context, projectID string, opts CreateAudioOverviewOptions) (*AudioOverviewResult, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project ID required")
	}
	opts = opts.withDefaults()

	// Use direct RPC if configured
	if c.config.UseDirectRPC && len(opts.SourceIDs) == 0 &&
		opts.AudioType == pb.AudioType_AUDIO_TYPE_DEEP_DIVE &&
		opts.Length == pb.AudioLength_AUDIO_LENGTH_DEFAULT &&
		opts.Language == "en" {
		return c.createAudioOverviewDirectRPC(ctx, projectID, opts.Instructions)
	}

	sourceIDs, err := c.createArtifactSourceIDs(ctx, projectID, opts.SourceIDs)
	if err != nil {
		return nil, err
	}
	if len(sourceIDs) == 0 {
		return nil, fmt.Errorf("project has no sources - add sources before creating audio overview")
	}
	if opts.Instructions == "" && opts.AudioType == pb.AudioType_AUDIO_TYPE_DEEP_DIVE &&
		opts.Length == pb.AudioLength_AUDIO_LENGTH_DEFAULT && opts.Language == "en" {
		audioSources := make([]*pb.SourceIdList, 0, len(sourceIDs))
		for _, sourceID := range sourceIDs {
			audioSources = append(audioSources, &pb.SourceIdList{SourceId: sourceID})
		}
		artifact, err := c.orchestrationService.CreateUniversalArtifact(ctx, &pb.CreateUniversalArtifactRequest{
			Context:   universalArtifactRequestContext(),
			ProjectId: projectID,
			Options: &pb.UniversalArtifactOptions{
				Kind:         1,
				SourceGroups: universalArtifactSourceGroups(sourceIDs),
				Audio: &pb.UniversalAudioOptions{Details: &pb.UniversalAudioDetails{
					Style:    int32(opts.Length),
					Sources:  audioSources,
					Language: opts.Language,
					Enabled:  int32(opts.AudioType),
				}},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("create audio overview: %w", wrapCreateAudioOverviewError(err))
		}
		return &AudioOverviewResult{ProjectID: projectID, AudioID: artifact.GetArtifactId(), Title: artifact.GetTitle(), IsReady: false}, nil
	}

	// Default: use orchestration service with new proto fields
	req := &pb.CreateAudioOverviewRequest{
		ProjectId:          projectID,
		AudioType:          opts.AudioType,
		SourceIds:          sourceIDs,
		CustomInstructions: opts.Instructions,
		Length:             opts.Length,
		Language:           opts.Language,
	}
	artifact, err := c.orchestrationService.CreateAudioOverview(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create audio overview: %w", wrapCreateAudioOverviewError(err))
	}
	// R7cb6c returns an artifact creation acknowledgment, not audio data.
	// Audio data must be fetched later via polling (audio-get/audio-download).
	result := &AudioOverviewResult{
		ProjectID: projectID,
		AudioID:   artifact.GetArtifactId(),
		Title:     artifact.GetTitle(),
		IsReady:   false, // Audio generation is always async
	}
	return result, nil
}

// createAudioOverviewDirectRPC uses direct RPC calls (original implementation)
func (c *Client) createAudioOverviewDirectRPC(ctx context.Context, projectID string, instructions string) (*AudioOverviewResult, error) {
	resp, err := c.rpc.Do(ctx, rpc.Call{
		ID: rpc.RPCCreateAudioOverview,
		Args: []interface{}{
			projectID,
			0, // audio_type
			[]string{instructions},
		},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("create audio overview (direct RPC): %w", err)
	}

	// Parse response - handle the actual response format
	// Response format: [[2,null,"audio-id"]] where 2 is success status
	var data []interface{}
	if err := json.Unmarshal(resp, &data); err != nil {
		// Try parsing as a different structure
		var strData string
		if err2 := json.Unmarshal(resp, &strData); err2 == nil {
			// Response might be a JSON string that needs double parsing
			if err3 := json.Unmarshal([]byte(strData), &data); err3 != nil {
				return nil, fmt.Errorf("parse response: %w", err)
			}
		} else {
			return nil, fmt.Errorf("parse response JSON: %w", err)
		}
	}

	result := &AudioOverviewResult{
		ProjectID: projectID,
		IsReady:   false, // Audio generation is async
	}

	// Extract fields from the actual response format
	if len(data) > 0 {
		if audioData, ok := data[0].([]interface{}); ok && len(audioData) > 0 {
			// First element is status (2 = success)
			if len(audioData) > 0 {
				if status, ok := audioData[0].(float64); ok && status == 2 {
					// Success status
					result.IsReady = false // Still processing
				}
			}
			// Third element is the audio ID
			if len(audioData) > 2 {
				if id, ok := audioData[2].(string); ok {
					result.AudioID = id
					// Log for debugging
					if c.config.Debug {
						fmt.Printf("Audio creation initiated with ID: %s\n", id)
					}
				}
			}
		}
	}

	return result, nil
}

func (c *Client) GetAudioOverview(ctx context.Context, projectID string) (*AudioOverviewResult, error) {
	// Try direct RPC first if enabled, as it provides more complete data
	if c.config.UseDirectRPC {
		return c.getAudioOverviewDirectRPC(ctx, projectID)
	}

	req := &pb.GetAudioOverviewRequest{
		ProjectId:   projectID,
		RequestType: 1,
	}
	audioOverview, err := c.orchestrationService.GetAudioOverview(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get audio overview: %w", err)
	}
	result := audioOverviewResultFromProto(projectID, audioOverview)
	if result.AudioID != "" || result.AudioData != "" || result.Title != "" {
		return result, nil
	}

	fallback, err := c.getAudioOverviewDirectRPC(ctx, projectID)
	if err == nil {
		mergeAudioOverviewResult(result, fallback)
	}
	return result, nil
}

func audioOverviewResultFromProto(projectID string, audioOverview *pb.AudioOverview) *AudioOverviewResult {
	result := &AudioOverviewResult{ProjectID: projectID}
	if audioOverview == nil {
		return result
	}

	result.AudioID = audioOverview.GetAudioId()
	result.Title = audioOverview.GetTitle()
	result.AudioData = audioOverview.GetContent()
	if status := audioOverview.GetStatus(); status != "" {
		result.IsReady = status != "CREATING"
	}
	return result
}

// getAudioOverviewDirectRPC uses direct RPC to get audio overview
func (c *Client) getAudioOverviewDirectRPC(ctx context.Context, projectID string) (*AudioOverviewResult, error) {
	result, err := c.getAudioOverviewDirectRPCArgs(ctx, projectID, []interface{}{projectID})
	if err == nil && (result.AudioID != "" || result.AudioData != "" || result.Title != "") {
		return result, nil
	}
	return c.getAudioOverviewDirectRPCWithType(ctx, projectID, 1)
}

// getAudioOverviewDirectRPCWithType uses direct RPC with a specific request type
func (c *Client) getAudioOverviewDirectRPCWithType(ctx context.Context, projectID string, requestType int) (*AudioOverviewResult, error) {
	return c.getAudioOverviewDirectRPCArgs(ctx, projectID, []interface{}{
		projectID,
		requestType, // request_type - try different values
	})
}

func (c *Client) getAudioOverviewDirectRPCArgs(ctx context.Context, projectID string, args []interface{}) (*AudioOverviewResult, error) {
	resp, err := c.rpc.Do(ctx, rpc.Call{
		ID:         rpc.RPCGetAudioOverview,
		Args:       args,
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("get audio overview (direct RPC): %w", err)
	}

	// Parse response
	var data []interface{}
	if err := json.Unmarshal(resp, &data); err != nil {
		return nil, fmt.Errorf("parse response JSON: %w", err)
	}
	return audioOverviewResultFromRPC(projectID, data), nil
}

func audioOverviewResultFromRPC(projectID string, data []interface{}) *AudioOverviewResult {
	result := &AudioOverviewResult{
		ProjectID: projectID,
	}

	if detail, ok := interfaceSliceAt(data, 2); ok {
		result.AudioData = stringAt(detail, 1)
		result.AudioID = stringAt(detail, 2)
		result.Title = stringAt(detail, 3)
		if ready, ok := boolAt(detail, 5); ok {
			result.IsReady = ready
		}
		return result
	}

	if legacy, ok := interfaceSliceAt(data, 0); ok {
		if status := stringAt(legacy, 0); status != "" {
			result.IsReady = status != "CREATING"
		}
		result.AudioData = stringAt(legacy, 1)
		result.Title = stringAt(legacy, 2)
	}

	return result
}

func mergeAudioOverviewResult(dst, src *AudioOverviewResult) {
	if dst == nil || src == nil {
		return
	}
	if dst.AudioID == "" {
		dst.AudioID = src.AudioID
	}
	if dst.Title == "" {
		dst.Title = src.Title
	}
	if dst.AudioData == "" {
		dst.AudioData = src.AudioData
	}
	if !dst.IsReady {
		dst.IsReady = src.IsReady
	}
}

func mergeAudioOverviewLists(existing []*AudioOverviewResult, extras ...*AudioOverviewResult) []*AudioOverviewResult {
	merged := make([]*AudioOverviewResult, 0, len(existing)+len(extras))
	byID := make(map[string]*AudioOverviewResult, len(existing)+len(extras))

	appendOverview := func(overview *AudioOverviewResult) {
		if overview == nil {
			return
		}
		if overview.AudioID == "" && overview.Title == "" && overview.AudioData == "" {
			return
		}
		if overview.AudioID != "" {
			if current := byID[overview.AudioID]; current != nil {
				mergeAudioOverviewResult(current, overview)
				return
			}
		}
		copy := *overview
		merged = append(merged, &copy)
		if copy.AudioID != "" {
			byID[copy.AudioID] = merged[len(merged)-1]
		}
	}

	for _, overview := range existing {
		appendOverview(overview)
	}
	for _, overview := range extras {
		appendOverview(overview)
	}
	return merged
}

func audioOverviewResultsFromArtifacts(projectID string, resp []byte) ([]*AudioOverviewResult, error) {
	var responseData []interface{}
	if err := json.Unmarshal(resp, &responseData); err != nil {
		return nil, fmt.Errorf("parse artifacts response: %w", err)
	}

	items := responseData
	if wrapped, ok := interfaceSliceAt(responseData, 0); ok {
		if len(wrapped) == 0 {
			items = wrapped
		} else if _, ok := wrapped[0].([]interface{}); ok {
			items = wrapped
		}
	}

	overviews := make([]*AudioOverviewResult, 0, len(items))
	for _, item := range items {
		overview := audioOverviewResultFromArtifact(projectID, item)
		if overview != nil {
			overviews = append(overviews, overview)
		}
	}
	return overviews, nil
}

func audioOverviewResultFromArtifact(projectID string, data interface{}) *AudioOverviewResult {
	artifactData, ok := data.([]interface{})
	if !ok || len(artifactData) == 0 {
		return nil
	}

	audioID := stringAt(artifactData, 0)
	if audioID == "" {
		return nil
	}
	typeCode, ok := int32At(artifactData, 2)
	if !ok || pb.ArtifactType(typeCode) != pb.ArtifactType_ARTIFACT_TYPE_AUDIO_OVERVIEW {
		return nil
	}

	stateCode, _ := int32At(artifactData, 4)
	return &AudioOverviewResult{
		ProjectID: projectID,
		AudioID:   audioID,
		Title:     stringAt(artifactData, 1),
		IsReady:   pb.ArtifactState(stateCode) == pb.ArtifactState_ARTIFACT_STATE_READY,
	}
}

func wrapCreateAudioOverviewError(err error) error {
	if err == nil {
		return nil
	}

	var apiErr *batchexecute.APIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode != nil && apiErr.ErrorCode.Type == batchexecute.ErrorTypeUnavailable {
		return fmt.Errorf("%w; NotebookLM usually returns this when the notebook does not yet contain enough source text for audio generation", err)
	}
	if strings.Contains(err.Error(), "API error 3 (Unavailable)") || strings.Contains(err.Error(), "Service unavailable") {
		return fmt.Errorf("%w; NotebookLM usually returns this when the notebook does not yet contain enough source text for audio generation", err)
	}
	return err
}

func interfaceSliceAt(values []interface{}, idx int) ([]interface{}, bool) {
	if idx < 0 || idx >= len(values) {
		return nil, false
	}
	slice, ok := values[idx].([]interface{})
	return slice, ok
}

func stringAt(values []interface{}, idx int) string {
	if idx < 0 || idx >= len(values) {
		return ""
	}
	s, _ := values[idx].(string)
	return s
}

func boolAt(values []interface{}, idx int) (bool, bool) {
	if idx < 0 || idx >= len(values) {
		return false, false
	}
	b, ok := values[idx].(bool)
	return b, ok
}

func int32At(values []interface{}, idx int) (int32, bool) {
	if idx < 0 || idx >= len(values) {
		return 0, false
	}
	f, ok := values[idx].(float64)
	if !ok {
		return 0, false
	}
	return int32(f), true
}

// AudioOverviewResult represents an audio overview response
type AudioOverviewResult struct {
	ProjectID string
	AudioID   string
	Title     string
	AudioData string // Base64 encoded audio data
	IsReady   bool
}

// AudioBytes returns the decoded audio data
func (r *AudioOverviewResult) AudioBytes() ([]byte, error) {
	if r.AudioData == "" {
		return nil, fmt.Errorf("no audio data available")
	}
	return base64.StdEncoding.DecodeString(r.AudioData)
}

func (c *Client) DeleteAudioOverview(ctx context.Context, projectID string) error {
	req := &pb.DeleteAudioOverviewRequest{
		ProjectId: projectID,
	}
	_, err := c.orchestrationService.DeleteAudioOverview(ctx, req)
	if err != nil {
		return fmt.Errorf("delete audio overview: %w", err)
	}
	return nil
}

// AudioFormat is a typed view of one entry in the GetAudioFormats
// response. The proto-generated AudioFormat lives behind a pending
// gen/ regeneration; this local mirror exists so callers don't have
// to wait for that to use the shape.
type AudioFormat struct {
	ID          int32
	Name        string
	Description string
}

// GetAudioFormats dispatches the sqTeoe RPC to retrieve the available
// audio-overview kinds (Deep Dive, Brief, Critique, Debate, …). The
// request is a fixed sentinel payload — no parameters — and the
// response carries video/slide/document-template variants alongside
// audio. We surface just the audio kinds; other inner arrays are
// reachable via the raw payload but have no typed parsers yet.
//
// HAR-verified against 11+ NotebookLM web UI captures (2026-04-19+);
// see proto/notebooklm/v1alpha1/orchestration.proto:1505 for the
// canonical shape and the four observed kinds.
func (c *Client) GetAudioFormats(ctx context.Context) ([]AudioFormat, error) {
	// Fixed sentinel captured from the web UI. Keep its context typed so the
	// generated encoder owns the positional envelope.
	req := &pb.GetAudioFormatsRequest{
		Context: &pb.RequestContext{
			Version: proto.Int32(2),
			Caps: &pb.RequestClientCaps{
				Version:         proto.Int32(1),
				CapabilityCodes: []int32{1},
			},
			ArtifactTypes: &pb.RequestArtifactTypeFilter{Types: []int32{1, 4, 8, 10, 2, 3, 6, 9}},
		},
		Mode: proto.Int32(1),
	}
	resp, err := c.rpc.Do(ctx, rpc.Call{
		ID:   rpc.RPCGetAudioFormats,
		Args: method.EncodeGetAudioFormatsArgs(req),
	})
	if err != nil {
		return nil, fmt.Errorf("get audio formats: %w", err)
	}
	var raw []interface{}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return nil, fmt.Errorf("parse audio formats: %w", err)
	}
	// Response: [[<audio_kinds>], [<video_kinds>], [<slide_kinds>], ...]
	if len(raw) == 0 {
		return nil, nil
	}
	audioKinds, ok := raw[0].([]interface{})
	if !ok {
		return nil, fmt.Errorf("audio formats: unexpected response shape")
	}
	out := make([]AudioFormat, 0, len(audioKinds))
	for _, k := range audioKinds {
		row, ok := k.([]interface{})
		if !ok || len(row) < 1 {
			continue
		}
		var f AudioFormat
		if id, ok := row[0].(float64); ok {
			f.ID = int32(id)
		}
		if len(row) >= 2 {
			if name, ok := row[1].(string); ok {
				f.Name = name
			}
		}
		if len(row) >= 3 {
			if desc, ok := row[2].(string); ok {
				f.Description = desc
			}
		}
		out = append(out, f)
	}
	return out, nil
}

// DownloadAudioOverview attempts to download the actual audio file
// by querying for audio artifacts and downloading from the URL
func (c *Client) DownloadAudioOverview(ctx context.Context, projectID string) (*AudioOverviewResult, error) {
	audioOverview, err := c.GetAudioOverview(ctx, projectID)
	if err == nil && audioOverview != nil && audioOverview.AudioData != "" {
		return audioOverview, nil
	}

	// Query for audio artifacts using direct RPC (response format is complex)
	resp, err := c.rpc.Do(ctx, rpc.Call{
		ID: rpc.RPCListArtifacts, // Use gArtLc RPC
		Args: []interface{}{
			[]interface{}{2}, // artifact_types=[2] for ARTIFACT_TYPE_AUDIO_OVERVIEW
			projectID,
			"NOT artifact.status = \"ARTIFACT_STATUS_SUGGESTED\"",
		},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("query audio artifacts: %w", err)
	}

	// Parse response - RPC client already extracts and parses the nested JSON for us
	var responseData []interface{}
	if err := json.Unmarshal(resp, &responseData); err != nil {
		return nil, fmt.Errorf("parse artifacts response: %w", err)
	}

	if c.config.Debug {
		fmt.Printf("Query artifacts response: %d top-level elements\n", len(responseData))
	}

	// Response is already parsed by RPC client: [[artifact1, artifact2, ...]]
	if len(responseData) == 0 {
		return nil, fmt.Errorf("no audio overview found for this notebook")
	}

	// Get the artifacts array (first element)
	artifacts, ok := responseData[0].([]interface{})
	if !ok || len(artifacts) == 0 {
		return nil, fmt.Errorf("no audio artifacts found")
	}

	if c.config.Debug {
		fmt.Printf("Found %d artifacts\n", len(artifacts))
	}

	// Get first artifact (most recent)
	artifactData, ok := artifacts[0].([]interface{})
	if !ok || len(artifactData) < 7 {
		return nil, fmt.Errorf("invalid artifact data structure (need at least 7 elements, got %d)", len(artifactData))
	}

	if c.config.Debug {
		fmt.Printf("Artifact data has %d elements\n", len(artifactData))
		// Print first 12 elements to find the URL - including deep nested arrays
		for i := 0; i < len(artifactData) && i < 12; i++ {
			fmt.Printf("  [%d] type=%T\n", i, artifactData[i])
			if str, ok := artifactData[i].(string); ok && len(str) > 0 && len(str) < 200 {
				fmt.Printf("      value=%s\n", str)
			}
			// Check nested arrays for URLs
			if arr, ok := artifactData[i].([]interface{}); ok && len(arr) > 0 {
				fmt.Printf("      array length=%d\n", len(arr))
				for j := 0; j < len(arr) && j < 20; j++ {
					if str, ok := arr[j].(string); ok {
						if strings.HasPrefix(str, "https://") {
							displayStr := str
							if len(str) > 80 {
								displayStr = str[:80] + "..."
							}
							fmt.Printf("      [%d][%d]=%s\n", i, j, displayStr)
						} else if len(str) < 100 {
							// Also show short non-URL strings (mime types, etc)
							fmt.Printf("      [%d][%d]=%q\n", i, j, str)
						}
					}
					// Check double-nested arrays
					if nestedArr, ok := arr[j].([]interface{}); ok && len(nestedArr) > 0 {
						fmt.Printf("      [%d][%d] is array with length %d\n", i, j, len(nestedArr))
						for k := 0; k < len(nestedArr) && k < 10; k++ {
							if str, ok := nestedArr[k].(string); ok {
								if strings.HasPrefix(str, "https://") {
									displayStr := str
									if len(str) > 80 {
										displayStr = str[:80] + "..."
									}
									fmt.Printf("        [%d][%d][%d]=%s\n", i, j, k, displayStr)
								} else if len(str) < 100 {
									fmt.Printf("        [%d][%d][%d]=%q\n", i, j, k, str)
								}
							}
							// Check for numbers (mime types, sizes, etc)
							if num, ok := nestedArr[k].(float64); ok {
								fmt.Printf("        [%d][%d][%d]=%v\n", i, j, k, num)
							}
						}
					}
				}
			}
		}
	}

	// Extract fields from artifact
	// Format: [audio_id, title, type, sources, state, ?, audio_overview, ...]
	// audio_overview at index 6 contains: [?, ?, audio_url, video_url, ...]
	audioID, _ := artifactData[0].(string)
	title, _ := artifactData[1].(string)

	// Get audio overview array at index 6
	audioPayload, ok := artifactData[6].([]interface{})
	if !ok || len(audioPayload) < 6 {
		return nil, fmt.Errorf("audio overview data not found or incomplete (has %d elements, need at least 6)", len(audioPayload))
	}

	// Audio URLs are in a nested array at audioOverview[5]
	// Format: [[url1, type1, mime1], [url2, type2, mime2], ...]
	audioURLList, ok := audioPayload[5].([]interface{})
	if !ok || len(audioURLList) == 0 {
		return nil, fmt.Errorf("audio URL list not found - audio may not be ready yet")
	}

	if c.config.Debug {
		fmt.Printf("Found %d audio format options in nested array\n", len(audioURLList))
	}

	// Try to find a URL that doesn't require authentication redirect
	// Prefer URLs with =m140-dv or =m140 format (direct download formats)
	var audioURL string
	for i, urlData := range audioURLList {
		if urlArr, ok := urlData.([]interface{}); ok && len(urlArr) > 0 {
			if url, ok := urlArr[0].(string); ok && url != "" {
				// Try all URLs, preferring certain formats
				// Format 0: usually =m140-dv (type 4, audio/mp4)
				// Format 1: usually =m140 (type 1, audio/mp4)
				if audioURL == "" || i == 0 {
					audioURL = url
					if c.config.Debug {
						display := url
						if len(display) > 80 {
							display = display[:80] + "..."
						}
						var mimeType string
						if len(urlArr) > 2 {
							mimeType, _ = urlArr[2].(string)
						}
						fmt.Printf("  Format %d: %s (mime: %s)\n", i, display, mimeType)
					}
				}
			}
		}
	}

	if audioURL == "" {
		return nil, fmt.Errorf("audio URL not found in URL list")
	}

	if c.config.Debug {
		fmt.Printf("Found audio: %s\n", title)
		fmt.Printf("Downloading audio from: %s\n", audioURL)
	}

	// Download the audio from the URL
	audioData, err := c.downloadAudioFromURL(ctx, audioURL)
	if err != nil {
		return nil, fmt.Errorf("download audio from URL: %w", err)
	}

	result := &AudioOverviewResult{
		ProjectID: projectID,
		AudioID:   audioID,
		Title:     title,
		AudioData: base64.StdEncoding.EncodeToString(audioData),
		IsReady:   true,
	}

	return result, nil
}

// downloadAudioFromURL downloads audio data from a googleusercontent URL
// Google CDN URLs require full browser authentication context, so we use chromedp
func (c *Client) downloadAudioFromURL(ctx context.Context, audioURL string) ([]byte, error) {
	// Create client that follows redirects automatically
	client := httpClientWithTimeout(60 * time.Second)
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		// Allow up to 10 redirects
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		// Copy cookies to redirect requests
		if len(via) > 0 {
			for _, cookie := range via[0].Cookies() {
				req.AddCookie(cookie)
			}
		}
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", audioURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Add browser-like headers and authentication cookies
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Referer", "https://notebooklm.google.com/")

	// Add authentication cookies from RPC client
	if cookies := c.rpc.Config.Cookies; cookies != "" {
		req.Header.Set("Cookie", cookies)
	}

	if c.config.Debug {
		fmt.Printf("Full audio URL: %s\n", audioURL)
		fmt.Printf("Using cookies: %v\n", c.rpc.Config.Cookies != "")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if c.config.Debug {
		fmt.Printf("Response status: %d\n", resp.StatusCode)
		fmt.Printf("Content-Type: %s\n", resp.Header.Get("Content-Type"))
	}

	// Check if we got an HTML auth redirect page
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/html") {
		// HTML response indicates authentication failure - use browser download
		if c.config.Debug {
			fmt.Printf("Got HTML auth redirect, falling back to browser download\n")
		}
		return nil, fmt.Errorf("google CDN requires browser authentication; use 'nlm audio download' to open in browser")
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if c.config.Debug {
		fmt.Printf("Downloaded %d bytes of audio data\n", len(audioData))
	}

	return audioData, nil
}

// SaveAudioToFile saves audio data to a file
func (r *AudioOverviewResult) SaveAudioToFile(filename string) error {
	if r.AudioData == "" {
		return fmt.Errorf("no audio data to save")
	}

	audioBytes, err := r.AudioBytes()
	if err != nil {
		return fmt.Errorf("decode audio data: %w", err)
	}

	if err := os.WriteFile(filename, audioBytes, 0644); err != nil {
		return fmt.Errorf("write audio file: %w", err)
	}

	return nil
}

// ListAudioOverviews returns audio overviews for a notebook
func (c *Client) ListAudioOverviews(ctx context.Context, projectID string) ([]*AudioOverviewResult, error) {
	var overviews []*AudioOverviewResult

	resp, err := c.rpc.Do(ctx, rpc.Call{
		ID: rpc.RPCListArtifacts,
		Args: method.EncodeListArtifactsArgs(&pb.ListArtifactsRequest{
			Context:   universalArtifactRequestContext(),
			ProjectId: projectID,
			Filter:    `NOT artifact.status = "ARTIFACT_STATUS_SUGGESTED"`,
		}),
		NotebookID: projectID,
	})
	if err == nil {
		var parseErr error
		overviews, parseErr = audioOverviewResultsFromProtoArtifactsWithOptions(projectID, resp, c.unmarshalOptions())
		if c.config.Debug && parseErr != nil {
			fmt.Printf("Error parsing audio overview artifacts: %v\n", parseErr)
		}
	}
	if err != nil && c.config.Debug {
		fmt.Printf("Error listing audio overview artifacts: %v\n", err)
	}

	audioOverview, err := c.GetAudioOverview(ctx, projectID)
	if err != nil {
		if len(overviews) > 0 {
			return overviews, nil
		}
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "does not exist") {
			return []*AudioOverviewResult{}, nil
		}
		if c.config.Debug {
			fmt.Printf("Error getting audio overview: %v\n", err)
		}
		return []*AudioOverviewResult{}, nil
	}
	if audioOverview != nil && (audioOverview.AudioData != "" || audioOverview.IsReady || audioOverview.AudioID != "") {
		overviews = mergeAudioOverviewLists(overviews, audioOverview)
	}
	if len(overviews) > 0 {
		return overviews, nil
	}
	return []*AudioOverviewResult{}, nil
}

func audioOverviewResultsFromProtoArtifacts(projectID string, raw []byte) ([]*AudioOverviewResult, error) {
	return audioOverviewResultsFromProtoArtifactsWithOptions(projectID, raw, beprotojson.UnmarshalOptions{DiscardUnknown: true})
}

func audioOverviewResultsFromProtoArtifactsWithOptions(projectID string, raw []byte, options beprotojson.UnmarshalOptions) ([]*AudioOverviewResult, error) {
	var response pb.ListArtifactsResponse
	if err := options.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("decode artifact response: %w", err)
	}
	overviews := make([]*AudioOverviewResult, 0)
	for _, artifact := range response.GetArtifacts() {
		if artifact == nil || artifact.GetType() != pb.ArtifactType_ARTIFACT_TYPE_AUDIO_OVERVIEW {
			continue
		}
		overviews = append(overviews, &AudioOverviewResult{
			ProjectID: projectID,
			AudioID:   artifact.GetArtifactId(),
			Title:     artifact.GetTitle(),
			IsReady:   artifact.GetState() == pb.ArtifactState_ARTIFACT_STATE_READY,
		})
	}
	return overviews, nil
}
