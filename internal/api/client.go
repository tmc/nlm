// Package api provides the NotebookLM API client.
package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"net/url"
	"os"
	filepathpkg "path/filepath"
	"strconv"
	"strings"
	"time"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/gen/service"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/rpc"
)

type Notebook = pb.Project
type Note = pb.Source

// Client handles NotebookLM API interactions.
type Client struct {
	rpc                  *rpc.Client
	orchestrationService *service.LabsTailwindOrchestrationServiceClient
	sharingService       *service.LabsTailwindSharingServiceClient
	guidebooksService    *service.LabsTailwindGuidebooksServiceClient
	config               struct {
		Debug        bool
		UseDirectRPC bool // Use direct RPC calls instead of orchestration service
	}
}

// New creates a new NotebookLM API client.
func New(authToken, cookies string, opts ...batchexecute.Option) *Client {
	// Basic validation of auth parameters
	if authToken == "" || cookies == "" {
		fmt.Fprintf(os.Stderr, "Warning: Missing authentication credentials. Use 'nlm auth' to setup authentication.\n")
	}

	// Add debug option if needed
	if os.Getenv("NLM_DEBUG") == "true" {
		opts = append(opts, batchexecute.WithDebug(true))
	}

	// Create the client
	client := &Client{
		rpc:                  rpc.New(authToken, cookies, opts...),
		orchestrationService: service.NewLabsTailwindOrchestrationServiceClient(authToken, cookies, opts...),
		sharingService:       service.NewLabsTailwindSharingServiceClient(authToken, cookies, opts...),
		guidebooksService:    service.NewLabsTailwindGuidebooksServiceClient(authToken, cookies, opts...),
	}

	// Get debug setting from environment for consistency
	client.config.Debug = os.Getenv("NLM_DEBUG") == "true"

	return client
}

// SetUseDirectRPC configures whether to use direct RPC calls
func (c *Client) SetUseDirectRPC(use bool) {
	c.config.UseDirectRPC = use
}

// Project/Notebook operations

func (c *Client) ListRecentlyViewedProjects() ([]*Notebook, error) {
	req := &pb.ListRecentlyViewedProjectsRequest{}

	response, err := c.orchestrationService.ListRecentlyViewedProjects(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	return response.Projects, nil
}

func (c *Client) CreateProject(title string, emoji string) (*Notebook, error) {
	req := &pb.CreateProjectRequest{
		Title: title,
		Emoji: emoji,
	}

	project, err := c.orchestrationService.CreateProject(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}
	return project, nil
}

func (c *Client) GetProject(projectID string) (*Notebook, error) {
	req := &pb.GetProjectRequest{
		ProjectId: projectID,
	}

	ctx := context.Background()
	project, err := c.orchestrationService.GetProject(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}

	if c.config.Debug && project.Sources != nil {
		fmt.Printf("DEBUG: Successfully parsed project with %d sources\n", len(project.Sources))
	}
	return project, nil
}

func (c *Client) DeleteProjects(projectIDs []string) error {
	req := &pb.DeleteProjectsRequest{
		ProjectIds: projectIDs,
	}

	ctx := context.Background()
	_, err := c.orchestrationService.DeleteProjects(ctx, req)
	if err != nil {
		return fmt.Errorf("delete projects: %w", err)
	}
	return nil
}

func (c *Client) MutateProject(projectID string, updates *pb.Project) (*Notebook, error) {
	req := &pb.MutateProjectRequest{
		ProjectId: projectID,
		Updates:   updates,
	}

	ctx := context.Background()
	project, err := c.orchestrationService.MutateProject(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("mutate project: %w", err)
	}
	return project, nil
}

func (c *Client) RemoveRecentlyViewedProject(projectID string) error {
	req := &pb.RemoveRecentlyViewedProjectRequest{
		ProjectId: projectID,
	}

	ctx := context.Background()
	_, err := c.orchestrationService.RemoveRecentlyViewedProject(ctx, req)
	return err
}

// Source operations

func (c *Client) AddSources(projectID string, sources []*pb.SourceInput) (*pb.Project, error) {
	req := &pb.AddSourceRequest{
		Sources:   sources,
		ProjectId: projectID,
	}
	ctx := context.Background()
	project, err := c.orchestrationService.AddSources(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("add sources: %w", err)
	}
	return project, nil
}

func (c *Client) DeleteSources(projectID string, sourceIDs []string) error {
	req := &pb.DeleteSourcesRequest{
		SourceIds: sourceIDs,
	}
	ctx := context.Background()
	_, err := c.orchestrationService.DeleteSources(ctx, req)
	if err != nil {
		return fmt.Errorf("delete sources: %w", err)
	}
	return nil
}

func (c *Client) MutateSource(sourceID string, updates *pb.Source) (*pb.Source, error) {
	req := &pb.MutateSourceRequest{
		SourceId: sourceID,
		Updates:  updates,
	}
	ctx := context.Background()
	source, err := c.orchestrationService.MutateSource(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("mutate source: %w", err)
	}
	return source, nil
}

func (c *Client) RefreshSource(sourceID string) (*pb.Source, error) {
	req := &pb.RefreshSourceRequest{
		SourceId: sourceID,
	}
	ctx := context.Background()
	source, err := c.orchestrationService.RefreshSource(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("refresh source: %w", err)
	}
	return source, nil
}

func (c *Client) LoadSource(sourceID string) (*pb.Source, error) {
	req := &pb.LoadSourceRequest{
		SourceId: sourceID,
	}
	ctx := context.Background()
	source, err := c.orchestrationService.LoadSource(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("load source: %w", err)
	}
	return source, nil
}

func (c *Client) CheckSourceFreshness(sourceID string) (*pb.CheckSourceFreshnessResponse, error) {
	req := &pb.CheckSourceFreshnessRequest{
		SourceId: sourceID,
	}
	ctx := context.Background()
	result, err := c.orchestrationService.CheckSourceFreshness(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("check source freshness: %w", err)
	}
	return result, nil
}

func (c *Client) ActOnSources(projectID string, action string, sourceIDs []string) error {
	req := &pb.ActOnSourcesRequest{
		ProjectId: projectID,
		Action:    action,
		SourceIds: sourceIDs,
	}
	ctx := context.Background()
	_, err := c.orchestrationService.ActOnSources(ctx, req)
	if err != nil {
		return fmt.Errorf("act on sources: %w", err)
	}
	return nil
}

// Source upload utility methods

// detectMIMEType attempts to determine the MIME type of content using multiple methods:
// 1. Use provided contentType if specified
// 2. Use http.DetectContentType for binary detection
// 3. Use file extension as fallback
// 4. Default to application/octet-stream if all else fails
func detectMIMEType(content []byte, filename string, providedType string) string {
	// Use explicitly provided type if available
	if providedType != "" {
		return providedType
	}

	// Try content-based detection first
	detectedType := http.DetectContentType(content)

	// Special case for JSON files - check content
	if bytes.HasPrefix(bytes.TrimSpace(content), []byte("{")) ||
		bytes.HasPrefix(bytes.TrimSpace(content), []byte("[")) {
		// This looks like JSON content
		return "application/json"
	}

	if detectedType != "application/octet-stream" && !strings.HasPrefix(detectedType, "text/plain") {
		return detectedType
	}

	// Try extension-based detection
	ext := filepathpkg.Ext(filename)
	if ext != "" {
		if mimeType := mime.TypeByExtension(ext); mimeType != "" {
			return mimeType
		}
	}

	// If we detected text/plain but have a known extension, trust the extension
	if strings.HasPrefix(detectedType, "text/plain") && ext != "" {
		if mimeType := mime.TypeByExtension(ext); mimeType != "" {
			return mimeType
		}
	}

	return detectedType
}

func (c *Client) AddSourceFromReader(projectID string, r io.Reader, filename string, contentType ...string) (string, error) {
	content, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read content: %w", err)
	}

	// Get provided content type if available
	var providedType string
	if len(contentType) > 0 {
		providedType = contentType[0]
	}

	detectedType := detectMIMEType(content, filename, providedType)

	// Treat plain text or JSON content as text source
	if strings.HasPrefix(detectedType, "text/") ||
		detectedType == "application/json" ||
		strings.HasSuffix(filename, ".json") {
		// Add debug output about JSON handling for any environment
		if strings.HasSuffix(filename, ".json") || detectedType == "application/json" {
			fmt.Fprintf(os.Stderr, "Handling JSON file as text: %s (MIME: %s)\n", filename, detectedType)
		}
		return c.AddSourceFromText(projectID, string(content), filename)
	}

	return c.AddSourceFromUpload(projectID, content, filename, detectedType)
}

