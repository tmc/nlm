package api

import (
	"net/http"
	"testing"

	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/httprr"
)

// TestCaptureAudioCreateRequest captures the HTTP request for audio overview creation
// Run with: go test -run TestCaptureAudioCreateRequest -httprecord=. -v
// Export with: go test -run TestCaptureAudioCreateRequest -export-txtar -v
func TestCaptureAudioCreateRequest(t *testing.T) {
	// Load credentials
	authToken, cookies := loadNLMCredentials()
	if authToken == "" || cookies == "" {
		t.Skip("Skipping: NLM credentials not available")
	}

	// Create HTTP client with recording
	httpClient := httprr.CreateNLMTestClient(t, http.DefaultTransport)

	// Create API client
	client := New(
		authToken,
		cookies,
		batchexecute.WithHTTPClient(httpClient),
		batchexecute.WithDebug(true),
	)

	// Use a test notebook ID (or create one)
	testProjectID := "8826219b-1c40-4513-a2ab-affc3f2b56eb"

	// Try to create audio overview
	t.Logf("Creating audio overview for project: %s", testProjectID)
	result, err := client.CreateAudioOverview(testProjectID, "test audio instructions")
	if err != nil {
		t.Logf("Audio creation error (may be expected): %v", err)
		// Don't fail - we want to capture the request even if it errors
	}

	if result != nil {
		t.Logf("Audio result: %+v", result)
	}
}
