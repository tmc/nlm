package batchexecute

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrUnauthorized represent an unauthorized request.
var ErrUnauthorized = errors.New("unauthorized")

// RPC represents a single RPC call
type RPC struct {
	ID        string            // RPC endpoint ID
	Args      []interface{}     // Arguments for the call
	Index     string            // "generic" or numeric index
	URLParams map[string]string // Request-specific URL parameters
}

// Response represents a decoded RPC response
type Response struct {
	Index int             `json:"index"`
	ID    string          `json:"id"`
	Data  json.RawMessage `json:"data"`
	Error string          `json:"error"`
}

// BatchExecuteError represents a batchexecute error
type BatchExecuteError struct {
	StatusCode int
	Message    string
	Response   *http.Response
}

func (e *BatchExecuteError) Error() string {
	return fmt.Sprintf("batchexecute error: %s (status: %d)", e.Message, e.StatusCode)
}

func (e *BatchExecuteError) Unwrap() error {
	if e.StatusCode == 401 {
		return ErrUnauthorized
	}
	return nil
}

// Do executes a single RPC call
func (c *Client) Do(rpc RPC) (*Response, error) {
	return c.Execute([]RPC{rpc})
}

// maskSensitiveValue masks sensitive values like tokens for debug output
func maskSensitiveValue(value string) string {
	if len(value) <= 8 {
		return strings.Repeat("*", len(value))
	} else if len(value) <= 16 {
		start := value[:2]
		end := value[len(value)-2:]
		return start + strings.Repeat("*", len(value)-4) + end
	} else {
		start := value[:3]
		end := value[len(value)-3:]
		return start + strings.Repeat("*", len(value)-6) + end
	}
}

// maskCookieValues masks cookie values in cookie header for debug output
func maskCookieValues(cookies string) string {
	// Split cookies by semicolon
	parts := strings.Split(cookies, ";")
	var masked []string

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if name, value, found := strings.Cut(part, "="); found {
			maskedValue := maskSensitiveValue(value)
			masked = append(masked, name+"="+maskedValue)
		} else {
			masked = append(masked, part) // Keep parts without = as-is
		}
	}

	return strings.Join(masked, "; ")
}

func buildRPCData(rpc RPC) []interface{} {
	// Convert args to JSON string
	argsJSON, _ := json.Marshal(rpc.Args)

	return []interface{}{
		rpc.ID,
		string(argsJSON),
		nil,
		"generic",
	}
}

