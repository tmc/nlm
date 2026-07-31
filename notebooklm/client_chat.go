package notebooklm

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/notebooklm/rpc"
	"google.golang.org/protobuf/proto"
)

// ChatMessage represents a message in chat history for the wire protocol.
type ChatMessage struct {
	Content   string           // Message text
	Role      int              // 1 = user, 2 = assistant
	MessageID string           // Server-assigned message UUID (from GetConversationHistory)
	Citations []Citation       // Source citations (assistant messages from GetConversationHistory)
	Rich      *pb.RichDocument // Answer-body span tree; nil when the turn carries none
}

// ChatRequest contains parameters for a chat request.
type ChatRequest struct {
	ProjectID      string
	Prompt         string
	SourceIDs      []string
	ConversationID string        // Persists across messages in a conversation
	History        []ChatMessage // Previous messages, newest first
	SeqNum         int           // Request sequence number within conversation
}

// ChatChunkPhase indicates which phase of the stream a chunk belongs to.
type ChatChunkPhase int

const (
	// ChatChunkThinking carries a cumulative reasoning trace.
	ChatChunkThinking ChatChunkPhase = iota
	// ChatChunkAnswer carries cumulative final-answer text.
	ChatChunkAnswer
)

// Citation represents a source citation from the chat response. SourceIndex
// is the 1-based citationData index — it matches the [N] the model wrote into
// the narrative, not a position in the project's source list. The same source
// index can emit several Citations when it grounds several answer ranges.
type Citation struct {
	SourceIndex int    // 1-based citation slot; matches [N] in the response text.
	SourceID    string // Granular chunk/passage identifier behind this citation (not a notebook source id).
	// ParentSourceID is the notebook-source UUID that owns SourceID's passage.
	// A citation grounds a chunk of a source, so SourceID is a chunk handle that
	// is absent from the project source list; ParentSourceID is the source that
	// IS in the list, so it — not SourceID — resolves to a title. Empty when the
	// frame did not embed the parent (older frames, or the reply-span slot shape).
	ParentSourceID string
	Title          string  // Source title, resolved from ParentSourceID in the project source list.
	StartChar      int     // Start character offset in the response (answer) text.
	EndChar        int     // End character offset in the response (answer) text.
	Confidence     float64 // Server-reported citation confidence score (0.0–1.0); 0 if unknown
	Excerpt        string  // Verbatim cited source text, as sent by the server.
	ExcerptRuns    []ExcerptRun
	SourceStart    int // Start offset of the excerpt within the source document (0 if unknown).
	SourceEnd      int // End offset of the excerpt within the source document (0 if unknown).
}

// ExcerptRun is one source-excerpt text run and its wire formatting marks.
// Code is observation-based: flags 8 and 9 have only been seen on code or
// identifier runs. Link is the confirmed hyperlink field. RawMarks retains all
// positional marks so unconfirmed flags are not lost, but renderers must leave
// them plain until their semantics are established.
type ExcerptRun struct {
	Text     string
	Code     bool
	Link     string
	Start    int
	End      int
	RawMarks []interface{}
}

// ChatChunk is a parsed chunk from the chat stream with phase metadata.
type ChatChunk struct {
	Text      string           // The text content (delta for answer, full replacement for thinking)
	Header    string           // For thinking chunks: the bold header line only
	Phase     ChatChunkPhase   // Whether this is thinking or answer
	Citations []Citation       // Source citations (populated on final/near-final chunks)
	FollowUps []string         // Suggested follow-up questions
	Rich      *pb.RichDocument // Answer-body span tree over the CUMULATIVE answer; replaced each chunk, so the last answer chunk carries the full tree. nil when the frame has no tree.
}

// chatEndpoint is the gRPC-Web endpoint for GenerateFreeFormStreamed.
// Chat does NOT use batchexecute — it uses a dedicated gRPC-Web endpoint.
const chatEndpoint = "/_/LabsTailwindUi/data/google.internal.labs.tailwind.orchestration.v1.LabsTailwindOrchestrationService/GenerateFreeFormStreamed"

const (
	// NotebookLM can take several minutes to begin a response for a large
	// notebook. Keep this bounded, but do not reject a live request merely
	// because its first parsed chunk takes longer than a minute.
	chatInitialResponseTimeout = 5 * time.Minute
	chatProgressTimeout        = 120 * time.Second
)

type chatStreamTimeoutError struct {
	timeout  time.Duration
	progress bool
}

// Error returns the chat stream timeout description.
func (e *chatStreamTimeoutError) Error() string {
	if e.progress {
		return fmt.Sprintf("chat stream timed out after %s without response progress", e.timeout)
	}
	return fmt.Sprintf("chat stream timed out after %s without an initial response", e.timeout)
}

// IsChatStreamTimeout reports whether err was caused by a chat stream that
// did not deliver a parsed response chunk before its deadline.
func IsChatStreamTimeout(err error) bool {
	var timeoutErr *chatStreamTimeoutError
	return errors.As(err, &timeoutErr)
}

// GenerateFreeFormStreamed generates a complete answer using the streaming endpoint.
func (c *Client) GenerateFreeFormStreamed(ctx context.Context, projectID string, prompt string, sourceIDs []string) (*pb.GenerateFreeFormStreamedResponse, error) {
	var resp strings.Builder
	err := c.StreamChat(ctx, ChatRequest{
		ProjectID: projectID,
		Prompt:    prompt,
		SourceIDs: sourceIDs,
	}, answerOnlyCallback(func(chunk string) bool {
		resp.WriteString(chunk)
		return true
	}))
	if err != nil {
		return nil, fmt.Errorf("generate free form streamed: %w", err)
	}
	return &pb.GenerateFreeFormStreamedResponse{
		Chunk:   resp.String(),
		IsFinal: true,
	}, nil
}

