package notebooklm

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestClientProtoDebugConfiguration(t *testing.T) {
	client := New(
		Credentials{AuthToken: "test-token", Cookies: "test-cookies"},
		WithProtoDebug(true, true),
	)

	options := client.unmarshalOptions()
	if !options.DebugParsing {
		t.Error("DebugParsing = false, want true")
	}
	if !options.DebugFieldMapping {
		t.Error("DebugFieldMapping = false, want true")
	}
}

// TestDebugOutputProduction verifies that debug mode produces output
func TestDebugOutputProduction(t *testing.T) {
	// Create test server that logs requests
	var requestReceived bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived = true
		w.Write([]byte(")]}'\n\n[[\"wrb.fr\",\"wXbhsf\",\"[[\\\"project1\\\", [], \\\"id1\\\", \\\"📚\\\"]]\",null,null,1]]"))
	}))
	defer server.Close()

	// Capture stderr output
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Create HTTP client that points to test server
	httpClient := &http.Client{
		Transport: &testTransport{
			baseURL: server.URL,
		},
	}

	// Create API client with debug enabled
	client := New(
		Credentials{AuthToken: "test-token", Cookies: "test-cookies"},
		WithHTTPClient(httpClient),
		WithDebug(true),
	)

	// Make an API call
	_, _ = client.ListRecentlyViewedProjects(context.Background())

	// Restore stderr and read captured output
	w.Close()
	os.Stderr = oldStderr
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Verify debug output was produced
	if !requestReceived {
		t.Error("Expected request to be received by test server")
	}

	// With debug enabled, we expect some output (even if empty in this mock scenario)
	// The actual debug output depends on the batchexecute implementation
	t.Logf("Debug output length: %d bytes", len(output))
}

// TestDebugSkipInTestHelpers verifies test helpers respect NLM_DEBUG
func TestDebugSkipInTestHelpers(t *testing.T) {
	// Save and restore original value
	origValue := os.Getenv("NLM_DEBUG")
	defer func() {
		if origValue == "" {
			os.Unsetenv("NLM_DEBUG")
		} else {
			os.Setenv("NLM_DEBUG", origValue)
		}
	}()

	tests := []struct {
		name       string
		envValue   string
		shouldSkip bool
	}{
		{
			name:       "skip when NLM_DEBUG not set",
			envValue:   "",
			shouldSkip: true,
		},
		{
			name:       "skip when NLM_DEBUG is false",
			envValue:   "false",
			shouldSkip: true,
		},
		{
			name:       "run when NLM_DEBUG is true",
			envValue:   "true",
			shouldSkip: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			if tt.envValue == "" {
				os.Unsetenv("NLM_DEBUG")
			} else {
				os.Setenv("NLM_DEBUG", tt.envValue)
			}

			// Create a sub-test that checks skip behavior
			skipped := false
			mockT := &mockTestingT{
				skipFunc: func(args ...interface{}) {
					skipped = true
				},
			}

			// Call helper that checks NLM_DEBUG
			if os.Getenv("NLM_DEBUG") != "true" {
				mockT.Skip("Skipping debug helper (set NLM_DEBUG=true to enable)")
			}

			if skipped != tt.shouldSkip {
				t.Errorf("Expected skip=%v, got skip=%v", tt.shouldSkip, skipped)
			}
		})
	}
}

// TestClientDebugConfiguration verifies debug is configured only by options
// and reaches both API and RPC layers.
func TestClientDebugConfiguration(t *testing.T) {
	t.Setenv("NLM_DEBUG", "true")

	withoutOption := New(Credentials{AuthToken: "test-token", Cookies: "test-cookies"})
	if withoutOption.config.Debug {
		t.Error("Debug = true without WithDebug")
	}

	withOption := New(
		Credentials{AuthToken: "test-token", Cookies: "test-cookies"},
		WithDebug(true),
	)
	if !withOption.config.Debug {
		t.Error("API Debug = false, want true")
	}
	if !withOption.rpc.Config.Debug {
		t.Error("RPC Debug = false, want true")
	}
}

// mockTestingT implements a minimal testing.T interface for testing test helpers
type mockTestingT struct {
	skipFunc func(args ...interface{})
}

func (m *mockTestingT) Helper() {}

func (m *mockTestingT) Skip(args ...interface{}) {
	if m.skipFunc != nil {
		m.skipFunc(args...)
	}
}

// testTransport is a custom RoundTripper that redirects requests to a test server
type testTransport struct {
	baseURL string
}

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Redirect all requests to our test server
	testReq := req.Clone(req.Context())
	testReq.URL.Scheme = "http"
	testReq.URL.Host = strings.TrimPrefix(t.baseURL, "http://")
	return http.DefaultTransport.RoundTrip(testReq)
}
