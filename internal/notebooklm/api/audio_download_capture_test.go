package api

import (
	"net/http"
	"testing"

	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/httprr"
)

func TestCaptureAudioDownloadRequest(t *testing.T) {
	authToken, cookies := loadNLMCredentials()
	if authToken == "" || cookies == "" {
		t.Skip("Skipping: NLM credentials not available")
	}

	httpClient := httprr.CreateNLMTestClient(t, http.DefaultTransport)

	client := New(
		authToken,
		cookies,
		batchexecute.WithHTTPClient(httpClient),
		batchexecute.WithDebug(true),
	)
	client.SetUseDirectRPC(true)

	// Use the test notebook that has audio ready
	testProjectID := "c2a56c84-ec7b-4415-a550-849c887f28f3"

	// Try each request type to see which one returns audio data
	for requestType := 0; requestType <= 5; requestType++ {
		t.Run(string(rune('0'+requestType)), func(t *testing.T) {
			t.Logf("Trying GetAudioOverview with request_type=%d for project: %s", requestType, testProjectID)
			result, err := client.getAudioOverviewDirectRPCWithType(testProjectID, requestType)
			if err != nil {
				t.Logf("Request type %d error: %v", requestType, err)
				return
			}

			t.Logf("Request type %d result:", requestType)
			t.Logf("  IsReady: %v", result.IsReady)
			t.Logf("  AudioData length: %d", len(result.AudioData))
			if result.AudioData != "" {
				t.Logf("  Found audio data with request_type=%d!", requestType)
			}
		})
	}
}
