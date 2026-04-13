// Package api provides the NotebookLM API client.
package api

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/gen/service"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/notebooklm/rpc"
)

type Notebook = pb.Project
type Note = pb.Note

// httpClientWithTimeout returns an IPv4-preferring HTTP client with the given timeout.
func httpClientWithTimeout(timeout time.Duration) *http.Client {
	c := batchexecute.NewIPv4HTTPClient()
	c.Timeout = timeout
	return c
}

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

	// Debug is set via SetDebug or NLM_DEBUG env
	client.config.Debug = os.Getenv("NLM_DEBUG") == "true"

	return client
}

// SetUseDirectRPC configures whether to use direct RPC calls
func (c *Client) SetUseDirectRPC(use bool) {
	c.config.UseDirectRPC = use
}

func (c *Client) SetDebug(debug bool) {
	c.config.Debug = debug
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
	// Wire format: [repeated_source_ids, project_context]
	//   field 1: repeated SourceId — each ID wrapped as ["id"]
	//   field 2: ProjectContext [2]
	wrappedIDs := make([]interface{}, len(sourceIDs))
	for i, id := range sourceIDs {
		wrappedIDs[i] = []interface{}{id}
	}
	_, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCDeleteSources,
		NotebookID: projectID,
		Args: []interface{}{
			wrappedIDs,
			[]interface{}{2}, // ProjectContext
		},
	})
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

