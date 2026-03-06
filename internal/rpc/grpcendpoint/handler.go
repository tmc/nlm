// Package grpcendpoint handles gRPC-style endpoints for NotebookLM
package grpcendpoint

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
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
		debug:      os.Getenv("NLM_DEBUG") == "true",
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
	bl := os.Getenv("NLM_BL")
	if bl == "" {
		bl = "boq_labs-tailwind-frontend_20260127.09_p1"
	}
	fsid := os.Getenv("NLM_F_SID")
	if fsid == "" {
		fsid = "3894541541181659848"
	}
	hl := os.Getenv("NLM_HL")
	if hl == "" {
		hl = "en"
	}
	params.Set("bl", bl)
	params.Set("f.sid", fsid)
	params.Set("hl", hl)
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
	bl := os.Getenv("NLM_BL")
	if bl == "" {
		bl = "boq_labs-tailwind-frontend_20260127.09_p1"
	}
	fsid := os.Getenv("NLM_F_SID")
	if fsid == "" {
		fsid = "3894541541181659848"
	}
	hl := os.Getenv("NLM_HL")
	if hl == "" {
		hl = "en"
	}
	params.Set("bl", bl)
	params.Set("f.sid", fsid)
	params.Set("hl", hl)
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
func BuildChatRequest(sourceIDs []string, prompt string) interface{} {
	// Build the array of source IDs
	sources := make([][]string, len(sourceIDs))
	for i, id := range sourceIDs {
		sources[i] = []string{id}
	}

	// Return the formatted request
	// Format: [null, "[[sources], prompt, null, [2]]"]
	innerArray := []interface{}{
		sources,
		prompt,
		nil,
		[]int{2},
	}

	// Marshal the inner array to JSON string
	innerJSON, _ := json.Marshal(innerArray)

	return []interface{}{
		nil,
		string(innerJSON),
	}
}
