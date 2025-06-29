package httprr

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestRecordingTransport(t *testing.T) {
	// Skip in normal testing
	if os.Getenv("TEST_HTTPRR") != "true" {
		t.Skip("Skipping httprr test. Set TEST_HTTPRR=true to run.")
	}

	// Create a temporary directory for test recordings
	testDir, err := os.MkdirTemp("", "httprr-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(testDir)

	// Create the recording transport
	rt := NewRecordingTransport(ModeRecord, testDir, nil)

	// Create a test client with the recording transport
	client := &http.Client{
		Transport: rt,
	}

	// Make a test request
	resp, err := client.Get("https://httpbin.org/get")
	if err != nil {
		t.Fatalf("Failed to make test request: %v", err)
	}
	defer resp.Body.Close()

	// Verify recording files were created
	files, err := filepath.Glob(filepath.Join(testDir, "*.json"))
	if err != nil {
		t.Fatalf("Failed to list recording files: %v", err)
	}
	if len(files) == 0 {
		t.Errorf("No recording files created")
	}

	// Test replay mode
	replayRt := NewRecordingTransport(ModeReplay, testDir, nil)
	replayClient := &http.Client{
		Transport: replayRt,
	}

	// Make the same request again
	replayResp, err := replayClient.Get("https://httpbin.org/get")
	if err != nil {
		t.Fatalf("Failed to make replay request: %v", err)
	}
	defer replayResp.Body.Close()

	// Verify we got the expected response
	if replayResp.StatusCode != resp.StatusCode {
		t.Errorf("Replay response status code didn't match: got %d, want %d",
			replayResp.StatusCode, resp.StatusCode)
	}
}
