package rpc

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/davecgh/go-spew/spew"
	"github.com/tmc/nlm/internal/batchexecute"
)

// RPC endpoint IDs for NotebookLM services
const (
	// NotebookLM service - Project/Notebook operations
	RPCListRecentlyViewedProjects = "wXbhsf" // ListRecentlyViewedProjects
	RPCCreateProject              = "CCqFvf" // CreateProject
	RPCGetProject                 = "rLM1Ne" // GetProject
	RPCDeleteProjects             = "WWINqb" // DeleteProjects
	RPCMutateProject              = "s0tc2d" // MutateProject
	RPCRemoveRecentlyViewed       = "fejl7e" // RemoveRecentlyViewedProject

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

	// NotebookLM service - Research operations
	RPCStartFastResearch     = "Ljjv0c" // StartFastResearch
	RPCStartDeepResearch     = "QA9ei"  // StartDeepResearch
	RPCPollResearchResults   = "e3bVqc" // PollResearchResults
	RPCImportResearchSources = "LBwxtb" // ImportResearchSources

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
	debugEnabled := os.Getenv("NLM_DEBUG") == "true"

	config := batchexecute.Config{
		Host:      "notebooklm.google.com",
		App:       "LabsTailwindUi",
		AuthToken: authToken,
		Cookies:   cookies,
		Debug:     debugEnabled,
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
			// Match browser build version for compatibility
			"bl":    "boq_labs-tailwind-frontend_20260129.10_p0",
			"f.sid": "-7121977511756781186",
			"hl":    "en",
			"rt":    "c", // Use chunked format like browser
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
		fmt.Fprintf(os.Stderr,"\n=== RPC Call ===\n")
		fmt.Fprintf(os.Stderr,"ID: %s\n", call.ID)
		fmt.Fprintf(os.Stderr,"NotebookID: %s\n", call.NotebookID)
		fmt.Fprintf(os.Stderr,"Args:\n")
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
		fmt.Fprintf(os.Stderr,"\nRPC Request:\n")
		spew.Dump(rpc)
	}

	resp, err := c.client.Do(rpc)
	if err != nil {
		return nil, fmt.Errorf("execute rpc: %w", err)
	}

	if c.Config.Debug {
		fmt.Fprintf(os.Stderr,"\nRPC Response:\n")
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