// GenerateFreeFormStreamedWithCallback streams the response and calls the callback for each chunk.
func (c *Client) GenerateFreeFormStreamedWithCallback(ctx context.Context, projectID string, prompt string, sourceIDs []string, callback func(chunk string) bool) error {
	return c.StreamChat(ctx, ChatRequest{
		ProjectID: projectID,
		Prompt:    prompt,
		SourceIDs: sourceIDs,
	}, answerOnlyCallback(callback))
}

// StreamChat streams the response with phase-aware ChatChunk callbacks.
// Thinking chunks are complete reasoning traces; answer chunks are cumulative deltas.
func (c *Client) StreamChat(ctx context.Context, req ChatRequest, callback func(ChatChunk) bool) error {
	return c.doChatStreamedChunked(ctx, req, callback)
}

func answerOnlyCallback(callback func(string) bool) func(ChatChunk) bool {
	return func(chunk ChatChunk) bool {
		if chunk.Phase != ChatChunkAnswer || chunk.Text == "" {
			return true
		}
		return callback(chunk.Text)
	}
}

// ChatWithHistory sends a chat message with full conversation history.
func (c *Client) ChatWithHistory(ctx context.Context, req ChatRequest) (string, error) {
	return c.doChat(ctx, req)
}

// resolveSourceIDs fills in source IDs from the project if not provided.
func (c *Client) resolveSourceIDs(ctx context.Context, projectID string, sourceIDs []string) []string {
	if len(sourceIDs) > 0 || c.config.SkipSources {
		return sourceIDs
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	project, err := c.GetProject(ctx, projectID)
	if err != nil {
		if c.config.Debug {
			fmt.Fprintf(os.Stderr, "DEBUG: failed to get project sources: %v\n", err)
		}
		return sourceIDs
	}
	for _, source := range project.Sources {
		if source.SourceId != nil {
			sourceIDs = append(sourceIDs, source.SourceId.SourceId)
		}
	}
	if c.config.Debug {
		fmt.Fprintf(os.Stderr, "DEBUG: using %d sources for chat\n", len(sourceIDs))
	}
	return sourceIDs
}

type chatWireHistoryEntry struct {
	Content string
	Role    int32
}

type chatWireOptions struct {
	Mode          int32
	CitationModes []int32
	FollowUpModes []int32
}

type chatWireRequest struct {
	ProjectID        string
	Prompt           string
	SourceIDs        []string
	History          []chatWireHistoryEntry
	Options          chatWireOptions
	ConversationID   string
	DraftResponseID  string
	ParentResponseID string
	NotebookID       string
	SequenceNumber   int32
}

func (c *Client) buildChatWireRequest(ctx context.Context, req ChatRequest) *chatWireRequest {
	req.SourceIDs = c.resolveSourceIDs(ctx, req.ProjectID, req.SourceIDs)

	if req.ConversationID == "" {
		req.ConversationID = uuid.New().String()
	}
	if req.SeqNum == 0 {
		req.SeqNum = 1
	}

	history := make([]chatWireHistoryEntry, 0, len(req.History))
	for _, msg := range req.History {
		history = append(history, chatWireHistoryEntry{
			Content: msg.Content,
			Role:    int32(msg.Role),
		})
	}

	return &chatWireRequest{
		ProjectID:      req.ProjectID,
		Prompt:         req.Prompt,
		SourceIDs:      req.SourceIDs,
		History:        history,
		Options:        chatWireOptions{Mode: 2, CitationModes: []int32{1}, FollowUpModes: []int32{1}},
		ConversationID: req.ConversationID,
		NotebookID:     req.ProjectID,
		SequenceNumber: int32(req.SeqNum),
	}
}

func buildChatWireArgs(req *chatWireRequest) []interface{} {
	var sourceIDArrays []interface{}
	for _, id := range req.SourceIDs {
		sourceIDArrays = append(sourceIDArrays, []interface{}{[]interface{}{id}})
	}

	var history interface{}
	if len(req.History) > 0 {
		var historyEntries []interface{}
		for _, msg := range req.History {
			historyEntries = append(historyEntries, []interface{}{msg.Content, nil, msg.Role})
		}
		history = historyEntries
	}

	options := []interface{}{2, nil, []interface{}{1}, []interface{}{1}}
	options = []interface{}{
		req.Options.Mode,
		nil,
		int32SliceToInterfaces(req.Options.CitationModes),
		int32SliceToInterfaces(req.Options.FollowUpModes),
	}

	notebookID := req.NotebookID
	if notebookID == "" {
		notebookID = req.ProjectID
	}

	return []interface{}{
		sourceIDArrays,
		req.Prompt,
		history,
		options,
		req.ConversationID,
		nilIfEmpty(req.DraftResponseID),
		nilIfEmpty(req.ParentResponseID),
		notebookID,
		req.SequenceNumber,
	}
}

func int32SliceToInterfaces(values []int32) []interface{} {
	if len(values) == 0 {
		return []interface{}{}
	}
	out := make([]interface{}, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// buildChatArgs builds the inner JSON args for a chat request.
// Wire format: [[[[source_ids]]],prompt,history,[2,null,[1],[1]],conv_id,null,null,notebook_id,seq_num]
func (c *Client) buildChatArgs(ctx context.Context, req ChatRequest) (string, error) {
	wireReq := c.buildChatWireRequest(ctx, req)
	sources := make([]*pb.ChatSourceSelection, 0, len(wireReq.SourceIDs))
	for _, sourceID := range wireReq.SourceIDs {
		sources = append(sources, &pb.ChatSourceSelection{Source: &pb.SourceIdList{SourceId: sourceID}})
	}
	var history []*pb.GenerateFreeFormStreamedHistoryEntry
	if len(wireReq.History) > 0 {
		history = make([]*pb.GenerateFreeFormStreamedHistoryEntry, 0, len(wireReq.History))
		for _, entry := range wireReq.History {
			content := entry.Content
			history = append(history, &pb.GenerateFreeFormStreamedHistoryEntry{Content: &content, Role: entry.Role})
		}
	}
	sequenceNumber := wireReq.SequenceNumber
	args := method.EncodeGenerateFreeFormStreamedWireArgs(&pb.GenerateFreeFormStreamedWireRequest{
		Sources:        sources,
		Prompt:         wireReq.Prompt,
		History:        history,
		Options:        &pb.ChatStreamOptions{Mode: wireReq.Options.Mode, CitationModes: &pb.Int32List{Value: 1}, FollowUp: &pb.ChatFollowUpOptions{Enabled: 1, Modes: []int32{1, 3}}},
		ConversationId: wireReq.ConversationID,
		NotebookId:     wireReq.NotebookID,
		SequenceNumber: &sequenceNumber,
	})

	argsJSON, err := json.Marshal(args)
	if err != nil {
		return "", fmt.Errorf("marshal chat args: %w", err)
	}
	return string(argsJSON), nil
}

// buildChatRequestBody builds the full HTTP form body for a chat request.
func (c *Client) buildChatRequestBody(ctx context.Context, req ChatRequest) (string, error) {
	innerJSON, err := c.buildChatArgs(ctx, req)
	if err != nil {
		return "", err
	}

	// Outer envelope: [null, "<inner-json-double-encoded>"]
	outerJSON, err := json.Marshal([]interface{}{nil, innerJSON})
	if err != nil {
		return "", fmt.Errorf("marshal chat envelope: %w", err)
	}

	// Form body: f.req=<url-encoded-outer>&at=<auth-token>
	authToken := c.rpc.Config.AuthToken
	body := fmt.Sprintf("f.req=%s&at=%s",
		url.QueryEscape(string(outerJSON)),
		url.QueryEscape(authToken))

	return body, nil
}

// buildChatURL constructs the full chat endpoint URL with query parameters.
func (c *Client) buildChatURL(notebookID string) string {
	u := fmt.Sprintf("https://%s%s", c.rpc.Config.Host, chatEndpoint)

	q := url.Values{}
	for k, v := range c.rpc.Config.URLParams {
		q.Set(k, v)
	}
	q.Set("rt", "c") // Chunked response format
	q.Set("_reqid", fmt.Sprintf("%d", time.Now().UnixMilli()%1000000))

	return u + "?" + q.Encode()
}

// doChat sends a chat request and returns the full response text.
func (c *Client) doChat(ctx context.Context, req ChatRequest) (string, error) {
	var result strings.Builder
	err := c.doChatStreamed(ctx, req, func(chunk string) bool {
		result.WriteString(chunk)
		return true
	})
	if err != nil {
		return "", err
	}
	return result.String(), nil
}

// doChatStreamed sends a chat request and streams response chunks via callback.
func (c *Client) doChatStreamed(ctx context.Context, req ChatRequest, callback func(chunk string) bool) error {
	body, err := c.buildChatRequestBody(ctx, req)
	if err != nil {
		return err
	}

	chatURL := c.buildChatURL(req.ProjectID)

	if c.config.Debug {
		fmt.Fprintf(os.Stderr, "DEBUG: chat URL: %s\n", chatURL)
		fmt.Fprintf(os.Stderr, "DEBUG: chat body length: %d\n", len(body))
		// Show the f.req value for debugging
		if idx := strings.Index(body, "f.req="); idx >= 0 {
			freqEnd := strings.Index(body[idx:], "&")
			if freqEnd > 0 {
				decoded, _ := url.QueryUnescape(body[idx+6 : idx+freqEnd])
				fmt.Fprintf(os.Stderr, "DEBUG: chat f.req: %s\n", decoded)
			}
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", chatURL, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("create chat request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	c.setAuthHeaders(httpReq)
	// Required header for chat endpoint (observed in HAR capture)
	httpReq.Header.Set("x-goog-ext-353267353-jspb", "[null,null,null,282611]")

	client := httpClientWithTimeout(5 * time.Minute)
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("chat request: %w", err)
	}
	defer resp.Body.Close()

	if c.config.Debug {
		fmt.Fprintf(os.Stderr, "DEBUG: chat response status: %s\n", resp.Status)
		for k, v := range resp.Header {
			fmt.Fprintf(os.Stderr, "DEBUG: chat response header %s: %v\n", k, v)
		}
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("chat request failed: %d %s: %s", resp.StatusCode, resp.Status, string(respBody)[:min(500, len(respBody))])
	}

	// Wrap body with idle timeout for streaming.
	idleBody := newIdleTimeoutReader(resp.Body, 120*time.Second)
	defer idleBody.Close()
	return c.parseChatResponse(idleBody, callback)
}

// doChatStreamedChunked sends a chat request and streams phase-aware ChatChunks via callback.
func (c *Client) doChatStreamedChunked(ctx context.Context, req ChatRequest, callback func(ChatChunk) bool) error {
	sourceIDs := c.resolveSourceIDs(ctx, req.ProjectID, req.SourceIDs)
	req.SourceIDs = sourceIDs

	body, err := c.buildChatRequestBody(ctx, req)
	if err != nil {
		return err
	}

	chatURL := c.buildChatURL(req.ProjectID)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", chatURL, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("create chat request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	c.setAuthHeaders(httpReq)
	httpReq.Header.Set("x-goog-ext-353267353-jspb", "[null,null,null,282611]")

	// Use a long total timeout for initial connection, but rely on
	// idle timeout for the streaming body — the server may think for
	// minutes before responding, but should send data regularly once started.
	client := httpClientWithTimeout(5 * time.Minute)
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("chat request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("chat request failed: %d %s: %s", resp.StatusCode, resp.Status, string(respBody)[:min(500, len(respBody))])
	}

	// Wrap body with an idle timeout for blocked reads, then enforce a separate
	// deadline for parsed response progress. The server can keep a connection
	// alive with framing bytes that never produce a chat chunk.
	idleBody := newIdleTimeoutReader(resp.Body, 120*time.Second)
	defer idleBody.Close()

	return c.parseChatResponseChunkedWithProgressTimeout(
		idleBody, sourceIDs, callback, chatInitialResponseTimeout, chatProgressTimeout)
}

// parseChatResponseChunkedWithProgressTimeout closes r when the stream has not
// produced an initial or subsequent parsed chat chunk within its deadline.
// It deliberately tracks parsed chunks rather than socket reads: keep-alive
// bytes and length prefixes are not useful progress to a command caller.
func (c *Client) parseChatResponseChunkedWithProgressTimeout(r io.ReadCloser, sourceIDs []string, callback func(ChatChunk) bool, initialTimeout, progressTimeout time.Duration) error {
	var (
		timedOut atomic.Bool
		sawChunk atomic.Bool
	)

	// The timeout is armed with time.AfterFunc, but Stop/Reset alone cannot make
	// resetting race-free: if the timer fires between a parsed chunk and the
	// Stop call, Stop returns false and the already-queued callback would still
	// close r even though progress was made. Guard the callback with a mutex and
	// a generation counter — a chunk bumps the generation, so a callback that was
	// queued for an older generation becomes a no-op. finished stops all further
	// timeouts once parsing returns.
	var (
		mu       sync.Mutex
		gen      uint64
		finished bool
	)
	var timer *time.Timer
	arm := func(d time.Duration, myGen uint64) {
		timer = time.AfterFunc(d, func() {
			mu.Lock()
			defer mu.Unlock()
			if finished || myGen != gen {
				return // superseded by a later chunk or by completion
			}
			timedOut.Store(true)
			_ = r.Close()
		})
	}
	mu.Lock()
	arm(initialTimeout, gen)
	mu.Unlock()

	timeout := initialTimeout
	err := c.parseChatResponseChunked(r, sourceIDs, func(chunk ChatChunk) bool {
		sawChunk.Store(true)
		mu.Lock()
		timeout = progressTimeout
		gen++        // invalidate any timeout callback already queued
		timer.Stop() // best-effort; the gen check is the real guard
		arm(progressTimeout, gen)
		mu.Unlock()
		return callback(chunk)
	})

	mu.Lock()
	finished = true
	timer.Stop()
	mu.Unlock()

	if timedOut.Load() {
		return &chatStreamTimeoutError{timeout: timeout, progress: sawChunk.Load()}
	}
	return err
}

// parseChatResponseChunked reads the stream incrementally and emits phase-aware
// ChatChunks as each wire frame arrives. The wire format is:
//
//	)]}'           (anti-XSSI prefix, first line only)
//	<length>\n     (decimal byte count of the following JSON line)
//	<json>\n       (the actual data — may contain ["wrb.fr", ...] envelope)
//
// Chunks are emitted immediately as they are read, enabling real-time streaming.
func (c *Client) parseChatResponseChunked(r io.Reader, sourceIDs []string, callback func(ChatChunk) bool) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024) // up to 1MB lines

	var lastThinking string
	var lastAnswer string
	var answerStarted bool
	firstLine := true

	for scanner.Scan() {
		line := scanner.Text()

		// Strip anti-XSSI prefix on first non-empty line.
		if firstLine {
			line = strings.TrimPrefix(line, ")]}'")
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			firstLine = false
		}

		// Skip length-prefix lines (pure digits).
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		isLengthLine := true
		for _, ch := range trimmed {
			if ch < '0' || ch > '9' {
				isLengthLine = false
				break
			}
		}
		if isLengthLine {
			continue
		}

		// Look for wrb.fr envelope in this line.
		startIdx := strings.Index(line, "[\"wrb.fr\"")
		if startIdx < 0 {
			continue
		}

		chunkJSON := extractJSONArray(line[startIdx:])
		if chunkJSON == "" {
			continue
		}

		var envelope []interface{}
		if err := json.Unmarshal([]byte(chunkJSON), &envelope); err != nil {
			continue
		}

		if len(envelope) < 3 {
			continue
		}
		innerStr, ok := envelope[2].(string)
		if !ok || innerStr == "" {
			continue
		}

		payload := extractChatPayloadWithOptions(innerStr, sourceIDs, c.unmarshalOptions(), c.config.Debug)
		text := payload.Text
		if text == "" {
			continue
		}

		if c.config.Debug {
			preview := text
			if len(preview) > 120 {
				preview = preview[:120] + "..."
			}
			fmt.Fprintf(os.Stderr, "DEBUG chunk: len=%d answerLen=%d thinkingLen=%d citations=%d followups=%d text=%q\n",
				len(text), len(lastAnswer), len(lastThinking),
				len(payload.Citations), len(payload.FollowUps),
				preview)
			// Dump raw citation wire data for debugging field positions.
			debugDumpChatWirePositions(innerStr)
		}

		isThinking := strings.HasPrefix(strings.TrimSpace(text), "**")
		if payload.hasWirePhase {
			isThinking = payload.wirePhase == chatWirePhaseThinking
		}

		// Thinking updates are full replacements. Track them separately from
		// answer text so a growing reasoning trace does not get misclassified
		// as the start of the final answer.
		if isThinking && !answerStarted {
			if text == lastThinking {
				continue
			}

			header := text
			if idx := strings.Index(text, "\n"); idx > 0 {
				header = text[:idx]
			}
			if !callback(ChatChunk{Text: text, Header: header, Phase: ChatChunkThinking}) {
				return nil
			}
			lastThinking = text
			continue
		}

		answerStarted = true
		if text == lastAnswer {
			continue
		}

		// The server sends cumulative text. Find the longest common
		// prefix with what we already emitted and only send the new
		// suffix. This handles citation consolidation where the server
		// revises earlier text (e.g. "[2, 3]" → "[2-5]").
		commonLen := 0
		limit := len(lastAnswer)
		if len(text) < limit {
			limit = len(text)
		}
		for commonLen < limit && text[commonLen] == lastAnswer[commonLen] {
			commonLen++
		}
		delta := text[commonLen:]
		if delta != "" {
			if !callback(ChatChunk{
				Text:      delta,
				Phase:     ChatChunkAnswer,
				Citations: payload.Citations,
				FollowUps: payload.FollowUps,
				Rich:      payload.Rich,
			}) {
				return nil
			}
		}
		lastAnswer = text
	}

	return scanner.Err()
}

// parseChatResponse reads the Google chunked response format and extracts text.
// Format: )]}'\n then repeated: <length>\n<json-chunk>\n
func (c *Client) parseChatResponse(r io.Reader, callback func(chunk string) bool) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("read chat response: %w", err)
	}

	body := string(data)

	if c.config.Debug {
		fmt.Fprintf(os.Stderr, "DEBUG: chat response length: %d\n", len(body))
	}

	// Strip )]}' prefix
	if strings.HasPrefix(body, ")]}'") {
		body = body[4:]
	}
	body = strings.TrimLeft(body, "\n")
	rawBody := body

	// Extract text from wrb.fr chunks
	// Each chunk is: <length>\n<json-array>\n
	// The json-array contains: ["wrb.fr", "service.Method", "<inner-json>", ...]
	var lastText string
	var sawFrame bool
	for len(body) > 0 {
		body = strings.TrimLeft(body, "\n ")

		// Read length
		nlIdx := strings.Index(body, "\n")
		if nlIdx < 0 {
			break
		}
		body = body[nlIdx+1:]

		// Find the JSON array in this chunk
		// Look for wrb.fr entries
		startIdx := strings.Index(body, "[\"wrb.fr\"")
		if startIdx >= 0 {
			sawFrame = true
		}
		if startIdx < 0 {
			// Try other envelope types like ["e", ...] and skip
			nextNL := strings.Index(body, "\n")
			if nextNL >= 0 {
				body = body[nextNL+1:]
			} else {
				break
			}
			continue
		}

		// Find the balanced end of this JSON array
		chunkJSON := extractJSONArray(body[startIdx:])
		if chunkJSON == "" {
			break
		}

		// Parse the wrb.fr envelope: ["wrb.fr", "method", "<inner-json>", ...]
		var envelope []interface{}
		if err := json.Unmarshal([]byte(chunkJSON), &envelope); err != nil {
			// Skip unparseable chunks
			nextNL := strings.Index(body, "\n")
			if nextNL >= 0 {
				body = body[nextNL+1:]
			} else {
				break
			}
			continue
		}

		if len(envelope) >= 3 {
			if innerStr, ok := envelope[2].(string); ok && innerStr != "" {
				text := extractChatText(innerStr)
				if text != "" && text != lastText {
					// The stream has two phases:
					// 1. Thinking: each chunk is a complete replacement (no shared prefix)
					// 2. Final answer: cumulative — each chunk extends the previous
					// Compute delta to avoid re-emitting already-printed text.
					delta := text
					if strings.HasPrefix(text, lastText) {
						delta = text[len(lastText):]
					}
					if delta != "" {
						if !callback(delta) {
							return nil
						}
					}
					lastText = text
				}
			}
		}

		// Advance past this chunk
		endIdx := startIdx + len(chunkJSON)
		if endIdx < len(body) {
			body = body[endIdx:]
		} else {
			break
		}
	}

	// A response with no data frame and no extracted text is the silent-empty
	// failure mode — most often expired auth, which the gRPC-Web chat endpoint
	// reports as an HTTP 200 with an error frame instead of a content frame.
	// Surface it as an error rather than returning an empty answer.
	if !sawFrame && lastText == "" {
		if err := classifyChatError(rawBody); err != nil {
			return err
		}
	}

	return nil
}

