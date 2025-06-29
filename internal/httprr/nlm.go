package httprr

import (
	"bytes"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// OpenForNLMTest creates a RecordReplay specifically configured for NLM testing.
// It sets up appropriate scrubbers for NotebookLM API calls and credentials.
func OpenForNLMTest(t *testing.T, rt http.RoundTripper) (*RecordReplay, error) {
	t.Helper()

	rr, err := OpenForTest(t, rt)
	if err != nil {
		return nil, err
	}

	// Add NLM-specific request scrubbers
	rr.ScrubReq(scrubNLMCredentials)
	rr.ScrubReq(scrubNLMTimestamps)

	// Add NLM-specific response scrubbers
	rr.ScrubResp(scrubNLMResponseTimestamps)
	rr.ScrubResp(scrubNLMResponseIDs)

	return rr, nil
}

// SkipIfNoNLMCredentialsOrRecording skips the test if NLM credentials are not set
// and no httprr recording exists. This is a convenience function for NLM tests.
func SkipIfNoNLMCredentialsOrRecording(t *testing.T) {
	t.Helper()
	SkipIfNoCredentialsOrRecording(t, "NLM_AUTH_TOKEN", "NLM_COOKIES")
}

// scrubNLMCredentials removes NLM-specific authentication headers and cookies.
func scrubNLMCredentials(req *http.Request) error {
	// Remove sensitive NLM headers
	nlmHeaders := []string{
		"Authorization",
		"Cookie",
		"X-Goog-AuthUser",
		"X-Client-Data", 
		"X-Goog-Visitor-Id",
	}
	
	for _, header := range nlmHeaders {
		if req.Header.Get(header) != "" {
			req.Header.Set(header, "[REDACTED]")
		}
	}

	return nil
}

// scrubNLMTimestamps removes timestamps from NLM RPC requests to make them deterministic.
func scrubNLMTimestamps(req *http.Request) error {
	if req.Body == nil {
		return nil
	}

	body, ok := req.Body.(*Body)
	if !ok {
		return nil
	}

	bodyStr := string(body.Data)

	// Remove timestamps from NotebookLM RPC calls
	// Pattern matches things like: "1672531200000" (Unix timestamp in milliseconds)
	timestampPattern := regexp.MustCompile(`[0-9]{13}`)
	bodyStr = timestampPattern.ReplaceAllString(bodyStr, `[TIMESTAMP]`)

	// Remove date strings that might appear in requests
	datePattern := regexp.MustCompile(`[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}[.0-9]*Z?`)
	bodyStr = datePattern.ReplaceAllString(bodyStr, `[DATE]`)

	body.Data = []byte(bodyStr)
	return nil
}

// scrubNLMResponseTimestamps removes timestamps from NLM API responses.
func scrubNLMResponseTimestamps(buf *bytes.Buffer) error {
	content := buf.String()

	// Remove timestamps from responses
	timestampPattern := regexp.MustCompile(`[0-9]{13}`)
	content = timestampPattern.ReplaceAllString(content, `[TIMESTAMP]`)

	// Remove date strings
	datePattern := regexp.MustCompile(`[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}[.0-9]*Z?`)
	content = datePattern.ReplaceAllString(content, `[DATE]`)

	// Remove creation/modification time fields
	creationTimePattern := regexp.MustCompile(`"creationTime":"[^"]*"`)
	content = creationTimePattern.ReplaceAllString(content, `"creationTime":"[TIMESTAMP]"`)

	modificationTimePattern := regexp.MustCompile(`"modificationTime":"[^"]*"`)
	content = modificationTimePattern.ReplaceAllString(content, `"modificationTime":"[TIMESTAMP]"`)

	buf.Reset()
	buf.WriteString(content)
	return nil
}

// scrubNLMResponseIDs removes or normalizes IDs in NLM API responses that might be non-deterministic.
func scrubNLMResponseIDs(buf *bytes.Buffer) error {
	content := buf.String()

	// Remove or normalize notebook IDs (typically long alphanumeric strings)
	notebookIDPattern := regexp.MustCompile(`"id":"[a-zA-Z0-9_-]{20,}"`)
	content = notebookIDPattern.ReplaceAllString(content, `"id":"[NOTEBOOK_ID]"`)

	// Remove or normalize source IDs
	sourceIDPattern := regexp.MustCompile(`"sourceId":"[a-zA-Z0-9_-]{20,}"`)
	content = sourceIDPattern.ReplaceAllString(content, `"sourceId":"[SOURCE_ID]"`)

	// Remove session IDs or similar temporary identifiers
	sessionIDPattern := regexp.MustCompile(`"sessionId":"[a-zA-Z0-9_-]{20,}"`)
	content = sessionIDPattern.ReplaceAllString(content, `"sessionId":"[SESSION_ID]"`)

	// Remove request IDs that might appear in error responses
	requestIDPattern := regexp.MustCompile(`"requestId":"[a-zA-Z0-9_-]{20,}"`)
	content = requestIDPattern.ReplaceAllString(content, `"requestId":"[REQUEST_ID]"`)

	buf.Reset()
	buf.WriteString(content)
	return nil
}

// CreateNLMTestClient creates an HTTP client configured for NLM testing with httprr.
// This is a convenience function that combines OpenForNLMTest with Client().
func CreateNLMTestClient(t *testing.T, rt http.RoundTripper) *http.Client {
	t.Helper()

	rr, err := OpenForNLMTest(t, rt)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { rr.Close() })
	return rr.Client()
}

// NotebookLMRecordMatcher creates a request matcher specifically for NotebookLM RPC calls.
// This function extracts the RPC function ID from the request body for better matching.
func NotebookLMRecordMatcher(req *http.Request) string {
	if req.Body == nil {
		// Fall back to URL path for requests without body
		path := req.URL.Path
		return req.Method + "_" + path
	}

	body, ok := req.Body.(*Body)
	if !ok {
		// Fall back to URL path for non-Body types
		path := req.URL.Path
		return req.Method + "_" + path
	}

	bodyStr := string(body.Data)

	// Extract RPC endpoint ID for NotebookLM API calls
	// The format is typically something like: [["VUsiyb",["arg1","arg2"]]]
	funcIDPattern := regexp.MustCompile(`\[\["([a-zA-Z0-9]+)",`)
	matches := funcIDPattern.FindStringSubmatch(bodyStr)

	if len(matches) >= 2 {
		funcID := matches[1]
		// Also include the HTTP method and a simplified body hash for uniqueness
		bodyHash := ""
		if len(bodyStr) > 100 {
			bodyHash = bodyStr[:50] + "..." + bodyStr[len(bodyStr)-50:]
		} else {
			bodyHash = bodyStr
		}
		
		// Remove dynamic content for better matching
		bodyHash = strings.ReplaceAll(bodyHash, `[TIMESTAMP]`, "")
		bodyHash = strings.ReplaceAll(bodyHash, `[DATE]`, "")
		
		return req.Method + "_" + funcID + "_" + bodyHash
	}

	// Fall back to URL path for non-RPC calls
	path := req.URL.Path
	return req.Method + "_" + path
}