package httprr

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenForNLMTest(t *testing.T) {
	// Clean up any existing test files
	testDataDir := "testdata"
	os.RemoveAll(testDataDir)
	defer os.RemoveAll(testDataDir)

	// Create a test recording file first
	if err := os.MkdirAll(testDataDir, 0755); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(testDataDir, "TestOpenForNLMTest.httprr")
	request := "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"
	response := "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{\"message\": \"test\"}"
	httprContent := fmt.Sprintf("httprr trace v1\n%d %d\n%s%s", len(request), len(response), request, response)
	if err := os.WriteFile(testFile, []byte(httprContent), 0644); err != nil {
		t.Fatal(err)
	}

	rr, err := OpenForNLMTest(t, http.DefaultTransport)
	if err != nil {
		t.Fatal(err)
	}
	defer rr.Close()

	// Verify that NLM-specific scrubbers are configured
	if len(rr.reqScrub) < 3 { // default + 2 NLM scrubbers
		t.Error("Expected at least 3 request scrubbers for NLM configuration")
	}

	if len(rr.respScrub) < 2 { // 2 NLM response scrubbers
		t.Error("Expected at least 2 response scrubbers for NLM configuration")
	}
}

func TestCreateNLMTestClient(t *testing.T) {
	// Clean up any existing test files
	testDataDir := "testdata"
	os.RemoveAll(testDataDir)
	defer os.RemoveAll(testDataDir)

	// Create a test recording file first
	if err := os.MkdirAll(testDataDir, 0755); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(testDataDir, "TestCreateNLMTestClient.httprr")
	request := "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"
	response := "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{\"message\": \"test\"}"
	httprContent := fmt.Sprintf("httprr trace v1\n%d %d\n%s%s", len(request), len(response), request, response)
	if err := os.WriteFile(testFile, []byte(httprContent), 0644); err != nil {
		t.Fatal(err)
	}

	client := CreateNLMTestClient(t, http.DefaultTransport)
	if client == nil {
		t.Fatal("CreateNLMTestClient returned nil")
	}

	if client.Transport == nil {
		t.Error("Client transport is nil")
	}
}

