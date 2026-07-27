// Package auth handles authentication and credential refresh for NotebookLM
package auth

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// Google Signaler API for credential refresh.
	SignalerAPIURL = "https://signaler-pa.clients6.google.com/punctual/v1/refreshCreds"

	// SignalerAPIKey is Google's public API key for the Signaler service,
	// embedded in the NotebookLM web client. This is not a secret — it
	// identifies the application, not the user. User authentication is
	// handled by session cookies.
	SignalerAPIKey = "AIzaSyC_pzrI0AjEDXDYcg7kkq3uQEjnXV50pBM"
)

// RefreshClient handles credential refreshing
type RefreshClient struct {
	cookies    string
	sapisid    string
	httpClient *http.Client
	debug      bool
}

// NotebookLMPageState is the page bootstrap state needed by batchexecute and
// related signaling flows.
type NotebookLMPageState struct {
	GSessionID string
	SessionID  string
	BLParam    string
}

// NewRefreshClient creates a new refresh client
func NewRefreshClient(cookies string) (*RefreshClient, error) {
	// Extract SAPISID from cookies
	sapisid := extractCookieValue(cookies, "SAPISID")
	if sapisid == "" {
		return nil, fmt.Errorf("SAPISID not found in cookies")
	}

	return &RefreshClient{
		cookies:    cookies,
		sapisid:    sapisid,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}, nil
}

// SetDebug enables or disables debug output
func (r *RefreshClient) SetDebug(debug bool) {
	r.debug = debug
}

// RefreshCredentials refreshes the authentication credentials
func (r *RefreshClient) RefreshCredentials(gsessionID string) error {
	// Build the URL with parameters
	params := url.Values{}
	params.Set("key", SignalerAPIKey)
	if gsessionID != "" {
		params.Set("gsessionid", gsessionID)
	}

	fullURL := SignalerAPIURL + "?" + params.Encode()

	// Generate SAPISIDHASH for authorization
	timestamp := time.Now().Unix()
	authHash := r.generateSAPISIDHASH(timestamp)

	// Create request body
	// The body appears to be a session identifier
	requestBody := []string{"tZf5V3ry"} // This might need to be dynamic
	bodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Create the HTTP request
	req, err := http.NewRequest("POST", fullURL, bytes.NewReader(bodyJSON))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Authorization", fmt.Sprintf("SAPISIDHASH %d_%s", timestamp, authHash))
	req.Header.Set("Content-Type", "application/json+protobuf")
	req.Header.Set("Cookie", r.cookies)
	req.Header.Set("Origin", "https://notebook.google.com")
	req.Header.Set("Referer", "https://notebook.google.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36")

	if r.debug {
		fmt.Printf("=== Credential Refresh Request ===\n")
		fmt.Printf("URL: %s\n", fullURL)
		fmt.Printf("Authorization: SAPISIDHASH %d_%s\n", timestamp, authHash)
		fmt.Printf("Body: %s\n", string(bodyJSON))
	}

	// Send the request
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send refresh request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if r.debug {
		fmt.Printf("=== Credential Refresh Response ===\n")
		fmt.Printf("Status: %s\n", resp.Status)
		fmt.Printf("Body: %s\n", string(body))
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response to check for success
	// The response format needs to be determined from actual API responses
	if r.debug {
		fmt.Println("Credentials refreshed successfully")
	}

	return nil
}

// generateSAPISIDHASH generates the authorization hash
// Format: SHA1(timestamp + " " + SAPISID + " " + origin)
func (r *RefreshClient) generateSAPISIDHASH(timestamp int64) string {
	return generateSAPISIDHASH(r.sapisid, timestamp)
}

func generateSAPISIDHASH(sapisid string, timestamp int64) string {
	origin := "https://notebook.google.com"
	data := fmt.Sprintf("%d %s %s", timestamp, sapisid, origin)

	hash := sha1.New()
	hash.Write([]byte(data))
	return fmt.Sprintf("%x", hash.Sum(nil))
}

// extractCookieValue extracts a specific cookie value from a cookie string
func extractCookieValue(cookies, name string) string {
	// Split cookies by semicolon
	parts := strings.Split(cookies, ";")
	for _, part := range parts {
		// Trim spaces
		part = strings.TrimSpace(part)
		// Check if this is the cookie we're looking for
		if strings.HasPrefix(part, name+"=") {
			return strings.TrimPrefix(part, name+"=")
		}
	}
	return ""
}

// ExtractNotebookLMPageState fetches NotebookLM and extracts bootstrap values
// from the page HTML.
func ExtractNotebookLMPageState(cookies string) (NotebookLMPageState, error) {
	body, finalURL, err := fetchNotebookLMPage(cookies)
	if err != nil {
		return NotebookLMPageState{}, err
	}
	if err := validateNotebookLMPageURL(finalURL); err != nil {
		return NotebookLMPageState{}, err
	}
	state := parseNotebookLMPageState(body)
	if state.GSessionID == "" && state.SessionID == "" && state.BLParam == "" {
		return NotebookLMPageState{}, fmt.Errorf("notebooklm page state not found in page")
	}
	return state, nil
}

func fetchNotebookLMPage(cookies string) ([]byte, string, error) {
	// Create HTTP client with longer timeout and redirect following
	client := &http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Allow up to 10 redirects
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			// Copy cookies to the redirect request
			if len(via) > 0 {
				req.Header.Set("Cookie", via[0].Header.Get("Cookie"))
			}
			return nil
		},
	}

	// Create request to NotebookLM
	req, err := http.NewRequest("GET", "https://notebook.google.com/", nil)
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}

	// Set headers
	req.Header.Set("Cookie", cookies)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36")

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch page: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("fetch page: status %d", resp.StatusCode)
	}

	finalURL := ""
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	return body, finalURL, nil
}

