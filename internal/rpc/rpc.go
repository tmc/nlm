package rpc

import (
	"encoding/json"
	"fmt"

	"github.com/tmc/nlm/internal/batchexecute"
)

// RPC endpoint IDs for NotebookLM
const (
	// RPCListNotebooks lists all notebooks
	RPCListNotebooks = "wXbhsf"

	// RPCLoadAudioOverview loads audio overview/metadata
	RPCLoadAudioOverview = "VUsiyb"

	// RPCCreateNotebook creates a new notebook
	RPCCreateNotebook = "CCqFvf"
	// Other verified RPCs:
	RPCLoadNotebook = "rLM1Ne" // Load notebook content
	RPCTextProcess  = "tr032e" // Text processing
	RPCMetadata     = "VfAZjd" // Get notebook metadata
	RPCSync         = "cFji9"  // Notebook sync
	RPCInsertNote   = "izAoDd" // Insert content
)

// Call represents a NotebookLM RPC call
type Call struct {
	ID         string        // RPC endpoint ID
	Args       []interface{} // Arguments for the call
	NotebookID string        // Optional notebook ID for context
}

// Client handles NotebookLM RPC communication
type Client struct {
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
		client: batchexecute.NewClient(config, options...),
	}
}

// Do executes a NotebookLM RPC call
func (c *Client) Do(call Call) (json.RawMessage, error) {
	// Update source path if notebook ID is provided
	cfg := c.client.Config()
	if call.NotebookID != "" {
		cfg.URLParams["source-path"] = "/notebook/" + call.NotebookID
	} else {
		cfg.URLParams["source-path"] = "/"
	}
	c.client = batchexecute.NewClient(cfg)

	// Convert to batchexecute RPC
	rpc := batchexecute.RPC{
		ID:    call.ID,
		Args:  call.Args,
		Index: "generic",
	}

	resp, err := c.client.Do(rpc)
	if err != nil {
		return nil, fmt.Errorf("execute rpc: %w", err)
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
		ID: RPCMetadata,
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
