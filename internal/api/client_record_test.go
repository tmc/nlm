package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/httprr"
)

// TestListProjectsWithRecording tests the ListRecentlyViewedProjects method
// with request recording and replay.
func TestListProjectsWithRecording(t *testing.T) {
	// Skip in normal testing
	if os.Getenv("RECORD_LIST_PROJECTS") != "true" && os.Getenv("REPLAY_LIST_PROJECTS") != "true" {
		t.Skip("Skipping recording test. Set RECORD_LIST_PROJECTS=true or REPLAY_LIST_PROJECTS=true to run.")
	}

	// Get credentials from environment
	authToken := os.Getenv("NLM_AUTH_TOKEN")
	cookies := os.Getenv("NLM_COOKIES")
	if authToken == "" || cookies == "" {
		t.Fatalf("Missing credentials. Set NLM_AUTH_TOKEN and NLM_COOKIES environment variables.")
	}

	// Determine testing mode (record or replay)
	mode := httprr.ModePassthrough
	if os.Getenv("RECORD_LIST_PROJECTS") == "true" {
		mode = httprr.ModeRecord
		t.Log("Recording mode enabled")
	} else if os.Getenv("REPLAY_LIST_PROJECTS") == "true" {
		mode = httprr.ModeReplay
		t.Log("Replay mode enabled")
	}

	// Create recordings directory
	recordingsDir := filepath.Join("testdata", "recordings")
	if mode == httprr.ModeRecord {
		os.MkdirAll(recordingsDir, 0755)
	}

	// Create HTTP client with recording transport
	httpClient := httprr.NewRecordingClient(mode, recordingsDir, nil)

	// Create API client
	client := New(authToken, cookies, 
		batchexecute.WithHTTPClient(httpClient), 
		batchexecute.WithDebug(true))

	// Call the API method
	t.Log("Listing projects...")
	projects, err := client.ListRecentlyViewedProjects()
	if err != nil {
		t.Fatalf("Failed to list projects: %v", err)
	}

	// Check results
	t.Logf("Found %d projects", len(projects))
	for i, p := range projects {
		t.Logf("Project %d: %s (%s)", i, p.Title, p.ProjectId)
	}

	if len(projects) == 0 {
		t.Logf("Warning: No projects found")
	}
}

// TestWithRecordingServer tests using a recording server for API calls
func TestWithRecordingServer(t *testing.T) {
	// Skip in normal testing
	if os.Getenv("TEST_RECORDING_SERVER") != "true" {
		t.Skip("Skipping recording server test. Set TEST_RECORDING_SERVER=true to run.")
	}

	recordingsDir := filepath.Join("testdata", "recordings")
	if _, err := os.Stat(recordingsDir); os.IsNotExist(err) {
		t.Skipf("No recordings found in %s. Run with RECORD_LIST_PROJECTS=true first", recordingsDir)
	}

	// Create a test server that serves recorded responses
	server := httprr.NewTestServer(recordingsDir)
	defer server.Close()

	// Create API client that points to the test server
	client := New("test-token", "test-cookie", 
		batchexecute.WithDebug(true),
	)

	// Call the API method
	t.Log("Listing projects from test server...")
	projects, err := client.ListRecentlyViewedProjects()
	if err != nil {
		t.Fatalf("Failed to list projects from test server: %v", err)
	}

	// Check results
	t.Logf("Found %d projects from test server", len(projects))
	for i, p := range projects {
		t.Logf("Project %d: %s (%s)", i, p.Title, p.ProjectId)
	}
}