// Execute performs the batch execute request
func (c *Client) Execute(rpcs []RPC) (*Response, error) {
	u, err := url.Parse(fmt.Sprintf("https://%s/_/%s/data/batchexecute", c.config.Host, c.config.App))
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	if c.config.UseHTTP {
		u.Scheme = "http"
	}

	// Add query parameters
	q := u.Query()
	q.Set("rpcids", strings.Join([]string{rpcs[0].ID}, ","))

	// Add all URL parameters (including rt parameter if set)
	for k, v := range c.config.URLParams {
		q.Set(k, v)
	}
	if len(rpcs) > 0 && rpcs[0].URLParams != nil {
		for k, v := range rpcs[0].URLParams {
			q.Set(k, v)
		}
	}
	// Note: rt parameter is now controlled via URLParams from client configuration
	// If not set, we'll get JSON array format (easier to parse)
	q.Set("_reqid", c.reqid.Next())
	u.RawQuery = q.Encode()

	if c.config.Debug {
		fmt.Fprintf(os.Stderr,"\n=== BatchExecute Request ===\n")
		fmt.Fprintf(os.Stderr,"URL: %s\n", u.String())
	}

	// Build request body
	var envelope []interface{}
	for _, rpc := range rpcs {
		envelope = append(envelope, buildRPCData(rpc))
	}

	reqBody, err := json.Marshal([]interface{}{envelope})
	if err != nil {
		return nil, fmt.Errorf("marshal request body: %w", err)
	}

	form := url.Values{}
	form.Set("f.req", string(reqBody))
	form.Set("at", c.config.AuthToken)

	if c.config.Debug {
		// Safely display auth token with conservative masking
		token := c.config.AuthToken
		var tokenDisplay string
		if len(token) <= 8 {
			// For very short tokens, mask completely
			tokenDisplay = strings.Repeat("*", len(token))
		} else if len(token) <= 16 {
			// For short tokens, show first 2 and last 2 chars
			start := token[:2]
			end := token[len(token)-2:]
			tokenDisplay = start + strings.Repeat("*", len(token)-4) + end
		} else {
			// For long tokens, show first 3 and last 3 chars
			start := token[:3]
			end := token[len(token)-3:]
			tokenDisplay = start + strings.Repeat("*", len(token)-6) + end
		}
		fmt.Fprintf(os.Stderr,"\nAuth Token: %s\n", tokenDisplay)

		// Mask auth token in request body display
		maskedForm := url.Values{}
		for k, v := range form {
			if k == "at" && len(v) > 0 {
				// Mask the auth token value
				maskedValue := maskSensitiveValue(v[0])
				maskedForm.Set(k, maskedValue)
			} else {
				maskedForm[k] = v
			}
		}
		fmt.Fprintf(os.Stderr,"\nRequest Body:\n%s\n", maskedForm.Encode())
		fmt.Fprintf(os.Stderr,"\nDecoded Request Body:\n%s\n", string(reqBody))
	}

	// Create request
	req, err := http.NewRequest("POST", u.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set headers
	req.Header.Set("content-type", "application/x-www-form-urlencoded;charset=UTF-8")
	for k, v := range c.config.Headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("cookie", c.config.Cookies)

	if c.config.Debug {
		fmt.Fprintf(os.Stderr,"\nRequest Headers:\n")
		for k, v := range req.Header {
			if strings.ToLower(k) == "cookie" && len(v) > 0 {
				// Mask cookie values for security
				maskedCookies := maskCookieValues(v[0])
				fmt.Fprintf(os.Stderr,"%s: [%s]\n", k, maskedCookies)
			} else {
				fmt.Fprintf(os.Stderr,"%s: %v\n", k, v)
			}
		}
	}

	// Execute request with retry logic
	var resp *http.Response
	var lastErr error

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate retry delay with exponential backoff
			multiplier := 1 << uint(attempt-1)
			delay := time.Duration(float64(c.config.RetryDelay) * float64(multiplier))
			if delay > c.config.RetryMaxDelay {
				delay = c.config.RetryMaxDelay
			}

			if c.config.Debug {
				fmt.Fprintf(os.Stderr,"\nRetrying request (attempt %d/%d) after %v...\n", attempt, c.config.MaxRetries, delay)
			}
			time.Sleep(delay)
		}

		// Clone the request for each attempt
		reqClone := req.Clone(req.Context())
		if req.Body != nil {
			reqClone.Body = io.NopCloser(strings.NewReader(form.Encode()))
		}

		resp, err = c.httpClient.Do(reqClone)
		if err != nil {
			lastErr = err
			// Check for common network errors and provide more helpful messages
			if strings.Contains(err.Error(), "dial tcp") {
				if strings.Contains(err.Error(), "i/o timeout") {
					lastErr = fmt.Errorf("connection timeout - check your network connection and try again: %w", err)
				} else if strings.Contains(err.Error(), "connect: bad file descriptor") {
					lastErr = fmt.Errorf("network connection error - try restarting your network connection: %w", err)
				}
			} else {
				lastErr = fmt.Errorf("execute request: %w", err)
			}

			// Check if error is retryable
			if isRetryableError(err) && attempt < c.config.MaxRetries {
				continue
			}
			return nil, lastErr
		}

		// Check if response status is retryable
		if isRetryableStatus(resp.StatusCode) && attempt < c.config.MaxRetries {
			resp.Body.Close()
			lastErr = fmt.Errorf("server returned status %d", resp.StatusCode)
			continue
		}

		// Success or non-retryable error
		break
	}

	if resp == nil {
		return nil, fmt.Errorf("all retry attempts failed: %w", lastErr)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if c.config.Debug {
		fmt.Fprintf(os.Stderr,"\nResponse Status: %s\n", resp.Status)
		fmt.Fprintf(os.Stderr,"Raw Response Body:\n%q\n", string(body))
		fmt.Fprintf(os.Stderr,"Response Body:\n%s\n", string(body))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &BatchExecuteError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("request failed: %s", resp.Status),
			Response:   resp,
		}
	}

	// Try to parse the response
	responses, err := decodeResponse(string(body))
	if err != nil {
		if c.config.Debug {
			fmt.Fprintf(os.Stderr,"Failed to decode response: %v\n", err)
			fmt.Fprintf(os.Stderr,"Raw response: %s\n", string(body))
		}

		// Special handling for certain responses
		if strings.Contains(string(body), "\"error\"") {
			// It contains an error field, let's try to extract it
			var errorResp struct {
				Error string `json:"error"`
			}
			if jerr := json.Unmarshal(body, &errorResp); jerr == nil && errorResp.Error != "" {
				return nil, fmt.Errorf("server error: %s", errorResp.Error)
			}
		}

		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(responses) == 0 {
		if c.config.Debug {
			fmt.Fprintf(os.Stderr,"No valid responses found in: %s\n", string(body))
		}
		return nil, fmt.Errorf("no valid responses found")
	}

	// Check the first response for API errors
	firstResponse := &responses[0]
	if apiError, isError := IsErrorResponse(firstResponse); isError {
		if c.config.Debug {
			fmt.Fprintf(os.Stderr,"Detected API error: %s\n", apiError.Error())
		}
		return nil, apiError
	}

	// Debug dump payload if requested
	if c.config.DebugDumpPayload {
		fmt.Fprint(os.Stderr, string(firstResponse.Data))
		return nil, fmt.Errorf("payload dumped")
	}

	return firstResponse, nil
}

