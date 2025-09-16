// Package httprr provides HTTP request recording and replay functionality
// for testing and debugging NotebookLM API calls.
//
// This package is inspired by and based on the httprr implementation from
// github.com/tmc/langchaingo, providing deterministic HTTP record/replay
// for testing with command-line flag integration.
package httprr

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	nethttputil "net/http/httputil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	record      = new(string)
	debug       = new(bool)
	httpDebug   = new(bool)
	recordDelay = new(time.Duration)
	recordMu    sync.Mutex
)

func init() {
	if testing.Testing() {
		record = flag.String("httprecord", "", "re-record traces for files matching `regexp`")
		debug = flag.Bool("httprecord-debug", false, "enable debug output for httprr recording details")
		httpDebug = flag.Bool("httpdebug", false, "enable HTTP request/response logging")
		recordDelay = flag.Duration("httprecord-delay", 0, "delay between HTTP requests during recording (helps avoid rate limits)")
	}
}

// RecordReplay is an http.RoundTripper that can operate in two modes: record and replay.
//
// In record mode, the RecordReplay invokes another RoundTripper
// and logs the (request, response) pairs to a file.
//
// In replay mode, the RecordReplay responds to requests by finding
// an identical request in the log and sending the logged response.
type RecordReplay struct {
	file string            // file being read or written
	real http.RoundTripper // real HTTP connection

	mu        sync.Mutex
	reqScrub  []func(*http.Request) error // scrubbers for logging requests
	respScrub []func(*bytes.Buffer) error // scrubbers for logging responses
	replay    map[string]string           // if replaying, the log
	record    *os.File                    // if recording, the file being written
	writeErr  error                       // if recording, any write error encountered
	logger    *slog.Logger                // logger for debug output
}

// Body is an io.ReadCloser used as an HTTP request body.
// In a Scrubber, if req.Body != nil, then req.Body is guaranteed
// to have type *Body, making it easy to access the body to change it.
type Body struct {
	Data       []byte
	ReadOffset int
}

// Read reads from the body, implementing io.Reader.
func (b *Body) Read(p []byte) (n int, err error) {
	if b.ReadOffset >= len(b.Data) {
		return 0, io.EOF
	}
	n = copy(p, b.Data[b.ReadOffset:])
	b.ReadOffset += n
	return n, nil
}

// Close closes the body, implementing io.Closer.
func (b *Body) Close() error {
	return nil
}

// ScrubReq adds new request scrubbing functions to rr.
//
// Before using a request as a lookup key or saving it in the record/replay log,
// the RecordReplay calls each scrub function, in the order they were registered,
// to canonicalize non-deterministic parts of the request and remove secrets.
// Scrubbing only applies to a copy of the request used in the record/replay log;
// the unmodified original request is sent to the actual server in recording mode.
// A scrub function can assume that if req.Body is not nil, then it has type *Body.
//
// Calling ScrubReq adds to the list of registered request scrubbing functions;
// it does not replace those registered by earlier calls.
func (rr *RecordReplay) ScrubReq(scrubs ...func(req *http.Request) error) {
	rr.reqScrub = append(rr.reqScrub, scrubs...)
}

// ScrubResp adds new response scrubbing functions to rr.
//
// Before using a response as a lookup key or saving it in the record/replay log,
// the RecordReplay calls each scrub function on a byte representation of the
// response, in the order they were registered, to canonicalize non-deterministic
// parts of the response and remove secrets.
//
// Calling ScrubResp adds to the list of registered response scrubbing functions;
// it does not replace those registered by earlier calls.
func (rr *RecordReplay) ScrubResp(scrubs ...func(*bytes.Buffer) error) {
	rr.respScrub = append(rr.respScrub, scrubs...)
}

// Recording reports whether the RecordReplay is in recording mode.
func (rr *RecordReplay) Recording() bool {
	return rr.record != nil
}

// Replaying reports whether the RecordReplay is in replaying mode.
func (rr *RecordReplay) Replaying() bool {
	return !rr.Recording()
}

// Client returns an http.Client using rr as its transport.
// It is a shorthand for:
//
//	return &http.Client{Transport: rr}
//
// For more complicated uses, use rr or the RecordReplay.RoundTrip method directly.
func (rr *RecordReplay) Client() *http.Client {
	return &http.Client{Transport: rr}
}

