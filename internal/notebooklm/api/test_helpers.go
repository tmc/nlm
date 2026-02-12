// Package api provides test helpers for debugging and development
package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/httprr"
)

// loadNLMCredentials loads credentials from ~/.nlm/env file if environment variables are not set
func loadNLMCredentials() (authToken, cookies string) {
	// First check environment variables
	authToken = os.Getenv("NLM_AUTH_TOKEN")
	cookies = os.Getenv("NLM_COOKIES")

	if authToken != "" && cookies != "" {
		return authToken, cookies
	}

	// Don't load from file if environment variables were explicitly set to empty
	// This allows for intentional skipping of tests
	if os.Getenv("NLM_AUTH_TOKEN") == "" && os.Getenv("NLM_COOKIES") == "" {
		// Check if environment variables were explicitly set (even to empty)
		if _, exists := os.LookupEnv("NLM_AUTH_TOKEN"); exists {
			if _, exists := os.LookupEnv("NLM_COOKIES"); exists {
				return "", "" // Both were explicitly set to empty
			}
		}
	}

	// Try to read from ~/.nlm/env file
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", ""
	}

	envFile := homeDir + "/.nlm/env"
	file, err := os.Open(envFile)
	if err != nil {
		return "", ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "NLM_AUTH_TOKEN=") {
			if authToken == "" { // Only set if not already set by env var
				authToken = strings.Trim(strings.TrimPrefix(line, "NLM_AUTH_TOKEN="), `"`)
			}
		} else if strings.HasPrefix(line, "NLM_COOKIES=") {
			if cookies == "" { // Only set if not already set by env var
				cookies = strings.Trim(strings.TrimPrefix(line, "NLM_COOKIES="), `"`)
			}
		}
	}

	return authToken, cookies
}

// DebugHTTPRecorder is a helper function for debugging HTTP recording issues
// It can be called from tests when needed for troubleshooting
func DebugHTTPRecorder(t *testing.T) {
	t.Helper()

	// Skip if not in debug mode
	if os.Getenv("NLM_DEBUG") != "true" {
		t.Skip("Skipping debug helper (set NLM_DEBUG=true to enable)")
	}

	// Load credentials
	authToken, cookies := loadNLMCredentials()
	if authToken == "" || cookies == "" {
		t.Skip("Skipping debug: credentials not available")
	}

	// Create HTTP client with recording
	httpClient := httprr.CreateNLMTestClient(t, http.DefaultTransport)

	// Create client
	client := New(
		authToken,
		cookies,
		batchexecute.WithHTTPClient(httpClient),
		batchexecute.WithDebug(true),
	)

	// Make a simple API call
	t.Log("Making test API call...")
	projects, err := client.ListRecentlyViewedProjects()
	if err != nil {
		t.Logf("API call failed (expected in replay mode): %v", err)
	} else {
		t.Logf("Found %d projects", len(projects))
	}

	t.Log("HTTP recording debug complete")
}

// DebugDirectRequest is a helper function for debugging direct API requests
// It can be used to test raw batchexecute protocol interactions
func DebugDirectRequest(t *testing.T) {
	t.Helper()

	// Skip if not in debug mode
	if os.Getenv("NLM_DEBUG") != "true" {
		t.Skip("Skipping debug helper (set NLM_DEBUG=true to enable)")
	}

	// Load credentials
	_, cookies := loadNLMCredentials()
	if cookies == "" {
		t.Skip("Skipping debug: cookies not available")
	}

	// Create request
	url := "https://notebooklm.google.com/_/NotebookLmUi/data/batchexecute"
	payload := `f.req=[["wXbhsf","[]",null,"generic"]]`

	req, err := http.NewRequest("POST", url, bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	req.Header.Set("Cookie", cookies)

	// Make request
	t.Log("Making direct request...")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	t.Logf("Response status: %d", resp.StatusCode)
	t.Logf("Response length: %d bytes", len(body))

	// Try to parse response
	if len(body) > 6 && string(body[:6]) == ")]}'\n\n" {
		body = body[6:]
		var parsed interface{}
		if err := json.Unmarshal(body, &parsed); err == nil {
			t.Logf("Parsed response: %+v", parsed)
		}
	}
}

// CreateMockClient creates a client configured for testing with mock responses
func CreateMockClient(t *testing.T) *Client {
	t.Helper()

	// Use httprr for deterministic testing
	httpClient := httprr.CreateNLMTestClient(t, http.DefaultTransport)

	// Use test credentials that will be scrubbed
	return New(
		"test-auth-token",
		"test-cookies",
		batchexecute.WithHTTPClient(httpClient),
		batchexecute.WithDebug(false),
	)
}

// AssertProjectValid validates a project has required fields
func AssertProjectValid(t *testing.T, p *pb.Project) {
	t.Helper()

	if p.ProjectId == "" {
		t.Error("Project has empty ProjectId")
	}
	if p.Title == "" {
		t.Error("Project has empty Title")
	}
	if len(p.ProjectId) != 36 {
		t.Errorf("Project has invalid ProjectId format: %s (expected UUID)", p.ProjectId)
	}
}

// AssertSourceValid validates a source has required fields
func AssertSourceValid(t *testing.T, s *pb.Source) {
	t.Helper()

	if s.SourceId == nil {
		t.Error("Source has nil SourceId")
	}
	if s.Title == "" {
		t.Error("Source has empty Title")
	}
	// Note: Type validation removed as pb.Source doesn't have Type field
}

// GenerateMockResponse creates a mock batchexecute response for testing
func GenerateMockResponse(rpcID string, data interface{}) string {
	jsonData, _ := json.Marshal(data)
	return fmt.Sprintf(`)]}'\n\n[["wrb.fr","%s",%s,null,null,1]]`, rpcID, jsonData)
}

// TestDataPath returns the path to test data files
func TestDataPath(filename string) string {
	return fmt.Sprintf("testdata/%s", filename)
}
