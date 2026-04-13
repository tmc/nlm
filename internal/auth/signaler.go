package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	signalerChooseServerURL = "https://signaler-pa.clients6.google.com/punctual/v1/chooseServer"
	signalerChannelURL      = "https://signaler-pa.clients6.google.com/punctual/multi-watch/channel"
	signalerChannelVersion  = "8"
	signalerClientVersion   = "22"
)

// SignalerClient maintains the NotebookLM long-poll channel used for state sync.
type SignalerClient struct {
	authorization string
	cookies       string
	httpClient    *http.Client
	debug         bool
	recorder      func(SignalerTrace)
}

// SignalerTrace captures one signaler HTTP exchange.
type SignalerTrace struct {
	StartedDateTime time.Time
	Duration        time.Duration
	RequestMethod   string
	RequestURL      string
	RequestHeaders  http.Header
	RequestBody     string
	ResponseStatus  int
	ResponseHeaders http.Header
	ResponseBody    []byte
	Error           string
}

// NewSignalerClient creates a new signaler client.
func NewSignalerClient(cookies, authorization string) (*SignalerClient, error) {
	authorization = strings.TrimSpace(authorization)
	if authorization == "" {
		var err error
		authorization, err = signalerAuthorizationFromCookies(cookies)
		if err != nil {
			return nil, err
		}
	}
	return &SignalerClient{
		authorization: authorization,
		cookies:       cookies,
		httpClient:    &http.Client{Timeout: 60 * time.Second},
	}, nil
}

// SetDebug enables or disables debug output.
func (c *SignalerClient) SetDebug(debug bool) {
	c.debug = debug
}

// SetRecorder records signaler HTTP exchanges.
func (c *SignalerClient) SetRecorder(recorder func(SignalerTrace)) {
	c.recorder = recorder
}

// StartInteractiveAudioChannel starts the NotebookLM signaler channel for a notebook.
func (c *SignalerClient) StartInteractiveAudioChannel(ctx context.Context, notebookID string) error {
	state, err := c.chooseServer(ctx, notebookID)
	if err != nil {
		return err
	}
	if _, err := c.bootstrapChannel(ctx, notebookID, state); err != nil {
		return err
	}
	return nil
}

type signalerSession struct {
	GSessionID string
	SID        string
	AID        int
}

func (c *SignalerClient) chooseServer(ctx context.Context, notebookID string) (signalerSession, error) {
	payload, err := json.Marshal(buildChooseServerRequest(notebookID))
	if err != nil {
		return signalerSession{}, fmt.Errorf("marshal chooseServer request: %w", err)
	}

	params := url.Values{}
	params.Set("key", SignalerAPIKey)
	req, err := http.NewRequestWithContext(ctx, "POST", signalerChooseServerURL+"?"+params.Encode(), strings.NewReader(string(payload)))
	if err != nil {
		return signalerSession{}, fmt.Errorf("create chooseServer request: %w", err)
	}
	c.setSignalerHeaders(req.Header, "application/json+protobuf")

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.recordTrace(SignalerTrace{
			StartedDateTime: start,
			Duration:        time.Since(start),
			RequestMethod:   req.Method,
			RequestURL:      req.URL.String(),
			RequestHeaders:  cloneSignalerHeader(req.Header),
			RequestBody:     string(payload),
			Error:           err.Error(),
		})
		return signalerSession{}, fmt.Errorf("choose server: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.recordTrace(SignalerTrace{
			StartedDateTime: start,
			Duration:        time.Since(start),
			RequestMethod:   req.Method,
			RequestURL:      req.URL.String(),
			RequestHeaders:  cloneSignalerHeader(req.Header),
			RequestBody:     string(payload),
			ResponseStatus:  resp.StatusCode,
			ResponseHeaders: cloneSignalerHeader(resp.Header),
			Error:           err.Error(),
		})
		return signalerSession{}, fmt.Errorf("read chooseServer response: %w", err)
	}
	c.recordTrace(SignalerTrace{
		StartedDateTime: start,
		Duration:        time.Since(start),
		RequestMethod:   req.Method,
		RequestURL:      req.URL.String(),
		RequestHeaders:  cloneSignalerHeader(req.Header),
		RequestBody:     string(payload),
		ResponseStatus:  resp.StatusCode,
		ResponseHeaders: cloneSignalerHeader(resp.Header),
		ResponseBody:    append([]byte(nil), body...),
	})
	if resp.StatusCode != http.StatusOK {
		return signalerSession{}, fmt.Errorf("choose server: status %d: %s", resp.StatusCode, string(body))
	}

	var values []interface{}
	if err := json.Unmarshal(body, &values); err != nil {
		return signalerSession{}, fmt.Errorf("decode chooseServer response: %w", err)
	}
	if len(values) == 0 {
		return signalerSession{}, fmt.Errorf("chooseServer response missing session")
	}
	gsessionID, ok := values[0].(string)
	if !ok || strings.TrimSpace(gsessionID) == "" {
		return signalerSession{}, fmt.Errorf("chooseServer response missing gsessionid")
	}
	if c.debug {
		fmt.Printf("=== Signaler chooseServer ===\n")
		fmt.Printf("gsessionid: %s\n", gsessionID)
	}
	return signalerSession{GSessionID: gsessionID}, nil
}

