package rpc

import (
	"encoding/json"
	"fmt"

	"github.com/davecgh/go-spew/spew"
	"github.com/tmc/nlm/internal/batchexecute"
)

// ServiceConfig defines configuration for a specific BatchExecute service
type ServiceConfig struct {
	Host      string            // e.g., "notebooklm.google.com" or "jules.google.com"
	App       string            // e.g., "LabsTailwindUi" or "Swebot"
	URLParams map[string]string // Optional service-specific URL parameters
}

// Call represents a generic RPC call
type Call struct {
	ID   string        // RPC endpoint ID
	Args []interface{} // Arguments for the call
}

// Client handles BatchExecute RPC communication for any service
type Client struct {
	Config batchexecute.Config
	client *batchexecute.Client
}

// New creates a new RPC client with default NotebookLM configuration
// Deprecated: Use NewWithConfig for new services
func New(authToken, cookies string, options ...batchexecute.Option) *Client {
	config := ServiceConfig{
		Host: "notebooklm.google.com",
		App:  "LabsTailwindUi",
	}
	return NewWithConfig(authToken, cookies, config, options...)
}

// NewWithConfig creates a new RPC client with custom service configuration
func NewWithConfig(authToken, cookies string, serviceConfig ServiceConfig, options ...batchexecute.Option) *Client {
	config := batchexecute.Config{
		Host:      serviceConfig.Host,
		App:       serviceConfig.App,
		AuthToken: authToken,
		Cookies:   cookies,
		Headers: map[string]string{
			"content-type":                "application/x-www-form-urlencoded;charset=UTF-8",
			"origin":                      fmt.Sprintf("https://%s", serviceConfig.Host),
			"referer":                     fmt.Sprintf("https://%s/", serviceConfig.Host),
			"x-same-domain":               "1",
			"accept":                      "*/*",
			"accept-language":             "en-US,en;q=0.6",
			"cache-control":               "no-cache",
			"pragma":                      "no-cache",
			"priority":                    "u=1, i",
			"sec-ch-ua":                   `"Chromium";v="142", "Brave";v="142", "Not_A Brand";v="99"`,
			"sec-ch-ua-arch":              `"arm"`,
			"sec-ch-ua-bitness":           `"64"`,
			"sec-ch-ua-full-version-list": `"Chromium";v="142.0.0.0", "Brave";v="142.0.0.0", "Not_A Brand";v="99.0.0.0"`,
			"sec-ch-ua-mobile":            "?0",
			"sec-ch-ua-model":             `""`,
			"sec-ch-ua-platform":          `"macOS"`,
			"sec-ch-ua-platform-version":  `"26.1.0"`,
			"sec-ch-ua-wow64":             "?0",
			"sec-fetch-dest":              "empty",
			"sec-fetch-mode":              "cors",
			"sec-fetch-site":              "same-origin",
			"sec-gpc":                     "1",
			"user-agent":                  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36",
		},
		URLParams: serviceConfig.URLParams,
	}

	return &Client{
		Config: config,
		client: batchexecute.NewClient(config, options...),
	}
}

// Do executes an RPC call
func (c *Client) Do(call Call) (json.RawMessage, error) {
	if c.Config.Debug {
		fmt.Printf("\n=== RPC Call ===\n")
		fmt.Printf("ID: %s\n", call.ID)
		fmt.Printf("Args:\n")
		spew.Dump(call.Args)
	}

	// Create the RPC struct for batchexecute
	rpc := batchexecute.RPC{
		ID:    call.ID,
		Args:  call.Args,
		Index: "generic",
	}

	// Execute the batchexecute request
	resp, err := c.client.Execute([]batchexecute.RPC{rpc})
	if err != nil {
		return nil, err
	}

	return resp.Data, nil
}
