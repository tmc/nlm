package rpc

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/davecgh/go-spew/spew"
	"github.com/tmc/nlm/internal/batchexecute"
)

// Default API parameters - used as fallback if extraction fails
const (
	DefaultBuildVersion = "boq_labs-tailwind-frontend_20251120.08_p0"
	DefaultSessionID    = "-8913782897795119716"
)

// APIParams holds dynamically extracted API parameters
type APIParams struct {
	BuildVersion string // bl parameter
	SessionID    string // f.sid parameter
}

var (
	cachedParams *APIParams
	paramsMutex  sync.Mutex
)

// GetAPIParams returns API parameters, either from cache, env vars, or by fetching from NotebookLM
func GetAPIParams(cookies string) *APIParams {
	paramsMutex.Lock()
	defer paramsMutex.Unlock()

	// Return cached if available
	if cachedParams != nil {
		return cachedParams
	}

	// Check environment variables first
	bl := os.Getenv("NLM_BUILD_VERSION")
	sid := os.Getenv("NLM_SESSION_ID")

	if bl != "" && sid != "" {
		cachedParams = &APIParams{BuildVersion: bl, SessionID: sid}
		return cachedParams
	}

	// Try to fetch from NotebookLM page
	if cookies != "" {
		if params := fetchAPIParamsFromPage(cookies); params != nil {
			cachedParams = params
			return cachedParams
		}
	}

	// Fallback to defaults
	cachedParams = &APIParams{
		BuildVersion: DefaultBuildVersion,
		SessionID:    DefaultSessionID,
	}
	return cachedParams
}

