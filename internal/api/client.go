// Package api provides the NotebookLM API client.
package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/davecgh/go-spew/spew"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/gen/service"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/beprotojson"
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
		Debug              bool
		UseGeneratedClient bool // Use generated service client vs manual RPC calls
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
	// Default to generated client pathway, allow opt-out with NLM_USE_GENERATED_CLIENT=false
	client.config.UseGeneratedClient = os.Getenv("NLM_USE_GENERATED_CLIENT") != "false"

	return client
}

// Project/Notebook operations

func (c *Client) ListRecentlyViewedProjects() ([]*Notebook, error) {
	if c.config.UseGeneratedClient {
		// Use generated service client
		req := &pb.ListRecentlyViewedProjectsRequest{}
		
		response, err := c.orchestrationService.ListRecentlyViewedProjects(context.Background(), req)
		if err != nil {
			return nil, fmt.Errorf("list projects (generated): %w", err)
		}
		
		return response.Projects, nil
	}

	// Use manual RPC call (original implementation)
	resp, err := c.rpc.Do(rpc.Call{
		ID:   rpc.RPCListRecentlyViewedProjects,
		Args: []interface{}{nil, 1, nil, []int{2}}, // Match web UI format: [null,1,null,[2]]
	})
	if err != nil {
		return nil, fmt.Errorf("list projects (manual): %w", err)
	}

	// Parse batchexecute response format
	var data []interface{}
	if c.config.Debug {
		fmt.Printf("DEBUG: Attempting to parse response: %s\n", string(resp))
	}
	if err := json.Unmarshal(resp, &data); err == nil {
		// Check for batchexecute format: [["wrb.fr","rpc_id",null,null,null,[response_code],"generic"],...]
		if len(data) > 0 {
			if firstArray, ok := data[0].([]interface{}); ok && len(firstArray) > 5 {
				// Check if response_code is [16] (empty list)
				if responseCodeArray, ok := firstArray[5].([]interface{}); ok && len(responseCodeArray) == 1 {
					if code, ok := responseCodeArray[0].(float64); ok && int(code) == 16 {
						// Return empty projects list
						return []*Notebook{}, nil
					}
				}
			}
		}
		
		// Legacy check for simple [16] format
		if len(data) == 1 {
			if code, ok := data[0].(float64); ok && int(code) == 16 {
				// Return empty projects list
				return []*Notebook{}, nil
			}
		}
	}

	// Try to extract projects using chunked response parser first
	// This is a more robust approach for handling the chunked response format
	body := string(resp)
	// Try to parse the response from the chunked response format
	p := NewChunkedResponseParser(body).WithDebug(c.config.Debug)
	projects, err := p.ParseListProjectsResponse()
	if err != nil {
		if c.config.Debug {
			fmt.Printf("DEBUG: Raw response before parsing:\n%s\n", body)
		}

		// Try to parse using the regular method as a fallback
		var response pb.ListRecentlyViewedProjectsResponse
		if err2 := beprotojson.Unmarshal(resp, &response); err2 != nil {
			// Both methods failed
			return nil, fmt.Errorf("parse response: %w (chunked parser: %v)", err2, err)
		}
		return response.Projects, nil
	}
	return projects, nil
}

func (c *Client) CreateProject(title string, emoji string) (*Notebook, error) {
	if c.config.UseGeneratedClient {
		// Use generated service client
		req := &pb.CreateProjectRequest{
			Title: title,
			Emoji: emoji,
		}
		
		project, err := c.orchestrationService.CreateProject(context.Background(), req)
		if err != nil {
			return nil, fmt.Errorf("create project (generated): %w", err)
		}
		return project, nil
	}

	// Use manual RPC call (original implementation)
	resp, err := c.rpc.Do(rpc.Call{
		ID:   rpc.RPCCreateProject,
		Args: []interface{}{title, emoji},
	})
	if err != nil {
		return nil, fmt.Errorf("create project (manual): %w", err)
	}

	var project pb.Project
	if err := beprotojson.Unmarshal(resp, &project); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &project, nil
}

