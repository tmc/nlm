// Package api provides the NotebookLM API client.
package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/davecgh/go-spew/spew"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/beprotojson"
	"github.com/tmc/nlm/internal/rpc"
)

type Notebook = pb.Project
type Note = pb.Source

// Client handles NotebookLM API interactions.
type Client struct {
	rpc *rpc.Client
}

// New creates a new NotebookLM API client.
func New(authToken, cookies string, opts ...batchexecute.Option) *Client {
	return &Client{
		rpc: rpc.New(authToken, cookies, opts...),
	}
}

// Project/Notebook operations

func (c *Client) ListRecentlyViewedProjects() ([]*Notebook, error) {
	resp, err := c.rpc.Do(rpc.Call{
		ID:   rpc.RPCListRecentlyViewedProjects,
		Args: []interface{}{nil, 1},
	})
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	var response pb.ListRecentlyViewedProjectsResponse
	if err := beprotojson.Unmarshal(resp, &response); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return response.Projects, nil
}

func (c *Client) CreateProject(title string, emoji string) (*Notebook, error) {
	resp, err := c.rpc.Do(rpc.Call{
		ID:   rpc.RPCCreateProject,
		Args: []interface{}{title, emoji},
	})
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}

	var project pb.Project
	if err := beprotojson.Unmarshal(resp, &project); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &project, nil
}

func (c *Client) GetProject(projectID string) (*Notebook, error) {
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCGetProject,
		Args:       []interface{}{projectID},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}

	var project pb.Project
	if err := beprotojson.Unmarshal(resp, &project); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &project, nil
}

func (c *Client) DeleteProjects(projectIDs []string) error {
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
	_, err := c.rpc.Do(rpc.Call{
		ID:   rpc.RPCRemoveRecentlyViewed,
		Args: []interface{}{projectID},
	})
	return err
}

// Source operations

/*
func (c *Client) AddSources(projectID string, sources []*pb.Source) ([]*pb.Source, error) {
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCAddSources,
		Args:       []interface{}{projectID, sources},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("add sources: %w", err)
	}

	var result []*pb.Source
	if err := beprotojson.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return result, nil
}
*/

func (c *Client) DeleteSources(projectID string, sourceIDs []string) error {
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

/*
func (c *Client) CheckSourceFreshness(sourceID string) (*pb.CheckSourceFreshnessResponse, error) {
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
*/

func (c *Client) ActOnSources(projectID string, action string, sourceIDs []string) error {
	_, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCActOnSources,
		Args:       []interface{}{projectID, action, sourceIDs},
		NotebookID: projectID,
	})
	return err
}

// Source upload utility methods

func (c *Client) AddSourceFromReader(projectID string, r io.Reader, filename string) (string, error) {
	content, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read content: %w", err)
	}

	contentType := http.DetectContentType(content)

	if strings.HasPrefix(contentType, "text/") {
		return c.AddSourceFromText(projectID, string(content), filename)
	}

	encoded := base64.StdEncoding.EncodeToString(content)
	return c.AddSourceFromBase64(projectID, encoded, filename, contentType)
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

func (c *Client) AddSourceFromFile(projectID string, filepath string) (string, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	return c.AddSourceFromReader(projectID, f, filepath)
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
	_, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCDeleteAudioOverview,
		Args:       []interface{}{projectID},
		NotebookID: projectID,
	})
	return err
}

// Generation operations

func (c *Client) GenerateDocumentGuides(projectID string) (*pb.GenerateDocumentGuidesResponse, error) {
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

func (c *Client) GenerateOutline(projectID string) (*pb.GenerateOutlineResponse, error) {
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
