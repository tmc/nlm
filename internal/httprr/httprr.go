// Package httprr provides HTTP request recording and replay functionality
// for testing and debugging NotebookLM API calls.
package httprr

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Mode represents the mode of operation for the RecordingTransport
type Mode string

const (
	// ModeRecord records all HTTP interactions to disk
	ModeRecord Mode = "record"

	// ModeReplay replays recorded HTTP interactions from disk
	ModeReplay Mode = "replay"

	// ModePassthrough bypasses recording/replaying
	ModePassthrough Mode = "passthrough"
)

// RequestKey represents the identifying information for a request
// Used to match requests during replay
type RequestKey struct {
	Method string
	Path   string
	Body   string
}

// Recording represents a recorded HTTP interaction (request and response)
type Recording struct {
	Request  RecordedRequest
	Response RecordedResponse
}

// RecordedRequest contains the data from a recorded HTTP request
type RecordedRequest struct {
	Method  string
	URL     string
	Path    string
	Headers http.Header
	Body    string
}

// RecordedResponse contains the data from a recorded HTTP response
type RecordedResponse struct {
	StatusCode int
	Headers    http.Header
	Body       string
}

// RecordingTransport is an http.RoundTripper that records and replays HTTP interactions
type RecordingTransport struct {
	Mode           Mode
	RecordingsDir  string
	Transport      http.RoundTripper
	RecordMatcher  func(req *http.Request) string
	RequestFilters []func(req *http.Request)

	recordings     map[string][]Recording
	recordingMutex sync.RWMutex
}

// NewRecordingTransport creates a new RecordingTransport
func NewRecordingTransport(mode Mode, recordingsDir string, baseTransport http.RoundTripper) *RecordingTransport {
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}

	// Create recordings directory if it doesn't exist
	if mode == ModeRecord {
		os.MkdirAll(recordingsDir, 0755)
	}

	return &RecordingTransport{
		Mode:           mode,
		RecordingsDir:  recordingsDir,
		Transport:      baseTransport,
		recordings:     make(map[string][]Recording),
		RecordMatcher:  defaultRecordMatcher,
		RequestFilters: []func(*http.Request){sanitizeAuthHeaders},
	}
}

// defaultRecordMatcher creates a key for a request based on the notebooklm RPC endpoint
func defaultRecordMatcher(req *http.Request) string {
	// Extract RPC function ID from the request body
	body, _ := io.ReadAll(req.Body)
	req.Body = io.NopCloser(bytes.NewReader(body)) // Restore for later use

	// Extract RPC endpoint ID for NotebookLM API calls
	// The format is typically something like: [["VUsiyb",["arg1","arg2"]]]
	funcIDPattern := regexp.MustCompile(`\[\["([a-zA-Z0-9]+)",`)
	matches := funcIDPattern.FindSubmatch(body)

	if len(matches) >= 2 {
		funcID := string(matches[1])
		return fmt.Sprintf("%s_%s", req.Method, funcID)
	}

	// Fall back to URL path based matching for non-RPC calls
	path := req.URL.Path
	return fmt.Sprintf("%s_%s", req.Method, path)
}

// keyForRequest generates a unique key for a request
func (rt *RecordingTransport) keyForRequest(req *http.Request) string {
	return rt.RecordMatcher(req)
}

// sanitizeAuthHeaders removes sensitive auth headers to avoid leaking credentials
func sanitizeAuthHeaders(req *http.Request) {
	sensitiveHeaders := []string{"Authorization", "Cookie"}
	for _, header := range sensitiveHeaders {
		if req.Header.Get(header) != "" {
			req.Header.Set(header, "[REDACTED]")
		}
	}
}

// RoundTrip implements the http.RoundTripper interface
func (rt *RecordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	switch rt.Mode {
	case ModeRecord:
		return rt.recordRequest(req)
	case ModeReplay:
		resp, err := rt.replayRequest(req)
		if err != nil {
			return nil, err
		}
		if resp != nil {
			return resp, nil
		}
		// Fall through to passthrough if no matching recording found
		fmt.Printf("No matching recording found for %s, falling back to live request\n", req.URL.String())
	}

	// Passthrough mode or fallback
	return rt.Transport.RoundTrip(req)
}

// recordRequest records an HTTP interaction
func (rt *RecordingTransport) recordRequest(req *http.Request) (*http.Response, error) {
	// Copy the request body for recording
	var bodyBuf bytes.Buffer
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}

	// Write body to buffer and restore for original request
	bodyBuf.Write(body)
	req.Body = io.NopCloser(bytes.NewReader(body))

	// Create a copy of the request for recording
	recordedReq := RecordedRequest{
		Method:  req.Method,
		URL:     req.URL.String(),
		Path:    req.URL.Path,
		Headers: req.Header.Clone(),
		Body:    bodyBuf.String(),
	}

	// Apply filters to the recorded request to remove sensitive data
	for _, filter := range rt.RequestFilters {
		reqCopy := &http.Request{
			Method: recordedReq.Method,
			URL:    req.URL,
			Header: recordedReq.Headers,
			Body:   io.NopCloser(strings.NewReader(recordedReq.Body)),
		}
		filter(reqCopy)
		recordedReq.Headers = reqCopy.Header
	}

	// Make the actual HTTP request
	resp, err := rt.Transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	// Read and restore the response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(respBody))

	// Create the recorded response
	recordedResp := RecordedResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header.Clone(),
		Body:       string(respBody),
	}

	recording := Recording{
		Request:  recordedReq,
		Response: recordedResp,
	}

	// Store the recording
	key := rt.keyForRequest(req)
	rt.saveRecording(key, recording)

	return resp, nil
}

