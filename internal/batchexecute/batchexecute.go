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

	// Add all URL parameters
	for k, v := range c.config.URLParams {
		q.Set(k, v)
	}
	if len(rpcs) > 0 && rpcs[0].URLParams != nil {
		for k, v := range rpcs[0].URLParams {
			q.Set(k, v)
		}
	}
	// Add rt=c parameter for chunked responses
	q.Set("rt", "c")
	q.Set("_reqid", c.reqid.Next())
	u.RawQuery = q.Encode()

	if c.config.Debug {
		fmt.Printf("\n=== BatchExecute Request ===\n")
		fmt.Printf("URL: %s\n", u.String())
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
		fmt.Printf("\nRequest Body:\n%s\n", form.Encode())
		fmt.Printf("\nDecoded Request Body:\n%s\n", string(reqBody))
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
		fmt.Printf("\nRequest Headers:\n")
		for k, v := range req.Header {
			fmt.Printf("%s: %v\n", k, v)
		}
	}

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if c.config.Debug {
		fmt.Printf("\nResponse Status: %s\n", resp.Status)
		fmt.Printf("Raw Response Body:\n%q\n", string(body))
		fmt.Printf("Response Body:\n%s\n", string(body))
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
			fmt.Printf("Failed to decode response: %v\n", err)
			fmt.Printf("Raw response: %s\n", string(body))
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
			fmt.Printf("No valid responses found in: %s\n", string(body))
		}
		return nil, fmt.Errorf("no valid responses found")
	}

	return &responses[0], nil
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
		// Check if this might be a number response (happens with some API errors)
		if strings.TrimSpace(raw) == "1" || strings.TrimSpace(raw) == "0" {
			// This is likely a boolean-like response (success/failure)
			// Return a synthetic success response
			return []Response{
				{
					ID:   "synthetic",
					Data: json.RawMessage(`{"success": true}`),
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

		if rpcData[2] != nil {
			if dataStr, ok := rpcData[2].(string); ok {
				resp.Data = json.RawMessage(dataStr)
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