func (c *Client) RefreshSource(projectID, sourceID string) (*pb.Source, error) {
	req := &pb.RefreshSourceRequest{
		SourceId:  sourceID,
		ProjectId: projectID,
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

func (c *Client) SubmitFeedback(projectID, feedbackType, feedbackText string) error {
	req := &pb.SubmitFeedbackRequest{
		ProjectId:    projectID,
		FeedbackType: feedbackType,
		FeedbackText: feedbackText,
	}

	_, err := c.orchestrationService.SubmitFeedback(context.Background(), req)
	if err != nil {
		return fmt.Errorf("submit feedback: %w", err)
	}
	return nil
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
	ext := filepath.Ext(filename)
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
		if strings.HasSuffix(filename, ".json") || detectedType == "application/json" {
			fmt.Fprintf(os.Stderr, "Handling JSON file as text: %s (MIME: %s)\n", filename, detectedType)
		}
		return c.AddSourceFromText(projectID, string(content), filename)
	}

	// Use resumable upload for binary files (PDF, etc.)
	return c.uploadFileSource(projectID, filepath.Base(filename), content)
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
					content,
					filename,
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

// uploadFileSource uploads a binary file using Google's Resumable Upload Protocol,
// then registers the uploaded file as a source in the notebook.
//
// The protocol has three steps:
//  1. Start upload: POST to /upload/_/ with metadata, get back an upload URL
//  2. Upload bytes: POST raw file bytes to the upload URL
//  3. Register source: RPC o4cbdc to associate the uploaded file with the notebook
func (c *Client) uploadFileSource(projectID, filename string, content []byte) (string, error) {
	sourceID := uuid.New().String()

	if c.config.Debug {
		fmt.Fprintf(os.Stderr, "DEBUG: uploading file %q (%d bytes) via resumable upload\n", filename, len(content))
		fmt.Fprintf(os.Stderr, "DEBUG: generated source ID: %s\n", sourceID)
	}

	// Step 1: Start the resumable upload session
	uploadURL, err := c.startResumableUpload(projectID, filename, sourceID, len(content))
	if err != nil {
		return "", fmt.Errorf("start upload: %w", err)
	}

	if c.config.Debug {
		fmt.Fprintf(os.Stderr, "DEBUG: got upload URL: %s\n", uploadURL)
	}

	// Step 2: Upload the file bytes
	if err := c.uploadFileBytes(uploadURL, content); err != nil {
		return "", fmt.Errorf("upload file bytes: %w", err)
	}

	if c.config.Debug {
		fmt.Fprintf(os.Stderr, "DEBUG: file bytes uploaded successfully\n")
	}

	// Step 3: Register the uploaded file as a source via RPC
	registeredID, err := c.registerFileSource(projectID, filename, sourceID)
	if err != nil {
		return "", fmt.Errorf("register file source: %w", err)
	}

	// Step 4: Process the source (generate document guides)
	if err := c.processFileSource(registeredID); err != nil {
		if c.config.Debug {
			fmt.Fprintf(os.Stderr, "DEBUG: process source warning: %v\n", err)
		}
		// Non-fatal: source is registered but processing may happen async
	}

	return registeredID, nil
}

// startResumableUpload initiates a resumable upload session and returns the upload URL.
func (c *Client) startResumableUpload(projectID, filename, sourceID string, contentLength int) (string, error) {
	// Build metadata payload: base64-encoded JSON
	metadata := map[string]string{
		"PROJECT_ID":  projectID,
		"SOURCE_NAME": filename,
		"SOURCE_ID":   sourceID,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("marshal metadata: %w", err)
	}
	metadataB64 := base64.StdEncoding.EncodeToString(metadataJSON)

	uploadInitURL := "https://notebooklm.google.com/upload/_/?authuser=0"
	req, err := http.NewRequest("POST", uploadInitURL, strings.NewReader(metadataB64))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	// Set required headers for resumable upload initiation
	// Per HAR capture: no X-Goog-Upload-Header-Content-Type is sent
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	req.Header.Set("X-Goog-Upload-Command", "start")
	req.Header.Set("X-Goog-Upload-Protocol", "resumable")
	req.Header.Set("X-Goog-Upload-Header-Content-Length", fmt.Sprintf("%d", contentLength))
	req.Header.Set("X-Goog-AuthUser", "0")

	// Upload uses cookies only — no Authorization, Origin, or X-Same-Domain headers
	if cookies := c.rpc.Config.Cookies; cookies != "" {
		req.Header.Set("Cookie", cookies)
	}
	req.Header.Set("Referer", "https://notebooklm.google.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36")

	if c.config.Debug {
		fmt.Fprintf(os.Stderr, "DEBUG: upload init URL: %s\n", uploadInitURL)
		fmt.Fprintf(os.Stderr, "DEBUG: upload init body: %s\n", metadataB64)
		fmt.Fprintf(os.Stderr, "DEBUG: upload init decoded: %s\n", string(metadataJSON))
		for k, v := range req.Header {
			if k != "Cookie" { // Don't dump cookies
				fmt.Fprintf(os.Stderr, "DEBUG: upload init header %s: %v\n", k, v)
			}
		}
	}

	client := httpClientWithTimeout(30 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload init request: %w", err)
	}
	defer resp.Body.Close()

	if c.config.Debug {
		fmt.Fprintf(os.Stderr, "DEBUG: upload init response status: %s\n", resp.Status)
		for k, v := range resp.Header {
			fmt.Fprintf(os.Stderr, "DEBUG: upload init response header %s: %v\n", k, v)
		}
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload init failed (status %d): %s", resp.StatusCode, string(body))
	}

	// The upload URL is returned in the X-Goog-Upload-URL header
	uploadURL := resp.Header.Get("X-Goog-Upload-Url")
	if uploadURL == "" {
		// Try lowercase
		uploadURL = resp.Header.Get("x-goog-upload-url")
	}
	if uploadURL == "" {
		return "", fmt.Errorf("no upload URL in response headers")
	}

	return uploadURL, nil
}

// uploadFileBytes uploads the raw file bytes to the resumable upload URL.
func (c *Client) uploadFileBytes(uploadURL string, content []byte) error {
	req, err := http.NewRequest("POST", uploadURL, bytes.NewReader(content))
	if err != nil {
		return fmt.Errorf("create upload request: %w", err)
	}

	// Per HAR: Content-Type is form-urlencoded even for binary data
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=utf-8")
	req.Header.Set("X-Goog-Upload-Command", "upload, finalize")
	req.Header.Set("X-Goog-Upload-Offset", "0")
	req.Header.Set("X-Goog-AuthUser", "0")

	// Upload uses cookies only — no Authorization header
	if cookies := c.rpc.Config.Cookies; cookies != "" {
		req.Header.Set("Cookie", cookies)
	}
	req.Header.Set("Referer", "https://notebooklm.google.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36")

	client := httpClientWithTimeout(5 * time.Minute)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("upload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// registerFileSource registers an uploaded file as a notebook source via RPC.
func (c *Client) registerFileSource(projectID, filename, sourceID string) (string, error) {
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCAddFileSource,
		NotebookID: projectID,
		Args: []interface{}{
			[]interface{}{
				[]interface{}{filename},
			},
			projectID,
			[]interface{}{2}, // source type: file upload
			[]interface{}{
				1, nil, nil, nil, nil, nil, nil, nil, nil, nil,
				[]interface{}{1},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("register file source RPC: %w", err)
	}

	registeredID, err := extractSourceID(resp)
	if err != nil {
		// If we can't extract an ID from the response, use the one we generated
		if c.config.Debug {
			fmt.Fprintf(os.Stderr, "DEBUG: could not extract source ID from register response, using generated ID\n")
			fmt.Fprintf(os.Stderr, "DEBUG: register response: %s\n", string(resp))
		}
		return sourceID, nil
	}
	return registeredID, nil
}

// processFileSource triggers document guide generation for a newly uploaded source.
func (c *Client) processFileSource(sourceID string) error {
	_, err := c.rpc.Do(rpc.Call{
		ID: rpc.RPCGenerateDocumentGuides,
		Args: []interface{}{
			[]interface{}{
				[]interface{}{
					[]interface{}{sourceID},
				},
			},
		},
	})
	return err
}

// setAuthHeaders adds authentication headers to an HTTP request.
func (c *Client) setAuthHeaders(req *http.Request) {
	cookies := c.rpc.Config.Cookies
	if cookies != "" {
		req.Header.Set("Cookie", cookies)
	}

	// Add SAPISIDHASH authorization
	if sapisid := extractSAPISID(cookies); sapisid != "" {
		origin := "https://notebooklm.google.com"
		req.Header.Set("Authorization", generateSAPISIDHASH(sapisid, origin))
	}

	req.Header.Set("Origin", "https://notebooklm.google.com")
	req.Header.Set("Referer", "https://notebooklm.google.com/")
	req.Header.Set("X-Same-Domain", "1")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36")
}

// extractSAPISID extracts the SAPISID cookie value from a cookie string.
func extractSAPISID(cookies string) string {
	for _, part := range strings.Split(cookies, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "SAPISID=") {
			return strings.TrimPrefix(part, "SAPISID=")
		}
	}
	return ""
}

// generateSAPISIDHASH generates the SAPISIDHASH authorization header value.
func generateSAPISIDHASH(sapisid, origin string) string {
	timestamp := time.Now().Unix()
	data := fmt.Sprintf("%d %s %s", timestamp, sapisid, origin)
	hash := sha1.Sum([]byte(data))
	return fmt.Sprintf("SAPISIDHASH %d_%x", timestamp, hash)
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
	return c.AddSourceFromReader(projectID, f, filepath, providedType)
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
		// JSON parsing failed — likely due to double-escaped batchexecute response.
		// Fall back to scanning raw bytes for a UUID pattern.
		if id := findUUIDInBytes(resp); id != "" {
			return id, nil
		}
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

	// Last resort: scan raw bytes for UUID
	if id := findUUIDInBytes(resp); id != "" {
		return id, nil
	}

	return "", fmt.Errorf("could not find source ID in response structure: %v", data)
}

// findUUIDInBytes scans raw bytes for a UUID v4 pattern (8-4-4-4-12 hex).
// Used as a fallback when JSON parsing fails due to double-escaped responses.
func findUUIDInBytes(b []byte) string {
	s := string(b)
	for i := 0; i <= len(s)-36; i++ {
		candidate := s[i : i+36]
		if isUUID(candidate) {
			return candidate
		}
	}
	return ""
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
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
	return note, nil
}

func (c *Client) DeleteNotes(projectID string, noteIDs []string) error {
	req := &pb.DeleteNotesRequest{
		ProjectId: projectID,
		NoteIds:   noteIDs,
	}
	_, err := c.orchestrationService.DeleteNotes(context.Background(), req)
	if err != nil {
		return fmt.Errorf("delete notes: %w", err)
	}
	return nil
}

func (c *Client) GetNotes(projectID string) ([]*Note, error) {
	req := &pb.GetNotesRequest{ProjectId: projectID}
	response, err := c.orchestrationService.GetNotes(context.Background(), req)
	if err == nil {
		return response.Notes, nil
	}
	if c.config.Debug {
		fmt.Printf("GetNotes orchestration parse failed, falling back to raw parser: %v\n", err)
	}

	resp, rpcErr := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCGetNotes,
		NotebookID: projectID,
		Args:       []interface{}{projectID, nil, nil, []interface{}{2}},
	})
	if rpcErr != nil {
		return nil, fmt.Errorf("get notes: %w", err)
	}

	notes, parseErr := parseNotesResponse(resp)
	if parseErr != nil {
		return nil, fmt.Errorf("get notes: %w", err)
	}
	return notes, nil
}

func parseNotesResponse(resp []byte) ([]*Note, error) {
	var data []interface{}
	if err := json.Unmarshal(resp, &data); err != nil {
		return nil, fmt.Errorf("parse notes response: %w", err)
	}

	items := data
	if top, ok := interfaceSliceAt(data, 0); ok {
		items = top
	}

	notes := make([]*Note, 0, len(items))
	for _, item := range items {
		note := parseNoteFromResponse(item)
		if note != nil {
			notes = append(notes, note)
		}
	}
	return notes, nil
}

func parseNoteFromResponse(data interface{}) *Note {
	wrapper, ok := data.([]interface{})
	if !ok || len(wrapper) == 0 {
		return nil
	}

	noteData := wrapper
	if nested, ok := interfaceSliceAt(wrapper, 1); ok {
		noteData = nested
	}

	noteID := stringAt(wrapper, 0)
	if noteID == "" {
		noteID = stringAt(noteData, 0)
	}
	if noteID == "" {
		return nil
	}

	return &pb.Note{
		NoteId:      noteID,
		ContentText: stringAt(noteData, 1),
		Title:       stringAt(noteData, 4),
		RichText:    stringAt(noteData, 5),
	}
}

// Audio operations

func (c *Client) CreateAudioOverview(projectID string, instructions string) (*AudioOverviewResult, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project ID required")
	}

	// Use direct RPC if configured
	if c.config.UseDirectRPC {
		return c.createAudioOverviewDirectRPC(projectID, instructions)
	}

	// Get project to extract source IDs
	project, err := c.GetProject(projectID)
	if err != nil {
		return nil, fmt.Errorf("get project sources: %w", err)
	}

	// Extract source IDs from project
	var sourceIDs []string
	for _, source := range project.Sources {
		if source.SourceId != nil {
			sourceIDs = append(sourceIDs, source.SourceId.SourceId)
		}
	}

	if len(sourceIDs) == 0 {
		return nil, fmt.Errorf("project has no sources - add sources before creating audio overview")
	}

	// Default: use orchestration service with new proto fields
	req := &pb.CreateAudioOverviewRequest{
		ProjectId:          projectID,
		AudioType:          pb.AudioType_AUDIO_TYPE_DEEP_DIVE, // Default to Deep Dive (value=1)
		SourceIds:          sourceIDs,
		CustomInstructions: instructions,
		Length:             pb.AudioLength_AUDIO_LENGTH_DEFAULT, // Default length (value=2)
		Language:           "en",
	}
	ctx := context.Background()
	audioOverview, err := c.orchestrationService.CreateAudioOverview(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create audio overview: %w", wrapCreateAudioOverviewError(err))
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
	result := audioOverviewResultFromProto(projectID, audioOverview)
	if result.AudioID != "" || result.AudioData != "" || result.Title != "" {
		return result, nil
	}

	fallback, err := c.getAudioOverviewDirectRPC(projectID)
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
func (c *Client) getAudioOverviewDirectRPC(projectID string) (*AudioOverviewResult, error) {
	result, err := c.getAudioOverviewDirectRPCArgs(projectID, []interface{}{projectID})
	if err == nil && (result.AudioID != "" || result.AudioData != "" || result.Title != "") {
		return result, nil
	}
	return c.getAudioOverviewDirectRPCWithType(projectID, 1)
}

// getAudioOverviewDirectRPCWithType uses direct RPC with a specific request type
func (c *Client) getAudioOverviewDirectRPCWithType(projectID string, requestType int) (*AudioOverviewResult, error) {
	return c.getAudioOverviewDirectRPCArgs(projectID, []interface{}{
		projectID,
		requestType, // request_type - try different values
	})
}

func (c *Client) getAudioOverviewDirectRPCArgs(projectID string, args []interface{}) (*AudioOverviewResult, error) {
	resp, err := c.rpc.Do(rpc.Call{
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

func (c *Client) CreateVideoOverview(projectID string, instructions string) (*VideoOverviewResult, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project ID required")
	}
	if instructions == "" {
		return nil, fmt.Errorf("instructions required")
	}

	// Get project to extract source IDs
	project, err := c.GetProject(projectID)
	if err != nil {
		return nil, fmt.Errorf("get project sources: %w", err)
	}

	var sourceIDs []string
	for _, source := range project.Sources {
		if source.SourceId != nil {
			sourceIDs = append(sourceIDs, source.SourceId.SourceId)
		}
	}
	if len(sourceIDs) == 0 {
		return nil, fmt.Errorf("project has no sources - add sources before creating video overview")
	}

	// Build request and use the proper encoder via R7cb6c
	req := &pb.CreateVideoOverviewRequest{
		ProjectId:          projectID,
		AudioType:          pb.AudioType_AUDIO_TYPE_BRIEF, // Videos default to brief style
		SourceIds:          sourceIDs,
		CustomInstructions: instructions,
		Language:           "en",
	}

	args := method.EncodeCreateVideoOverviewArgs(req)

	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCCreateVideoOverview,
		NotebookID: projectID,
		Args:       args,
	})
	if err != nil {
		return nil, fmt.Errorf("create video overview: %w", err)
	}

	result := &VideoOverviewResult{
		ProjectID: projectID,
		IsReady:   false, // Video generation is async
	}

	var responseData []interface{}
	if err := json.Unmarshal(resp, &responseData); err != nil {
		return nil, fmt.Errorf("parse video response: %w", err)
	}

	if len(responseData) > 0 {
		if videoData, ok := responseData[0].([]interface{}); ok {
			result = videoOverviewResultFromArtifactData(projectID, videoData)
		}
	}

	return result, nil
}

// DownloadAudioOverview attempts to download the actual audio file
// by querying for audio artifacts and downloading from the URL
func (c *Client) DownloadAudioOverview(projectID string) (*AudioOverviewResult, error) {
	audioOverview, err := c.GetAudioOverview(projectID)
	if err == nil && audioOverview != nil && audioOverview.AudioData != "" {
		return audioOverview, nil
	}

	// Query for audio artifacts using direct RPC (response format is complex)
	resp, err := c.rpc.Do(rpc.Call{
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
	audioData, err := c.downloadAudioFromURL(audioURL)
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
func (c *Client) downloadAudioFromURL(audioURL string) ([]byte, error) {
	// Import the auth package to use browser-based download
	auth := &struct {
		Download func(string, string) ([]byte, error)
	}{}

	// For now, keep the simple HTTP approach as fallback
	// TODO: Integrate auth.DownloadWithBrowser when fully tested

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

	req, err := http.NewRequest("GET", audioURL, nil)
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
		_ = auth // Silence unused variable warning
		return nil, fmt.Errorf("Google CDN requires browser authentication - use browser-based download (not yet implemented)")
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
	var overviews []*AudioOverviewResult

	resp, err := c.rpc.Do(rpc.Call{
		ID: rpc.RPCListArtifacts,
		Args: []interface{}{
			[]interface{}{2},
			projectID,
		},
		NotebookID: projectID,
	})
	if err == nil {
		var parseErr error
		overviews, parseErr = audioOverviewResultsFromArtifacts(projectID, resp)
		if c.config.Debug && parseErr != nil {
			fmt.Printf("Error parsing audio overview artifacts: %v\n", parseErr)
		}
	}
	if err != nil && c.config.Debug {
		fmt.Printf("Error listing audio overview artifacts: %v\n", err)
	}

	audioOverview, err := c.GetAudioOverview(projectID)
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
	client := httpClientWithTimeout(30 * time.Second)

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
func (c *Client) DownloadVideoWithAuth(videoURL, filename string) error {
	// Create HTTP client with timeout
	client := httpClientWithTimeout(300 * time.Second)

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

	return c.parseArtifactsResponse(resp)
}

// GetArtifact returns a single artifact using direct RPC.
func (c *Client) GetArtifact(artifactID string) (*pb.Artifact, error) {
	resp, err := c.rpc.Do(rpc.Call{
		ID:   rpc.RPCGetArtifact,
		Args: []interface{}{artifactID},
	})
	if err == nil {
		var responseData []interface{}
		if err := json.Unmarshal(resp, &responseData); err == nil {
			if artifact := c.parseArtifactFromResponse(responseData); artifact != nil {
				return artifact, nil
			}
			if len(responseData) > 0 {
				if artifact := c.parseArtifactFromResponse(responseData[0]); artifact != nil {
					return artifact, nil
				}
			}
		}
	}

	projects, listErr := c.ListRecentlyViewedProjects()
	if listErr != nil {
		if err != nil {
			return nil, fmt.Errorf("get artifact RPC: %w", err)
		}
		return nil, fmt.Errorf("list projects for artifact lookup: %w", listErr)
	}
	for _, project := range projects {
		artifacts, listArtifactsErr := c.ListArtifacts(project.GetProjectId())
		if listArtifactsErr != nil {
			continue
		}
		for _, artifact := range artifacts {
			if artifact.GetArtifactId() == artifactID {
				return artifact, nil
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("get artifact RPC: %w", err)
	}
	return nil, fmt.Errorf("artifact %q not found", artifactID)
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

	return c.parseRenameArtifactResponse(resp, artifactID)
}

func (c *Client) parseRenameArtifactResponse(resp []byte, artifactID string) (*pb.Artifact, error) {
	var responseData []interface{}
	if err := json.Unmarshal(resp, &responseData); err != nil {
		return nil, fmt.Errorf("parse rename response: %w", err)
	}

	if c.config.Debug {
		fmt.Printf("Rename artifact response: %+v\n", responseData)
	}

	if len(responseData) > 0 {
		if artifact := c.parseArtifactFromResponse(responseData[0]); artifact != nil {
			return artifact, nil
		}
	}

	// Rename succeeds even when the RPC only returns a status marker.
	return &pb.Artifact{ArtifactId: artifactID}, nil
}

func (c *Client) parseArtifactsResponse(resp []byte) ([]*pb.Artifact, error) {
	var responseData []interface{}
	if err := json.Unmarshal(resp, &responseData); err != nil {
		return nil, fmt.Errorf("parse artifacts response: %w", err)
	}

	if c.config.Debug {
		fmt.Printf("Artifacts response: %+v\n", responseData)
	}

	items := responseData
	if wrapped, ok := interfaceSliceAt(responseData, 0); ok {
		if len(wrapped) == 0 {
			items = wrapped
		} else if _, ok := wrapped[0].([]interface{}); ok {
			items = wrapped
		}
	}

	artifacts := make([]*pb.Artifact, 0, len(items))
	for _, item := range items {
		artifact := c.parseArtifactFromResponse(item)
		if artifact != nil {
			artifacts = append(artifacts, artifact)
		}
	}
	return artifacts, nil
}

// parseArtifactFromResponse parses an artifact from RPC response data
func (c *Client) parseArtifactFromResponse(data interface{}) *pb.Artifact {
	artifactData, ok := data.([]interface{})
	if !ok || len(artifactData) == 0 {
		return nil
	}

	artifactID := stringAt(artifactData, 0)
	if artifactID == "" {
		return nil
	}

	artifact := &pb.Artifact{
		ArtifactId: artifactID,
	}

	// Observed gArtLc artifact shape:
	//   [artifact_id, title, type_code, source_refs, state_code, ...]
	if typeCode, ok := int32At(artifactData, 2); ok {
		artifact.Type = pb.ArtifactType(typeCode)
	}
	if stateCode, ok := int32At(artifactData, 4); ok {
		artifact.State = pb.ArtifactState(stateCode)
	}
	for _, sourceID := range parseArtifactSourceIDs(artifactData) {
		artifact.Sources = append(artifact.Sources, &pb.ArtifactSource{
			SourceId: &pb.SourceId{SourceId: sourceID},
		})
	}

	return artifact
}

func parseArtifactSourceIDs(artifactData []interface{}) []string {
	if len(artifactData) <= 3 {
		return nil
	}

	sourcesData, ok := artifactData[3].([]interface{})
	if !ok {
		return nil
	}

	var sourceIDs []string
	seen := make(map[string]bool)
	appendArtifactSourceIDs(sourcesData, seen, &sourceIDs)
	return sourceIDs
}

func appendArtifactSourceIDs(values []interface{}, seen map[string]bool, out *[]string) {
	if len(values) == 1 {
		if id, ok := values[0].(string); ok && id != "" && !seen[id] {
			seen[id] = true
			*out = append(*out, id)
			return
		}
	}
	for _, value := range values {
		nested, ok := value.([]interface{})
		if !ok {
			continue
		}
		appendArtifactSourceIDs(nested, seen, out)
	}
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

// ChatMessage represents a message in chat history for the wire protocol.
type ChatMessage struct {
	Content string // Message text
	Role    int    // 1 = user, 2 = assistant
}

// ChatRequest contains parameters for a chat request.
type ChatRequest struct {
	ProjectID      string
	Prompt         string
	SourceIDs      []string
	ConversationID string        // Persists across messages in a conversation
	History        []ChatMessage // Previous messages, newest first
	SeqNum         int           // Request sequence number within conversation
}

// ChatChunkPhase indicates which phase of the stream a chunk belongs to.
type ChatChunkPhase int

const (
	ChatChunkThinking ChatChunkPhase = iota // Reasoning trace (replaced by next thinking chunk)
	ChatChunkAnswer                         // Final answer text (cumulative delta)
)

// ChatChunk is a parsed chunk from the chat stream with phase metadata.
type ChatChunk struct {
	Text   string         // The text content (delta for answer, full replacement for thinking)
	Header string         // For thinking chunks: the bold header line only
	Phase  ChatChunkPhase // Whether this is thinking or answer
}

// chatEndpoint is the gRPC-Web endpoint for GenerateFreeFormStreamed.
// Chat does NOT use batchexecute — it uses a dedicated gRPC-Web endpoint.
const chatEndpoint = "/_/LabsTailwindUi/data/google.internal.labs.tailwind.orchestration.v1.LabsTailwindOrchestrationService/GenerateFreeFormStreamed"

func (c *Client) GenerateFreeFormStreamed(projectID string, prompt string, sourceIDs []string) (*pb.GenerateFreeFormStreamedResponse, error) {
	var resp strings.Builder
	err := c.StreamChat(ChatRequest{
		ProjectID: projectID,
		Prompt:    prompt,
		SourceIDs: sourceIDs,
	}, answerOnlyCallback(func(chunk string) bool {
		resp.WriteString(chunk)
		return true
	}))
	if err != nil {
		return nil, fmt.Errorf("generate free form streamed: %w", err)
	}
	return &pb.GenerateFreeFormStreamedResponse{
		Chunk:   resp.String(),
		IsFinal: true,
	}, nil
}

// GenerateFreeFormStreamedWithCallback streams the response and calls the callback for each chunk.
func (c *Client) GenerateFreeFormStreamedWithCallback(projectID string, prompt string, sourceIDs []string, callback func(chunk string) bool) error {
	return c.StreamChat(ChatRequest{
		ProjectID: projectID,
		Prompt:    prompt,
		SourceIDs: sourceIDs,
	}, answerOnlyCallback(callback))
}

// StreamChat streams the response with phase-aware ChatChunk callbacks.
// Thinking chunks are complete reasoning traces; answer chunks are cumulative deltas.
func (c *Client) StreamChat(req ChatRequest, callback func(ChatChunk) bool) error {
	return c.doChatStreamedChunked(req, callback)
}

func answerOnlyCallback(callback func(string) bool) func(ChatChunk) bool {
	return func(chunk ChatChunk) bool {
		if chunk.Phase != ChatChunkAnswer || chunk.Text == "" {
			return true
		}
		return callback(chunk.Text)
	}
}

// ChatWithHistory sends a chat message with full conversation history.
func (c *Client) ChatWithHistory(req ChatRequest) (string, error) {
	return c.doChat(req)
}

// resolveSourceIDs fills in source IDs from the project if not provided.
func (c *Client) resolveSourceIDs(projectID string, sourceIDs []string) []string {
	if len(sourceIDs) > 0 || os.Getenv("NLM_SKIP_SOURCES") == "true" {
		return sourceIDs
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	project, err := c.GetProjectWithContext(ctx, projectID)
	if err != nil {
		if c.config.Debug {
			fmt.Fprintf(os.Stderr, "DEBUG: failed to get project sources: %v\n", err)
		}
		return sourceIDs
	}
	for _, source := range project.Sources {
		if source.SourceId != nil {
			sourceIDs = append(sourceIDs, source.SourceId.SourceId)
		}
	}
	if c.config.Debug {
		fmt.Fprintf(os.Stderr, "DEBUG: using %d sources for chat\n", len(sourceIDs))
	}
	return sourceIDs
}

type chatWireHistoryEntry struct {
	Content string
	Role    int32
}

type chatWireOptions struct {
	Mode          int32
	CitationModes []int32
	FollowUpModes []int32
}

type chatWireRequest struct {
	ProjectID        string
	Prompt           string
	SourceIDs        []string
	History          []chatWireHistoryEntry
	Options          chatWireOptions
	ConversationID   string
	DraftResponseID  string
	ParentResponseID string
	NotebookID       string
	SequenceNumber   int32
}

func (c *Client) buildChatWireRequest(req ChatRequest) *chatWireRequest {
	req.SourceIDs = c.resolveSourceIDs(req.ProjectID, req.SourceIDs)

	if req.ConversationID == "" {
		req.ConversationID = uuid.New().String()
	}
	if req.SeqNum == 0 {
		req.SeqNum = 1
	}

	history := make([]chatWireHistoryEntry, 0, len(req.History))
	for _, msg := range req.History {
		history = append(history, chatWireHistoryEntry{
			Content: msg.Content,
			Role:    int32(msg.Role),
		})
	}

	return &chatWireRequest{
		ProjectID:      req.ProjectID,
		Prompt:         req.Prompt,
		SourceIDs:      req.SourceIDs,
		History:        history,
		Options:        chatWireOptions{Mode: 2, CitationModes: []int32{1}, FollowUpModes: []int32{1}},
		ConversationID: req.ConversationID,
		NotebookID:     req.ProjectID,
		SequenceNumber: int32(req.SeqNum),
	}
}

func buildChatWireArgs(req *chatWireRequest) []interface{} {
	var sourceIDArrays []interface{}
	for _, id := range req.SourceIDs {
		sourceIDArrays = append(sourceIDArrays, []interface{}{[]interface{}{id}})
	}

	var history interface{}
	if len(req.History) > 0 {
		var historyEntries []interface{}
		for _, msg := range req.History {
			historyEntries = append(historyEntries, []interface{}{msg.Content, nil, msg.Role})
		}
		history = historyEntries
	}

	options := []interface{}{2, nil, []interface{}{1}, []interface{}{1}}
	options = []interface{}{
		req.Options.Mode,
		nil,
		int32SliceToInterfaces(req.Options.CitationModes),
		int32SliceToInterfaces(req.Options.FollowUpModes),
	}

	notebookID := req.NotebookID
	if notebookID == "" {
		notebookID = req.ProjectID
	}

	return []interface{}{
		sourceIDArrays,
		req.Prompt,
		history,
		options,
		req.ConversationID,
		nilIfEmpty(req.DraftResponseID),
		nilIfEmpty(req.ParentResponseID),
		notebookID,
		req.SequenceNumber,
	}
}

func int32SliceToInterfaces(values []int32) []interface{} {
	if len(values) == 0 {
		return []interface{}{}
	}
	out := make([]interface{}, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// buildChatArgs builds the inner JSON args for a chat request.
// Wire format: [[[[source_ids]]],prompt,history,[2,null,[1],[1]],conv_id,null,null,notebook_id,seq_num]
func (c *Client) buildChatArgs(req ChatRequest) (string, error) {
	args := buildChatWireArgs(c.buildChatWireRequest(req))

	argsJSON, err := json.Marshal(args)
	if err != nil {
		return "", fmt.Errorf("marshal chat args: %w", err)
	}
	return string(argsJSON), nil
}

// buildChatRequestBody builds the full HTTP form body for a chat request.
func (c *Client) buildChatRequestBody(req ChatRequest) (string, error) {
	innerJSON, err := c.buildChatArgs(req)
	if err != nil {
		return "", err
	}

	// Outer envelope: [null, "<inner-json-double-encoded>"]
	outerJSON, err := json.Marshal([]interface{}{nil, innerJSON})
	if err != nil {
		return "", fmt.Errorf("marshal chat envelope: %w", err)
	}

	// Form body: f.req=<url-encoded-outer>&at=<auth-token>
	authToken := c.rpc.Config.AuthToken
	body := fmt.Sprintf("f.req=%s&at=%s",
		url.QueryEscape(string(outerJSON)),
		url.QueryEscape(authToken))

	return body, nil
}

// buildChatURL constructs the full chat endpoint URL with query parameters.
func (c *Client) buildChatURL(notebookID string) string {
	u := fmt.Sprintf("https://%s%s", c.rpc.Config.Host, chatEndpoint)

	q := url.Values{}
	for k, v := range c.rpc.Config.URLParams {
		q.Set(k, v)
	}
	q.Set("rt", "c") // Chunked response format
	q.Set("_reqid", fmt.Sprintf("%d", time.Now().UnixMilli()%1000000))

	return u + "?" + q.Encode()
}

// doChat sends a chat request and returns the full response text.
func (c *Client) doChat(req ChatRequest) (string, error) {
	var result strings.Builder
	err := c.doChatStreamed(req, func(chunk string) bool {
		result.WriteString(chunk)
		return true
	})
	if err != nil {
		return "", err
	}
	return result.String(), nil
}

// doChatStreamed sends a chat request and streams response chunks via callback.
func (c *Client) doChatStreamed(req ChatRequest, callback func(chunk string) bool) error {
	body, err := c.buildChatRequestBody(req)
	if err != nil {
		return err
	}

	chatURL := c.buildChatURL(req.ProjectID)

	if c.config.Debug {
		fmt.Fprintf(os.Stderr, "DEBUG: chat URL: %s\n", chatURL)
		fmt.Fprintf(os.Stderr, "DEBUG: chat body length: %d\n", len(body))
		// Show the f.req value for debugging
		if idx := strings.Index(body, "f.req="); idx >= 0 {
			freqEnd := strings.Index(body[idx:], "&")
			if freqEnd > 0 {
				decoded, _ := url.QueryUnescape(body[idx+6 : idx+freqEnd])
				fmt.Fprintf(os.Stderr, "DEBUG: chat f.req: %s\n", decoded)
			}
		}
	}

	httpReq, err := http.NewRequest("POST", chatURL, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("create chat request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	c.setAuthHeaders(httpReq)
	// Required header for chat endpoint (observed in HAR capture)
	httpReq.Header.Set("x-goog-ext-353267353-jspb", "[null,null,null,282611]")

	client := httpClientWithTimeout(120 * time.Second)
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("chat request: %w", err)
	}
	defer resp.Body.Close()

	if c.config.Debug {
		fmt.Fprintf(os.Stderr, "DEBUG: chat response status: %s\n", resp.Status)
		for k, v := range resp.Header {
			fmt.Fprintf(os.Stderr, "DEBUG: chat response header %s: %v\n", k, v)
		}
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("chat request failed: %d %s: %s", resp.StatusCode, resp.Status, string(respBody)[:min(500, len(respBody))])
	}

	// Parse chunked response format: )]}'\n followed by length-prefixed chunks
	return c.parseChatResponse(resp.Body, callback)
}

// doChatStreamedChunked sends a chat request and streams phase-aware ChatChunks via callback.
func (c *Client) doChatStreamedChunked(req ChatRequest, callback func(ChatChunk) bool) error {
	body, err := c.buildChatRequestBody(req)
	if err != nil {
		return err
	}

	chatURL := c.buildChatURL(req.ProjectID)

	httpReq, err := http.NewRequest("POST", chatURL, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("create chat request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	c.setAuthHeaders(httpReq)
	httpReq.Header.Set("x-goog-ext-353267353-jspb", "[null,null,null,282611]")

	client := httpClientWithTimeout(120 * time.Second)
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("chat request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("chat request failed: %d %s: %s", resp.StatusCode, resp.Status, string(respBody)[:min(500, len(respBody))])
	}

	return c.parseChatResponseChunked(resp.Body, callback)
}

// parseChatResponseChunked reads the stream incrementally and emits phase-aware
// ChatChunks as each wire frame arrives. The wire format is:
//
//	)]}'           (anti-XSSI prefix, first line only)
//	<length>\n     (decimal byte count of the following JSON line)
//	<json>\n       (the actual data — may contain ["wrb.fr", ...] envelope)
//
// Chunks are emitted immediately as they are read, enabling real-time streaming.
func (c *Client) parseChatResponseChunked(r io.Reader, callback func(ChatChunk) bool) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024) // up to 1MB lines

	var lastThinking string
	var lastAnswer string
	var answerStarted bool
	firstLine := true

	for scanner.Scan() {
		line := scanner.Text()

		// Strip anti-XSSI prefix on first non-empty line.
		if firstLine {
			line = strings.TrimPrefix(line, ")]}'")
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			firstLine = false
		}

		// Skip length-prefix lines (pure digits).
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		isLengthLine := true
		for _, ch := range trimmed {
			if ch < '0' || ch > '9' {
				isLengthLine = false
				break
			}
		}
		if isLengthLine {
			continue
		}

		// Look for wrb.fr envelope in this line.
		startIdx := strings.Index(line, "[\"wrb.fr\"")
		if startIdx < 0 {
			continue
		}

		chunkJSON := extractJSONArray(line[startIdx:])
		if chunkJSON == "" {
			continue
		}

		var envelope []interface{}
		if err := json.Unmarshal([]byte(chunkJSON), &envelope); err != nil {
			continue
		}

		if len(envelope) < 3 {
			continue
		}
		innerStr, ok := envelope[2].(string)
		if !ok || innerStr == "" {
			continue
		}

		text := extractChatText(innerStr)
		if text == "" {
			continue
		}

		if c.config.Debug {
			preview := text
			if len(preview) > 120 {
				preview = preview[:120] + "..."
			}
			fmt.Fprintf(os.Stderr, "DEBUG chunk: len=%d answerLen=%d thinkingLen=%d cumulativeAnswer=%v thinking=%v text=%q\n",
				len(text), len(lastAnswer), len(lastThinking),
				strings.HasPrefix(text, lastAnswer) && lastAnswer != "",
				strings.HasPrefix(strings.TrimSpace(text), "**"),
				preview)
		}

		isThinkingText := strings.HasPrefix(strings.TrimSpace(text), "**")

		// Thinking updates are full replacements. Track them separately from
		// answer text so a growing reasoning trace does not get misclassified
		// as the start of the final answer.
		if isThinkingText && !answerStarted {
			if text == lastThinking {
				continue
			}

			header := text
			if idx := strings.Index(text, "\n"); idx > 0 {
				header = text[:idx]
			}
			if !callback(ChatChunk{Text: text, Header: header, Phase: ChatChunkThinking}) {
				return nil
			}
			lastThinking = text
			continue
		}

		answerStarted = true
		if text == lastAnswer {
			continue
		}

		// The server sends cumulative text. Find the longest common
		// prefix with what we already emitted and only send the new
		// suffix. This handles citation consolidation where the server
		// revises earlier text (e.g. "[2, 3]" → "[2-5]").
		commonLen := 0
		limit := len(lastAnswer)
		if len(text) < limit {
			limit = len(text)
		}
		for commonLen < limit && text[commonLen] == lastAnswer[commonLen] {
			commonLen++
		}
		delta := text[commonLen:]
		if delta != "" {
			if !callback(ChatChunk{Text: delta, Phase: ChatChunkAnswer}) {
				return nil
			}
		}
		lastAnswer = text
	}

	return scanner.Err()
}

// parseChatResponse reads the Google chunked response format and extracts text.
// Format: )]}'\n then repeated: <length>\n<json-chunk>\n
func (c *Client) parseChatResponse(r io.Reader, callback func(chunk string) bool) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("read chat response: %w", err)
	}

	body := string(data)

	if c.config.Debug {
		fmt.Fprintf(os.Stderr, "DEBUG: chat response length: %d\n", len(body))
	}

	// Strip )]}' prefix
	if strings.HasPrefix(body, ")]}'") {
		body = body[4:]
	}
	body = strings.TrimLeft(body, "\n")

	// Extract text from wrb.fr chunks
	// Each chunk is: <length>\n<json-array>\n
	// The json-array contains: ["wrb.fr", "service.Method", "<inner-json>", ...]
	var lastText string
	for len(body) > 0 {
		body = strings.TrimLeft(body, "\n ")

		// Read length
		nlIdx := strings.Index(body, "\n")
		if nlIdx < 0 {
			break
		}
		body = body[nlIdx+1:]

		// Find the JSON array in this chunk
		// Look for wrb.fr entries
		startIdx := strings.Index(body, "[\"wrb.fr\"")
		if startIdx < 0 {
			// Try other envelope types like ["e", ...] and skip
			nextNL := strings.Index(body, "\n")
			if nextNL >= 0 {
				body = body[nextNL+1:]
			} else {
				break
			}
			continue
		}

		// Find the balanced end of this JSON array
		chunkJSON := extractJSONArray(body[startIdx:])
		if chunkJSON == "" {
			break
		}

		// Parse the wrb.fr envelope: ["wrb.fr", "method", "<inner-json>", ...]
		var envelope []interface{}
		if err := json.Unmarshal([]byte(chunkJSON), &envelope); err != nil {
			// Skip unparseable chunks
			nextNL := strings.Index(body, "\n")
			if nextNL >= 0 {
				body = body[nextNL+1:]
			} else {
				break
			}
			continue
		}

		if len(envelope) >= 3 {
			if innerStr, ok := envelope[2].(string); ok && innerStr != "" {
				text := extractChatText(innerStr)
				if text != "" && text != lastText {
					// The stream has two phases:
					// 1. Thinking: each chunk is a complete replacement (no shared prefix)
					// 2. Final answer: cumulative — each chunk extends the previous
					// Compute delta to avoid re-emitting already-printed text.
					delta := text
					if strings.HasPrefix(text, lastText) {
						delta = text[len(lastText):]
					}
					if delta != "" {
						if !callback(delta) {
							return nil
						}
					}
					lastText = text
				}
			}
		}

		// Advance past this chunk
		endIdx := startIdx + len(chunkJSON)
		if endIdx < len(body) {
			body = body[endIdx:]
		} else {
			break
		}
	}

	return nil
}

// extractJSONArray extracts a balanced JSON array from the start of a string.
func extractJSONArray(s string) string {
	if len(s) == 0 || s[0] != '[' {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for i, ch := range s {
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if ch == '[' {
			depth++
		} else if ch == ']' {
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}
	return ""
}

// extractChatText extracts the readable text from the inner JSON of a chat response chunk.
// The inner JSON has varying structure but the main text is typically at position [0][0].
func extractChatText(innerJSON string) string {
	var data interface{}
	if err := json.Unmarshal([]byte(innerJSON), &data); err != nil {
		return ""
	}

	// The response structure contains the full response text (not deltas).
	// Navigate: [0] -> [0] to find the text string
	arr, ok := data.([]interface{})
	if !ok || len(arr) == 0 {
		return ""
	}

	// Try [0][0] - the main text content
	if inner, ok := arr[0].([]interface{}); ok && len(inner) > 0 {
		if text, ok := inner[0].(string); ok {
			return text
		}
	}

	return ""
}

// DeleteChatHistory deletes all chat history for a notebook.
func (c *Client) DeleteChatHistory(projectID string) error {
	_, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCDeleteChatHistory,
		NotebookID: projectID,
		Args: []interface{}{
			nil,
			nil,
			projectID,
		},
	})
	if err != nil {
		return fmt.Errorf("delete chat history: %w", err)
	}
	return nil
}

// GetConversations returns conversation IDs for a notebook.
func (c *Client) GetConversations(projectID string) ([]string, error) {
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCGetConversations,
		NotebookID: projectID,
		Args: []interface{}{
			[]interface{}{},
			nil,
			projectID,
			20,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get conversations: %w", err)
	}

	var data []interface{}
	if err := json.Unmarshal(resp, &data); err != nil {
		return nil, fmt.Errorf("parse conversations: %w", err)
	}

	var convIDs []string
	// Response format: [[[conv_id1], [conv_id2]]]
	if len(data) > 0 {
		if outer, ok := data[0].([]interface{}); ok {
			for _, item := range outer {
				if arr, ok := item.([]interface{}); ok && len(arr) > 0 {
					if id, ok := arr[0].(string); ok {
						convIDs = append(convIDs, id)
					}
				}
			}
		}
	}
	return convIDs, nil
}

// GetConversationHistory retrieves the message history for a specific conversation.
func (c *Client) GetConversationHistory(projectID, conversationID string) ([]ChatMessage, error) {
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCGetConversationHistory,
		NotebookID: projectID,
		Args: []interface{}{
			projectID,
			conversationID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get conversation history: %w", err)
	}

	var data []interface{}
	if err := json.Unmarshal(resp, &data); err != nil {
		return nil, fmt.Errorf("parse conversation history: %w", err)
	}

	// Response format: [[content, null, role], [content, null, role], ...]
	// or wrapped: [[[content, null, role], [content, null, role], ...]]
	var messages []ChatMessage
	var msgArrays []interface{}

	if len(data) > 0 {
		// Check if it's wrapped in an extra array
		if outer, ok := data[0].([]interface{}); ok {
			if len(outer) > 0 {
				if _, ok := outer[0].([]interface{}); ok {
					// Wrapped format: [[[msg1], [msg2]]]
					msgArrays = outer
				} else {
					// Flat format: [[msg1], [msg2]]
					msgArrays = data
				}
			}
		}
	}

	for _, item := range msgArrays {
		arr, ok := item.([]interface{})
		if !ok || len(arr) < 3 {
			continue
		}
		content, _ := arr[0].(string)
		role := 0
		if r, ok := arr[2].(float64); ok {
			role = int(r)
		}
		if content != "" && role > 0 {
			messages = append(messages, ChatMessage{
				Content: content,
				Role:    role,
			})
		}
	}

	return messages, nil
}

// ChatGoal represents a conversational goal setting.
type ChatGoal int

const (
	ChatGoalDefault       ChatGoal = 3 // Default conversational style
	ChatGoalLearningGuide ChatGoal = 1 // Learning Guide mode (not yet confirmed)
	ChatGoalCustom        ChatGoal = 2 // Custom with user-provided prompt
)

// ResponseLength represents a response length setting.
type ResponseLength int

const (
	ResponseLengthDefault ResponseLength = 0 // Default (empty array)
	ResponseLengthLonger  ResponseLength = 4 // Longer responses
	ResponseLengthShorter ResponseLength = 3 // Shorter responses (inferred)
)

// SetChatConfig updates the chat configuration for a notebook via MutateProject.
// goalConfig: [goal_type] or [goal_type, "custom_prompt"]
// responseLengthConfig: [] for default, [4] for longer, [3] for shorter
func (c *Client) SetChatConfig(projectID string, goal ChatGoal, customPrompt string, responseLength ResponseLength) error {
	var goalConfig interface{}
	if goal == ChatGoalCustom && customPrompt != "" {
		goalConfig = []interface{}{int(goal), customPrompt}
	} else if goal != 0 {
		goalConfig = []interface{}{int(goal)}
	} else {
		goalConfig = []interface{}{}
	}

	var lengthConfig interface{}
	if responseLength != ResponseLengthDefault {
		lengthConfig = []interface{}{int(responseLength)}
	} else {
		lengthConfig = []interface{}{}
	}

	_, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCMutateProject,
		NotebookID: projectID,
		Args: []interface{}{
			projectID,
			[]interface{}{
				[]interface{}{
					nil, nil, nil, nil, nil, nil, nil,
					[]interface{}{goalConfig, lengthConfig},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("set chat config: %w", err)
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
	sourceIDs := c.resolveSourceIDs(projectID, nil)
	req := &pb.GenerateReportSuggestionsRequest{
		ProjectId: projectID,
		SourceIds: sourceIDs,
	}
	ctx := context.Background()
	response, err := c.orchestrationService.GenerateReportSuggestions(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("generate report suggestions: %w", err)
	}
	return response, nil
}

func (c *Client) GetProjectDetails(shareID string) (*pb.ProjectDetails, error) {
	resp, err := c.rpc.Do(rpc.Call{
		ID: rpc.RPCGetProjectDetails,
		Args: []interface{}{
			shareID,
			[]interface{}{2},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get project details: %w", err)
	}
	return parseProjectDetailsResponse(resp)
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
	response, err := c.shareProjectDirect(projectID, shareOption == SharePublic)
	if err != nil {
		return nil, fmt.Errorf("share audio: %w", err)
	}

	return &ShareAudioResult{
		ShareURL: response.ShareUrl,
		ShareID:  response.ShareId,
		IsPublic: shareOption == SharePublic,
	}, nil
}

// ShareProject shares a project with specified settings
func (c *Client) ShareProject(projectID string, settings *pb.ShareSettings) (*pb.ShareProjectResponse, error) {
	if settings == nil {
		settings = &pb.ShareSettings{}
	}
	return c.shareProjectDirect(projectID, settings.GetIsPublic())
}

func (c *Client) shareProjectDirect(projectID string, isPublic bool) (*pb.ShareProjectResponse, error) {
	linkSettings := []interface{}{1, 0}
	if isPublic {
		linkSettings[1] = 1
	}

	resp, err := c.rpc.Do(rpc.Call{
		ID: rpc.RPCShareProject,
		Args: []interface{}{
			[]interface{}{[]interface{}{projectID, nil, linkSettings, []interface{}{0, ""}}},
			1,
			nil,
			[]interface{}{2},
		},
		NotebookID: projectID,
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
		return strings.HasPrefix(s, "http") && strings.Contains(s, "notebooklm.google.com")
	}); url != "" {
		result.ShareUrl = url
	}
	if result.ShareUrl == "" && isPublic {
		result.ShareUrl = fmt.Sprintf("https://notebooklm.google.com/notebook/%s", projectID)
	}
	if shareID := findStringMatching(responseData, isUUIDLike); shareID != "" {
		result.ShareId = shareID
	}
	return result, nil
}

func parseProjectDetailsResponse(resp []byte) (*pb.ProjectDetails, error) {
	var responseData []interface{}
	if err := json.Unmarshal(resp, &responseData); err != nil {
		return nil, fmt.Errorf("parse project details response: %w", err)
	}

	details := &pb.ProjectDetails{}
	owners, ok := interfaceSliceAt(responseData, 0)
	if ok && len(owners) > 0 {
		if firstOwner, ok := interfaceSliceAt(owners, 0); ok {
			if profile, ok := interfaceSliceAt(firstOwner, 3); ok {
				details.OwnerName = stringAt(profile, 0)
			}
		}
	}
	if flags, ok := interfaceSliceAt(responseData, 1); ok {
		if isPublic, ok := boolAt(flags, 1); ok {
			details.IsPublic = isPublic
		} else if isPublic, ok := boolAt(flags, 0); ok {
			details.IsPublic = isPublic
		}
	}
	return details, nil
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