// replayRequest replays a recorded HTTP interaction
func (rt *RecordingTransport) replayRequest(req *http.Request) (*http.Response, error) {
	key := rt.keyForRequest(req)

	rt.recordingMutex.RLock()
	recordings, ok := rt.recordings[key]
	rt.recordingMutex.RUnlock()

	if !ok {
		// Try to load recordings from disk
		var err error
		recordings, err = rt.loadRecordings(key)
		if err != nil || len(recordings) == 0 {
			return nil, nil
		}
	}

	// Find the best matching recording based on request body similarity
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(reqBody))

	bestMatch := findBestMatch(recordings, string(reqBody))

	// Create a response from the recording
	header := http.Header{}
	for k, v := range bestMatch.Response.Headers {
		header[k] = v
	}

	return &http.Response{
		StatusCode:    bestMatch.Response.StatusCode,
		Header:        header,
		Body:          io.NopCloser(strings.NewReader(bestMatch.Response.Body)),
		ContentLength: int64(len(bestMatch.Response.Body)),
		Request:       req,
	}, nil
}

// saveRecording saves a recording to disk
func (rt *RecordingTransport) saveRecording(key string, recording Recording) {
	rt.recordingMutex.Lock()
	defer rt.recordingMutex.Unlock()

	// Add to in-memory cache
	if _, ok := rt.recordings[key]; !ok {
		rt.recordings[key] = make([]Recording, 0)
	}
	rt.recordings[key] = append(rt.recordings[key], recording)

	// Create a unique filename for this recording
	timestamp := time.Now().Format("20060102-150405")
	hash := md5.Sum([]byte(recording.Request.Body))
	hashStr := hex.EncodeToString(hash[:])[:8]

	filename := filepath.Join(rt.RecordingsDir, fmt.Sprintf("%s_%s_%s.json", key, timestamp, hashStr))

	// Save to disk
	file, err := os.Create(filename)
	if err != nil {
		fmt.Printf("Error creating recording file: %v\n", err)
		return
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	if err := enc.Encode(recording); err != nil {
		fmt.Printf("Error encoding recording: %v\n", err)
	}
}

// loadRecordings loads all recordings for a key from disk
func (rt *RecordingTransport) loadRecordings(key string) ([]Recording, error) {
	pattern := filepath.Join(rt.RecordingsDir, fmt.Sprintf("%s_*.json", key))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	recordings := make([]Recording, 0, len(matches))
	for _, match := range matches {
		file, err := os.Open(match)
		if err != nil {
			continue
		}

		var recording Recording
		err = json.NewDecoder(file).Decode(&recording)
		file.Close()

		if err != nil {
			continue
		}

		recordings = append(recordings, recording)
	}

	// Update in-memory cache
	if len(recordings) > 0 {
		rt.recordingMutex.Lock()
		rt.recordings[key] = recordings
		rt.recordingMutex.Unlock()
	}

	return recordings, nil
}

// findBestMatch finds the recording that best matches the request body
func findBestMatch(recordings []Recording, reqBody string) Recording {
	if len(recordings) == 1 {
		return recordings[0]
	}

	// Find the recording with the most similar request body
	// This is a very simple implementation - could be improved
	bestMatch := recordings[0]
	bestScore := 0

	for _, recording := range recordings {
		score := similarityScore(recording.Request.Body, reqBody)
		if score > bestScore {
			bestScore = score
			bestMatch = recording
		}
	}

	return bestMatch
}

// similarityScore computes a simple similarity score between two strings
// Higher score means more similar
func similarityScore(s1, s2 string) int {
	// This is a very simple implementation
	// For beproto calls, we just check if the core arguments (excluding timestamps)
	// are the same

	// For more complex matching, we could use algorithms like
	// Levenshtein distance, Jaccard similarity, etc.

	if s1 == s2 {
		return 100 // Exact match
	}

	// Count common characters
	score := 0
	minLen := min(len(s1), len(s2))
	for i := 0; i < minLen; i++ {
		if s1[i] == s2[i] {
			score++
		}
	}

	return score
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// NewRecordingClient creates an http.Client with a RecordingTransport
func NewRecordingClient(mode Mode, recordingsDir string, baseClient *http.Client) *http.Client {
	var baseTransport http.RoundTripper
	if baseClient != nil {
		baseTransport = baseClient.Transport
	}
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}

	return &http.Client{
		Transport: NewRecordingTransport(mode, recordingsDir, baseTransport),
		Timeout:   30 * time.Second,
	}
}

// NewTestServer creates a test server that returns responses from recorded files
func NewTestServer(recordingsDir string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read the request body
		_, _ = io.ReadAll(r.Body)

		// Generate a key for this request
		key := r.Method + "_" + r.URL.Path

		// Try to find a matching recording file
		pattern := filepath.Join(recordingsDir, fmt.Sprintf("%s_*.json", key))
		matches, err := filepath.Glob(pattern)
		if err != nil || len(matches) == 0 {
			http.Error(w, "No matching recording found", http.StatusNotFound)
			return
		}

		// Load the first matching recording
		file, err := os.Open(matches[0])
		if err != nil {
			http.Error(w, "Failed to open recording file", http.StatusInternalServerError)
			return
		}
		defer file.Close()

		var recording Recording
		err = json.NewDecoder(file).Decode(&recording)
		if err != nil {
			http.Error(w, "Failed to decode recording", http.StatusInternalServerError)
			return
		}

		// Write the recorded response
		for k, v := range recording.Response.Headers {
			for _, vv := range v {
				w.Header().Add(k, vv)
			}
		}
		w.WriteHeader(recording.Response.StatusCode)
		w.Write([]byte(recording.Response.Body))
	}))
}