func validateNotebookLMPageURL(finalURL string) error {
	if strings.TrimSpace(finalURL) == "" {
		return nil
	}
	u, err := url.Parse(finalURL)
	if err != nil {
		return fmt.Errorf("parse notebooklm final url: %w", err)
	}
	if !strings.EqualFold(u.Host, "notebook.google.com") {
		return fmt.Errorf("unexpected notebooklm final url %q", finalURL)
	}
	return nil
}

func parseNotebookLMPageState(body []byte) NotebookLMPageState {
	return NotebookLMPageState{
		GSessionID: firstNotebookLMPageMatch(body,
			regexp.MustCompile(`"gsessionid"\s*:\s*"([^"]+)"`),
			regexp.MustCompile(`gsessionid\s*=\s*['"]([^'"]+)['"]`),
		),
		SessionID: extractNotebookLMJSONStringField(body, "FdrFJe"),
		BLParam:   extractNotebookLMJSONStringField(body, "cfb2h"),
	}
}

func firstNotebookLMPageMatch(body []byte, patterns ...*regexp.Regexp) string {
	for _, pattern := range patterns {
		matches := pattern.FindSubmatch(body)
		if len(matches) > 1 {
			return string(matches[1])
		}
	}
	return ""
}

func extractNotebookLMJSONStringField(body []byte, field string) string {
	pattern := regexp.MustCompile(fmt.Sprintf(`"%s"\s*:\s*"((?:\\.|[^"\\])*)"`, regexp.QuoteMeta(field)))
	matches := pattern.FindSubmatch(body)
	if len(matches) < 2 {
		return ""
	}
	value, err := strconv.Unquote(`"` + string(matches[1]) + `"`)
	if err != nil {
		return string(matches[1])
	}
	return value
}

// ExtractGSessionID extracts the gsessionid from NotebookLM by fetching the page.
func ExtractGSessionID(cookies string) (string, error) {
	state, err := ExtractNotebookLMPageState(cookies)
	if err != nil {
		return "", err
	}
	if state.GSessionID == "" {
		return "", fmt.Errorf("gsessionid not found in page")
	}
	return state.GSessionID, nil
}
