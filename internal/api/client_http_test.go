package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/rpc"
)

// TestHTTPRecorder creates a proxy server that records all HTTP traffic
// to files for inspection. This is not an automated test but a helper
// for debugging HTTP issues.
func TestHTTPRecorder(t *testing.T) {
	// Skip in normal testing
	if os.Getenv("RECORD_HTTP") != "true" {
		t.Skip("Skipping HTTP recorder test. Set RECORD_HTTP=true to run.")
	}

	// Create a temporary directory for storing request/response data
	recordDir := filepath.Join(os.TempDir(), "nlm-http-records")
	err := os.MkdirAll(recordDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create record directory: %v", err)
	}
	t.Logf("Recording HTTP traffic to: %s", recordDir)

	// Set up a proxy server to record all HTTP traffic
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Record the request
		timestamp := time.Now().Format("20060102-150405.000")
		filename := filepath.Join(recordDir, fmt.Sprintf("%s-request.txt", timestamp))

		reqFile, err := os.Create(filename)
		if err != nil {
			t.Logf("Failed to create request file: %v", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		defer reqFile.Close()

		// Write request details
		fmt.Fprintf(reqFile, "Method: %s\n", r.Method)
		fmt.Fprintf(reqFile, "URL: %s\n", r.URL.String())
		fmt.Fprintf(reqFile, "Headers:\n")
		for k, v := range r.Header {
			fmt.Fprintf(reqFile, "  %s: %v\n", k, v)
		}

		// Record request body if present
		if r.Body != nil {
			fmt.Fprintf(reqFile, "\nBody:\n")
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Logf("Failed to read request body: %v", err)
			} else {
				fmt.Fprintf(reqFile, "%s\n", string(body))
				// Restore body for forwarding
				r.Body = io.NopCloser(bytes.NewReader(body))
			}
		}

		// Forward the request to the actual server
		client := &http.Client{}
		resp, err := client.Do(r)
		if err != nil {
			t.Logf("Failed to forward request: %v", err)
			http.Error(w, "Failed to connect to server", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// Record the response
		respFilename := filepath.Join(recordDir, fmt.Sprintf("%s-response.txt", timestamp))
		respFile, err := os.Create(respFilename)
		if err != nil {
			t.Logf("Failed to create response file: %v", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		defer respFile.Close()

		// Write response details
		fmt.Fprintf(respFile, "Status: %s\n", resp.Status)
		fmt.Fprintf(respFile, "Headers:\n")
		for k, v := range resp.Header {
			fmt.Fprintf(respFile, "  %s: %v\n", k, v)
		}

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Logf("Failed to read response body: %v", err)
		} else {
			fmt.Fprintf(respFile, "\nBody:\n")
			fmt.Fprintf(respFile, "%s\n", string(respBody))
		}

		// Write response to client
		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		w.Write(respBody)
	}))
	defer proxy.Close()

	// Set environment variables to use our proxy
	os.Setenv("HTTP_PROXY", proxy.URL)
	os.Setenv("HTTPS_PROXY", proxy.URL)
	t.Logf("Proxy server started at: %s", proxy.URL)

	// Get credentials from environment
	authToken := os.Getenv("NLM_AUTH_TOKEN")
	cookies := os.Getenv("NLM_COOKIES")
	if authToken == "" || cookies == "" {
		t.Fatalf("Missing credentials. Set NLM_AUTH_TOKEN and NLM_COOKIES environment variables.")
	}

	// Create client with debug mode enabled
	client := New(
		authToken,
		cookies,
		batchexecute.WithDebug(true),
	)

	// Try to list projects
	t.Log("Listing projects...")
	projects, err := client.ListRecentlyViewedProjects()
	if err != nil {
		t.Logf("Error listing projects: %v", err)
		// Continue to record the error response
	} else {
		t.Logf("Found %d projects", len(projects))
		for i, p := range projects {
			t.Logf("Project %d: %s (%s)", i, p.Title, p.ProjectId)
		}
	}

	// Test passed if we recorded the HTTP traffic
	t.Logf("HTTP traffic recorded to: %s", recordDir)
}

// TestDirectRequest sends direct HTTP requests to troubleshoot the ListProjects API
func TestDirectRequest(t *testing.T) {
	// Skip in normal testing
	if os.Getenv("TEST_DIRECT_REQUEST") != "true" {
		t.Skip("Skipping direct request test. Set TEST_DIRECT_REQUEST=true to run.")
	}

	// Get credentials from environment
	authToken := os.Getenv("NLM_AUTH_TOKEN")
	cookies := os.Getenv("NLM_COOKIES")
	if authToken == "" || cookies == "" {
		t.Fatalf("Missing credentials. Set NLM_AUTH_TOKEN and NLM_COOKIES environment variables.")
	}

	// Create an RPC client directly
	rpcClient := rpc.New(authToken, cookies, batchexecute.WithDebug(true))

	// Try to list projects
	t.Log("Listing projects...")
	resp, err := rpcClient.Do(rpc.Call{
		ID:   rpc.RPCListRecentlyViewedProjects,
		Args: []interface{}{nil, 1},
	})

	if err != nil {
		t.Fatalf("Failed to list projects: %v", err)
	}

	// Save the raw response to a file
	responseDir := filepath.Join(os.TempDir(), "nlm-direct-response")
	err = os.MkdirAll(responseDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create response directory: %v", err)
	}

	responseFile := filepath.Join(responseDir, "list_projects_raw.json")
	err = os.WriteFile(responseFile, resp, 0644)
	if err != nil {
		t.Fatalf("Failed to write response: %v", err)
	}

	t.Logf("Saved raw response to: %s", responseFile)
}