func (c *SignalerClient) bootstrapChannel(ctx context.Context, notebookID string, state signalerSession) (signalerSession, error) {
	params := url.Values{}
	params.Set("CVER", signalerClientVersion)
	params.Set("RID", strconv.Itoa(signalerInt(90000)+10000))
	params.Set("VER", signalerChannelVersion)
	params.Set("gsessionid", state.GSessionID)
	params.Set("key", SignalerAPIKey)
	params.Set("t", "1")
	params.Set("zx", signalerZX())

	body := buildMultiWatchForm(notebookID).Encode()
	req, err := http.NewRequestWithContext(ctx, "POST", signalerChannelURL+"?"+params.Encode(), strings.NewReader(body))
	if err != nil {
		return signalerSession{}, fmt.Errorf("create signaler bootstrap request: %w", err)
	}
	c.setSignalerHeaders(req.Header, "application/x-www-form-urlencoded")
	req.Header.Set("X-WebChannel-Content-Type", "application/json+protobuf")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return signalerSession{}, fmt.Errorf("bootstrap signaler channel: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return signalerSession{}, fmt.Errorf("read signaler bootstrap response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return signalerSession{}, fmt.Errorf("bootstrap signaler channel: status %d: %s", resp.StatusCode, string(respBody))
	}

	sid, err := parseBootstrapSID(respBody)
	if err != nil {
		return signalerSession{}, err
	}
	state.SID = sid
	state.AID = 0

	go c.pollChannel(ctx, state)

	if c.debug {
		fmt.Printf("=== Signaler bootstrap ===\n")
		fmt.Printf("sid: %s\n", sid)
	}

	return state, nil
}

func (c *SignalerClient) pollChannel(ctx context.Context, state signalerSession) {
	aid := state.AID
	ci := 0
	t := 1
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		nextAID, err := c.pollOnce(ctx, state, aid, ci, t)
		if err != nil {
			if c.debug && ctx.Err() == nil {
				fmt.Printf("signaler poll error: %v\n", err)
			}
			return
		}
		if nextAID < aid {
			nextAID = aid
		}
		aid = nextAID
		ci = 1
		t++
	}
}

func (c *SignalerClient) pollOnce(ctx context.Context, state signalerSession, aid, ci, t int) (int, error) {
	params := url.Values{}
	params.Set("AID", strconv.Itoa(aid))
	params.Set("CI", strconv.Itoa(ci))
	params.Set("RID", "rpc")
	params.Set("SID", state.SID)
	params.Set("TYPE", "xmlhttp")
	params.Set("VER", signalerChannelVersion)
	params.Set("gsessionid", state.GSessionID)
	params.Set("key", SignalerAPIKey)
	params.Set("t", strconv.Itoa(t))
	params.Set("zx", signalerZX())

	req, err := http.NewRequestWithContext(ctx, "GET", signalerChannelURL+"?"+params.Encode(), nil)
	if err != nil {
		return aid, fmt.Errorf("create signaler poll request: %w", err)
	}
	c.setSignalerHeaders(req.Header, "")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return aid, fmt.Errorf("poll signaler channel: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return aid, fmt.Errorf("read signaler poll response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return aid, fmt.Errorf("poll signaler channel: status %d: %s", resp.StatusCode, string(body))
	}

	nextAID, err := parseChannelAID(body, aid)
	if err != nil {
		return aid, err
	}
	return nextAID, nil
}

func (c *SignalerClient) setSignalerHeaders(headers http.Header, contentType string) {
	headers.Set("Authorization", c.authorization)
	if c.cookies != "" {
		headers.Set("Cookie", c.cookies)
	}
	if contentType != "" {
		headers.Set("Content-Type", contentType)
	}
	headers.Set("Accept", "*/*")
	headers.Set("Origin", "https://notebooklm.google.com")
	headers.Set("Referer", "https://notebooklm.google.com/")
	headers.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36")
	headers.Set("X-Goog-AuthUser", "0")
}

func signalerAuthorizationFromCookies(cookies string) (string, error) {
	sapisid := strings.TrimSpace(extractCookieValue(cookies, "SAPISID"))
	if sapisid == "" {
		return "", fmt.Errorf("signaler authorization unavailable")
	}
	timestamp := time.Now().Unix()
	return fmt.Sprintf("SAPISIDHASH %d_%s", timestamp, generateSAPISIDHASH(sapisid, timestamp)), nil
}