// decodeResponse decodes the batchexecute response
func decodeResponse(raw string) ([]Response, error) {
	raw = strings.TrimSpace(strings.TrimPrefix(raw, ")]}'"))
	if raw == "" {
		return nil, fmt.Errorf("empty response after trimming prefix")
	}

	// Try to parse as a chunked response first
	if isDigit(rune(raw[0])) {
		reader := strings.NewReader(raw)
		return decodeChunkedResponse(reader)
	}

	// Try to parse as a regular response
	var responses [][]interface{}
	if err := json.NewDecoder(strings.NewReader(raw)).Decode(&responses); err != nil {
		// Check if this might be a numeric response (happens with API errors)
		trimmedRaw := strings.TrimSpace(raw)
		if code, parseErr := strconv.Atoi(trimmedRaw); parseErr == nil {
			// This is a numeric response, potentially an error code
			return []Response{
				{
					ID:   "numeric",
					Data: json.RawMessage(fmt.Sprintf("%d", code)),
				},
			}, nil
		}

		// Try to parse as a single array
		var singleArray []interface{}
		if err := json.NewDecoder(strings.NewReader(raw)).Decode(&singleArray); err == nil {
			// Convert it to our expected format
			responses = [][]interface{}{singleArray}
		} else {
			return nil, fmt.Errorf("decode response: %w", err)
		}
	}

	var result []Response
	for _, rpcData := range responses {
		if len(rpcData) < 7 {
			continue
		}
		rpcType, ok := rpcData[0].(string)
		if !ok || rpcType != "wrb.fr" {
			continue
		}

		id, _ := rpcData[1].(string)
		resp := Response{
			ID: id,
		}

		// Intelligently parse response data from multiple possible positions
		// Format: ["wrb.fr", "rpcId", response_data, null, null, actual_data, "generic"]
		var responseData interface{}

		// Try position 2 first (traditional location)
		if rpcData[2] != nil {
			if dataStr, ok := rpcData[2].(string); ok {
				resp.Data = json.RawMessage(dataStr)
				responseData = dataStr
			} else {
				// If position 2 is not a string, use it directly
				responseData = rpcData[2]
			}
		}

		// If position 2 is null/empty, try position 5 (actual data)
		if responseData == nil && len(rpcData) > 5 && rpcData[5] != nil {
			responseData = rpcData[5]
		}

		// Convert responseData to JSON if it's not already a string
		if responseData != nil && resp.Data == nil {
			if dataBytes, err := json.Marshal(responseData); err == nil {
				resp.Data = json.RawMessage(dataBytes)
			}
		}

		if rpcData[6] == "generic" {
			resp.Index = 0
		} else if indexStr, ok := rpcData[6].(string); ok {
			resp.Index, _ = strconv.Atoi(indexStr)
		}

		result = append(result, resp)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no valid responses found")
	}

	return result, nil
}

// decodeChunkedResponse decodes the batchexecute response
func decodeChunkedResponse(r io.Reader) ([]Response, error) {
	return parseChunkedResponse(r)
}

func isDigit(c rune) bool {
	return c >= '0' && c <= '9'
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Option configures a Client
type Option func(*Client)

// WithHTTPClient sets the HTTP client
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		c.httpClient = client
	}
}

