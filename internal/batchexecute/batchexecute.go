package batchexecute

import (
	"bufio"
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
	ID    string        // RPC endpoint ID
	Args  []interface{} // Arguments for the call
	Index string        // "generic" or numeric index
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
	for k, v := range c.config.URLParams {
		q.Set(k, v)
	}
	q.Set("_reqid", c.reqid.Next())
	u.RawQuery = q.Encode()

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

	// Create request
	req, err := http.NewRequest("POST", u.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set headers
	for k, v := range c.config.Headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("cookie", c.config.Cookies)

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

	if resp.StatusCode != http.StatusOK {
		return nil, &BatchExecuteError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("request failed: %s", resp.Status),
			Response:   resp,
		}
	}

	var responses []Response
	// if "rt" == "c", then it's a chunked response
	if req.URL.Query().Get("rt") == "c" {
		responses, err = decodeChunkedResponse(string(body))
		if err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}
	} else if req.URL.Query().Get("rt") == "" {
		responses, err = decodeResponse(string(body))
		if err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}
	} else {
		return nil, fmt.Errorf("unsupported response type '%s'", req.URL.Query().Get("rt"))
	}
	if len(responses) == 0 {
		return nil, fmt.Errorf("empty response")
	}

	return &responses[0], nil
}

var debug = true

// decodeResponse decodes the batchexecute response
func decodeResponse(raw string) ([]Response, error) {
	raw = strings.TrimPrefix(raw, ")]}'")
	if raw == "" {
		return nil, fmt.Errorf("empty response after trimming prefix")
	}
	var responses [][]interface{}
	if err := json.NewDecoder(strings.NewReader(raw)).Decode(&responses); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
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
func decodeChunkedResponse(raw string) ([]Response, error) {
	raw = strings.TrimSpace(strings.TrimPrefix(raw, ")]}'"))
	if raw == "" {
		return nil, fmt.Errorf("empty response after trimming prefix")
	}

	var responses []Response
	reader := bufio.NewReader(strings.NewReader(raw))

	for {
		// Read the length line
		lengthLine, err := reader.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read length: %w", err)
		}

		// Skip empty lines
		lengthStr := strings.TrimSpace(lengthLine)
		if lengthStr == "" {
			continue
		}

		totalLength, err := strconv.Atoi(lengthStr)
		if err != nil {
			if debug {
				fmt.Printf("Invalid length string: %q\n", lengthStr)
			}
			return nil, fmt.Errorf("invalid chunk length: invalid syntax")
		}

		if debug {
			fmt.Printf("Found chunk length: %d from string: %q\n",
				totalLength, lengthStr)
		}

		// Read exactly totalLength bytes for the chunk
		chunk := make([]byte, totalLength)
		n, err := io.ReadFull(reader, chunk)
		if err != nil {
			if debug {
				fmt.Printf("Failed to read chunk: got %d bytes, wanted %d: %v\n",
					n, totalLength, err)
			}
			return nil, fmt.Errorf("read chunk: %w", err)
		}

		if debug {
			fmt.Printf("Read chunk (%d bytes): %q\n",
				len(chunk), string(chunk[:min(50, len(chunk))]))
		}

		// First try to parse as regular JSON
		var rpcBatch [][]interface{}
		if err := json.Unmarshal(chunk, &rpcBatch); err != nil {
			// If that fails, try unescaping the JSON string first
			unescaped, err := strconv.Unquote("\"" + string(chunk) + "\"")
			if err != nil {
				if debug {
					fmt.Printf("Failed to unescape chunk: %v\n", err)
				}
				return nil, fmt.Errorf("failed to parse chunk: %w", err)
			}
			if err := json.Unmarshal([]byte(unescaped), &rpcBatch); err != nil {
				if debug {
					fmt.Printf("Failed to parse unescaped chunk: %v\n", err)
				}
				return nil, fmt.Errorf("failed to parse chunk: %w", err)
			}
		}

		// Process each RPC response in the batch
		for _, rpcData := range rpcBatch {
			if len(rpcData) < 7 {
				if debug {
					fmt.Printf("Skipping short RPC data: %v\n", rpcData)
				}
				continue
			}
			rpcType, ok := rpcData[0].(string)
			if !ok || rpcType != "wrb.fr" {
				if debug {
					fmt.Printf("Skipping non-wrb.fr RPC: %v\n", rpcData[0])
				}
				continue
			}

			id, _ := rpcData[1].(string)
			resp := Response{
				ID: id,
			}

			// Handle data - parse the nested JSON string
			if rpcData[2] != nil {
				if dataStr, ok := rpcData[2].(string); ok {
					// Try to parse the data string
					var data interface{}
					if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
						// If direct parsing fails, try unescaping first
						unescaped, err := strconv.Unquote("\"" + dataStr + "\"")
						if err != nil {
							if debug {
								fmt.Printf("Failed to unescape data: %v\n", err)
							}
							continue
						}
						if err := json.Unmarshal([]byte(unescaped), &data); err != nil {
							if debug {
								fmt.Printf("Failed to parse unescaped data: %v\n", err)
							}
							continue
						}
					}
					// Re-encode to get properly formatted JSON
					rawData, err := json.Marshal(data)
					if err != nil {
						if debug {
							fmt.Printf("Failed to re-encode response data: %v\n", err)
						}
						continue
					}
					resp.Data = rawData
				}
			}

			// Handle index
			if rpcData[6] == "generic" {
				resp.Index = 0
			} else if indexStr, ok := rpcData[6].(string); ok {
				resp.Index, _ = strconv.Atoi(indexStr)
			}

			responses = append(responses, resp)
		}
	}

	if len(responses) == 0 {
		return nil, fmt.Errorf("no valid responses found")
	}

	return responses, nil
}

func handleChunk(chunk []byte, responses *[]Response) error {
	if debug {
		fmt.Printf("Processing chunk (%d bytes): %q\n", len(chunk),
			string(chunk[:min(100, len(chunk))]))
	}

	// Parse the chunk
	var rpcBatch [][]interface{}
	if err := json.Unmarshal(chunk, &rpcBatch); err != nil {
		return fmt.Errorf("parse chunk: %w", err)
	}

	// Process each RPC response in the batch
	for _, rpcData := range rpcBatch {
		if len(rpcData) < 7 {
			if debug {
				fmt.Printf("Skipping short RPC data: %v\n", rpcData)
			}
			continue
		}
		rpcType, ok := rpcData[0].(string)
		if !ok || rpcType != "wrb.fr" {
			if debug {
				fmt.Printf("Skipping non-wrb.fr RPC: %v\n", rpcData[0])
			}
			continue
		}

		id, _ := rpcData[1].(string)
		resp := Response{
			ID: id,
		}

		// Handle data
		if rpcData[2] != nil {
			if dataStr, ok := rpcData[2].(string); ok {
				resp.Data = json.RawMessage(dataStr)
			}
		}

		// Handle index
		if rpcData[6] == "generic" {
			resp.Index = 0
		} else if indexStr, ok := rpcData[6].(string); ok {
			resp.Index, _ = strconv.Atoi(indexStr)
		}

		*responses = append(*responses, resp)
	}

	return nil
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
