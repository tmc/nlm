package rpc

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/davecgh/go-spew/spew"
	"github.com/tmc/nlm/internal/batchexecute"
)

// ServiceConfig defines configuration for a generated BatchExecute service client.
type ServiceConfig struct {
	Host      string
	App       string
	URLParams map[string]string
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

	// NotebookLM service - Source operations
	RPCAddSources           = "izAoDd" // AddSources
	RPCAddFileSource        = "o4cbdc" // AddFileSource (register uploaded file)
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
	RPCCreateAudioOverview     = "AHyHrd" // CreateAudioOverview
	RPCGetAudioOverview        = "VUsiyb" // GetAudioOverview
	RPCDeleteAudioOverview     = "sJDbic" // DeleteAudioOverview
	RPCFetchInteractivityToken = "Of0kDd" // FetchInteractivityToken - voice session auth + ICE config
	RPCSDPExchange             = "eyWvXc" // SDP offer/answer exchange for WebRTC negotiation

	// NotebookLM service - Video/Artifact operations
	RPCCreateUniversalArtifact = "R7cb6c" // Universal artifact creator (audio, video, reports, flashcards, slides, infographics)
	RPCCreateVideoOverview     = "R7cb6c" // Alias for backward compatibility

	// NotebookLM service - Chat operations
	// NOTE: GenerateFreeFormStreamed does NOT use batchexecute. It uses a gRPC-Web
	// endpoint at /_/LabsTailwindUi/data/google.internal.labs.tailwind.orchestration.v1.LabsTailwindOrchestrationService/GenerateFreeFormStreamed
	// The "BD" ID was incorrectly assumed; kept only for reference.
	RPCGenerateFreeFormStreamed = "BD"     // DEPRECATED: chat uses gRPC-Web, not batchexecute
	RPCGetConversations         = "hPTbtc" // GetConversations - list conversation IDs for a notebook
	RPCGetConversationHistory   = "khqZz"  // GetConversationHistory - retrieve chat messages
	RPCDeleteChatHistory        = "e3bVqc" // DeleteChatHistory / PollDeepResearch - server routes by args
	RPCRateConversationTurn     = "J7Gthc" // RateConversationTurn - mark conversation turn (thumbs up/down?)

	// NotebookLM service - Research operations
	RPCStartFastResearch       = "Ljjv0c"             // StartFastResearch - HAR-verified 2026-04-17; response is [conversation_id]. Retracts speculative "Es3dTe" inference from commit b7b04e7.
	RPCFastResearch            = RPCStartFastResearch // DEPRECATED: alias kept so older callers compile. Prefer RPCStartFastResearch.
	RPCStartDeepResearch       = "QA9ei"              // StartDeepResearch - initiate deep research; returns [research_id, conversation_id]
	RPCGetDeepResearchSessions = "e3bVqc"             // GetDeepResearchSessions - returns ALL deep/fast research sessions for a notebook; clients scan by (research_id, mode=5) for deep or (conversation_id, mode=1) for fast
	RPCDeleteDeepResearch      = "LBwxtb"             // DeleteDeepResearch - soft-delete a session (state transitions 2→5, row remains with tombstone state)

	// Deprecated: e3bVqc is a polymorphic RPC that serves
	// DeleteChatHistory, GetDeepResearchSessions, AND (as of 2026-04-17)
	// the session list for fast-research. Use the explicit constants
	// above at call sites; this alias kept for legacy callers.
	RPCPollDeepResearch = RPCGetDeepResearchSessions

	// NotebookLM service - Generation operations
	RPCGenerateDocumentGuides    = "tr032e" // GenerateDocumentGuides
	RPCGenerateNotebookGuide     = "VfAZjd" // GenerateNotebookGuide
	RPCGenerateOutline           = "lCjAd"  // GenerateOutline (DEPRECATED: use ciyUvf → R7cb6c workflow)
	RPCGenerateSection           = "BeTrYd" // GenerateSection (DEPRECATED: use ciyUvf → R7cb6c workflow)
	RPCStartDraft                = "exXvGf" // StartDraft
	RPCStartSection              = "pGC7gf" // StartSection
	RPCGenerateReportSuggestions = "ciyUvf" // GenerateReportSuggestions (HAR-verified; was GHsKob)
	RPCGetAudioFormats           = "sqTeoe" // GetAudioFormats - returns available audio overview types

	// NotebookLM service - Account operations
	RPCGetOrCreateAccount = "ZwVcOc" // GetOrCreateAccount
	RPCMutateAccount      = "hT54vc" // MutateAccount

	// NotebookLM service - Analytics operations
	RPCGetProjectAnalytics = "AUrzMb" // GetProjectAnalytics
	RPCSubmitFeedback      = "uNyJKe" // SubmitFeedback
	RPCLogEvent            = "ozz5Z"  // LogEvent - analytics/telemetry

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
	//
	// There is no dedicated GetArtifact RPC: the web UI reads individual
	// artifacts by calling ListArtifacts (gArtLc) and filtering client-side.
	// api.Client.GetArtifact matches that pattern via a scan fallback.
	// DeleteArtifact is V5N4be; the earlier WxBZtb/BnLyuf IDs returned 400
	// against the live server and were never reachable. HAR evidence in
	// NotebookLM web UI batchexecute capture (2026-04-07).
	RPCCreateArtifact = "xpWGLf" // CreateArtifact
	RPCUpdateArtifact = "DJezBc" // UpdateArtifact
	RPCRenameArtifact = "rc3d8d" // RenameArtifact - for title updates
	RPCDeleteArtifact = "V5N4be" // DeleteArtifact — HAR-verified 2026-04-07
	RPCListArtifacts  = "gArtLc" // ListArtifacts - get artifacts list

	// LabsTailwindOrchestrationService - Additional operations
	RPCListFeaturedProjects  = "ub2Bae" // ListFeaturedProjects
	RPCReportContent         = "rJKx8e" // ReportContent
	RPCReviseArtifact        = "KmcKPe" // ReviseArtifact - revise artifact with instructions
	RPCListCollections       = "ub2Bae" // ListCollections - list notebook collections/folders
	RPCAudioTopicSuggestions = "otmP3b" // AudioTopicSuggestions - audio topic suggestions
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
	return NewWithConfig(authToken, cookies, ServiceConfig{
		Host: "notebooklm.google.com",
		App:  "LabsTailwindUi",
	}, options...)
}