func buildChooseServerRequest(notebookID string) []interface{} {
	return []interface{}{
		[]interface{}{
			nil,
			nil,
			nil,
			[]interface{}{9, 5},
			nil,
			[]interface{}{
				[]interface{}{"tailwind"},
				[]interface{}{nil, 1},
				[]interface{}{
					[]interface{}{
						[]interface{}{"discoveredSource"},
						[]interface{}{notebookID},
					},
				},
			},
			nil,
			nil,
			0,
			0,
		},
	}
}

func buildMultiWatchForm(notebookID string) url.Values {
	values := url.Values{}
	values.Set("count", "5")
	values.Set("ofs", "0")
	for i, watch := range []struct {
		name    string
		channel []interface{}
	}{
		{name: "discoveredSource", channel: []interface{}{nil, 1}},
		{name: "source", channel: []interface{}{nil, 1}},
		{name: "project", channel: []interface{}{1}},
		{name: "artifact", channel: []interface{}{nil, 1}},
		{name: "notes", channel: []interface{}{nil, 1}},
	} {
		payload, _ := json.Marshal([]interface{}{
			[]interface{}{
				[]interface{}{
					i + 1,
					[]interface{}{
						nil,
						nil,
						nil,
						[]interface{}{9, 5},
						nil,
						[]interface{}{
							[]interface{}{"tailwind"},
							watch.channel,
							[]interface{}{
								[]interface{}{
									[]interface{}{watch.name},
									[]interface{}{notebookID},
								},
							},
						},
						nil,
						nil,
						1,
					},
					nil,
					3,
				},
			},
		})
		values.Set(fmt.Sprintf("req%d___data__", i), string(payload))
	}
	return values
}

func parseBootstrapSID(body []byte) (string, error) {
	chunks, err := parseSignalerChunks(body)
	if err != nil {
		return "", err
	}
	for _, chunk := range chunks {
		rows, ok := chunk.([]interface{})
		if !ok {
			continue
		}
		for _, row := range rows {
			entry, ok := row.([]interface{})
			if !ok || len(entry) < 2 {
				continue
			}
			payload, ok := entry[1].([]interface{})
			if !ok || len(payload) < 2 {
				continue
			}
			if kind, _ := payload[0].(string); kind == "c" {
				if sid, _ := payload[1].(string); sid != "" {
					return sid, nil
				}
			}
		}
	}
	return "", fmt.Errorf("signaler bootstrap response missing sid")
}

func parseChannelAID(body []byte, current int) (int, error) {
	chunks, err := parseSignalerChunks(body)
	if err != nil {
		return current, err
	}
	maxAID := current
	for _, chunk := range chunks {
		rows, ok := chunk.([]interface{})
		if !ok {
			continue
		}
		for _, row := range rows {
			entry, ok := row.([]interface{})
			if !ok || len(entry) == 0 {
				continue
			}
			switch id := entry[0].(type) {
			case float64:
				if int(id) > maxAID {
					maxAID = int(id)
				}
			}
		}
	}
	return maxAID, nil
}

func parseSignalerChunks(body []byte) ([]interface{}, error) {
	body = bytes.TrimLeft(body, "\r\n")
	if len(body) == 0 {
		return nil, nil
	}
	var chunks []interface{}
	for len(body) > 0 {
		nl := bytes.IndexByte(body, '\n')
		if nl < 0 {
			return nil, fmt.Errorf("invalid signaler chunk framing")
		}
		size, err := strconv.Atoi(strings.TrimSpace(string(body[:nl])))
		if err != nil {
			return nil, fmt.Errorf("parse signaler chunk length: %w", err)
		}
		body = body[nl+1:]
		if size == 0 {
			body = bytes.TrimLeft(body, "\r\n")
			continue
		}
		if len(body) < size {
			return nil, fmt.Errorf("short signaler chunk")
		}
		chunkBytes := body[:size]
		body = bytes.TrimLeft(body[size:], "\r\n")
		chunkBytes = bytes.TrimRight(chunkBytes, "\r\n")
		if len(bytes.TrimSpace(chunkBytes)) == 0 {
			continue
		}

		var chunk interface{}
		if err := json.Unmarshal(chunkBytes, &chunk); err != nil {
			return nil, fmt.Errorf("decode signaler chunk: %w", err)
		}
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

func signalerZX() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 12)
	for i := range b {
		b[i] = alphabet[signalerInt(len(alphabet))]
	}
	return string(b)
}

func signalerInt(max int) int {
	if max <= 1 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0
	}
	return int(n.Int64())
}

func (c *SignalerClient) recordTrace(trace SignalerTrace) {
	if c.recorder == nil {
		return
	}
	c.recorder(trace)
}

func cloneSignalerHeader(h http.Header) http.Header {
	if h == nil {
		return nil
	}
	out := make(http.Header, len(h))
	for k, values := range h {
		out[k] = append([]string(nil), values...)
	}
	return out
}