// chatAuthErrorCodes are the batchexecute dictionary codes that mean the stored
// session is no longer valid. They appear in the gRPC-Web chat error frame as a
// bare integer, e.g. 16 (Unauthenticated) or the legacy 277566/277567.
var chatAuthErrorCodes = map[int]bool{16: true, 277566: true, 277567: true}

// classifyChatError inspects a chat response body that produced no content
// frame and returns a typed error when it carries a recognizable failure
// signal. It returns nil for a genuinely empty answer (no signature) so callers
// do not turn an empty response into a spurious error.
//
// The gRPC-Web chat error frame is length-prefixed and wraps the gRPC status
// code somewhere in a nested array, so the body does not parse cleanly as a
// batchexecute response. We scan the integer tokens in the body for a known
// auth code rather than assert an exact (uncaptured) frame shape; if an auth
// code is present, expired auth is by far the most likely cause.
func classifyChatError(rawBody string) error {
	body := strings.TrimSpace(rawBody)
	if body == "" {
		return nil
	}
	for _, code := range scanIntTokens(body) {
		if chatAuthErrorCodes[code] {
			return fmt.Errorf("chat returned no content: %w (server signaled error code %d)", ErrAuthExpired, code)
		}
	}
	return nil
}

// scanIntTokens returns the JSON-delimited integer literals in s — runs of
// digits bounded on both sides by JSON structural punctuation (brackets,
// commas, colons) or whitespace, or by the start/end of the string. The strict
// boundary is deliberate: it excludes digits embedded in UUIDs, method names,
// and hyphenated identifiers (e.g. the trailing "...000016" segment of a source
// UUID), which would otherwise be false positives when scanning for error
// codes inside an opaque wire frame.
func scanIntTokens(s string) []int {
	var out []int
	for i := 0; i < len(s); {
		if s[i] < '0' || s[i] > '9' {
			i++
			continue
		}
		j := i
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		prevOK := i == 0 || isJSONBoundary(s[i-1])
		nextOK := j == len(s) || isJSONBoundary(s[j])
		if prevOK && nextOK {
			if n, err := strconv.Atoi(s[i:j]); err == nil {
				out = append(out, n)
			}
		}
		i = j
	}
	return out
}