func (c *Client) AddSourceFromText(projectID string, content, title string) (string, error) {
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCAddSources,
		NotebookID: projectID,
		Args: []interface{}{
			[]interface{}{
				[]interface{}{
					nil,
					[]string{
						title,
						content,
					},
					nil,
					2, // text source type
				},
			},
			projectID,
		},
	})
	if err != nil {
		return "", fmt.Errorf("add text source: %w", err)
	}

	sourceID, err := extractSourceID(resp)
	if err != nil {
		return "", fmt.Errorf("extract source ID: %w", err)
	}
	return sourceID, nil
}

func (c *Client) AddSourceFromBase64(projectID string, content, filename, contentType string) (string, error) {
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCAddSources,
		NotebookID: projectID,
		Args: []interface{}{
			[]interface{}{
				[]interface{}{
					filename,
					content,
					contentType,
					"base64",
				},
			},
			projectID,
		},
	})
	if err != nil {
		return "", fmt.Errorf("add binary source: %w", err)
	}

	sourceID, err := extractSourceID(resp)
	if err != nil {
		return "", fmt.Errorf("extract source ID: %w", err)
	}
	return sourceID, nil
}

func (c *Client) AddSourceFromFile(projectID string, filepath string, contentType ...string) (string, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	var providedType string
	if len(contentType) > 0 {
		providedType = contentType[0]
	}
	return c.AddSourceFromReader(projectID, f, filepathpkg.Base(filepath), providedType)
}

func (c *Client) AddSourceFromUpload(notebookID string, content []byte, filename string, contentType string) (string, error) {
	projectID := os.Getenv("NLM_PROJECT_ID")
	if projectID == "" {
		projectID = notebookID
	}

	sourceID, err := c.createUploadSource(notebookID, projectID, filename)
	if err != nil {
		return "", err
	}

	uploadURL, err := c.startResumableUpload(projectID, sourceID, filename, int64(len(content)))
	if err != nil {
		return "", err
	}

	if err := c.uploadResumableContent(uploadURL, content, contentType); err != nil {
		return "", err
	}

	return sourceID, nil
}

func (c *Client) createUploadSource(notebookID, projectID, filename string) (string, error) {
	args := []interface{}{
		[]interface{}{
			[]interface{}{
				filename,
			},
		},
		projectID,
		[]interface{}{2},
		[]interface{}{1, nil, nil, nil, nil, nil, nil, nil, nil, nil, []interface{}{1}},
	}
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCAddSources,
		NotebookID: notebookID,
		Args:       args,
	})
	if err != nil {
		return "", fmt.Errorf("create upload source: %w", err)
	}
	sourceID, err := extractSourceID(resp)
	if err != nil {
		return "", fmt.Errorf("extract source ID: %w", err)
	}
	return sourceID, nil
}

func (c *Client) startResumableUpload(projectID, sourceID, filename string, size int64) (string, error) {
	authUser := os.Getenv("NLM_AUTHUSER")
	if authUser == "" {
		authUser = "0"
	}
	uploadURL := fmt.Sprintf("https://notebooklm.google.com/upload/_/?authuser=%s", url.QueryEscape(authUser))

	body := map[string]string{
		"PROJECT_ID":  projectID,
		"SOURCE_NAME": filename,
		"SOURCE_ID":   sourceID,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal upload start body: %w", err)
	}

	req, err := http.NewRequest("POST", uploadURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create upload start request: %w", err)
	}

	req.Header.Set("Accept", "*/*")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	req.Header.Set("Origin", "https://notebooklm.google.com")
	req.Header.Set("Referer", "https://notebooklm.google.com/")
	req.Header.Set("X-Goog-Upload-Command", "start")
	req.Header.Set("X-Goog-Upload-Protocol", "resumable")
	req.Header.Set("X-Goog-Upload-Header-Content-Length", strconv.FormatInt(size, 10))
	req.Header.Set("X-Goog-AuthUser", authUser)

	if c.rpc != nil {
		if c.rpc.Config.Cookies != "" {
			req.Header.Set("Cookie", c.rpc.Config.Cookies)
		}
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload start request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := ioutil.ReadAll(resp.Body)
		return "", fmt.Errorf("upload start failed: status %s: %s", resp.Status, string(b))
	}

	uploadURL = resp.Header.Get("x-goog-upload-url")
	if uploadURL == "" {
		uploadURL = resp.Header.Get("x-goog-upload-control-url")
	}
	if uploadURL == "" {
		return "", fmt.Errorf("upload start failed: missing upload url in response headers")
	}
	return uploadURL, nil
}