// NewWithConfig creates a new NotebookLM RPC client with explicit service settings.
func NewWithConfig(authToken, cookies string, serviceConfig ServiceConfig, options ...batchexecute.Option) *Client {
	// Use session-specific parameters from env if available (set during auth)
	blParam := os.Getenv("NLM_BL_PARAM")
	if blParam == "" {
		blParam = "boq_labs-tailwind-frontend_20260406.14_p0"
	}
	sessionID := os.Getenv("NLM_SESSION_ID")
	if sessionID == "" {
		sessionID = "-3785608638908410209"
	}
	host := serviceConfig.Host
	if host == "" {
		host = "notebooklm.google.com"
	}
	app := serviceConfig.App
	if app == "" {
		app = "LabsTailwindUi"
	}
	urlParams := map[string]string{
		"bl":    blParam,
		"f.sid": sessionID,
		"hl":    "en",
		"rt":    "c",
	}
	for k, v := range serviceConfig.URLParams {
		if v == "" {
			continue
		}
		switch k {
		case "bl", "f.sid", "rt":
			// Prefer live session parameters over generated defaults.
			continue
		default:
			urlParams[k] = v
		}
	}

	config := batchexecute.Config{
		Host:      host,
		App:       app,
		AuthToken: authToken,
		Cookies:   cookies,
		Headers: map[string]string{
			"content-type":    "application/x-www-form-urlencoded;charset=UTF-8",
			"origin":          fmt.Sprintf("https://%s", host),
			"referer":         fmt.Sprintf("https://%s/", host),
			"x-same-domain":   "1",
			"accept":          "*/*",
			"accept-language": "en-US,en;q=0.9",
			"cache-control":   "no-cache",
			"pragma":          "no-cache",
		},
		URLParams: urlParams,
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
	return c.Do(Call{
		ID: RPCCreateProject,
		Args: []interface{}{
			title,
			"",
		},
	})
}

// DeleteNotebook deletes a notebook by ID
func (c *Client) DeleteNotebook(id string) error {
	_, err := c.Do(Call{
		ID: RPCDeleteProjects,
		Args: []interface{}{
			[]interface{}{id},
		},
	})
	if err != nil {
		return fmt.Errorf("delete notebook: %w", err)
	}
	return nil
}
