// Package grpcendpoint handles gRPC-style endpoints for NotebookLM
package grpcendpoint

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Client handles gRPC-style endpoint requests
type Client struct {
	authToken  string
	cookies    string
	httpClient *http.Client
	debug      bool
}

// NewClient creates a new gRPC endpoint client
func NewClient(authToken, cookies string) *Client {
	return &Client{
		authToken:  authToken,
		cookies:    cookies,
		httpClient: &http.Client{},
	}
}

// Request represents a gRPC-style request
type Request struct {
	Endpoint string      // e.g., "/google.internal.labs.tailwind.orchestration.v1.LabsTailwindOrchestrationService/GenerateFreeFormStreamed"
	Body     interface{} // The request body (will be JSON encoded)
}

// Execute sends a gRPC-style request to NotebookLM
func (c *Client) Execute(req Request) ([]byte, error) {
	baseURL := "https://notebooklm.google.com/_/LabsTailwindUi/data"

	// Build the full URL with the endpoint
	fullURL := baseURL + req.Endpoint

	// Add query parameters
	params := url.Values{}
	params.Set("bl", "boq_labs-tailwind-frontend_20250903.07_p0")
	params.Set("f.sid", "-2216531235646590877") // This may need to be dynamic
	params.Set("hl", "en")
	params.Set("_reqid", fmt.Sprintf("%d", generateRequestID()))
	params.Set("rt", "c")

	fullURL = fullURL + "?" + params.Encode()

	// Encode the request body
	bodyJSON, err := json.Marshal(req.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to encode request body: %w", err)
	}

	// Create form data
	formData := url.Values{}
	formData.Set("f.req", string(bodyJSON))
	formData.Set("at", c.authToken)

	// Create the HTTP request
	httpReq, err := http.NewRequest("POST", fullURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	httpReq.Header.Set("Cookie", c.cookies)
	httpReq.Header.Set("Origin", "https://notebooklm.google.com")
	httpReq.Header.Set("Referer", "https://notebooklm.google.com/")
	httpReq.Header.Set("X-Same-Domain", "1")
	httpReq.Header.Set("Accept", "*/*")
	httpReq.Header.Set("Accept-Language", "en-US,en;q=0.9")

	if c.debug {
		fmt.Printf("=== gRPC Request ===\n")
		fmt.Printf("URL: %s\n", fullURL)
		fmt.Printf("Body: %s\n", formData.Encode())
	}

	// Send the request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if c.debug {
		fmt.Printf("=== gRPC Response ===\n")
		fmt.Printf("Status: %s\n", resp.Status)
		fmt.Printf("Body: %s\n", string(body))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// StreamResponse handles streaming responses from gRPC endpoints
func (c *Client) Stream(req Request, handler func(chunk []byte) error) error {
	baseURL := "https://notebooklm.google.com/_/LabsTailwindUi/data"
	fullURL := baseURL + req.Endpoint

	// Add query parameters
	params := url.Values{}
	params.Set("bl", "boq_labs-tailwind-frontend_20250903.07_p0")
	params.Set("f.sid", "-2216531235646590877")
	params.Set("hl", "en")
	params.Set("_reqid", fmt.Sprintf("%d", generateRequestID()))
	params.Set("rt", "c")

	fullURL = fullURL + "?" + params.Encode()

	// Encode the request body
	bodyJSON, err := json.Marshal(req.Body)
	if err != nil {
		return fmt.Errorf("failed to encode request body: %w", err)
	}

	// Create form data
	formData := url.Values{}
	formData.Set("f.req", string(bodyJSON))
	formData.Set("at", c.authToken)

	// Create the HTTP request
	httpReq, err := http.NewRequest("POST", fullURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	httpReq.Header.Set("Cookie", c.cookies)
	httpReq.Header.Set("Origin", "https://notebooklm.google.com")
	httpReq.Header.Set("Referer", "https://notebooklm.google.com/")
	httpReq.Header.Set("X-Same-Domain", "1")

	// Send the request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Read the streaming response
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if err := handler(buf[:n]); err != nil {
				return fmt.Errorf("handler error: %w", err)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read error: %w", err)
		}
	}

	return nil
}

// Helper to generate request IDs
var requestCounter int

func generateRequestID() int {
	requestCounter++
	return 1000000 + requestCounter
}

// BuildChatRequest builds a request for the GenerateFreeFormStreamed endpoint
// Captured format from browser (2026-02-01):
// [null, "<inner_json>"] where inner_json is:
// [
//   [[[source_id_1]], [[source_id_2]]],  // [0] Sources - each wrapped as [[id]]
//   "prompt",                            // [1] User question
//   [],                                  // [2] Chat history (empty for first message)
//   [2, null, [1], [1]],                 // [3] Config options
//   "session_uuid",                      // [4] Chat session ID
//   null,                                // [5]
//   null,                                // [6]
//   "project_id",                        // [7] Notebook/project ID
//   1                                    // [8] Flag
// ]
func BuildChatRequest(sourceIDs []string, prompt string) interface{} {
	// Build source array where each source is wrapped as [[id]]
	sources := make([]interface{}, len(sourceIDs))
	for i, id := range sourceIDs {
		sources[i] = []interface{}{[]interface{}{id}}
	}

	// Generate a session UUID
	sessionID := generateUUID()

	// Config options: [2, null, [1], [1]]
	config := []interface{}{2, nil, []interface{}{1}, []interface{}{1}}

	// Note: For the gRPC endpoint, we don't have the project ID in the function signature
	// This needs to be passed in or extracted from a different source
	// For now, we'll leave it empty - the caller should use BuildChatRequestWithProject
	innerArray := []interface{}{
		sources,         // [0] Sources as [[[id1]], [[id2]], ...]
		prompt,          // [1] The prompt/question
		[]interface{}{}, // [2] Chat history (empty)
		config,          // [3] Config options
		sessionID,       // [4] Chat session UUID
		nil,             // [5] null
		nil,             // [6] null
		"",              // [7] Project ID - needs to be set by caller
		1,               // [8] Flag
	}

	// Marshal the inner array to JSON string
	innerJSON, _ := json.Marshal(innerArray)

	return []interface{}{
		nil,
		string(innerJSON),
	}
}

// BuildChatRequestWithProject builds a request for the GenerateFreeFormStreamed endpoint with project ID
func BuildChatRequestWithProject(sourceIDs []string, prompt string, projectID string) interface{} {
	// Build source array where each source is wrapped as [[id]]
	sources := make([]interface{}, len(sourceIDs))
	for i, id := range sourceIDs {
		sources[i] = []interface{}{[]interface{}{id}}
	}

	// Generate a session UUID
	sessionID := generateUUID()

	// Config options: [2, null, [1], [1]]
	config := []interface{}{2, nil, []interface{}{1}, []interface{}{1}}

	innerArray := []interface{}{
		sources,         // [0] Sources as [[[id1]], [[id2]], ...]
		prompt,          // [1] The prompt/question
		[]interface{}{}, // [2] Chat history (empty)
		config,          // [3] Config options
		sessionID,       // [4] Chat session UUID
		nil,             // [5] null
		nil,             // [6] null
		projectID,       // [7] Project/notebook ID
		1,               // [8] Flag
	}

	// Marshal the inner array to JSON string
	innerJSON, _ := json.Marshal(innerArray)

	return []interface{}{
		nil,
		string(innerJSON),
	}
}

// generateUUID generates a simple UUID v4
func generateUUID() string {
	// Simple UUID generation - in production use github.com/google/uuid
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(requestCounter + i)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
