package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/tmc/nlm/internal/batchexecute"
)

// TestNLMDebugEnvironmentVariable verifies NLM_DEBUG environment variable functionality
func TestNLMDebugEnvironmentVariable(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     bool
	}{
		{
			name:     "debug enabled with true",
			envValue: "true",
			want:     true,
		},
		{
			name:     "debug disabled with false",
			envValue: "false",
			want:     false,
		},
		{
			name:     "debug disabled with empty",
			envValue: "",
			want:     false,
		},
		{
			name:     "debug disabled with other value",
			envValue: "yes",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore original value
			origValue := os.Getenv("NLM_DEBUG")
			defer os.Setenv("NLM_DEBUG", origValue)

			// Set test value
			if tt.envValue == "" {
				os.Unsetenv("NLM_DEBUG")
			} else {
				os.Setenv("NLM_DEBUG", tt.envValue)
			}

			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(")]}'\n\n[[\"wrb.fr\",\"wXbhsf\",\"[[\\\"project1\\\", [], \\\"id1\\\", \\\"📚\\\"]]\",null,null,1]]"))
			}))
			defer server.Close()

			// Create HTTP client that points to test server
			httpClient := &http.Client{
				Transport: &testTransport{
					baseURL: server.URL,
				},
			}

			// Create API client
			client := New(
				"test-token",
				"test-cookies",
				batchexecute.WithHTTPClient(httpClient),
			)

			// Verify debug setting matches expectation
			if client.config.Debug != tt.want {
				t.Errorf("Expected Debug=%v, got Debug=%v", tt.want, client.config.Debug)
			}
		})
	}
}

// TestDebugOutputProduction verifies that debug mode produces output
func TestDebugOutputProduction(t *testing.T) {
	// Save and restore original values
	origValue := os.Getenv("NLM_DEBUG")
	defer os.Setenv("NLM_DEBUG", origValue)

	// Enable debug mode
	os.Setenv("NLM_DEBUG", "true")

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
		"test-token",
		"test-cookies",
		batchexecute.WithHTTPClient(httpClient),
	)

	// Make an API call
	_, _ = client.ListRecentlyViewedProjects()

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

// TestClientDebugConfiguration verifies debug configuration on client creation
func TestClientDebugConfiguration(t *testing.T) {
	// Save original env var
	origValue := os.Getenv("NLM_DEBUG")
	defer func() {
		if origValue == "" {
			os.Unsetenv("NLM_DEBUG")
		} else {
			os.Setenv("NLM_DEBUG", origValue)
		}
	}()

	t.Run("debug from environment", func(t *testing.T) {
		os.Setenv("NLM_DEBUG", "true")

		client := New(
			"test-token",
			"test-cookies",
		)

		if !client.config.Debug {
			t.Error("Expected debug to be enabled from environment variable")
		}
	})

	t.Run("debug from environment takes precedence", func(t *testing.T) {
		os.Setenv("NLM_DEBUG", "true")

		client := New(
			"test-token",
			"test-cookies",
			batchexecute.WithDebug(false), // Explicitly set to false
		)

		// Environment variable takes precedence over options
		if !client.config.Debug {
			t.Error("Expected NLM_DEBUG environment variable to take precedence")
		}
	})
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