func TestScrubNLMCredentials(t *testing.T) {
	req, err := http.NewRequest("POST", "https://notebooklm.google.com/api", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Set NLM-specific headers
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Cookie", "session=secret-session")
	req.Header.Set("X-Goog-AuthUser", "1")
	req.Header.Set("X-Client-Data", "sensitive-data")
	req.Header.Set("X-Goog-Visitor-Id", "visitor-id")

	if err := scrubNLMCredentials(req); err != nil {
		t.Fatal(err)
	}

	// Check that all sensitive headers were removed/cleared
	sensitiveHeaders := []string{
		"Authorization", "X-Goog-AuthUser", "X-Client-Data", "X-Goog-Visitor-Id",
	}

	for _, header := range sensitiveHeaders {
		if value := req.Header.Get(header); value != "" {
			t.Errorf("Header %s was not removed: %q", header, value)
		}
	}

	// Cookie should be set to empty string for consistent replay
	if value := req.Header.Get("Cookie"); value != "" {
		t.Errorf("Cookie header should be empty, got: %q", value)
	}
}

func TestScrubNLMTimestamps(t *testing.T) {
	bodyContent := `[["createNotebook",["test notebook","1672531200000","2023-01-01T12:00:00.000Z"]]]`
	body := &Body{Data: []byte(bodyContent)}

	req, err := http.NewRequest("POST", "https://notebooklm.google.com/api", body)
	if err != nil {
		t.Fatal(err)
	}

	if err := scrubNLMTimestamps(req); err != nil {
		t.Fatal(err)
	}

	resultBody := string(req.Body.(*Body).Data)
	expectedBody := `[["createNotebook",["test notebook","[TIMESTAMP]","[DATE]"]]]`

	if resultBody != expectedBody {
		t.Errorf("Timestamp scrubbing failed.\nExpected: %s\nGot: %s", expectedBody, resultBody)
	}
}

func TestScrubNLMResponseTimestamps(t *testing.T) {
	responseContent := `{
		"id": "notebook123",
		"creationTime": "2023-01-01T12:00:00.000Z",
		"modificationTime": "1672531200000",
		"lastAccessed": "2023-01-01T13:00:00Z"
	}`

	buf := bytes.NewBufferString(responseContent)
	if err := scrubNLMResponseTimestamps(buf); err != nil {
		t.Fatal(err)
	}

	result := buf.String()

	// Check that timestamps were scrubbed
	if bytes.Contains([]byte(result), []byte("2023-01-01T12:00:00.000Z")) {
		t.Error("ISO timestamp was not scrubbed from response")
	}

	if bytes.Contains([]byte(result), []byte("1672531200000")) {
		t.Error("Unix timestamp was not scrubbed from response")
	}

	if bytes.Contains([]byte(result), []byte("creationTime\":\"2023-01-01T12:00:00.000Z\"")) {
		t.Error("Creation time field was not scrubbed")
	}
}

func TestScrubNLMResponseIDs(t *testing.T) {
	responseContent := `{
		"id": "abcd1234567890abcdef1234",
		"sourceId": "source_abcd1234567890abcdef",
		"sessionId": "session_xyz789abc123def456",
		"requestId": "req_123456789012345678901234"
	}`

	buf := bytes.NewBufferString(responseContent)
	if err := scrubNLMResponseIDs(buf); err != nil {
		t.Fatal(err)
	}

	result := buf.String()

	// Check that IDs were scrubbed
	if bytes.Contains([]byte(result), []byte("abcd1234567890abcdef1234")) {
		t.Error("Notebook ID was not scrubbed from response")
	}

	if bytes.Contains([]byte(result), []byte("source_abcd1234567890abcdef")) {
		t.Error("Source ID was not scrubbed from response")
	}

	// Check that placeholders were inserted
	if !bytes.Contains([]byte(result), []byte("[NOTEBOOK_ID]")) {
		t.Error("NOTEBOOK_ID placeholder was not inserted")
	}

	if !bytes.Contains([]byte(result), []byte("[SOURCE_ID]")) {
		t.Error("SOURCE_ID placeholder was not inserted")
	}
}

func TestNotebookLMRecordMatcher(t *testing.T) {
	// Test with NotebookLM RPC call
	bodyContent := `[["VUsiyb",["test notebook","description"]]]`
	body := &Body{Data: []byte(bodyContent)}

	req, err := http.NewRequest("POST", "https://notebooklm.google.com/api", body)
	if err != nil {
		t.Fatal(err)
	}

	result := NotebookLMRecordMatcher(req)
	if !bytes.Contains([]byte(result), []byte("POST_VUsiyb_")) {
		t.Errorf("RPC matcher should include method and function ID, got: %s", result)
	}

	// Test with non-RPC call
	req2, err := http.NewRequest("GET", "https://notebooklm.google.com/status", nil)
	if err != nil {
		t.Fatal(err)
	}

	result2 := NotebookLMRecordMatcher(req2)
	expected2 := "GET_/status"
	if result2 != expected2 {
		t.Errorf("Non-RPC matcher failed. Expected: %s, Got: %s", expected2, result2)
	}
}

func TestSkipIfNoNLMCredentialsOrRecording(t *testing.T) {
	// This test should not skip because we're not setting the env vars
	// and we're not checking for recordings in a way that would cause a skip

	// Save original env vars
	originalToken := os.Getenv("NLM_AUTH_TOKEN")
	originalCookies := os.Getenv("NLM_COOKIES")
	defer func() {
		if originalToken != "" {
			os.Setenv("NLM_AUTH_TOKEN", originalToken)
		} else {
			os.Unsetenv("NLM_AUTH_TOKEN")
		}
		if originalCookies != "" {
			os.Setenv("NLM_COOKIES", originalCookies)
		} else {
			os.Unsetenv("NLM_COOKIES")
		}
	}()

	// Unset the env vars
	os.Unsetenv("NLM_AUTH_TOKEN")
	os.Unsetenv("NLM_COOKIES")

	// Test that the function would call the underlying skip function
	// We can't actually test skipping without creating a separate test function
	// So we'll just verify the function exists and doesn't panic
	// SkipIfNoNLMCredentialsOrRecording(t) // This would skip the test

	// Instead, test with credentials set
	os.Setenv("NLM_AUTH_TOKEN", "test-token")
	// This should not skip
	// SkipIfNoNLMCredentialsOrRecording(t) // Still can't call this without skipping
}