// WithDebug enables debug output
func WithDebug(debug bool) Option {
	return func(c *Client) {
		c.config.Debug = debug
		if debug {
			c.debug = func(format string, args ...interface{}) {
				fmt.Fprintf(os.Stderr, "DEBUG: "+format+"\n", args...)
			}
		}
	}
}

func WithDebugDumpPayload(debugDumpPayload bool) Option {
	return func(c *Client) {
		c.config.DebugDumpPayload = debugDumpPayload
	}
}

// WithTimeout sets the HTTP client timeout
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		if c.httpClient == http.DefaultClient {
			c.httpClient = &http.Client{
				Timeout: timeout,
			}
		} else {
			c.httpClient.Timeout = timeout
		}
	}
}

// WithHeaders adds additional headers
func WithHeaders(headers map[string]string) Option {
	return func(c *Client) {
		if c.config.Headers == nil {
			c.config.Headers = make(map[string]string)
		}
		for k, v := range headers {
			c.config.Headers[k] = v
		}
	}
}

// WithURLParams adds additional URL parameters
func WithURLParams(params map[string]string) Option {
	return func(c *Client) {
		if c.config.URLParams == nil {
			c.config.URLParams = make(map[string]string)
		}
		for k, v := range params {
			c.config.URLParams[k] = v
		}
	}
}

// WithReqIDGenerator sets the request ID generator
func WithReqIDGenerator(reqid *ReqIDGenerator) Option {
	return func(c *Client) {
		c.reqid = reqid
	}
}

// Config holds the configuration for batch execute
type Config struct {
	Host      string
	App       string
	AuthToken string
	Cookies   string
	Headers   map[string]string
	URLParams map[string]string
	Debug     bool
	UseHTTP   bool

	// Retry configuration
	MaxRetries    int           // Maximum number of retry attempts (default: 3)
	RetryDelay    time.Duration // Initial delay between retries (default: 1s)
	RetryMaxDelay time.Duration // Maximum delay between retries (default: 10s)

	// Debug payload dumping
	DebugDumpPayload bool // If true, dumps raw payload and exits
}

// Client handles batchexecute operations
type Client struct {
	config     Config
	httpClient *http.Client
	debug      func(format string, args ...interface{})
	reqid      *ReqIDGenerator
}

// NewClient creates a new batchexecute client
func NewClient(config Config, opts ...Option) *Client {
	// Set default retry configuration if not specified
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = 1 * time.Second
	}
	if config.RetryMaxDelay == 0 {
		config.RetryMaxDelay = 10 * time.Second
	}

	c := &Client{
		config:     config,
		httpClient: http.DefaultClient,
		debug:      func(format string, args ...interface{}) {}, // noop by default
		reqid:      NewReqIDGenerator(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) Config() Config {
	return c.config
}

// ReqIDGenerator generates sequential request IDs
type ReqIDGenerator struct {
	base     int // Initial 4-digit number
	sequence int // Current sequence number
	mu       sync.Mutex
}

// NewReqIDGenerator creates a new request ID generator
func NewReqIDGenerator() *ReqIDGenerator {
	// Generate random 4-digit number (1000-9999)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	base := r.Intn(9000) + 1000

	return &ReqIDGenerator{
		base:     base,
		sequence: 0,
		mu:       sync.Mutex{},
	}
}

// Next returns the next request ID in sequence
func (g *ReqIDGenerator) Next() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	reqid := g.base + (g.sequence * 100000)
	g.sequence++
	return strconv.Itoa(reqid)
}

// Reset resets the sequence counter but keeps the same base
func (g *ReqIDGenerator) Reset() {
	g.mu.Lock()
	g.sequence = 0
	g.mu.Unlock()
}

// readUntil reads from the reader until the delimiter is found
func readUntil(r io.Reader, delim byte) (string, error) {
	var result strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := r.Read(buf)
		if err != nil {
			if err == io.EOF && result.Len() > 0 {
				return result.String(), nil
			}
			return "", err
		}
		if n == 0 {
			continue
		}
		if buf[0] == delim {
			return result.String(), nil
		}
		result.WriteByte(buf[0])
	}
}

// isRetryableError checks if an error is retryable
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Network-related errors that are retryable
	retryablePatterns := []string{
		"connection refused",
		"connection reset",
		"i/o timeout",
		"TLS handshake timeout",
		"EOF",
		"broken pipe",
		"no such host",
		"network is unreachable",
		"temporary failure",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// isRetryableStatus checks if an HTTP status code is retryable
func isRetryableStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}
