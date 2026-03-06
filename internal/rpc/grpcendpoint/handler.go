// Package grpcendpoint handles gRPC-style endpoints for NotebookLM
package grpcendpoint

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/tmc/nlm/internal/rpc"
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

	// Get API parameters dynamically
	apiParams := rpc.GetAPIParams(c.cookies)

	// Add query parameters
	params := url.Values{}
	params.Set("bl", apiParams.BuildVersion)
	params.Set("f.sid", apiParams.SessionID)
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
		fmt.Printf("=== gRPC Endpoint Request ===\n")
		fmt.Printf("URL: %s\n", fullURL)
		fmt.Printf("f.req (raw JSON): %s\n", string(bodyJSON))
		fmt.Printf("Body (URL-encoded): %s\n", formData.Encode())
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

	// Strip the )]}' prefix that Google adds to prevent JSON hijacking
	bodyStr := string(body)
	if strings.HasPrefix(bodyStr, ")]}'") {
		bodyStr = strings.TrimPrefix(bodyStr, ")]}'")
		bodyStr = strings.TrimLeft(bodyStr, "\n")
	}

	// Response is in chunked format: <length>\n<json>\n<length>\n<json>...
	// Extract the first JSON chunk which contains the actual response
	lines := strings.SplitN(bodyStr, "\n", 3)
	if len(lines) >= 2 {
		// First line is the length, second line is the JSON
		bodyStr = lines[1]
	}

	// Parse the batchexecute response format: [["wrb.fr",null,"<json_data>",...]]]
	// We need to extract the json_data (third element)
	var outerArray [][]interface{}
	if err := json.Unmarshal([]byte(bodyStr), &outerArray); err != nil {
		return nil, fmt.Errorf("parse outer response: %w", err)
	}

	if len(outerArray) == 0 || len(outerArray[0]) < 3 {
		return nil, fmt.Errorf("invalid response format: expected [['wrb.fr',null,'data',...]]")
	}

	// The third element (index 2) contains the JSON string we need
	dataStr, ok := outerArray[0][2].(string)
	if !ok {
		return nil, fmt.Errorf("invalid response data type: expected string")
	}

	if c.debug {
		fmt.Printf("=== gRPC Endpoint Response ===\n")
		fmt.Printf("Extracted data: %s\n", dataStr[:min(300, len(dataStr))])
	}

	return []byte(dataStr), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// StreamResponse handles streaming responses from gRPC endpoints
func (c *Client) Stream(req Request, handler func(chunk []byte) error) error {
	baseURL := "https://notebooklm.google.com/_/LabsTailwindUi/data"
	fullURL := baseURL + req.Endpoint

	// Get API parameters dynamically
	apiParams := rpc.GetAPIParams(c.cookies)

	// Add query parameters
	params := url.Values{}
	params.Set("bl", apiParams.BuildVersion)
	params.Set("f.sid", apiParams.SessionID)
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
// Browser format: [null,"[[[[\"source_id\"]]],\"prompt\",null,[2,null,[1]]]"]
func BuildChatRequest(sourceIDs []string, prompt string) interface{} {
	// Build the nested source IDs array with 3 levels of wrapping
	// innerArray adds 1 level, so we need 3 wraps to get 4 levels total
	// Format: [[[source_id1, source_id2, ...]]]
	var sourceIDsInner []interface{}
	for _, id := range sourceIDs {
		sourceIDsInner = append(sourceIDsInner, id)
	}
	// 3 wraps: [[[ids]]] -> becomes [[[[ids]]]] in innerArray
	sourceIDsNested := []interface{}{[]interface{}{sourceIDsInner}}

	// Build the inner array: [[[[sources]]], prompt, null, [2,null,[1]]]
	innerArray := []interface{}{
		sourceIDsNested,
		prompt,
		nil,
		[]interface{}{2, nil, []interface{}{1}},
	}

	// Marshal the inner array to JSON string
	innerJSON, _ := json.Marshal(innerArray)

	// Final format: [null, "inner_json_string"]
	return []interface{}{
		nil,
		string(innerJSON),
	}
}
