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
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
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

// TokenManager handles automatic token refresh based on expiration
type TokenManager struct {
	mu           sync.RWMutex
	stopChan     chan struct{}
	running      bool
	debug        bool
	refreshAhead time.Duration // How far ahead of expiry to refresh (e.g., 5 minutes)
}

// NewTokenManager creates a new token manager
func NewTokenManager(debug bool) *TokenManager {
	return &TokenManager{
		debug:        debug,
		refreshAhead: 5 * time.Minute, // Refresh 5 minutes before expiry
		stopChan:     make(chan struct{}),
	}
}

// ParseAuthToken parses the auth token to extract expiration time
// Token format: "token:timestamp" where timestamp is Unix milliseconds
func ParseAuthToken(token string) (string, time.Time, error) {
	parts := strings.Split(token, ":")
	if len(parts) != 2 {
		return "", time.Time{}, fmt.Errorf("invalid token format")
	}
	
	timestamp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("invalid timestamp: %w", err)
	}
	
	// Convert milliseconds to time.Time
	// Tokens typically expire after 1 hour
	expiryTime := time.Unix(timestamp/1000, (timestamp%1000)*1e6).Add(1 * time.Hour)
	
	return parts[0], expiryTime, nil
}

// GetStoredToken reads the stored auth token from disk
func GetStoredToken() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	
	envFile := filepath.Join(homeDir, ".nlm", "env")
	data, err := os.ReadFile(envFile)
	if err != nil {
		return "", err
	}
	
	// Parse the env file for NLM_AUTH_TOKEN
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "NLM_AUTH_TOKEN=") {
			token := strings.TrimPrefix(line, "NLM_AUTH_TOKEN=")
			// Remove quotes if present
			token = strings.Trim(token, `"`)
			return token, nil
		}
	}
	
	return "", fmt.Errorf("auth token not found in env file")
}

// GetStoredCookies reads the stored cookies from disk
func GetStoredCookies() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	
	envFile := filepath.Join(homeDir, ".nlm", "env")
	data, err := os.ReadFile(envFile)
	if err != nil {
		return "", err
	}
	
	// Parse the env file for NLM_COOKIES
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "NLM_COOKIES=") {
			cookies := strings.TrimPrefix(line, "NLM_COOKIES=")
			// Remove quotes if present
			cookies = strings.Trim(cookies, `"`)
			return cookies, nil
		}
	}
	
	return "", fmt.Errorf("cookies not found in env file")
}

// StartAutoRefreshManager starts the automatic token refresh manager
func (tm *TokenManager) StartAutoRefreshManager() error {
	tm.mu.Lock()
	if tm.running {
		tm.mu.Unlock()
		return fmt.Errorf("auto-refresh manager already running")
	}
	tm.running = true
	tm.mu.Unlock()
	
	go tm.monitorTokenExpiry()
	
	if tm.debug {
		fmt.Fprintf(os.Stderr, "Auto-refresh manager started\n")
	}
	
	return nil
}

// monitorTokenExpiry monitors token expiration and refreshes when needed
func (tm *TokenManager) monitorTokenExpiry() {
	ticker := time.NewTicker(1 * time.Minute) // Check every minute
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			if err := tm.checkAndRefresh(); err != nil {
				if tm.debug {
					fmt.Fprintf(os.Stderr, "Auto-refresh check failed: %v\n", err)
				}
			}
		case <-tm.stopChan:
			if tm.debug {
				fmt.Fprintf(os.Stderr, "Auto-refresh manager stopped\n")
			}
			return
		}
	}
}

// checkAndRefresh checks if token needs refresh and performs it if necessary
func (tm *TokenManager) checkAndRefresh() error {
	// Get current token
	token, err := GetStoredToken()
	if err != nil {
		return fmt.Errorf("failed to get stored token: %w", err)
	}
	
	// Parse token to get expiry time
	_, expiryTime, err := ParseAuthToken(token)
	if err != nil {
		return fmt.Errorf("failed to parse token: %w", err)
	}
	
	// Check if we need to refresh
	timeUntilExpiry := time.Until(expiryTime)
	if timeUntilExpiry > tm.refreshAhead {
		if tm.debug {
			fmt.Fprintf(os.Stderr, "Token still valid for %v, no refresh needed\n", timeUntilExpiry)
		}
		return nil
	}
	
	if tm.debug {
		fmt.Fprintf(os.Stderr, "Token expiring in %v, refreshing now...\n", timeUntilExpiry)
	}
	
	// Get cookies for refresh
	cookies, err := GetStoredCookies()
	if err != nil {
		return fmt.Errorf("failed to get stored cookies: %w", err)
	}
	
	// Create refresh client
	refreshClient, err := NewRefreshClient(cookies)
	if err != nil {
		return fmt.Errorf("failed to create refresh client: %w", err)
	}
	
	if tm.debug {
		refreshClient.SetDebug(true)
	}
	
	// Use hardcoded gsessionID for now (TODO: extract dynamically)
	gsessionID := "LsWt3iCG3ezhLlQau_BO2Gu853yG1uLi0RnZlSwqVfg"
	
	// Perform refresh
	if err := refreshClient.RefreshCredentials(gsessionID); err != nil {
		return fmt.Errorf("failed to refresh credentials: %w", err)
	}
	
	if tm.debug {
		fmt.Fprintf(os.Stderr, "Credentials refreshed successfully\n")
	}
	
	return nil
}

// Stop stops the auto-refresh manager
func (tm *TokenManager) Stop() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	if tm.running {
		close(tm.stopChan)
		tm.running = false
	}
}