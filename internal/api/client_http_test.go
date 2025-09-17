package api

import (
	"testing"
)

// TestHTTPRecorder is deprecated - use DebugHTTPRecorder helper instead
// Run with: NLM_DEBUG=true go test -run TestHTTPRecorder ./internal/api
func TestHTTPRecorder(t *testing.T) {
	// Delegate to the new helper function
	DebugHTTPRecorder(t)

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

	// The actual implementation has been moved to test_helpers.go
	// This stub remains for backward compatibility
}

// TestDirectRequest is deprecated - use DebugDirectRequest helper instead
// Run with: NLM_DEBUG=true go test -run TestDirectRequest ./internal/api
func TestDirectRequest(t *testing.T) {
	// Delegate to the new helper function
	DebugDirectRequest(t)

	// The actual implementation has been moved to test_helpers.go
	// This stub remains for backward compatibility
}