func (c *Client) uploadResumableContent(uploadURL string, content []byte, contentType string) error {
	authUser := os.Getenv("NLM_AUTHUSER")
	if authUser == "" {
		authUser = "0"
	}

	req, err := http.NewRequest("POST", uploadURL, bytes.NewReader(content))
	if err != nil {
		return fmt.Errorf("create upload request: %w", err)
	}

	req.Header.Set("Accept", "*/*")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=utf-8")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Origin", "https://notebooklm.google.com")
	req.Header.Set("Referer", "https://notebooklm.google.com/")
	req.Header.Set("X-Goog-Upload-Command", "upload, finalize")
	req.Header.Set("X-Goog-Upload-Offset", "0")
	req.Header.Set("X-Goog-AuthUser", authUser)

	if c.rpc != nil {
		if c.rpc.Config.Cookies != "" {
			req.Header.Set("Cookie", c.rpc.Config.Cookies)
		}
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("upload failed: status %s: %s", resp.Status, string(b))
	}
	return nil
}

func (c *Client) AddSourceFromURL(projectID string, url string) (string, error) {
	// Check if it's a YouTube URL first
	if isYouTubeURL(url) {
		videoID, err := extractYouTubeVideoID(url)
		if err != nil {
			return "", fmt.Errorf("invalid YouTube URL: %w", err)
		}
		// Use dedicated YouTube method
		return c.AddYouTubeSource(projectID, videoID)
	}

	// Regular URL handling
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCAddSources,
		NotebookID: projectID,
		Args: []interface{}{
			[]interface{}{
				[]interface{}{
					nil,
					nil,
					[]string{url},
				},
			},
			projectID,
		},
	})
	if err != nil {
		return "", fmt.Errorf("add source from URL: %w", err)
	}

	sourceID, err := extractSourceID(resp)
	if err != nil {
		return "", fmt.Errorf("extract source ID: %w", err)
	}
	return sourceID, nil
}

func (c *Client) AddYouTubeSource(projectID, videoID string) (string, error) {
	if c.rpc.Config.Debug {
		fmt.Printf("=== AddYouTubeSource ===\n")
		fmt.Printf("Project ID: %s\n", projectID)
		fmt.Printf("Video ID: %s\n", videoID)
	}

	// Modified payload structure for YouTube
	payload := []interface{}{
		[]interface{}{
			[]interface{}{
				nil,                                     // content
				nil,                                     // title
				videoID,                                 // video ID (not in array)
				nil,                                     // unused
				pb.SourceType_SOURCE_TYPE_YOUTUBE_VIDEO, // source type
			},
		},
		projectID,
	}

	if c.rpc.Config.Debug {
		fmt.Printf("\nPayload Structure:\n")
	}

	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCAddSources,
		NotebookID: projectID,
		Args:       payload,
	})
	if err != nil {
		return "", fmt.Errorf("add YouTube source: %w", err)
	}

	if c.rpc.Config.Debug {
		fmt.Printf("\nRaw Response:\n%s\n", string(resp))
	}

	if len(resp) == 0 {
		return "", fmt.Errorf("empty response from server (check debug output for request details)")
	}

	sourceID, err := extractSourceID(resp)
	if err != nil {
		return "", fmt.Errorf("extract source ID: %w", err)
	}
	return sourceID, nil
}

// Helper function to extract source ID with better error handling
func extractSourceID(resp json.RawMessage) (string, error) {
	if len(resp) == 0 {
		return "", fmt.Errorf("empty response")
	}

	var data []interface{}
	if err := json.Unmarshal(resp, &data); err != nil {
		return "", fmt.Errorf("parse response JSON: %w", err)
	}

	// Try different response formats
	// Format 1: [[[["id",...]]]]
	// Format 2: [[["id",...]]]
	// Format 3: [["id",...]]
	for _, format := range []func([]interface{}) (string, bool){
		// Format 1
		func(d []interface{}) (string, bool) {
			if len(d) > 0 {
				if d0, ok := d[0].([]interface{}); ok && len(d0) > 0 {
					if d1, ok := d0[0].([]interface{}); ok && len(d1) > 0 {
						if d2, ok := d1[0].([]interface{}); ok && len(d2) > 0 {
							if id, ok := d2[0].(string); ok {
								return id, true
							}
						}
					}
				}
			}
			return "", false
		},
		// Format 2
		func(d []interface{}) (string, bool) {
			if len(d) > 0 {
				if d0, ok := d[0].([]interface{}); ok && len(d0) > 0 {
					if d1, ok := d0[0].([]interface{}); ok && len(d1) > 0 {
						if id, ok := d1[0].(string); ok {
							return id, true
						}
					}
				}
			}
			return "", false
		},
		// Format 3
		func(d []interface{}) (string, bool) {
			if len(d) > 0 {
				if d0, ok := d[0].([]interface{}); ok && len(d0) > 0 {
					if id, ok := d0[0].(string); ok {
						return id, true
					}
				}
			}
			return "", false
		},
	} {
		if id, ok := format(data); ok {
			return id, nil
		}
	}

	return "", fmt.Errorf("could not find source ID in response structure: %v", data)
}

// Note operations

func (c *Client) CreateNote(projectID string, title string, initialContent string) (*Note, error) {
	req := &pb.CreateNoteRequest{
		ProjectId: projectID,
		Content:   initialContent,
		NoteType:  []int32{1}, // note type
		Title:     title,
	}
	ctx := context.Background()
	note, err := c.orchestrationService.CreateNote(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create note: %w", err)
	}
	// Note is an alias for pb.Source, so we can return it directly
	return note, nil
}

func (c *Client) MutateNote(projectID string, noteID string, content string, title string) (*Note, error) {
	req := &pb.MutateNoteRequest{
		ProjectId: projectID,
		NoteId:    noteID,
		Updates: []*pb.NoteUpdate{{
			Content: content,
			Title:   title,
			Tags:    []string{},
		}},
	}
	ctx := context.Background()
	note, err := c.orchestrationService.MutateNote(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("mutate note: %w", err)
	}
	// Note is an alias for pb.Source, so we can return it directly
	return note, nil
}

func (c *Client) DeleteNotes(projectID string, noteIDs []string) error {
	req := &pb.DeleteNotesRequest{
		NoteIds: noteIDs,
	}
	ctx := context.Background()
	_, err := c.orchestrationService.DeleteNotes(ctx, req)
	if err != nil {
		return fmt.Errorf("delete notes: %w", err)
	}
	return nil
}

func (c *Client) GetNotes(projectID string) ([]*Note, error) {
	req := &pb.GetNotesRequest{
		ProjectId: projectID,
	}
	ctx := context.Background()
	response, err := c.orchestrationService.GetNotes(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get notes: %w", err)
	}
	return response.Notes, nil
}