// Recording reports whether the "-httprecord" flag is set
// for the given file.
// It returns an error if the flag is set to an invalid value.
func Recording(file string) (bool, error) {
	recordMu.Lock()
	defer recordMu.Unlock()
	if *record != "" {
		re, err := regexp.Compile(*record)
		if err != nil {
			return false, fmt.Errorf("invalid -httprecord flag: %w", err)
		}
		if re.MatchString(file) {
			return true, nil
		}
	}
	return false, nil
}

// defaultRequestScrubbers returns the default request scrubbing functions.
func defaultRequestScrubbers() []func(req *http.Request) error {
	return []func(req *http.Request) error{
		sanitizeAuthHeaders,
	}
}

// defaultResponseScrubbers returns the default response scrubbing functions.
func defaultResponseScrubbers() []func(*bytes.Buffer) error {
	return []func(*bytes.Buffer) error{}
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

// sanitizeAuthHeaders removes sensitive auth headers to avoid leaking credentials
func sanitizeAuthHeaders(req *http.Request) error {
	sensitiveHeaders := []string{"Authorization", "Cookie"}
	for _, header := range sensitiveHeaders {
		if req.Header.Get(header) != "" {
			req.Header.Set(header, "[REDACTED]")
		}
	}
	return nil
}

// Open opens a new record/replay log in the named file and
// returns a RecordReplay backed by that file.
//
// By default Open expects the file to exist and contain a
// previously-recorded log of (request, response) pairs,
// which RecordReplay.RoundTrip consults to prepare its responses.
//
// If the command-line flag -httprecord is set to a non-empty
// regular expression that matches file, then Open creates
// the file as a new log. In that mode, RecordReplay.RoundTrip
// makes actual HTTP requests using rt but then logs the requests and
// responses to the file for replaying in a future run.
func Open(file string, rt http.RoundTripper) (*RecordReplay, error) {
	record, err := Recording(file)
	if err != nil {
		return nil, err
	}
	if record {
		return create(file, rt)
	}

	// Check if a compressed version exists
	if _, err := os.Stat(file); os.IsNotExist(err) {
		if _, err := os.Stat(file + ".gz"); err == nil {
			file = file + ".gz"
		}
	}

	return open(file, rt)
}

// creates a new record-mode RecordReplay in the file.
func create(file string, rt http.RoundTripper) (*RecordReplay, error) {
	f, err := os.Create(file)
	if err != nil {
		return nil, err
	}

	// Write header line.
	// Each round-trip will write a new request-response record.
	if _, err := fmt.Fprintf(f, "httprr trace v1\n"); err != nil {
		// unreachable unless write error immediately after os.Create
		f.Close()
		return nil, err
	}
	rr := &RecordReplay{
		file:   file,
		real:   rt,
		record: f,
	}
	// Apply default scrubbing
	rr.ScrubReq(defaultRequestScrubbers()...)
	rr.ScrubResp(defaultResponseScrubbers()...)
	return rr, nil
}

// open opens a replay-mode RecordReplay using the data in the file.
func open(file string, rt http.RoundTripper) (*RecordReplay, error) {
	var bdata []byte
	var err error

	// Check if file is compressed
	if strings.HasSuffix(file, ".gz") {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		gz, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer gz.Close()

		bdata, err = io.ReadAll(gz)
		if err != nil {
			return nil, err
		}
	} else {
		bdata, err = os.ReadFile(file)
		if err != nil {
			return nil, err
		}
	}

	// Trace begins with header line.
	data := string(bdata)
	line, data, ok := strings.Cut(data, "\n")
	// Trim any trailing CR for compatibility with both LF and CRLF line endings
	line = strings.TrimSuffix(line, "\r")
	if !ok || line != "httprr trace v1" {
		return nil, fmt.Errorf("read %s: not an httprr trace", file)
	}

	replay := make(map[string]string)
	for data != "" {
		// Each record starts with a line of the form "n1 n2\n" (or "n1 n2\r\n")
		// followed by n1 bytes of request encoding and
		// n2 bytes of response encoding.
		line, data, ok = strings.Cut(data, "\n")
		line = strings.TrimSuffix(line, "\r")
		f1, f2, _ := strings.Cut(line, " ")
		n1, err1 := strconv.Atoi(f1)
		n2, err2 := strconv.Atoi(f2)
		if !ok || err1 != nil || err2 != nil || n1 > len(data) || n2 > len(data[n1:]) {
			return nil, fmt.Errorf("read %s: corrupt httprr trace", file)
		}
		var req, resp string
		req, resp, data = data[:n1], data[n1:n1+n2], data[n1+n2:]
		replay[req] = resp
	}

	rr := &RecordReplay{
		file:   file,
		real:   rt,
		replay: replay,
	}
	// Apply default scrubbing
	rr.ScrubReq(defaultRequestScrubbers()...)
	rr.ScrubResp(defaultResponseScrubbers()...)
	return rr, nil
}

// RoundTrip implements the http.RoundTripper interface
func (rr *RecordReplay) RoundTrip(req *http.Request) (*http.Response, error) {
	if rr.record != nil {
		return rr.recordRoundTrip(req)
	}
	return rr.replayRoundTrip(req)
}

// recordRoundTrip implements RoundTrip for recording mode
func (rr *RecordReplay) recordRoundTrip(req *http.Request) (*http.Response, error) {
	// Apply recording delay if set
	if *recordDelay > 0 {
		time.Sleep(*recordDelay)
	}

	reqLog, err := rr.reqWire(req)
	if err != nil {
		rr.writeErr = err
		return nil, err
	}

	resp, err := rr.real.RoundTrip(req)
	if err != nil {
		rr.writeErr = err
		return nil, err
	}

	respLog, err := rr.respWire(resp)
	if err != nil {
		rr.writeErr = err
		return nil, err
	}

	// Write to log file
	rr.mu.Lock()
	defer rr.mu.Unlock()
	if rr.writeErr != nil {
		return nil, rr.writeErr
	}
	if _, err := fmt.Fprintf(rr.record, "%d %d\n%s%s", len(reqLog), len(respLog), reqLog, respLog); err != nil {
		rr.writeErr = err
		return nil, err
	}

	return resp, nil
}

// replayRoundTrip implements RoundTrip for replay mode
func (rr *RecordReplay) replayRoundTrip(req *http.Request) (*http.Response, error) {
	reqLog, err := rr.reqWire(req)
	if err != nil {
		return nil, err
	}

	// Log the incoming request if debug is enabled
	if rr.logger != nil && *debug {
		rr.logger.Debug("httprr: attempting to match request in replay cache",
			"method", req.Method,
			"url", req.URL.String(),
			"file", rr.file,
		)
		// Also dump the full request for detailed debugging
		if reqDump, err := nethttputil.DumpRequestOut(req, true); err == nil {
			rr.logger.Debug("httprr: request details\n" + string(reqDump))
		}
	}

	respLog, ok := rr.replay[reqLog]
	if !ok {
		if rr.logger != nil && *debug {
			rr.logger.Debug("httprr: request not found in replay cache",
				"method", req.Method,
				"url", req.URL.String(),
				"file", rr.file,
			)
		}
		return nil, fmt.Errorf("cached HTTP response not found for:\n%s\n\nHint: Re-run tests with -httprecord=. to record new HTTP interactions\nDebug flags: -httprecord-debug for recording details, -httpdebug for HTTP traffic", reqLog)
	}

	// Parse response from log
	resp, err := http.ReadResponse(bufio.NewReader(strings.NewReader(respLog)), req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// recordRequest records an HTTP interaction (legacy compatibility)
func (rt *RecordReplay) recordRequest(req *http.Request) (*http.Response, error) {
	return rt.recordRoundTrip(req)
}

// reqWire returns the wire-format HTTP request log entry.
func (rr *RecordReplay) reqWire(req *http.Request) (string, error) {
	// Make a copy to avoid modifying the original
	rkey := req.Clone(req.Context())

	// Read the original body
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return "", err
		}
		req.Body = &Body{Data: body}
		rkey.Body = &Body{Data: bytes.Clone(body)}
	}

	// Canonicalize and scrub request key.
	for _, scrub := range rr.reqScrub {
		if err := scrub(rkey); err != nil {
			return "", err
		}
	}

	// Now that scrubbers are done potentially modifying body, set length.
	if rkey.Body != nil {
		rkey.ContentLength = int64(len(rkey.Body.(*Body).Data))
	}

	// Serialize rkey to produce the log entry.
	// Use WriteProxy instead of Write to preserve the URL's scheme.
	var key strings.Builder
	if err := rkey.WriteProxy(&key); err != nil {
		return "", err
	}
	return key.String(), nil
}

// respWire returns the wire-format HTTP response log entry.
// It preserves the original response body while creating a copy for logging.
func (rr *RecordReplay) respWire(resp *http.Response) (string, error) {
	// Read the original body
	var bodyBytes []byte
	var err error
	if resp.Body != nil {
		bodyBytes, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return "", err
		}
		// Replace the body with a fresh reader for the client
		resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	// Create a copy of the response for serialization
	respCopy := *resp
	if bodyBytes != nil {
		respCopy.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		respCopy.ContentLength = int64(len(bodyBytes))
	}

	// Serialize the copy to produce the log entry
	var key bytes.Buffer
	if err := respCopy.Write(&key); err != nil {
		return "", err
	}

	// Close the copy's body since we're done with it
	if respCopy.Body != nil {
		respCopy.Body.Close()
	}

	// Apply scrubbers to the serialized data
	for _, scrub := range rr.respScrub {
		if err := scrub(&key); err != nil {
			return "", err
		}
	}
	return key.String(), nil
}

// Close closes the RecordReplay.
func (rr *RecordReplay) Close() error {
	if rr.record != nil {
		err := rr.record.Close()
		rr.record = nil
		return err
	}
	return nil
}

// saveRecording saves a recording to disk (legacy compatibility)
func (rt *RecordReplay) saveRecording(key string, recording interface{}) {
	// Legacy compatibility - no-op
}

// loadRecordings loads all recordings for a key from disk (legacy compatibility)
func (rt *RecordReplay) loadRecordings(key string) ([]interface{}, error) {
	return nil, fmt.Errorf("legacy method not implemented")
}

// CleanFileName cleans a test name to be suitable as a filename.
func CleanFileName(name string) string {
	// Replace invalid filename characters with hyphens
	re := regexp.MustCompile(`[^a-zA-Z0-9._-]`)
	return re.ReplaceAllString(name, "-")
}

// logWriter creates a test-compatible log writer.
func logWriter(t *testing.T) io.Writer {
	return testWriter{t}
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (n int, err error) {
	w.t.Helper()
	w.t.Log(string(p))
	return len(p), nil
}

// OpenForTest creates a RecordReplay for the given test.
// The primary API for most test cases. Creates a recorder/replayer for the given test.
//
// - Recording mode: Creates testdata/TestName.httprr
// - Replay mode: Loads existing recording
// - File naming: Derived automatically from t.Name()
// - Directory: Always uses testdata/ subdirectory
func OpenForTest(t *testing.T, rt http.RoundTripper) (*RecordReplay, error) {
	t.Helper()

	// Default to http.DefaultTransport if no transport provided
	if rt == nil {
		rt = http.DefaultTransport
	}

	testName := CleanFileName(t.Name())
	filename := filepath.Join("testdata", testName+".httprr")

	// Ensure testdata directory exists
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		return nil, fmt.Errorf("httprr: failed to create testdata directory: %w", err)
	}

	// Create logger for debug mode
	var logger *slog.Logger
	if *debug || *httpDebug {
		logger = slog.New(slog.NewTextHandler(logWriter(t), &slog.HandlerOptions{Level: slog.LevelDebug}))
	}

	// Check if we're in recording mode
	recording, err := Recording(filename)
	if err != nil {
		return nil, err
	}

	if recording && testing.Short() {
		t.Skipf("httprr: skipping recording for %s in short mode", filename)
	}

	if recording {
		// Recording mode: clean up existing files and create uncompressed
		cleanupExistingFiles(t, filename)
		rr, err := Open(filename, rt)
		if err != nil {
			return nil, fmt.Errorf("httprr: failed to open recording file %s: %w", filename, err)
		}
		rr.logger = logger
		t.Cleanup(func() { rr.Close() })
		return rr, nil
	}

	// Replay mode: find the best existing file
	filename = findBestReplayFile(t, filename)
	rr, err := Open(filename, rt)
	if err != nil {
		return nil, err
	}
	rr.logger = logger
	return rr, nil
}

// cleanupExistingFiles removes any existing files to avoid conflicts during recording
func cleanupExistingFiles(t *testing.T, baseFilename string) {
	t.Helper()
	filesToCheck := []string{baseFilename, baseFilename + ".gz"}

	for _, filename := range filesToCheck {
		if _, err := os.Stat(filename); err == nil {
			if err := os.Remove(filename); err != nil {
				t.Logf("httprr: warning - failed to remove %s: %v", filename, err)
			}
		}
	}
}

// findBestReplayFile finds the best existing file for replay mode
func findBestReplayFile(t *testing.T, baseFilename string) string {
	t.Helper()
	compressedFilename := baseFilename + ".gz"

	uncompressedStat, uncompressedErr := os.Stat(baseFilename)
	compressedStat, compressedErr := os.Stat(compressedFilename)

	// Both files exist - use the newer one and warn
	if uncompressedErr == nil && compressedErr == nil {
		if uncompressedStat.ModTime().After(compressedStat.ModTime()) {
			t.Logf("httprr: found both files, using newer uncompressed version")
			return baseFilename
		} else {
			t.Logf("httprr: found both files, using newer compressed version")
			return compressedFilename
		}
	}

	// Prefer compressed file if only it exists
	if compressedErr == nil {
		return compressedFilename
	}

	// Return base filename (may or may not exist)
	return baseFilename
}

// SkipIfNoCredentialsOrRecording skips the test if required environment variables
// are not set and no httprr recording exists. This allows tests to gracefully
// skip when they cannot run.
//
// Example usage:
//
//	func TestMyAPI(t *testing.T) {
//	    httprr.SkipIfNoCredentialsOrRecording(t, "API_KEY", "API_URL")
//
//	    rr, err := httprr.OpenForTest(t, http.DefaultTransport)
//	    if err != nil {
//	        t.Fatal(err)
//	    }
//	    defer rr.Close()
//	    // use rr.Client() for HTTP requests...
//	}
func SkipIfNoCredentialsOrRecording(t *testing.T, envVars ...string) {
	t.Helper()
	if !hasExistingRecording(t) && !hasRequiredCredentials(envVars) {
		skipMessage := "no httprr recording available. Hint: Re-run tests with -httprecord=. to record new HTTP interactions\nDebug flags: -httprecord-debug for recording details, -httpdebug for HTTP traffic"

		if len(envVars) > 0 {
			missingEnvVars := []string{}
			for _, envVar := range envVars {
				if os.Getenv(envVar) == "" {
					missingEnvVars = append(missingEnvVars, envVar)
				}
			}
			skipMessage = fmt.Sprintf("%s not set and %s", strings.Join(missingEnvVars, ","), skipMessage)
		}

		t.Skip(skipMessage)
	}
}

// hasRequiredCredentials checks if any of the required environment variables are set
func hasRequiredCredentials(envVars []string) bool {
	for _, envVar := range envVars {
		if os.Getenv(envVar) != "" {
			return true
		}
	}
	return false
}

// hasExistingRecording checks if a recording file exists for the current test
func hasExistingRecording(t *testing.T) bool {
	t.Helper()
	testName := CleanFileName(t.Name())
	baseFilename := filepath.Join("testdata", testName+".httprr")
	compressedFilename := baseFilename + ".gz"

	_, uncompressedErr := os.Stat(baseFilename)
	_, compressedErr := os.Stat(compressedFilename)

	return uncompressedErr == nil || compressedErr == nil
}

// NewRecordingClient creates an http.Client with a RecordReplay transport
// This is a convenience method for backwards compatibility.
func NewRecordingClient(filename string, baseClient *http.Client) (*http.Client, error) {
	var baseTransport http.RoundTripper
	if baseClient != nil {
		baseTransport = baseClient.Transport
	}
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}

	rr, err := Open(filename, baseTransport)
	if err != nil {
		return nil, err
	}

	return &http.Client{
		Transport: rr,
		Timeout:   30 * time.Second,
	}, nil
}
