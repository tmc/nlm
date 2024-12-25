package rpc

import (
	"encoding/json"
	"fmt"

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

	// NotebookLM service - Note operations
	RPCCreateNote  = "CYK0Xb" // CreateNote
	RPCMutateNote  = "cYAfTb" // MutateNote
	RPCDeleteNotes = "AH0mwd" // DeleteNotes
	RPCGetNotes    = "cFji9"  // GetNotes

	// NotebookLM service - Audio operations
	RPCCreateAudioOverview = "AHyHrd" // CreateAudioOverview
	RPCGetAudioOverview    = "VUsiyb" // GetAudioOverview
	RPCDeleteAudioOverview = "sJDbic" // DeleteAudioOverview

	// NotebookLM service - Generation operations
	RPCGenerateDocumentGuides = "tr032e" // GenerateDocumentGuides
	RPCGenerateNotebookGuide  = "VfAZjd" // GenerateNotebookGuide
	RPCGenerateOutline        = "lCjAd"  // GenerateOutline
	RPCGenerateSection        = "BeTrYd" // GenerateSection
	RPCStartDraft             = "exXvGf" // StartDraft
	RPCStartSection           = "pGC7gf" // StartSection

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
// New creates a new NotebookLM RPC client
func New(authToken, cookies string, options ...batchexecute.Option) *Client {
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
			"bl":    "boq_labs-tailwind-frontend_20241114.01_p0",
			"f.sid": "-7121977511756781186",
			"hl":    "en",
			// Omit this to get cleaner output.
			//"rt":    "c",
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