func (c *Client) GetProject(projectID string) (*Notebook, error) {
	if c.config.UseGeneratedClient {
		// Use generated service client
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
	
	// Legacy manual RPC path
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCGetProject,
		Args:       []interface{}{projectID},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	
	// Check for null response
	if resp == nil || len(resp) == 0 || string(resp) == "null" {
		return nil, fmt.Errorf("get project: received null response - notebook may not exist or authentication may have expired")
	}

	// Debug: print raw response
	if c.config.Debug {
		fmt.Printf("DEBUG: GetProject raw response: %s\n", string(resp))
	}

	var project pb.Project
	if err := beprotojson.Unmarshal(resp, &project); err != nil {
		if c.config.Debug {
			fmt.Printf("DEBUG: Failed to unmarshal project: %v\n", err)
			fmt.Printf("DEBUG: Response length: %d\n", len(resp))
			if len(resp) > 200 {
				fmt.Printf("DEBUG: Response preview: %s...\n", string(resp[:200]))
			} else {
				fmt.Printf("DEBUG: Full response: %s\n", string(resp))
			}
		}
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if c.config.Debug && project.Sources != nil {
		fmt.Printf("DEBUG: Successfully parsed project with %d sources\n", len(project.Sources))
	}
	return &project, nil
}

func (c *Client) DeleteProjects(projectIDs []string) error {
	if c.config.UseGeneratedClient {
		// Use generated service client
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
	
	// Legacy manual RPC path
	_, err := c.rpc.Do(rpc.Call{
		ID:   rpc.RPCDeleteProjects,
		Args: []interface{}{projectIDs},
	})
	if err != nil {
		return fmt.Errorf("delete projects: %w", err)
	}
	return nil
}

func (c *Client) MutateProject(projectID string, updates *pb.Project) (*Notebook, error) {
	if c.config.UseGeneratedClient {
		// Use generated service client
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
	
	// Legacy manual RPC path
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCMutateProject,
		Args:       []interface{}{projectID, updates},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("mutate project: %w", err)
	}

	var project pb.Project
	if err := beprotojson.Unmarshal(resp, &project); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &project, nil
}

func (c *Client) RemoveRecentlyViewedProject(projectID string) error {
	if c.config.UseGeneratedClient {
		// Use generated service client
		req := &pb.RemoveRecentlyViewedProjectRequest{
			ProjectId: projectID,
		}
		
		ctx := context.Background()
		_, err := c.orchestrationService.RemoveRecentlyViewedProject(ctx, req)
		return err
	}
	
	// Legacy manual RPC path
	_, err := c.rpc.Do(rpc.Call{
		ID:   rpc.RPCRemoveRecentlyViewed,
		Args: []interface{}{projectID},
	})
	return err
}

// Source operations

func (c *Client) AddSources(projectID string, sources []*pb.SourceInput) (*pb.Project, error) {
	if c.config.UseGeneratedClient {
		// Use generated service client
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

	// Legacy manual RPC path - convert SourceInput to the old format
	var legacyArgs []interface{}
	for _, source := range sources {
		// Convert SourceInput to legacy format based on source type
		switch source.SourceType {
		case pb.SourceType_SOURCE_TYPE_SHARED_NOTE:
			legacyArgs = append(legacyArgs, []interface{}{
				nil,
				[]string{source.Title, source.Content},
				nil,
				2, // text source type
			})
		case pb.SourceType_SOURCE_TYPE_LOCAL_FILE:
			legacyArgs = append(legacyArgs, []interface{}{
				source.Base64Content,
				source.Filename,
				source.MimeType,
				"base64",
			})
		case pb.SourceType_SOURCE_TYPE_WEB_PAGE:
			legacyArgs = append(legacyArgs, []interface{}{
				nil,
				nil,
				[]string{source.Url},
			})
		case pb.SourceType_SOURCE_TYPE_YOUTUBE_VIDEO:
			legacyArgs = append(legacyArgs, []interface{}{
				nil,
				nil,
				nil,
				source.YoutubeVideoId,
			})
		}
	}

	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCAddSources,
		Args:       []interface{}{legacyArgs, projectID},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("add sources: %w", err)
	}

	var result pb.Project
	if err := beprotojson.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &result, nil
}

func (c *Client) DeleteSources(projectID string, sourceIDs []string) error {
	if c.config.UseGeneratedClient {
		// Use generated service client
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

	// Legacy manual RPC path
	_, err := c.rpc.Do(rpc.Call{
		ID: rpc.RPCDeleteSources,
		Args: []interface{}{
			[][][]string{{sourceIDs}},
		},
		NotebookID: projectID,
	})
	return err
}

func (c *Client) MutateSource(sourceID string, updates *pb.Source) (*pb.Source, error) {
	if c.config.UseGeneratedClient {
		// Use generated service client
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

	// Legacy manual RPC path
	resp, err := c.rpc.Do(rpc.Call{
		ID:   rpc.RPCMutateSource,
		Args: []interface{}{sourceID, updates},
	})
	if err != nil {
		return nil, fmt.Errorf("mutate source: %w", err)
	}

	var source pb.Source
	if err := beprotojson.Unmarshal(resp, &source); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &source, nil
}

func (c *Client) RefreshSource(sourceID string) (*pb.Source, error) {
	if c.config.UseGeneratedClient {
		// Use generated service client
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

	// Legacy manual RPC path
	resp, err := c.rpc.Do(rpc.Call{
		ID:   rpc.RPCRefreshSource,
		Args: []interface{}{sourceID},
	})
	if err != nil {
		return nil, fmt.Errorf("refresh source: %w", err)
	}

	var source pb.Source
	if err := beprotojson.Unmarshal(resp, &source); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &source, nil
}

func (c *Client) LoadSource(sourceID string) (*pb.Source, error) {
	if c.config.UseGeneratedClient {
		// Use generated service client
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

	// Legacy manual RPC path
	resp, err := c.rpc.Do(rpc.Call{
		ID:   rpc.RPCLoadSource,
		Args: []interface{}{sourceID},
	})
	if err != nil {
		return nil, fmt.Errorf("load source: %w", err)
	}

	var source pb.Source
	if err := beprotojson.Unmarshal(resp, &source); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &source, nil
}

func (c *Client) CheckSourceFreshness(sourceID string) (*pb.CheckSourceFreshnessResponse, error) {
	if c.config.UseGeneratedClient {
		// Use generated service client
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

	// Legacy manual RPC path
	resp, err := c.rpc.Do(rpc.Call{
		ID:   rpc.RPCCheckSourceFreshness,
		Args: []interface{}{sourceID},
	})
	if err != nil {
		return nil, fmt.Errorf("check source freshness: %w", err)
	}

	var result pb.CheckSourceFreshnessResponse
	if err := beprotojson.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &result, nil
}

func (c *Client) ActOnSources(projectID string, action string, sourceIDs []string) error {
	if c.config.UseGeneratedClient {
		// Use generated service client
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

	// Legacy manual RPC path
	_, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCActOnSources,
		Args:       []interface{}{projectID, action, sourceIDs},
		NotebookID: projectID,
	})
	return err
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
		// Add debug output about JSON handling for any environment
		if strings.HasSuffix(filename, ".json") || detectedType == "application/json" {
			fmt.Fprintf(os.Stderr, "Handling JSON file as text: %s (MIME: %s)\n", filename, detectedType)
		}
		return c.AddSourceFromText(projectID, string(content), filename)
	}

	encoded := base64.StdEncoding.EncodeToString(content)
	return c.AddSourceFromBase64(projectID, encoded, filename, detectedType)
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
		fmt.Fprintln(os.Stderr, resp)
		spew.Dump(resp)
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
		spew.Dump(payload)
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
	if c.config.UseGeneratedClient {
		// Use generated service client
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

	// Legacy manual RPC path
	resp, err := c.rpc.Do(rpc.Call{
		ID: rpc.RPCCreateNote,
		Args: []interface{}{
			projectID,
			initialContent,
			[]int{1}, // note type
			nil,
			title,
		},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("create note: %w", err)
	}

	var note Note
	if err := beprotojson.Unmarshal(resp, &note); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &note, nil
}

func (c *Client) MutateNote(projectID string, noteID string, content string, title string) (*Note, error) {
	if c.config.UseGeneratedClient {
		// Use generated service client
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

	// Legacy manual RPC path
	resp, err := c.rpc.Do(rpc.Call{
		ID: rpc.RPCMutateNote,
		Args: []interface{}{
			projectID,
			noteID,
			[][][]interface{}{{
				{content, title, []interface{}{}},
			}},
		},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("mutate note: %w", err)
	}

	var note Note
	if err := beprotojson.Unmarshal(resp, &note); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &note, nil
}

func (c *Client) DeleteNotes(projectID string, noteIDs []string) error {
	if c.config.UseGeneratedClient {
		// Use generated service client
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

	// Legacy manual RPC path
	_, err := c.rpc.Do(rpc.Call{
		ID: rpc.RPCDeleteNotes,
		Args: []interface{}{
			[][][]string{{noteIDs}},
		},
		NotebookID: projectID,
	})
	return err
}

func (c *Client) GetNotes(projectID string) ([]*Note, error) {
	if c.config.UseGeneratedClient {
		// Use generated service client
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

	// Legacy manual RPC path
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCGetNotes,
		Args:       []interface{}{projectID},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("get notes: %w", err)
	}

	var response pb.GetNotesResponse
	if err := beprotojson.Unmarshal(resp, &response); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
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

	if c.config.UseGeneratedClient {
		// Use generated service client
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
			AudioID:   "", // Not available in pb.AudioOverview
			Title:     "", // Not available in pb.AudioOverview
			AudioData: audioOverview.Content, // Map Content to AudioData
			IsReady:   audioOverview.Status != "CREATING", // Infer from Status
		}
		return result, nil
	}

	// Legacy manual RPC path
	resp, err := c.rpc.Do(rpc.Call{
		ID: rpc.RPCCreateAudioOverview,
		Args: []interface{}{
			projectID,
			0,
			[]string{
				instructions,
			},
		},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("create audio overview: %w", err)
	}

	var data []interface{}
	if err := json.Unmarshal(resp, &data); err != nil {
		return nil, fmt.Errorf("parse response JSON: %w", err)
	}

	result := &AudioOverviewResult{
		ProjectID: projectID,
	}

	// Handle empty or nil response
	if len(data) == 0 {
		return result, nil
	}

	// Parse the wrb.fr response format
	// Format: [null,null,[3,"<base64-audio>","<id>","<title>",null,true],null,[false]]
	if len(data) > 2 {
		audioData, ok := data[2].([]interface{})
		if !ok || len(audioData) < 4 {
			// Creation might be in progress, return result without error
			return result, nil
		}

		// Extract audio data (index 1)
		if audioBase64, ok := audioData[1].(string); ok {
			result.AudioData = audioBase64
		}

		// Extract ID (index 2)
		if id, ok := audioData[2].(string); ok {
			result.AudioID = id
		}

		// Extract title (index 3)
		if title, ok := audioData[3].(string); ok {
			result.Title = title
		}

		// Extract ready status (index 5)
		if len(audioData) > 5 {
			if ready, ok := audioData[5].(bool); ok {
				result.IsReady = ready
			}
		}
	}

	return result, nil
}

func (c *Client) GetAudioOverview(projectID string) (*AudioOverviewResult, error) {
	if c.config.UseGeneratedClient {
		// Use generated service client
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
			AudioID:   "", // Not available in pb.AudioOverview
			Title:     "", // Not available in pb.AudioOverview
			AudioData: audioOverview.Content, // Map Content to AudioData
			IsReady:   audioOverview.Status != "CREATING", // Infer from Status
		}
		return result, nil
	}

	// Legacy manual RPC path
	resp, err := c.rpc.Do(rpc.Call{
		ID: rpc.RPCGetAudioOverview,
		Args: []interface{}{
			projectID,
			1,
		},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("get audio overview: %w", err)
	}

	var data []interface{}
	if err := json.Unmarshal(resp, &data); err != nil {
		return nil, fmt.Errorf("parse response JSON: %w", err)
	}

	result := &AudioOverviewResult{
		ProjectID: projectID,
	}

	// Handle empty or nil response
	if len(data) == 0 {
		return result, nil
	}

	// Parse the wrb.fr response format
	// Format: [null,null,[3,"<base64-audio>","<id>","<title>",null,true],null,[false]]
	if len(data) > 2 {
		audioData, ok := data[2].([]interface{})
		if !ok || len(audioData) < 4 {
			return nil, fmt.Errorf("invalid audio data format")
		}

		// Extract audio data (index 1)
		if audioBase64, ok := audioData[1].(string); ok {
			result.AudioData = audioBase64
		}

		// Extract ID (index 2)
		if id, ok := audioData[2].(string); ok {
			result.AudioID = id
		}

		// Extract title (index 3)
		if title, ok := audioData[3].(string); ok {
			result.Title = title
		}

		// Extract ready status (index 5)
		if len(audioData) > 5 {
			if ready, ok := audioData[5].(bool); ok {
				result.IsReady = ready
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
	if c.config.UseGeneratedClient {
		// Use generated service client
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

	// Legacy manual RPC path
	_, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCDeleteAudioOverview,
		Args:       []interface{}{projectID},
		NotebookID: projectID,
	})
	return err
}

// Generation operations

func (c *Client) GenerateDocumentGuides(projectID string) (*pb.GenerateDocumentGuidesResponse, error) {
	if c.config.UseGeneratedClient {
		// Use generated service client
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

	// Legacy manual RPC path
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCGenerateDocumentGuides,
		Args:       []interface{}{projectID},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("generate document guides: %w", err)
	}

	var guides pb.GenerateDocumentGuidesResponse
	if err := beprotojson.Unmarshal(resp, &guides); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &guides, nil
}

func (c *Client) GenerateNotebookGuide(projectID string) (*pb.GenerateNotebookGuideResponse, error) {
	if c.config.UseGeneratedClient {
		// Use generated service client
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

	// Legacy manual RPC path
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCGenerateNotebookGuide,
		Args:       []interface{}{projectID},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("generate notebook guide: %w", err)
	}

	var guide pb.GenerateNotebookGuideResponse
	if err := beprotojson.Unmarshal(resp, &guide); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &guide, nil
}

func (c *Client) GenerateMagicView(projectID string, sourceIDs []string) (*pb.GenerateMagicViewResponse, error) {
	if c.config.UseGeneratedClient {
		// Use generated service client
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

	// Legacy manual RPC path
	resp, err := c.rpc.Do(rpc.Call{
		ID:         "uK8f7c", // RPC ID for GenerateMagicView
		Args:       []interface{}{projectID, sourceIDs},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("generate magic view: %w", err)
	}

	var magicView pb.GenerateMagicViewResponse
	if err := beprotojson.Unmarshal(resp, &magicView); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &magicView, nil
}

func (c *Client) GenerateOutline(projectID string) (*pb.GenerateOutlineResponse, error) {
	if c.config.UseGeneratedClient {
		// Use generated service client
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

	// Legacy manual RPC path
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCGenerateOutline,
		Args:       []interface{}{projectID},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("generate outline: %w", err)
	}

	var outline pb.GenerateOutlineResponse
	if err := beprotojson.Unmarshal(resp, &outline); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &outline, nil
}

func (c *Client) GenerateSection(projectID string) (*pb.GenerateSectionResponse, error) {
	if c.config.UseGeneratedClient {
		// Use generated service client
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

	// Legacy manual RPC path
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCGenerateSection,
		Args:       []interface{}{projectID},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("generate section: %w", err)
	}

	var section pb.GenerateSectionResponse
	if err := beprotojson.Unmarshal(resp, &section); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &section, nil
}

func (c *Client) StartDraft(projectID string) (*pb.StartDraftResponse, error) {
	if c.config.UseGeneratedClient {
		// Use generated service client
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

	// Legacy manual RPC path
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCStartDraft,
		Args:       []interface{}{projectID},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("start draft: %w", err)
	}

	var draft pb.StartDraftResponse
	if err := beprotojson.Unmarshal(resp, &draft); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &draft, nil
}

func (c *Client) StartSection(projectID string) (*pb.StartSectionResponse, error) {
	if c.config.UseGeneratedClient {
		// Use generated service client
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

	// Legacy manual RPC path
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCStartSection,
		Args:       []interface{}{projectID},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("start section: %w", err)
	}

	var section pb.StartSectionResponse
	if err := beprotojson.Unmarshal(resp, &section); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &section, nil
}

func (c *Client) GenerateFreeFormStreamed(projectID string, prompt string, sourceIDs []string) (*pb.GenerateFreeFormStreamedResponse, error) {
	if c.config.UseGeneratedClient {
		// Use generated service client
		req := &pb.GenerateFreeFormStreamedRequest{
			ProjectId: projectID,
			Prompt:    prompt,
			SourceIds: sourceIDs,
		}
		ctx := context.Background()
		response, err := c.orchestrationService.GenerateFreeFormStreamed(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("generate free form streamed: %w", err)
		}
		return response, nil
	}

	// Legacy manual RPC path
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCGenerateFreeFormStreamed,
		Args:       []interface{}{projectID, prompt, sourceIDs},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("generate free form streamed: %w", err)
	}

	var response pb.GenerateFreeFormStreamedResponse
	if err := beprotojson.Unmarshal(resp, &response); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &response, nil
}

func (c *Client) GenerateReportSuggestions(projectID string) (*pb.GenerateReportSuggestionsResponse, error) {
	if c.config.UseGeneratedClient {
		// Use generated service client
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

	// Legacy manual RPC path  
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCGenerateReportSuggestions,
		Args:       []interface{}{projectID},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("generate report suggestions: %w", err)
	}

	var response pb.GenerateReportSuggestionsResponse
	if err := beprotojson.Unmarshal(resp, &response); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &response, nil
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
	if c.config.UseGeneratedClient {
		// Use generated service client
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

	// Legacy manual RPC path
	resp, err := c.rpc.Do(rpc.Call{
		ID: rpc.RPCShareAudio,
		Args: []interface{}{
			[]int{int(shareOption)},
			projectID,
		},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("share audio: %w", err)
	}

	// Parse the raw response
	var data []interface{}
	if err := json.Unmarshal(resp, &data); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	result := &ShareAudioResult{
		IsPublic: shareOption == SharePublic,
	}

	// Extract share URL and ID from response
	if len(data) > 0 {
		if shareData, ok := data[0].([]interface{}); ok && len(shareData) > 0 {
			if shareURL, ok := shareData[0].(string); ok {
				result.ShareURL = shareURL
			}
			if len(shareData) > 1 {
				if shareID, ok := shareData[1].(string); ok {
					result.ShareID = shareID
				}
			}
		}
	}

	return result, nil
}

// ShareProject shares a project with specified settings
func (c *Client) ShareProject(projectID string, settings *pb.ShareSettings) (*pb.ShareProjectResponse, error) {
	if c.config.UseGeneratedClient {
		// Use generated service client
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

	// Legacy manual RPC path
	resp, err := c.rpc.Do(rpc.Call{
		ID: rpc.RPCShareProject,
		Args: []interface{}{
			projectID,
			settings,
		},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("share project: %w", err)
	}

	var response pb.ShareProjectResponse
	if err := beprotojson.Unmarshal(resp, &response); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &response, nil
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