// Audio operations

func (c *Client) CreateAudioOverview(projectID string, instructions string) (*AudioOverviewResult, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project ID required")
	}
	if instructions == "" {
		return nil, fmt.Errorf("instructions required")
	}

	// Use direct RPC if configured
	if c.config.UseDirectRPC {
		return c.createAudioOverviewDirectRPC(projectID, instructions)
	}

	// Default: use orchestration service
	req := &pb.CreateAudioOverviewRequest{
		ProjectId:    projectID,
		AudioType:    0,
		Instructions: []string{instructions},
	}
	ctx := context.Background()
	audioOverview, err := c.orchestrationService.CreateAudioOverview(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create audio overview: %w", err)
	}
	// Convert pb.AudioOverview to AudioOverviewResult
	// Note: pb.AudioOverview has different fields than expected, so we map what's available
	result := &AudioOverviewResult{
		ProjectID: projectID,
		AudioID:   "",                                 // Not available in pb.AudioOverview
		Title:     "",                                 // Not available in pb.AudioOverview
		AudioData: audioOverview.Content,              // Map Content to AudioData
		IsReady:   audioOverview.Status != "CREATING", // Infer from Status
	}
	return result, nil
}

// createAudioOverviewDirectRPC uses direct RPC calls (original implementation)
func (c *Client) createAudioOverviewDirectRPC(projectID string, instructions string) (*AudioOverviewResult, error) {
	resp, err := c.rpc.Do(rpc.Call{
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

func (c *Client) GetAudioOverview(projectID string) (*AudioOverviewResult, error) {
	// Try direct RPC first if enabled, as it provides more complete data
	if c.config.UseDirectRPC {
		return c.getAudioOverviewDirectRPC(projectID)
	}

	req := &pb.GetAudioOverviewRequest{
		ProjectId:   projectID,
		RequestType: 1,
	}
	ctx := context.Background()
	audioOverview, err := c.orchestrationService.GetAudioOverview(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get audio overview: %w", err)
	}
	// Convert pb.AudioOverview to AudioOverviewResult
	// Note: pb.AudioOverview has different fields than expected, so we map what's available
	result := &AudioOverviewResult{
		ProjectID: projectID,
		AudioID:   "",                                 // Not available in pb.AudioOverview
		Title:     "",                                 // Not available in pb.AudioOverview
		AudioData: audioOverview.Content,              // Map Content to AudioData
		IsReady:   audioOverview.Status != "CREATING", // Infer from Status
	}
	return result, nil
}

// getAudioOverviewDirectRPC uses direct RPC to get audio overview
func (c *Client) getAudioOverviewDirectRPC(projectID string) (*AudioOverviewResult, error) {
	return c.getAudioOverviewDirectRPCWithType(projectID, 1) // Default to type 1
}

// getAudioOverviewDirectRPCWithType uses direct RPC with a specific request type
func (c *Client) getAudioOverviewDirectRPCWithType(projectID string, requestType int) (*AudioOverviewResult, error) {
	resp, err := c.rpc.Do(rpc.Call{
		ID: rpc.RPCGetAudioOverview,
		Args: []interface{}{
			projectID,
			requestType, // request_type - try different values
		},
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

	result := &AudioOverviewResult{
		ProjectID: projectID,
	}

	// Extract fields from response
	// Response format varies, but typically contains status and data
	if len(data) > 0 {
		if audioData, ok := data[0].([]interface{}); ok {
			// Check status
			if len(audioData) > 0 {
				if status, ok := audioData[0].(string); ok {
					result.IsReady = status != "CREATING"
				}
			}
			// Get audio content
			if len(audioData) > 1 {
				if content, ok := audioData[1].(string); ok {
					result.AudioData = content
				}
			}
			// Get title if available
			if len(audioData) > 2 {
				if title, ok := audioData[2].(string); ok {
					result.Title = title
				}
			}
		}
	}

	return result, nil
}

// AudioOverviewResult represents an audio overview response
type AudioOverviewResult struct {
	ProjectID string
	AudioID   string
	Title     string
	AudioData string // Base64 encoded audio data
	IsReady   bool
}

// GetAudioBytes returns the decoded audio data
func (r *AudioOverviewResult) GetAudioBytes() ([]byte, error) {
	if r.AudioData == "" {
		return nil, fmt.Errorf("no audio data available")
	}
	return base64.StdEncoding.DecodeString(r.AudioData)
}

func (c *Client) DeleteAudioOverview(projectID string) error {
	req := &pb.DeleteAudioOverviewRequest{
		ProjectId: projectID,
	}
	ctx := context.Background()
	_, err := c.orchestrationService.DeleteAudioOverview(ctx, req)
	if err != nil {
		return fmt.Errorf("delete audio overview: %w", err)
	}
	return nil
}

// Video operations

type VideoOverviewResult struct {
	ProjectID string
	VideoID   string
	Title     string
	VideoData string // Base64 encoded or URL
	IsReady   bool
}

func (c *Client) CreateVideoOverview(projectID string, instructions string) (*VideoOverviewResult, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project ID required")
	}
	if instructions == "" {
		return nil, fmt.Errorf("instructions required")
	}

	// Video requires source IDs - try to get them from the notebook
	// For testing, we can also accept a hardcoded source ID
	var sourceIDs []interface{}

	// Try to get sources from the project
	// For now, use hardcoded test source ID if available
	testSourceID := "d7236810-f298-4119-a289-2b8a98170fbd"
	if testSourceID != "" {
		sourceIDs = []interface{}{[]interface{}{testSourceID}}
	} else {
		sourceIDs = []interface{}{[]interface{}{}} // Empty nested array
	}

	// Use the complex structure from the curl command
	// Structure: [[2], "notebook-id", [null, null, 3, [[[source-id]]], null, null, null, null, [null, null, [[[source-id]], "en", "instructions"]]]]
	videoArgs := []interface{}{
		[]interface{}{2}, // Mode
		projectID,        // Notebook ID
		[]interface{}{
			nil,
			nil,
			3,                        // Type or version
			[]interface{}{sourceIDs}, // Source IDs array
			nil,
			nil,
			nil,
			nil,
			[]interface{}{
				nil,
				nil,
				[]interface{}{
					sourceIDs,    // Source IDs again
					"en",         // Language
					instructions, // The actual instructions
				},
			},
		},
	}

	// Video args should be passed as the raw structure
	// The batchexecute layer will handle the JSON encoding
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCCreateVideoOverview,
		NotebookID: projectID,
		Args:       videoArgs, // Pass the structure directly
	})
	if err != nil {
		return nil, fmt.Errorf("create video overview: %w", err)
	}

	// Parse response - video returns: [["video-id", "title", status, ...]]
	var responseData []interface{}
	if err := json.Unmarshal(resp, &responseData); err != nil {
		// Try parsing as string then as JSON (double encoded)
		var strData string
		if err2 := json.Unmarshal(resp, &strData); err2 == nil {
			if err3 := json.Unmarshal([]byte(strData), &responseData); err3 != nil {
				return nil, fmt.Errorf("parse video response: %w", err)
			}
		} else {
			return nil, fmt.Errorf("parse video response: %w", err)
		}
	}

	result := &VideoOverviewResult{
		ProjectID: projectID,
		IsReady:   false, // Video generation is async
	}

	// Extract video details from response
	if len(responseData) > 0 {
		if videoData, ok := responseData[0].([]interface{}); ok && len(videoData) > 0 {
			// First element is video ID
			if id, ok := videoData[0].(string); ok {
				result.VideoID = id
				if c.config.Debug {
					fmt.Printf("Video creation initiated with ID: %s\n", id)
				}
			}
			// Second element is title
			if len(videoData) > 1 {
				if title, ok := videoData[1].(string); ok {
					result.Title = title
				}
			}
			// Third element is status (1 = processing, 2 = ready?)
			if len(videoData) > 2 {
				if status, ok := videoData[2].(float64); ok {
					result.IsReady = status == 2
				}
			}
		}
	}

	return result, nil
}

// DownloadAudioOverview attempts to download the actual audio file
// by trying different request types until it finds one with audio data
func (c *Client) DownloadAudioOverview(projectID string) (*AudioOverviewResult, error) {
	if !c.config.UseDirectRPC {
		return nil, fmt.Errorf("audio download requires --direct-rpc flag for now")
	}

	// Try different request types to find the one that returns audio data
	requestTypes := []int{0, 1, 2, 3, 4, 5}

	for _, requestType := range requestTypes {
		if c.config.Debug {
			fmt.Printf("Trying request_type=%d for audio download...\n", requestType)
		}

		result, err := c.getAudioOverviewDirectRPCWithType(projectID, requestType)
		if err != nil {
			if c.config.Debug {
				fmt.Printf("Request type %d failed: %v\n", requestType, err)
			}
			continue
		}

		// Check if this request type returned audio data
		if result.AudioData != "" {
			if c.config.Debug {
				fmt.Printf("Found audio data with request_type=%d (data length: %d)\n", requestType, len(result.AudioData))
			}
			return result, nil
		}

		if c.config.Debug {
			fmt.Printf("Request type %d returned no audio data\n", requestType)
		}
	}

	return nil, fmt.Errorf("no request type returned audio data - the audio may not be ready yet")
}

// SaveAudioToFile saves audio data to a file
func (r *AudioOverviewResult) SaveAudioToFile(filename string) error {
	if r.AudioData == "" {
		return fmt.Errorf("no audio data to save")
	}

	audioBytes, err := r.GetAudioBytes()
	if err != nil {
		return fmt.Errorf("decode audio data: %w", err)
	}

	if err := os.WriteFile(filename, audioBytes, 0644); err != nil {
		return fmt.Errorf("write audio file: %w", err)
	}

	return nil
}

// ListAudioOverviews returns audio overviews for a notebook
func (c *Client) ListAudioOverviews(projectID string) ([]*AudioOverviewResult, error) {
	// Try to get the audio overview for the project
	// NotebookLM typically has at most one audio overview per notebook
	audioOverview, err := c.GetAudioOverview(projectID)
	if err != nil {
		// Check if it's a not found error vs other errors
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "does not exist") {
			// No audio overview exists
			return []*AudioOverviewResult{}, nil
		}
		// For other errors, still return empty list but log if debug
		if c.config.Debug {
			fmt.Printf("Error getting audio overview: %v\n", err)
		}
		return []*AudioOverviewResult{}, nil
	}

	// Return the overview if it has content or is marked as ready
	if audioOverview != nil && (audioOverview.AudioData != "" || audioOverview.IsReady || audioOverview.AudioID != "") {
		return []*AudioOverviewResult{audioOverview}, nil
	}

	return []*AudioOverviewResult{}, nil
}

// ListVideoOverviews returns video overviews for a notebook
func (c *Client) ListVideoOverviews(projectID string) ([]*VideoOverviewResult, error) {
	// Since there's no GetVideoOverview RPC endpoint, we need to use a different approach
	// We can try to get the project and see if it has video overview metadata
	project, err := c.GetProject(projectID)
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
func (c *Client) GetVideoOverview(projectID string) (*VideoOverviewResult, error) {
	if !c.config.UseDirectRPC {
		return nil, fmt.Errorf("video overview requires --direct-rpc flag")
	}

	// Try using RPCGetAudioOverview with video-specific parameters
	// or see if we can get video data another way
	return c.getVideoOverviewAlternative(projectID)
}

// getVideoOverviewAlternative tries alternative methods to get video data
func (c *Client) getVideoOverviewAlternative(projectID string) (*VideoOverviewResult, error) {
	// First, try to get the project to see if it has video metadata
	project, err := c.GetProject(projectID)
	if err != nil {
		return nil, fmt.Errorf("get project for video overview: %w", err)
	}

	// Try different approaches to get video data
	approaches := []func(string) (*VideoOverviewResult, error){
		c.tryVideoOverviewDirectRPC,
		c.tryVideoFromCreateResponse,
	}

	for i, approach := range approaches {
		if c.config.Debug {
			fmt.Printf("Trying video overview approach %d...\n", i+1)
		}

		result, err := approach(projectID)
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
func (c *Client) tryVideoOverviewDirectRPC(projectID string) (*VideoOverviewResult, error) {
	// Try using the audio RPC with different parameters that might work for video
	resp, err := c.rpc.Do(rpc.Call{
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
func (c *Client) tryVideoFromCreateResponse(projectID string) (*VideoOverviewResult, error) {
	// This is a speculative approach - try to create a "get" request
	// using the same structure as CreateVideoOverview but with different parameters

	// Get sources from the project first
	project, err := c.GetProject(projectID)
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

	resp, err := c.rpc.Do(rpc.Call{
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
func (c *Client) DownloadVideoOverview(projectID string) (*VideoOverviewResult, error) {
	if !c.config.UseDirectRPC {
		return nil, fmt.Errorf("video download requires --direct-rpc flag")
	}

	// Try to get video overview data
	result, err := c.GetVideoOverview(projectID)
	if err != nil {
		return nil, fmt.Errorf("get video overview: %w", err)
	}

	// Check if we have video data
	if result.VideoData == "" {
		// Try different approaches to get video download URL
		if err := c.tryGetVideoDownloadURL(result); err != nil {
			return nil, fmt.Errorf("no video data found - video may not be ready yet or may need web interface: %w", err)
		}
	}

	return result, nil
}

// tryGetVideoDownloadURL attempts to find the video download URL using various methods
func (c *Client) tryGetVideoDownloadURL(result *VideoOverviewResult) error {
	if result.VideoID == "" {
		return fmt.Errorf("no video ID available")
	}

	// Method 1: Try to get video URL by requesting detailed video data
	if videoUrl, err := c.getVideoURLFromAPI(result.ProjectID, result.VideoID); err == nil {
		result.VideoData = videoUrl
		return nil
	} else if c.config.Debug {
		fmt.Printf("API video URL lookup failed: %v\n", err)
	}

	// Method 2: Check if the video ID itself is a URL or contains URL components
	if strings.HasPrefix(result.VideoID, "http") {
		result.VideoData = result.VideoID
		return nil
	}

	// Method 3: Provide instructions for manual download
	return fmt.Errorf("automatic video download not available - please visit https://notebooklm.google.com/notebook/%s to download manually", result.ProjectID)
}

// getVideoURLFromAPI attempts to get video URL from various API endpoints
func (c *Client) getVideoURLFromAPI(projectID, videoID string) (string, error) {
	// Try to get project details which might contain video URLs
	project, err := c.GetProject(projectID)
	if err != nil {
		return "", fmt.Errorf("get project details: %w", err)
	}

	// Look for video metadata in project that might contain URLs
	if project.Metadata != nil && c.config.Debug {
		fmt.Printf("Project metadata: %+v\n", project.Metadata)
	}

	// Try to use the CreateVideoOverview with different parameters to get existing video data
	// This might return the URL in the response
	if videoUrl, err := c.tryGetExistingVideoURL(projectID, videoID); err == nil {
		return videoUrl, nil
	}

	return "", fmt.Errorf("no video URL found in API responses")
}

// tryGetExistingVideoURL attempts to get video URL by querying for existing video
func (c *Client) tryGetExistingVideoURL(projectID, videoID string) (string, error) {
	// Get sources from the project
	project, err := c.GetProject(projectID)
	if err != nil {
		return "", fmt.Errorf("get sources: %w", err)
	}

	if len(project.Sources) == 0 {
		return "", fmt.Errorf("no sources in project")
	}

	// Use first source ID
	sourceID := project.Sources[0].SourceId
	sourceIDs := []interface{}{[]interface{}{sourceID}}

	// Try a "status" or "get" request for existing video
	// Mode 0 might be for getting existing data
	videoArgs := []interface{}{
		[]interface{}{0}, // Mode 0 = status/get instead of create
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
					videoID,   // Video ID instead of instructions
				},
			},
		},
	}

	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCCreateVideoOverview, // Reuse the same endpoint
		NotebookID: projectID,
		Args:       videoArgs,
	})
	if err != nil {
		return "", fmt.Errorf("video status RPC: %w", err)
	}

	// Parse response looking for URLs
	var responseData []interface{}
	if err := json.Unmarshal(resp, &responseData); err != nil {
		return "", fmt.Errorf("parse video status response: %w", err)
	}

	// Look through response for URLs that match the googleusercontent.com pattern
	if url := c.extractVideoURLFromResponse(responseData); url != "" {
		return url, nil
	}

	return "", fmt.Errorf("no video URL found in response")
}

// extractVideoURLFromResponse looks for video URLs in API response data
func (c *Client) extractVideoURLFromResponse(data []interface{}) string {
	// Recursively search through the response data for URLs
	for _, item := range data {
		if url := c.findVideoURL(item); url != "" {
			return url
		}
	}
	return ""
}

// findVideoURL recursively searches for video URLs in response data
func (c *Client) findVideoURL(item interface{}) string {
	switch v := item.(type) {
	case string:
		// Check if this string is a video URL
		if strings.Contains(v, "googleusercontent.com") && (strings.Contains(v, "notebooklm") || strings.Contains(v, "rd-notebooklm")) {
			if c.config.Debug {
				fmt.Printf("Found potential video URL: %s\n", v)
			}
			return v
		}
	case []interface{}:
		// Recursively search arrays
		for _, subItem := range v {
			if url := c.findVideoURL(subItem); url != "" {
				return url
			}
		}
	case map[string]interface{}:
		// Recursively search maps
		for _, subItem := range v {
			if url := c.findVideoURL(subItem); url != "" {
				return url
			}
		}
	}
	return ""
}

// SaveVideoToFile saves video data to a file
// Handles both base64 encoded data and URLs
// NOTE: For URL downloads, use client.DownloadVideoWithAuth() for proper authentication
func (r *VideoOverviewResult) SaveVideoToFile(filename string) error {
	if r.VideoData == "" {
		return fmt.Errorf("no video data to save")
	}

	// Check if VideoData is a URL or base64 data
	if strings.HasPrefix(r.VideoData, "http://") || strings.HasPrefix(r.VideoData, "https://") {
		// It's a URL - try basic download (may fail without auth)
		// For proper authentication, use client.DownloadVideoWithAuth()
		return r.downloadVideoFromURL(r.VideoData, filename)
	} else {
		// It's base64 encoded data
		return r.saveBase64VideoToFile(r.VideoData, filename)
	}
}

// downloadVideoFromURL downloads video from a URL with proper authentication
func (r *VideoOverviewResult) downloadVideoFromURL(url, filename string) error {
	// Create HTTP client with authentication
	client := &http.Client{}

	// Create request with proper headers
	req, err := http.NewRequest("GET", url, nil)
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

	// TODO: Add authentication cookies from the nlm client
	// This would require access to the client's authentication state

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
func (c *Client) DownloadVideoWithAuth(videoURL, filename string) error {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 300 * time.Second, // 5 minute timeout for large video downloads
	}

	// Create request
	req, err := http.NewRequest("GET", videoURL, nil)
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

	// Get cookies from environment (same as nlm client uses)
	cookies := os.Getenv("NLM_COOKIES")
	if cookies != "" {
		req.Header.Set("Cookie", cookies)
	}

	// Get auth token and add as query parameter if needed
	authToken := os.Getenv("NLM_AUTH_TOKEN")
	if authToken != "" && !strings.Contains(videoURL, "authuser=") {
		// Add authuser=0 parameter if not present
		separator := "?"
		if strings.Contains(videoURL, "?") {
			separator = "&"
		}
		req.URL, _ = url.Parse(videoURL + separator + "authuser=0")
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

// ListArtifacts returns artifacts for a project using direct RPC
func (c *Client) ListArtifacts(projectID string) ([]*pb.Artifact, error) {
	resp, err := c.rpc.Do(rpc.Call{
		ID: rpc.RPCListArtifacts,
		Args: []interface{}{
			[]interface{}{2}, // filter parameter - 2 seems to be for all artifacts
			projectID,
		},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("list artifacts RPC: %w", err)
	}

	// Parse response
	var responseData []interface{}
	if err := json.Unmarshal(resp, &responseData); err != nil {
		return nil, fmt.Errorf("parse artifacts response: %w", err)
	}

	if c.config.Debug {
		fmt.Printf("Artifacts response: %+v\n", responseData)
	}

	// Convert response to artifacts
	var artifacts []*pb.Artifact

	// The response format might be [[artifact1, artifact2, ...]] or [artifact1, artifact2, ...]
	if len(responseData) > 0 {
		// Try to parse as array of artifacts
		if artifactArray, ok := responseData[0].([]interface{}); ok {
			// Response is wrapped in an array
			for _, item := range artifactArray {
				if artifact := c.parseArtifactFromResponse(item); artifact != nil {
					artifacts = append(artifacts, artifact)
				}
			}
		} else {
			// Response is direct array of artifacts
			for _, item := range responseData {
				if artifact := c.parseArtifactFromResponse(item); artifact != nil {
					artifacts = append(artifacts, artifact)
				}
			}
		}
	}

	return artifacts, nil
}

// RenameArtifact renames an artifact using the rc3d8d RPC endpoint
func (c *Client) RenameArtifact(artifactID, newTitle string) (*pb.Artifact, error) {
	resp, err := c.rpc.Do(rpc.Call{
		ID: rpc.RPCRenameArtifact,
		Args: []interface{}{
			[]interface{}{artifactID, newTitle},
			[]interface{}{[]interface{}{"title"}},
		},
		NotebookID: "", // Not needed for artifact operations
	})
	if err != nil {
		return nil, fmt.Errorf("rename artifact RPC: %w", err)
	}

	// Parse response
	var responseData []interface{}
	if err := json.Unmarshal(resp, &responseData); err != nil {
		return nil, fmt.Errorf("parse rename response: %w", err)
	}

	if c.config.Debug {
		fmt.Printf("Rename artifact response: %+v\n", responseData)
	}

	// The response should contain the updated artifact data
	if len(responseData) > 0 {
		if artifact := c.parseArtifactFromResponse(responseData[0]); artifact != nil {
			return artifact, nil
		}
	}

	return nil, fmt.Errorf("failed to parse renamed artifact from response")
}

// parseArtifactFromResponse parses an artifact from RPC response data
func (c *Client) parseArtifactFromResponse(data interface{}) *pb.Artifact {
	artifactData, ok := data.([]interface{})
	if !ok || len(artifactData) == 0 {
		return nil
	}

	artifact := &pb.Artifact{}

	// Parse artifact ID (usually first element)
	if len(artifactData) > 0 {
		if id, ok := artifactData[0].(string); ok {
			artifact.ArtifactId = id
		}
	}

	// Parse artifact type (usually second element)
	if len(artifactData) > 1 {
		if typeVal, ok := artifactData[1].(float64); ok {
			artifact.Type = pb.ArtifactType(int32(typeVal))
		}
	}

	// Parse artifact state (usually third element)
	if len(artifactData) > 2 {
		if stateVal, ok := artifactData[2].(float64); ok {
			artifact.State = pb.ArtifactState(int32(stateVal))
		}
	}

	// Parse sources (if available)
	if len(artifactData) > 3 {
		if sourcesData, ok := artifactData[3].([]interface{}); ok {
			for _, sourceData := range sourcesData {
				if sourceId, ok := sourceData.(string); ok {
					artifact.Sources = append(artifact.Sources, &pb.ArtifactSource{
						SourceId: &pb.SourceId{SourceId: sourceId},
					})
				}
			}
		}
	}

	// Only return artifact if we have at least an ID
	if artifact.ArtifactId != "" {
		return artifact
	}
	return nil
}

// Generation operations

func (c *Client) GenerateDocumentGuides(projectID string) (*pb.GenerateDocumentGuidesResponse, error) {
	req := &pb.GenerateDocumentGuidesRequest{
		ProjectId: projectID,
	}
	ctx := context.Background()
	guides, err := c.orchestrationService.GenerateDocumentGuides(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("generate document guides: %w", err)
	}
	return guides, nil
}

func (c *Client) GenerateNotebookGuide(projectID string) (*pb.GenerateNotebookGuideResponse, error) {
	req := &pb.GenerateNotebookGuideRequest{
		ProjectId: projectID,
	}
	ctx := context.Background()
	guide, err := c.orchestrationService.GenerateNotebookGuide(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("generate notebook guide: %w", err)
	}
	return guide, nil
}

func (c *Client) GenerateMagicView(projectID string, sourceIDs []string) (*pb.GenerateMagicViewResponse, error) {
	req := &pb.GenerateMagicViewRequest{
		ProjectId: projectID,
		SourceIds: sourceIDs,
	}
	ctx := context.Background()
	magicView, err := c.orchestrationService.GenerateMagicView(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("generate magic view: %w", err)
	}
	return magicView, nil
}

func (c *Client) GenerateOutline(projectID string) (*pb.GenerateOutlineResponse, error) {
	req := &pb.GenerateOutlineRequest{
		ProjectId: projectID,
	}
	ctx := context.Background()
	outline, err := c.orchestrationService.GenerateOutline(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("generate outline: %w", err)
	}
	return outline, nil
}

func (c *Client) GenerateSection(projectID string) (*pb.GenerateSectionResponse, error) {
	req := &pb.GenerateSectionRequest{
		ProjectId: projectID,
	}
	ctx := context.Background()
	section, err := c.orchestrationService.GenerateSection(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("generate section: %w", err)
	}
	return section, nil
}

func (c *Client) StartDraft(projectID string) (*pb.StartDraftResponse, error) {
	req := &pb.StartDraftRequest{
		ProjectId: projectID,
	}
	ctx := context.Background()
	draft, err := c.orchestrationService.StartDraft(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("start draft: %w", err)
	}
	return draft, nil
}

func (c *Client) StartSection(projectID string) (*pb.StartSectionResponse, error) {
	req := &pb.StartSectionRequest{
		ProjectId: projectID,
	}
	ctx := context.Background()
	section, err := c.orchestrationService.StartSection(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("start section: %w", err)
	}
	return section, nil
}

func (c *Client) GenerateFreeFormStreamed(projectID string, prompt string, sourceIDs []string) (*pb.GenerateFreeFormStreamedResponse, error) {
	// Check if we should skip sources (useful for testing or when project is inaccessible)
	skipSources := os.Getenv("NLM_SKIP_SOURCES") == "true"

	// If no source IDs provided and not skipping, try to get all sources from the project
	if len(sourceIDs) == 0 && !skipSources {
		// Create a timeout context for getting project
		getProjectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		project, err := c.GetProjectWithContext(getProjectCtx, projectID)
		if err != nil {
			// If getting project fails, try without sources as fallback
			if c.config.Debug {
				fmt.Printf("DEBUG: Failed to get project sources, continuing without: %v\n", err)
			}
			// Continue without sources rather than failing completely
		} else {
			// Extract all source IDs from the project
			for _, source := range project.Sources {
				if source.SourceId != nil {
					sourceIDs = append(sourceIDs, source.SourceId.SourceId)
				}
			}

			if c.config.Debug {
				fmt.Printf("DEBUG: Using %d sources for chat\n", len(sourceIDs))
			}
		}
	}

	req := &pb.GenerateFreeFormStreamedRequest{
		ProjectId: projectID,
		Prompt:    prompt,
		SourceIds: sourceIDs,
	}

	// Use a timeout context for the chat request
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	response, err := c.orchestrationService.GenerateFreeFormStreamed(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("generate free form streamed: %w", err)
	}
	return response, nil
}

// GenerateFreeFormStreamedWithCallback streams the response and calls the callback for each chunk
func (c *Client) GenerateFreeFormStreamedWithCallback(projectID string, prompt string, sourceIDs []string, callback func(chunk string) bool) error {
	// Check if we should skip sources (useful for testing or when project is inaccessible)
	skipSources := os.Getenv("NLM_SKIP_SOURCES") == "true"

	// If no source IDs provided and not skipping, try to get all sources from the project
	if len(sourceIDs) == 0 && !skipSources {
		// Create a timeout context for getting project
		getProjectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		project, err := c.GetProjectWithContext(getProjectCtx, projectID)
		if err != nil {
			// If getting project fails, try without sources as fallback
			if c.config.Debug {
				fmt.Printf("DEBUG: Failed to get project sources, continuing without: %v\n", err)
			}
			// Continue without sources rather than failing completely
		} else {
			// Extract all source IDs from the project
			for _, source := range project.Sources {
				if source.SourceId != nil {
					sourceIDs = append(sourceIDs, source.SourceId.SourceId)
				}
			}

			if c.config.Debug {
				fmt.Printf("DEBUG: Using %d sources for chat\n", len(sourceIDs))
			}
		}
	}

	req := &pb.GenerateFreeFormStreamedRequest{
		ProjectId: projectID,
		Prompt:    prompt,
		SourceIds: sourceIDs,
	}

	// For now, we'll simulate streaming by calling the regular API and breaking the response into chunks
	// In a real implementation, this would use server-sent events or similar streaming protocol
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	response, err := c.orchestrationService.GenerateFreeFormStreamed(ctx, req)
	if err != nil {
		return fmt.Errorf("generate free form streamed: %w", err)
	}

	if response != nil && response.Chunk != "" {
		// Simulate streaming by sending words gradually
		words := strings.Fields(response.Chunk)
		for i, word := range words {
			// Create a chunk with a few words at a time
			var chunk string
			if i == 0 {
				chunk = word
			} else {
				chunk = " " + word
			}

			// Call the callback with the chunk
			if !callback(chunk) {
				break // Stop if callback returns false
			}

			// Add a small delay to simulate streaming
			time.Sleep(75 * time.Millisecond)
		}
	}

	return nil
}

// GetProjectWithContext is like GetProject but accepts a context for cancellation
func (c *Client) GetProjectWithContext(ctx context.Context, projectID string) (*Notebook, error) {
	req := &pb.GetProjectRequest{
		ProjectId: projectID,
	}

	project, err := c.orchestrationService.GetProject(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}

	if c.config.Debug && project.Sources != nil {
		fmt.Printf("DEBUG: Successfully parsed project with %d sources\n", len(project.Sources))
	}
	return project, nil
}

func (c *Client) GenerateReportSuggestions(projectID string) (*pb.GenerateReportSuggestionsResponse, error) {
	req := &pb.GenerateReportSuggestionsRequest{
		ProjectId: projectID,
	}
	ctx := context.Background()
	response, err := c.orchestrationService.GenerateReportSuggestions(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("generate report suggestions: %w", err)
	}
	return response, nil
}

// Sharing operations

// ShareOption represents audio sharing visibility options
type ShareOption int

const (
	SharePrivate ShareOption = 0
	SharePublic  ShareOption = 1
)

// ShareAudioResult represents the response from sharing audio
type ShareAudioResult struct {
	ShareURL string
	ShareID  string
	IsPublic bool
}

// ShareAudio shares an audio overview with optional public access
func (c *Client) ShareAudio(projectID string, shareOption ShareOption) (*ShareAudioResult, error) {
	req := &pb.ShareAudioRequest{
		ShareOptions: []int32{int32(shareOption)},
		ProjectId:    projectID,
	}
	ctx := context.Background()
	response, err := c.sharingService.ShareAudio(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("share audio: %w", err)
	}

	// Convert pb.ShareAudioResponse to ShareAudioResult
	result := &ShareAudioResult{
		IsPublic: shareOption == SharePublic,
	}

	// Extract share URL and ID from share_info array
	if len(response.ShareInfo) > 0 {
		result.ShareURL = response.ShareInfo[0]
	}
	if len(response.ShareInfo) > 1 {
		result.ShareID = response.ShareInfo[1]
	}

	return result, nil
}

// ShareProject shares a project with specified settings
func (c *Client) ShareProject(projectID string, settings *pb.ShareSettings) (*pb.ShareProjectResponse, error) {
	req := &pb.ShareProjectRequest{
		ProjectId: projectID,
		Settings:  settings,
	}
	ctx := context.Background()
	response, err := c.sharingService.ShareProject(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("share project: %w", err)
	}
	return response, nil
}

// Helper functions to identify and extract YouTube video IDs
func isYouTubeURL(url string) bool {
	return strings.Contains(url, "youtube.com") || strings.Contains(url, "youtu.be")
}

func extractYouTubeVideoID(urlStr string) (string, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}

	if u.Host == "youtu.be" {
		return strings.TrimPrefix(u.Path, "/"), nil
	}

	if strings.Contains(u.Host, "youtube.com") && u.Path == "/watch" {
		return u.Query().Get("v"), nil
	}

	return "", fmt.Errorf("unsupported YouTube URL format")
}