// fetchAPIParamsFromPage extracts bl and f.sid from the NotebookLM HTML page
func fetchAPIParamsFromPage(cookies string) *APIParams {
	req, err := http.NewRequest("GET", "https://notebooklm.google.com/", nil)
	if err != nil {
		return nil
	}

	req.Header.Set("Cookie", cookies)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	html := string(body)
	params := &APIParams{}

	// Extract build version (bl) - pattern: "cfb2h":"boq_labs-tailwind-frontend_..."
	blRegex := regexp.MustCompile(`"cfb2h":"(boq_labs-tailwind-frontend_[^"]+)"`)
	if matches := blRegex.FindStringSubmatch(html); len(matches) > 1 {
		params.BuildVersion = matches[1]
	}

	// Extract session ID (f.sid) - pattern: "FdrFJe":"-1234567890"
	sidRegex := regexp.MustCompile(`"FdrFJe":"(-?\d+)"`)
	if matches := sidRegex.FindStringSubmatch(html); len(matches) > 1 {
		params.SessionID = matches[1]
	}

	// Also try alternative patterns if primary ones fail
	if params.BuildVersion == "" {
		// Try: bl=boq_labs... in script
		blAltRegex := regexp.MustCompile(`bl['":\s=]+['"]?(boq_labs-tailwind-frontend_[^'"&\s]+)`)
		if matches := blAltRegex.FindStringSubmatch(html); len(matches) > 1 {
			params.BuildVersion = matches[1]
		}
	}

	if params.SessionID == "" {
		// Try: f.sid= pattern
		sidAltRegex := regexp.MustCompile(`f\.sid['":\s=]+['"]?(-?\d+)`)
		if matches := sidAltRegex.FindStringSubmatch(html); len(matches) > 1 {
			params.SessionID = matches[1]
		}
	}

	// Only return if we got at least one value
	if params.BuildVersion != "" || params.SessionID != "" {
		// Fill in defaults for missing values
		if params.BuildVersion == "" {
			params.BuildVersion = DefaultBuildVersion
		}
		if params.SessionID == "" {
			params.SessionID = DefaultSessionID
		}
		if os.Getenv("NLM_DEBUG") != "" {
			fmt.Printf("DEBUG: Extracted API params - bl: %s, f.sid: %s\n",
				params.BuildVersion[:min(50, len(params.BuildVersion))], params.SessionID)
		}
		return params
	}

	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ClearAPIParamsCache clears the cached API parameters (useful for refresh)
func ClearAPIParamsCache() {
	paramsMutex.Lock()
	defer paramsMutex.Unlock()
	cachedParams = nil
}

// Helper to check if a string contains NotebookLM-related content
func isNotebookLMPage(html string) bool {
	return strings.Contains(html, "notebooklm") || strings.Contains(html, "LabsTailwind")
}

// RPC endpoint IDs for NotebookLM services
const (
	// NotebookLM service - Project/Notebook operations
	RPCListRecentlyViewedProjects = "wXbhsf" // ListRecentlyViewedProjects
	RPCCreateProject              = "CCqFvf" // CreateProject
	RPCGetProject                 = "rLM1Ne" // GetProject
	RPCDeleteProjects             = "WWINqb" // DeleteProjects
	RPCMutateProject              = "s0tc2d" // MutateProject
	RPCRemoveRecentlyViewed       = "fejl7e" // RemoveRecentlyViewedProject
	RPCRegisterBinarySource       = "o4cbdc" // RegisterBinarySource (for file upload)

	// NotebookLM service - Source operations
	RPCAddSources           = "izAoDd" // AddSources
	RPCDeleteSources        = "tGMBJ"  // DeleteSources
	RPCMutateSource         = "b7Wfje" // MutateSource
	RPCRefreshSource        = "FLmJqe" // RefreshSource
	RPCLoadSource           = "hizoJc" // LoadSource
	RPCCheckSourceFreshness = "yR9Yof" // CheckSourceFreshness
	RPCActOnSources         = "yyryJe" // ActOnSources
	RPCDiscoverSources      = "qXyaNe" // DiscoverSources

	// NotebookLM service - Note operations
	RPCCreateNote  = "CYK0Xb" // CreateNote
	RPCMutateNote  = "cYAfTb" // MutateNote
	RPCDeleteNotes = "AH0mwd" // DeleteNotes
	RPCGetNotes    = "cFji9"  // GetNotes

	// NotebookLM service - Audio operations
	RPCCreateAudioOverview = "AHyHrd" // CreateAudioOverview
	RPCGetAudioOverview    = "VUsiyb" // GetAudioOverview
	RPCDeleteAudioOverview = "sJDbic" // DeleteAudioOverview

	// NotebookLM service - Video operations
	RPCCreateVideoOverview = "R7cb6c" // CreateVideoOverview

	// NotebookLM service - Generation operations
	RPCGenerateDocumentGuides    = "tr032e" // GenerateDocumentGuides
	RPCGenerateNotebookGuide     = "VfAZjd" // GenerateNotebookGuide
	RPCGenerateOutline           = "lCjAd"  // GenerateOutline
	RPCGenerateSection           = "BeTrYd" // GenerateSection
	RPCStartDraft                = "exXvGf" // StartDraft
	RPCStartSection              = "pGC7gf" // StartSection
	RPCGenerateFreeFormStreamed  = "BD"     // GenerateFreeFormStreamed (from Gemini's analysis)
	RPCGenerateReportSuggestions = "GHsKob" // GenerateReportSuggestions

	// NotebookLM service - Account operations
	RPCGetOrCreateAccount = "ZwVcOc" // GetOrCreateAccount
	RPCMutateAccount      = "hT54vc" // MutateAccount

	// NotebookLM service - Analytics operations
	RPCGetProjectAnalytics = "AUrzMb" // GetProjectAnalytics
	RPCSubmitFeedback      = "uNyJKe" // SubmitFeedback

	// NotebookLMSharing service operations
	RPCShareAudio        = "RGP97b" // ShareAudio
	RPCGetProjectDetails = "JFMDGd" // GetProjectDetails
	RPCShareProject      = "QDyure" // ShareProject

	// NotebookLMGuidebooks service operations
	RPCDeleteGuidebook              = "ARGkVc" // DeleteGuidebook
	RPCGetGuidebook                 = "EYqtU"  // GetGuidebook
	RPCListRecentlyViewedGuidebooks = "YJBpHc" // ListRecentlyViewedGuidebooks
	RPCPublishGuidebook             = "R6smae" // PublishGuidebook
	RPCGetGuidebookDetails          = "LJyzeb" // GetGuidebookDetails
	RPCShareGuidebook               = "OTl0K"  // ShareGuidebook
	RPCGuidebookGenerateAnswer      = "itA0pc" // GuidebookGenerateAnswer

	// LabsTailwindOrchestrationService - Artifact operations
	RPCCreateArtifact = "xpWGLf" // CreateArtifact
	RPCGetArtifact    = "BnLyuf" // GetArtifact
	RPCUpdateArtifact = "DJezBc" // UpdateArtifact
	RPCRenameArtifact = "rc3d8d" // RenameArtifact - for title updates
	RPCDeleteArtifact = "WxBZtb" // DeleteArtifact
	RPCListArtifacts  = "gArtLc" // ListArtifacts - get artifacts list

	// LabsTailwindOrchestrationService - Additional operations
	RPCListFeaturedProjects = "nS9Qlc" // ListFeaturedProjects
	RPCReportContent        = "rJKx8e" // ReportContent
)

// Call represents a NotebookLM RPC call
type Call struct {
	ID         string        // RPC endpoint ID
	Args       []interface{} // Arguments for the call
	NotebookID string        // Optional notebook ID for context
}

// Client handles NotebookLM RPC communication
type Client struct {
	Config batchexecute.Config
	client *batchexecute.Client
}

// New creates a new NotebookLM RPC client
func New(authToken, cookies string, options ...batchexecute.Option) *Client {
	// Get API parameters dynamically (from env, page extraction, or defaults)
	params := GetAPIParams(cookies)

	config := batchexecute.Config{
		Host:      "notebooklm.google.com",
		App:       "LabsTailwindUi",
		AuthToken: authToken,
		Cookies:   cookies,
		Headers: map[string]string{
			"content-type":    "application/x-www-form-urlencoded;charset=UTF-8",
			"origin":          "https://notebooklm.google.com",
			"referer":         "https://notebooklm.google.com/",
			"x-same-domain":   "1",
			"accept":          "*/*",
			"accept-language": "en-US,en;q=0.9",
			"cache-control":   "no-cache",
			"pragma":          "no-cache",
		},
		URLParams: map[string]string{
			"bl":    params.BuildVersion,
			"f.sid": params.SessionID,
			"hl":    "en",
		},
	}
	return &Client{
		Config: config,
		client: batchexecute.NewClient(config, options...),
	}
}

// Do executes a NotebookLM RPC call
func (c *Client) Do(call Call) (json.RawMessage, error) {
	if c.Config.Debug {
		fmt.Printf("\n=== RPC Call ===\n")
		fmt.Printf("ID: %s\n", call.ID)
		fmt.Printf("NotebookID: %s\n", call.NotebookID)
		fmt.Printf("Args:\n")
		spew.Dump(call.Args)
	}

	// Create request-specific URL parameters
	urlParams := make(map[string]string)
	for k, v := range c.Config.URLParams {
		urlParams[k] = v
	}

	if call.NotebookID != "" {
		urlParams["source-path"] = "/notebook/" + call.NotebookID
	} else {
		urlParams["source-path"] = "/"
	}

	rpc := batchexecute.RPC{
		ID:        call.ID,
		Args:      call.Args,
		Index:     "generic",
		URLParams: urlParams,
	}

	if c.Config.Debug {
		fmt.Printf("\nRPC Request:\n")
		spew.Dump(rpc)
	}

	resp, err := c.client.Do(rpc)
	if err != nil {
		return nil, fmt.Errorf("execute rpc: %w", err)
	}

	if c.Config.Debug {
		fmt.Printf("\nRPC Response:\n")
		spew.Dump(resp)
	}

	return resp.Data, nil
}

// Heartbeat sends a heartbeat to keep the session alive
func (c *Client) Heartbeat() error {
	return nil
}

// ListNotebooks returns all notebooks
func (c *Client) ListNotebooks() (json.RawMessage, error) {
	return c.Do(Call{
		ID: RPCListRecentlyViewedProjects,
	})
}

// CreateNotebook creates a new notebook with the given title
func (c *Client) CreateNotebook(title string) (json.RawMessage, error) {
	return nil, fmt.Errorf("not implemented")
}

// DeleteNotebook deletes a notebook by ID
func (c *Client) DeleteNotebook(id string) error {
	return fmt.Errorf("not implemented")
}
