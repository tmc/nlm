package httprr

import (
	"bytes"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// OpenForNLMTest creates a RecordReplay specifically configured for NLM usage.
// It sets up appropriate scrubbers for NotebookLM API calls and credentials.
func OpenForNLMTest(t *testing.T, rt http.RoundTripper) (*RecordReplay, error) {
	t.Helper()

	rr, err := OpenForTest(t, rt)
	if err != nil {
		return nil, err
	}

	// Add NLM-specific request scrubbers
	rr.ScrubReq(scrubNLMRequestID)         // Normalize request IDs for consistent matching
	rr.ScrubReq(scrubNLMAuthTokenFromBody) // Remove auth tokens from body for replay
	// rr.ScrubReq(scrubNLMTimestamps) // Keep commented until needed

	// Add NLM-specific response scrubbers
	rr.ScrubResp(scrubNLMProjectListLimit) // Content-aware truncation of project lists
	rr.ScrubResp(scrubNLMResponseTimestamps)
	rr.ScrubResp(scrubNLMResponseIDs)

	return rr, nil
}

// SkipIfNoNLMCredentialsOrRecording skips execution if NLM credentials are not set
// and no httprr data exists. This is a convenience function for NLM operations.
func SkipIfNoNLMCredentialsOrRecording(t *testing.T) {
	t.Helper()
	SkipIfNoCredentialsOrRecording(t, "NLM_AUTH_TOKEN", "NLM_COOKIES")
}

// scrubNLMCredentials removes NLM-specific authentication headers and cookies.
func scrubNLMCredentials(req *http.Request) error {
	// Remove sensitive NLM headers completely (not just redact)
	// This ensures data can be replayed without any credentials
	nlmHeaders := []string{
		"Authorization",
		"Cookie",
		"X-Goog-AuthUser",
		"X-Client-Data",
		"X-Goog-Visitor-Id",
	}

	for _, header := range nlmHeaders {
		req.Header.Del(header)
	}

	// Ensure Cookie header is empty for consistent replay
	req.Header.Set("Cookie", "")

	return nil
}

// scrubNLMAuthTokenFromBody removes the auth token from the request body.
func scrubNLMAuthTokenFromBody(req *http.Request) error {
	if req.Body == nil {
		return nil
	}

	body, ok := req.Body.(*Body)
	if !ok {
		return nil
	}

	bodyStr := string(body.Data)

	// Remove auth token from at= parameter
	// The auth token looks like: at=AJpMio2G6FWsQX6bhFORlLK5gSjO:1757812453964
	// We want to normalize this to at= (empty) for replay without credentials
	authTokenPattern := regexp.MustCompile(`at=[^&]*`)
	bodyStr = authTokenPattern.ReplaceAllString(bodyStr, `at=`)

	body.Data = []byte(bodyStr)
	return nil
}

// scrubNLMRequestID normalizes request IDs in URLs to make them deterministic.
func scrubNLMRequestID(req *http.Request) error {
	// Normalize the _reqid parameter in the URL
	if req.URL != nil {
		query := req.URL.Query()
		if query.Get("_reqid") != "" {
			query.Set("_reqid", "00000")
			req.URL.RawQuery = query.Encode()
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

// scrubNLMProjectListLimit truncates the project list to the first 10 projects in ListProjects responses.
// This keeps cached data smaller and more manageable.
// This is a content-aware scrubber that understands the HTTP response format.
func scrubNLMProjectListLimit(buf *bytes.Buffer) error {
	content := buf.String()

	// Check if this looks like a ListProjects response (contains wXbhsf which is the RPC ID for ListProjects)
	if !strings.Contains(content, "wXbhsf") {
		return nil // Not a ListProjects response, don't modify
	}

	// HTTP responses have headers and body separated by \r\n\r\n or \n\n
	var headerEnd int
	if idx := strings.Index(content, "\r\n\r\n"); idx >= 0 {
		headerEnd = idx + 4
	} else if idx := strings.Index(content, "\n\n"); idx >= 0 {
		headerEnd = idx + 2
	} else {
		// No body found, return as-is
		return nil
	}

	headers := content[:headerEnd]
	body := content[headerEnd:]

	// Now work on the body only
	// Find project UUIDs in the body
	projectIDPattern := regexp.MustCompile(`[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`)
	matches := projectIDPattern.FindAllStringIndex(body, -1)

	// If we have more than 10 project IDs, truncate
	const maxProjects = 10
	if len(matches) > maxProjects {
		// Find the position of the 10th project ID
		tenthProjectEnd := matches[maxProjects-1][1]

		// Find the 11th project ID start
		eleventhProjectStart := matches[maxProjects][0]

		// Look for a good truncation point between them
		// Typically projects are separated by ],[ pattern
		searchRegion := body[tenthProjectEnd:eleventhProjectStart]

		// Find the last ],[ in this region
		if idx := strings.LastIndex(searchRegion, "],["); idx >= 0 {
			truncatePos := tenthProjectEnd + idx + 1 // Keep the ]

			// Truncate the body
			truncatedBody := body[:truncatePos]

			// Count brackets to close properly
			openBrackets := strings.Count(truncatedBody, "[")
			closeBrackets := strings.Count(truncatedBody, "]")
			needClose := openBrackets - closeBrackets

			// Add closing brackets
			if needClose > 0 {
				truncatedBody += strings.Repeat("]", needClose)
			}

			// Update Content-Length header if present
			newBody := truncatedBody
			newHeaders := headers

			// Update Content-Length if it exists
			if strings.Contains(newHeaders, "Content-Length:") {
				lines := strings.Split(newHeaders, "\n")
				for i, line := range lines {
					if strings.HasPrefix(line, "Content-Length:") {
						lines[i] = fmt.Sprintf("Content-Length: %d", len(newBody))
						if strings.HasSuffix(lines[i+1 : i+2][0], "\r") {
							lines[i] += "\r"
						}
						break
					}
				}
				newHeaders = strings.Join(lines, "\n")
			}

			// Reconstruct the full response
			content = newHeaders + newBody
		}
	}

	buf.Reset()
	buf.WriteString(content)
	return nil
}

// scrubNLMResponseIDs removes or normalizes IDs in NLM API responses that might be non-deterministic.
func scrubNLMResponseIDs(buf *bytes.Buffer) error {
	content := buf.String()

	// Remove or normalize notebook IDs (typically long alphanumeric strings)
	notebookIDPattern := regexp.MustCompile(`"id"\s*:\s*"[a-zA-Z0-9_-]{10,}"`)
	content = notebookIDPattern.ReplaceAllString(content, `"id":"[NOTEBOOK_ID]"`)

	// Remove or normalize source IDs
	sourceIDPattern := regexp.MustCompile(`"sourceId"\s*:\s*"[a-zA-Z0-9_-]{10,}"`)
	content = sourceIDPattern.ReplaceAllString(content, `"sourceId":"[SOURCE_ID]"`)

	// Remove session IDs or similar temporary identifiers
	sessionIDPattern := regexp.MustCompile(`"sessionId"\s*:\s*"[a-zA-Z0-9_-]{10,}"`)
	content = sessionIDPattern.ReplaceAllString(content, `"sessionId":"[SESSION_ID]"`)

	// Remove request IDs that might appear in error responses
	requestIDPattern := regexp.MustCompile(`"requestId"\s*:\s*"[a-zA-Z0-9_-]{10,}"`)
	content = requestIDPattern.ReplaceAllString(content, `"requestId":"[REQUEST_ID]"`)

	buf.Reset()
	buf.WriteString(content)
	return nil
}

// CreateNLMTestClient creates an HTTP client configured for NLM usage with httprr.
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
	funcIDPattern := regexp.MustCompile(`\[\[\"([a-zA-Z0-9]+)\",`)
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
