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
	"strings"
	"time"
)

const (
	// Google Signaler API for credential refresh
	SignalerAPIURL = "https://signaler-pa.clients6.google.com/punctual/v1/refreshCreds"
	SignalerAPIKey = "AIzaSyC_pzrI0AjEDXDYcg7kkq3uQEjnXV50pBM"
)

// RefreshClient handles credential refreshing
type RefreshClient struct {
	cookies    string
	sapisid    string
	httpClient *http.Client
	debug      bool
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
	req.Header.Set("Origin", "https://notebooklm.google.com")
	req.Header.Set("Referer", "https://notebooklm.google.com/")
	req.Header.Set("X-Goog-AuthUser", "0")
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
	origin := "https://notebooklm.google.com"
	data := fmt.Sprintf("%d %s %s", timestamp, r.sapisid, origin)
	
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

// ExtractGSessionID extracts the gsessionid from NotebookLM by fetching the page
func ExtractGSessionID(cookies string) (string, error) {
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
	req, err := http.NewRequest("GET", "https://notebooklm.google.com/", nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	
	// Set headers
	req.Header.Set("Cookie", cookies)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36")
	
	// Send request
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch page: %w", err)
	}
	defer resp.Body.Close()
	
	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	
	// Look for gsessionid in the page
	// Pattern: "gsessionid":"<value>"
	pattern := regexp.MustCompile(`"gsessionid"\s*:\s*"([^"]+)"`)
	matches := pattern.FindSubmatch(body)
	if len(matches) > 1 {
		return string(matches[1]), nil
	}
	
	// Alternative pattern: gsessionid='<value>'
	pattern2 := regexp.MustCompile(`gsessionid\s*=\s*['"]([^'"]+)['"]`)
	matches2 := pattern2.FindSubmatch(body)
	if len(matches2) > 1 {
		return string(matches2[1]), nil
	}
	
	// If not found, return error
	return "", fmt.Errorf("gsessionid not found in page")
}

// RefreshLoop runs a background refresh loop to keep credentials alive
func (r *RefreshClient) RefreshLoop(gsessionID string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	for range ticker.C {
		if err := r.RefreshCredentials(gsessionID); err != nil {
			if r.debug {
				fmt.Printf("Failed to refresh credentials: %v\n", err)
			}
		}
	}
}

// AutoRefreshConfig holds configuration for auto-refresh
type AutoRefreshConfig struct {
	Enabled  bool          `json:"enabled"`
	Interval time.Duration `json:"interval"`
	Debug    bool          `json:"debug"`
}

// DefaultAutoRefreshConfig returns default auto-refresh settings
func DefaultAutoRefreshConfig() AutoRefreshConfig {
	return AutoRefreshConfig{
		Enabled:  true,
		Interval: 10 * time.Minute, // Refresh every 10 minutes
		Debug:    false,
	}
}

// StartAutoRefresh starts automatic credential refresh in the background
func StartAutoRefresh(cookies string, gsessionID string, config AutoRefreshConfig) error {
	if !config.Enabled {
		return nil
	}
	
	client, err := NewRefreshClient(cookies)
	if err != nil {
		return fmt.Errorf("failed to create refresh client: %w", err)
	}
	
	client.debug = config.Debug
	
	// Do an initial refresh to verify it works
	if err := client.RefreshCredentials(gsessionID); err != nil {
		return fmt.Errorf("initial refresh failed: %w", err)
	}
	
	// Start the refresh loop in a goroutine
	go client.RefreshLoop(gsessionID, config.Interval)
	
	return nil
}