// isJSONBoundary reports whether b is JSON structural punctuation or whitespace
// — i.e. a character that can legitimately border a standalone integer literal.
func isJSONBoundary(b byte) bool {
	switch b {
	case '[', ']', '{', '}', ',', ':', ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}

// extractJSONArray extracts a balanced JSON array from the start of a string.
func extractJSONArray(s string) string {
	if len(s) == 0 || s[0] != '[' {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for i, ch := range s {
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if ch == '[' {
			depth++
		} else if ch == ']' {
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}
	return ""
}

// extractChatText extracts the readable text from the inner JSON of a chat response chunk.
// The inner JSON has varying structure but the main text is typically at position [0][0].
// chatPayload holds the parsed fields from a chat stream inner JSON array.
type chatPayload struct {
	Text      string
	Citations []Citation
	FollowUps []string
	Rich      *pb.RichDocument // answer-body span tree (inner[0][4]); nil when absent

	wirePhase    int
	hasWirePhase bool
}

const (
	chatWirePhaseThinking = 0
	chatWirePhaseAnswer   = 1
)

// parseFollowUps extracts follow-up suggestions from wire position [4].
// Each entry is [text, null, ..., type_code] where type 9 = question.
func parseFollowUps(data interface{}) []string {
	arr, ok := data.([]interface{})
	if !ok || len(arr) == 0 {
		return nil
	}
	var followUps []string
	for _, item := range arr {
		itemArr, ok := item.([]interface{})
		if !ok || len(itemArr) == 0 {
			continue
		}
		if text, ok := itemArr[0].(string); ok && text != "" {
			followUps = append(followUps, text)
		}
	}
	return followUps
}

// DeleteChatHistory deletes all chat history for a notebook.
func (c *Client) DeleteChatHistory(ctx context.Context, projectID string) error {
	_, err := c.orchestrationService.DeleteChatHistory(ctx, &pb.DeleteChatHistoryRequest{
		ProjectId: projectID,
	})
	if err != nil {
		return fmt.Errorf("delete chat history: %w", err)
	}
	return nil
}

// GetConversations returns conversation IDs for a notebook.
func (c *Client) GetConversations(ctx context.Context, projectID string) ([]string, error) {
	resp, err := c.orchestrationService.GetConversations(ctx, &pb.GetConversationsRequest{
		Context:   &pb.RequestContext{},
		ProjectId: projectID,
		Limit:     20,
	})
	if err != nil {
		return nil, fmt.Errorf("get conversations: %w", err)
	}
	return conversationIDsFromProto(resp), nil
}

func conversationIDsFromProto(resp *pb.GetConversationsResponse) []string {
	if resp == nil || len(resp.GetConversations()) == 0 {
		return []string{}
	}
	ids := make([]string, 0, len(resp.GetConversations()))
	for _, conversation := range resp.GetConversations() {
		if conversation != nil {
			ids = append(ids, conversation.GetConversationId())
		}
	}
	return ids
}

// GetConversationHistory retrieves the message history for a specific conversation.
func (c *Client) GetConversationHistory(ctx context.Context, projectID, conversationID string) ([]ChatMessage, error) {
	req := &pb.GetConversationHistoryRequest{
		Context:        conversationRequestContext(),
		ConversationId: conversationID,
		Limit:          proto.Int32(20),
	}
	raw, err := c.rpc.Do(ctx, rpc.Call{
		ID:         rpc.RPCGetConversationHistory,
		NotebookID: projectID,
		Args:       method.EncodeGetConversationHistoryArgs(req),
	})
	if err != nil {
		return nil, fmt.Errorf("get conversation history: %w", err)
	}
	var response pb.GetConversationHistoryResponse
	if err := c.unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("parse conversation history: %w", err)
	}
	return conversationMessagesFromProto(&response, raw), nil
}

func conversationRequestContext() *pb.RequestContext {
	return &pb.RequestContext{
		Version: proto.Int32(2),
		Surface: &pb.RequestSurface{Value: proto.Int32(1)},
		Caps: &pb.RequestClientCaps{
			Version:         proto.Int32(1),
			CapabilityCodes: []int32{1, 3},
		},
	}
}

func conversationMessagesFromProto(resp *pb.GetConversationHistoryResponse, raw []byte) []ChatMessage {
	if resp == nil || len(resp.GetMessages()) == 0 {
		return nil
	}
	// The proto GetConversationHistoryResponse models the message and rich
	// document, but not the content block's citation detail slots
	// ([4][1]/[4][3]), which carry each citation's verbatim excerpt and embedded
	// source UUID. Recover those details from the raw payload, aligned by
	// message index.
	extras := historyCitationsByIndex(raw)
	messages := make([]ChatMessage, 0, len(resp.GetMessages()))
	for i, message := range resp.GetMessages() {
		if message == nil || message.GetRole() == 0 {
			continue
		}
		content := message.GetText()
		if content == "" && message.GetRichContent() != nil {
			content = contentSegmentText(message.GetRichContent().GetSegment())
		}
		if content == "" {
			continue
		}
		msg := ChatMessage{
			MessageID: message.GetMessageId(),
			Content:   content,
			Role:      int(message.GetRole()),
			Rich:      message.GetRichContent().GetSegment().GetRichDocument(),
		}
		if i < len(extras) {
			msg.Citations = extras[i].citations
		}
		messages = append(messages, msg)
	}
	return messages
}

// historyCitationsByIndex extracts per-message citations (with verbatim
// excerpts and embedded source UUIDs) from the raw GetConversationHistory
// payload, indexed to match the proto message order. The proto decode drops
// the citation detail slot, so citations are parsed positionally: each
// assistant message's content block [4] carries citationData at [1] and
// mappingData at [3], which parseCitationsV2 fans out per (marker, source).
// A message with no citation slot yields a nil entry, keeping indices aligned.
func historyCitationsByIndex(raw []byte) []historyMessageExtras {
	var data []interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil
	}
	msgArrays := historyMessageArrays(data)
	out := make([]historyMessageExtras, len(msgArrays))
	for i, m := range msgArrays {
		arr, ok := m.([]interface{})
		if !ok || len(arr) <= 4 {
			continue
		}
		block, ok := arr[4].([]interface{})
		if !ok || len(block) <= 3 {
			continue
		}
		out[i].citations = parseCitationsV2(block[1], block[3], nil)
	}
	return out
}

// historyMessageExtras holds the per-message citation details that the proto
// does not model.
type historyMessageExtras struct {
	citations []Citation
}

// historyMessageArrays unwraps the GetConversationHistory envelope to the list
// of message arrays. The payload is [[[msg1, msg2, ...]]] (wrapped) or the
// message list directly; this mirrors the proto decoder's message ordering.
func historyMessageArrays(data []interface{}) []interface{} {
	if len(data) == 0 {
		return nil
	}
	outer, ok := data[0].([]interface{})
	if !ok || len(outer) == 0 {
		return nil
	}
	if _, ok := outer[0].([]interface{}); ok {
		return outer
	}
	return data
}

func contentSegmentText(segment *pb.ContentSegment) string {
	if segment == nil {
		return ""
	}
	return segment.GetText()
}

// ChatGoal represents a conversational goal setting.
type ChatGoal int

const (
	// ChatGoalDefault selects the default conversational style.
	ChatGoalDefault ChatGoal = 3
	// ChatGoalLearningGuide selects the unconfirmed learning-guide mode.
	ChatGoalLearningGuide ChatGoal = 1
	// ChatGoalCustom selects a caller-provided prompt.
	ChatGoalCustom ChatGoal = 2
)

// ResponseLength represents a response length setting.
type ResponseLength int

const (
	// ResponseLengthDefault leaves the response length unspecified.
	ResponseLengthDefault ResponseLength = 0
	// ResponseLengthLonger requests longer responses.
	ResponseLengthLonger ResponseLength = 4
	// ResponseLengthShorter requests shorter responses.
	ResponseLengthShorter ResponseLength = 3
)

// ChatConfig is a notebook's chat configuration: the conversational goal,
// an optional custom prompt (when Goal is ChatGoalCustom), and the response
// length. A zero field means the notebook uses the server default.
type ChatConfig struct {
	Goal           ChatGoal
	CustomPrompt   string
	ResponseLength ResponseLength
}

// GetChatConfig returns the chat configuration set for a notebook. It reads
// the configuration the project already carries, so it costs one GetProject.
// A notebook that has never had SetChatConfig applied reports zero values.
func (c *Client) GetChatConfig(ctx context.Context, projectID string) (ChatConfig, error) {
	project, err := c.GetProject(ctx, projectID)
	if err != nil {
		return ChatConfig{}, fmt.Errorf("get chat config: %w", err)
	}
	config := project.GetChatbotConfig()
	return ChatConfig{
		Goal:           ChatGoal(config.GetGoal().GetGoal()),
		CustomPrompt:   config.GetGoal().GetCustomPrompt(),
		ResponseLength: ResponseLength(config.GetResponseLength().GetValue()),
	}, nil
}

// SetChatConfig updates the chat configuration for a notebook via MutateProject.
// goalConfig: [goal_type] or [goal_type, "custom_prompt"]
// responseLengthConfig: [] for default, [4] for longer, [3] for shorter
func (c *Client) SetChatConfig(ctx context.Context, projectID string, goal ChatGoal, customPrompt string, responseLength ResponseLength) error {
	var goalConfig interface{}
	if goal == ChatGoalCustom && customPrompt != "" {
		goalConfig = []interface{}{int(goal), customPrompt}
	} else if goal != 0 {
		goalConfig = []interface{}{int(goal)}
	} else {
		goalConfig = []interface{}{}
	}

	var lengthConfig interface{}
	if responseLength != ResponseLengthDefault {
		lengthConfig = []interface{}{int(responseLength)}
	} else {
		lengthConfig = []interface{}{}
	}

	_, err := c.rpc.Do(ctx, rpc.Call{
		ID:         rpc.RPCMutateProject,
		NotebookID: projectID,
		Args: []interface{}{
			projectID,
			[]interface{}{
				[]interface{}{
					nil, nil, nil, nil, nil, nil, nil,
					[]interface{}{goalConfig, lengthConfig},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("set chat config: %w", err)
	}
	return nil
}

// GenerateReportSuggestions generates report-section suggestions for a notebook.
func (c *Client) GenerateReportSuggestions(ctx context.Context, projectID string) (*pb.GenerateReportSuggestionsResponse, error) {
	sourceIDs := c.resolveSourceIDs(ctx, projectID, nil)

	// Build source refs in wire format: [["src1"],["src2"],...]
	var sourceRefs []interface{}
	for _, id := range sourceIDs {
		sourceRefs = append(sourceRefs, []interface{}{id})
	}

	projectContext := []interface{}{
		2, nil, nil,
		[]interface{}{1, nil, nil, nil, nil, nil, nil, nil, nil, nil, []interface{}{1}},
		[]interface{}{[]interface{}{1, 4, 2, 3, 6, 5}},
	}

	resp, err := c.rpc.Do(ctx, rpc.Call{
		ID:         "ciyUvf",
		NotebookID: projectID,
		Args:       []interface{}{projectContext, projectID, sourceRefs},
	})
	if err != nil {
		return nil, fmt.Errorf("generate report suggestions: %w", err)
	}

	// Raw response: [[ [title, desc, null, [[src_id],...], prompt, count], ... ]]
	var raw []interface{}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return nil, fmt.Errorf("parse report suggestions: %w", err)
	}

	// Unwrap outer array: response is [[suggestions...]]
	suggestions := raw
	if len(raw) > 0 {
		if inner, ok := raw[0].([]interface{}); ok {
			// Check if inner[0] is itself an array (i.e., a suggestion)
			if len(inner) > 0 {
				if _, ok := inner[0].([]interface{}); ok {
					suggestions = inner
				}
			}
		}
	}

	result := &pb.GenerateReportSuggestionsResponse{}
	for _, item := range suggestions {
		arr, ok := item.([]interface{})
		if !ok || len(arr) < 2 {
			continue
		}
		s := &pb.ReportSuggestion{}
		if v, ok := arr[0].(string); ok {
			s.Title = v
		}
		if v, ok := arr[1].(string); ok {
			s.Description = v
		}
		// arr[2] is null
		// arr[3] is source refs: [[src_id1], [src_id2], ...]
		if len(arr) > 3 {
			if refs, ok := arr[3].([]interface{}); ok {
				for _, ref := range refs {
					if inner, ok := ref.([]interface{}); ok && len(inner) > 0 {
						if id, ok := inner[0].(string); ok {
							s.SourceIds = append(s.SourceIds, &pb.SourceIdList{SourceId: id})
						}
					}
				}
			}
		}
		if len(arr) > 4 {
			if v, ok := arr[4].(string); ok {
				s.Prompt = v
			}
		}
		if len(arr) > 5 {
			if v, ok := arr[5].(float64); ok {
				s.Count = int32(v)
			}
		}
		result.Suggestions = append(result.Suggestions, s)
	}
	return result, nil
}

// SetInstructions sets the notebook's custom chat instructions (system prompt).
func (c *Client) SetInstructions(ctx context.Context, projectID string, instructions string) error {
	return c.SetChatConfig(ctx, projectID, ChatGoalCustom, instructions, ResponseLengthDefault)
}

// GetInstructions returns the notebook's custom chat instructions (system prompt).
func (c *Client) GetInstructions(ctx context.Context, projectID string) (string, error) {
	project, err := c.GetProject(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("get project: %w", err)
	}
	prompt := ""
	if cfg := project.GetChatbotConfig(); cfg != nil {
		prompt = cfg.GetGoal().GetCustomPrompt()
	}
	return strings.TrimSpace(prompt), nil
}
