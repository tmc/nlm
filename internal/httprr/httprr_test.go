package httprr

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenForTest(t *testing.T) {
	// Clean up any existing test files
	testDataDir := "testdata"
	testFile := filepath.Join(testDataDir, "TestOpenForTest.httprr")
	os.RemoveAll(testDataDir)

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message": "test response"}`))
	}))
	defer server.Close()

	// Test recording mode (simulated by creating the file first)
	if err := os.MkdirAll(testDataDir, 0755); err != nil {
		t.Fatal(err)
	}

	// For this test, we'll test replay mode by creating a simple httprr file
	request := "GET / HTTP/1.1\r\nHost: example.com\r\nUser-Agent: Go-http-client/1.1\r\n\r\n"
	response := "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{\"message\": \"test response\"}"
	httprContent := fmt.Sprintf("httprr trace v1\n%d %d\n%s%s", len(request), len(response), request, response)

	if err := os.WriteFile(testFile, []byte(httprContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Test OpenForTest in replay mode
	rr, err := OpenForTest(t, http.DefaultTransport)
	if err != nil {
		t.Fatal(err)
	}
	defer rr.Close()

	if rr.Recording() {
		t.Error("Expected replay mode, got recording mode")
	}

	// Test the client
	client := rr.Client()
	if client == nil {
		t.Fatal("Client() returned nil")
	}

	// Clean up
	os.RemoveAll(testDataDir)
}

func TestSkipIfNoCredentialsOrRecording(t *testing.T) {
	// Test the helper functions for credential checking
	testEnvVar := "HTTPRR_TEST_CREDENTIAL"

	// Save original value
	originalEnv := os.Getenv(testEnvVar)
	defer func() {
		if originalEnv != "" {
			os.Setenv(testEnvVar, originalEnv)
		} else {
			os.Unsetenv(testEnvVar)
		}
	}()

	// Test when env var is not set
	os.Unsetenv(testEnvVar)
	if hasRequiredCredentials([]string{testEnvVar}) {
		t.Error("hasRequiredCredentials should return false when env var is not set")
	}

	// Test when env var is set
	os.Setenv(testEnvVar, "test-value")
	if !hasRequiredCredentials([]string{testEnvVar}) {
		t.Error("hasRequiredCredentials should return true when env var is set")
	}
}

func TestCleanFileName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"TestSimple", "TestSimple"},
		{"Test/With/Slashes", "Test-With-Slashes"},
		{"Test With Spaces", "Test-With-Spaces"},
		{"Test_With_Underscores", "Test_With_Underscores"},
		{"Test.With.Dots", "Test.With.Dots"},
		{"Test-With-Hyphens", "Test-With-Hyphens"},
		{"Test()[]{}Special", "Test------Special"},
	}

	for _, test := range tests {
		result := CleanFileName(test.input)
		if result != test.expected {
			t.Errorf("CleanFileName(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestDefaultScrubbers(t *testing.T) {
	req, err := http.NewRequest("GET", "http://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Set sensitive headers
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Cookie", "session=secret-session")

	// Apply default request scrubbers
	scrubbers := defaultRequestScrubbers()
	for _, scrub := range scrubbers {
		if err := scrub(req); err != nil {
			t.Fatal(err)
		}
	}

	// Check that sensitive headers were redacted
	if auth := req.Header.Get("Authorization"); auth != "[REDACTED]" {
		t.Errorf("Authorization header was not redacted: %q", auth)
	}

	if cookie := req.Header.Get("Cookie"); cookie != "[REDACTED]" {
		t.Errorf("Cookie header was not redacted: %q", cookie)
	}
}

func TestBodyReadWrite(t *testing.T) {
	data := []byte("test body content")
	body := &Body{Data: data}

	// Test reading
	buf := make([]byte, 5)
	n, err := body.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("Expected to read 5 bytes, got %d", n)
	}
	if string(buf) != "test " {
		t.Errorf("Expected 'test ', got %q", string(buf))
	}

	// Read the rest
	buf = make([]byte, 20)
	n, err = body.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(data)-5 {
		t.Errorf("Expected to read %d bytes, got %d", len(data)-5, n)
	}
	if string(buf[:n]) != "body content" {
		t.Errorf("Expected 'body content', got %q", string(buf[:n]))
	}

	// EOF test
	_, err = body.Read(buf)
	if err == nil {
		t.Error("Expected EOF error")
	}

	// Test Close
	if err := body.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

func TestRecordingFunction(t *testing.T) {
	// Test when record flag is not set
	if recording, err := Recording("test.httprr"); err != nil {
		t.Fatal(err)
	} else if recording {
		t.Error("Recording should return false when flag is not set")
	}

	// Test invalid regex (would need to mock the flag value)
	// This is harder to test without modifying global state
}

func TestRecordingTransportLegacy(t *testing.T) {
	// Create testdata directory if it doesn't exist
	if err := os.MkdirAll("testdata", 0755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	// Create a minimal httprr file for testing
	testFile := "testdata/test.httprr"
	request := "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"
	response := "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\ntest"
	httprContent := fmt.Sprintf("httprr trace v1\n%d %d\n%s%s", len(request), len(response), request, response)
	if err := os.WriteFile(testFile, []byte(httprContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Test creating a recording client
	client, err := NewRecordingClient(testFile, nil)
	if err != nil {
		t.Fatal(err)
	}

	if client == nil {
		t.Error("NewRecordingClient returned nil")
	}
}
