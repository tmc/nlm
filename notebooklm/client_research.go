package notebooklm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
	"github.com/tmc/nlm/internal/notebooklm/rpc"
)

// DeepResearchResult holds the outcome of a deep research start or poll.
// QA9ei StartDeepResearch returns two IDs; both are retained because
// different downstream operations key on different IDs:
//
//	ResearchID     — matches session[1][5][0] in GetDeepResearchSessions
//	                 (primary scan key for PollDeepResearch).
//	ConversationID — matches session[0] in GetDeepResearchSessions
//	                 (required by LBwxtb DeleteDeepResearch).
type DeepResearchResult struct {
	ResearchID     string           `json:"research_id"`
	ConversationID string           `json:"conversation_id,omitempty"`
	Done           bool             `json:"done"`
	Query          string           `json:"query,omitempty"`
	Report         string           `json:"report,omitempty"`
	Sources        []ResearchSource `json:"sources,omitempty"`
	// Plan is the base64-decoded protobuf of the LLM's numbered search
	// strategy (session[1][5][1]). Ignored by PollDeepResearch itself
	// but preserved so a future --show-plan mode can surface reasoning.
	Plan []byte `json:"plan,omitempty"`
}

// ResearchSource describes one source discovered by a research call.
// Rank, FaviconURL, and CitationIndex come from the web-UI source blob
// layout main_blob[0][i] for i=1..N; preserving them lets downstream
// tools reproduce what the browser surfaces.
type ResearchSource struct {
	URL           string `json:"url,omitempty"`
	Title         string `json:"title,omitempty"`
	Snippet       string `json:"snippet,omitempty"`
	Rank          int    `json:"rank,omitempty"`
	FaviconURL    string `json:"favicon_url,omitempty"`
	CitationIndex int    `json:"citation_index,omitempty"`
}

// startFastResearchArgs produces the 4-position wire shape captured from
// the NotebookLM web UI on 2026-04-17:
//
//	[[query, 1], null, 1, project_id]
//
// Position [0] is [query, 1] (same pair the wire uses for deep-research
// at position [2]). Position [2] is the mode enum — 1 for fast, 5 for
// deep. Exposed as a standalone function so tests can golden-check the
// shape independent of an rpc.Client.
func startFastResearchArgs(query, projectID string) []interface{} {
	return []interface{}{
		[]interface{}{query, 1},
		nil,
		1,
		projectID,
	}
}

// StartFastResearch kicks off a fast-research session for query against
// projectID. Returns a DeepResearchResult with ConversationID populated
// (fast-mode uses conversation_id as the poll key; ResearchID stays
// empty, unlike deep-research). The caller polls via PollFastResearch.
//
// Wire-verified 2026-04-17 against notebook
// 00000000-0000-4000-8000-000000000006 and query "har harl file formats"
// (NotebookLM web UI capture, 2026-04-17).
// The JS bundle binds Ljjv0c to DiscoverSourcesManifold; the
// "research" feature is built on top of the DiscoverSources job
// system, so notebooklm.Client uses the StartFastResearch alias while the
// service contract calls it DiscoverSourcesManifold.
//
// (Earlier commits speculated that Es3dTe was the fast-research RPC;
// Es3dTe is actually a different DiscoverSources entry point.)
func (c *Client) StartFastResearch(ctx context.Context, projectID, query string) (*DeepResearchResult, error) {
	resp, err := c.rpc.Do(ctx, rpc.Call{
		ID:         rpc.RPCStartFastResearch,
		NotebookID: projectID,
		Args:       startFastResearchArgs(query, projectID),
	})
	if err != nil {
		return nil, fmt.Errorf("start fast research: %w", err)
	}
	var ids []string
	if err := json.Unmarshal(resp, &ids); err != nil {
		return nil, fmt.Errorf("start fast research: decode response: %w", err)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("start fast research: empty response")
	}
	return &DeepResearchResult{
		ConversationID: ids[0],
		Query:          query,
	}, nil
}

// PollFastResearch scans the e3bVqc session list for a fast-mode
// conversation. Returns ErrResearchPolling while the session is still
// running or not yet visible (the caller loops until done or cap
// exceeded). Shares the same e3bVqc RPC as PollDeepResearch; the
// scanner matches on ConversationID rather than ResearchID and the
// main_blob decoder uses the fast-mode layout (sources + summary
// string, no markdown report header).
func (c *Client) PollFastResearch(ctx context.Context, projectID, conversationID string) (*DeepResearchResult, error) {
	match := func(s deepResearchSession) bool {
		return s.ConversationID == conversationID && s.Mode == 1
	}
	return c.pollResearch(ctx, projectID, "fast", match, decodeFastMainBlob)
}

// FastResearch is a convenience wrapper: start a fast-research session
// and block until it completes, returning the final result. For a
// start-and-poll pattern with explicit pacing, call StartFastResearch
// and PollFastResearch directly.
func (c *Client) FastResearch(ctx context.Context, projectID, query string) (*DeepResearchResult, error) {
	started, err := c.StartFastResearch(ctx, projectID, query)
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 60; attempt++ {
		result, err := c.PollFastResearch(ctx, projectID, started.ConversationID)
		if err == nil && result.Done {
			return result, nil
		}
		if err != nil && !errors.Is(err, ErrResearchPolling) {
			return nil, err
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("fast research: %w", ctx.Err())
		}
	}
	return nil, fmt.Errorf("fast research: timed out waiting for completion")
}

// StartDeepResearch kicks off a deep-research session for query against
// projectID. The call returns two identifiers, both retained in the
// result: ResearchID is the primary key for polling via
// GetDeepResearchSessions; ConversationID is the key for
// DeleteDeepResearch.
//
// Request args are five positions:
//
//	[0] context           standard request envelope
//	[1] nil               placeholder
//	[2] [query, 1]        query string plus an opaque trailing 1
//	[3] 5                 scalar that matches session[1][2] in the
//	                      GetDeepResearchSessions response
//	[4] project_id        notebook identifier
//
// Response is a two-element JSON array: [research_id, conversation_id].
func (c *Client) StartDeepResearch(ctx context.Context, projectID, query string) (*DeepResearchResult, error) {
	req := &pb.StartDeepResearchWireRequest{
		Context:   conversationRequestContext(),
		ProjectId: projectID,
		Query:     &pb.ResearchQuery{Query: query, Mode: 1},
		Mode:      5,
	}
	resp, err := c.rpc.Do(ctx, rpc.Call{
		ID:         rpc.RPCStartDeepResearch,
		NotebookID: projectID,
		Args:       method.EncodeStartDeepResearchWireArgs(req),
	})
	if err != nil {
		return nil, fmt.Errorf("start deep research: %w", err)
	}

	var data []string
	if err := json.Unmarshal(resp, &data); err != nil {
		return nil, fmt.Errorf("parse start response: %w", err)
	}
	if len(data) < 2 {
		return nil, fmt.Errorf("start deep research: expected [research_id, conversation_id], got %d elements", len(data))
	}
	return &DeepResearchResult{
		ResearchID:     data[0],
		ConversationID: data[1],
		Query:          query,
	}, nil
}

// PollDeepResearch fetches the full GetDeepResearchSessions list for
// projectID and returns the session matching researchID. A session is
// "done" when state == 2 AND main_blob is non-null; any other state
// (1 running, 5 tombstoned, anything else) returns the in-progress
// sentinel so the exit-code classifier maps to exit 7.
//
// State enum (observed via CDP capture, 2026-04-17):
//
//	1 = RUNNING   (main_blob == null; ts[2] may update as heartbeat)
//	2 = COMPLETE  (main_blob populated with report + sources)
//	5 = DELETED   (server-side soft-delete; invisible to future queries)
//
// Values 0, 3, and 4 have not been observed. The scan treats unknown
// states as still-running rather than false-done, which is the safe
// default.
func (c *Client) PollDeepResearch(ctx context.Context, projectID, researchID string) (*DeepResearchResult, error) {
	match := func(s deepResearchSession) bool {
		return s.ResearchID == researchID && s.Mode == 5
	}
	result, err := c.pollResearch(ctx, projectID, "deep", match, decodeDeepResearchContent)
	if result != nil {
		result.ResearchID = researchID
	}
	return result, err
}

// pollResearch is the shared scan-and-decode core behind both
// PollDeepResearch and PollFastResearch. It fetches the current
// e3bVqc session list, runs match against each session, and when a
// done session is found calls decode to extract report+sources. The
// ErrResearchPolling sentinel is returned while the session is either
// not yet visible (race between Start and first poll) or still
// running; the caller loops until done or a cap is hit. kind labels
// the error messages so panic traces distinguish deep-vs-fast.
func (c *Client) pollResearch(
	ctx context.Context,
	projectID, kind string,
	match func(deepResearchSession) bool,
	decode func(json.RawMessage) (string, []ResearchSource),
) (*DeepResearchResult, error) {
	resp, err := c.rpc.Do(ctx, rpc.Call{
		ID:         rpc.RPCGetDeepResearchSessions,
		NotebookID: projectID,
		Args:       []interface{}{nil, nil, projectID},
	})
	if err != nil {
		return nil, fmt.Errorf("poll %s research: %w", kind, err)
	}

	sessions, err := parseDeepResearchSessionsProtoWithOptions(resp, c.unmarshalOptions())
	if err != nil {
		return nil, fmt.Errorf("poll %s research: %w", kind, err)
	}

	for _, s := range sessions {
		if !match(s) {
			continue
		}
		if s.State == 5 {
			// Tombstone: server-side soft-delete. Invisible to
			// future poll queries from the CLI user's POV.
			continue
		}
		if s.State == 2 && len(s.MainBlob) > 0 {
			result := &DeepResearchResult{
				ResearchID:     s.ResearchID,
				ConversationID: s.ConversationID,
				Done:           true,
				Query:          s.Query,
				Plan:           s.Plan,
			}
			if s.Report != "" || len(s.Sources) > 0 {
				result.Report, result.Sources = s.Report, s.Sources
			} else {
				result.Report, result.Sources = decode(s.MainBlob)
			}
			return result, nil
		}
		// State 1 (running) or an unrecognized state (6 observed but
		// not documented) is still running. Return partial result with
		// the busy sentinel so the caller loops.
		return &DeepResearchResult{
				ResearchID:     s.ResearchID,
				ConversationID: s.ConversationID,
				Query:          s.Query,
				Plan:           s.Plan,
			},
			fmt.Errorf("poll %s research: %w", kind, ErrResearchPolling)
	}

	// No matching session — either not yet visible (race between
	// Start and the first session-list fetch) or server tombstoned
	// it. Return the busy sentinel so callers loop; the outer
	// max-polls budget bounds the wait.
	return &DeepResearchResult{}, fmt.Errorf("poll %s research: %w", kind, ErrResearchPolling)
}

// deepResearchSession is the internal decoded shape of one session
// entry within an e3bVqc response. The RPC is polymorphic: it serves
// both deep-research (mode=5, inner has 6 positions with a trailing
// [research_id, plan_b64] pair) and fast-research (mode=1, inner has
// 5 positions and the poll key is ConversationID). Position [2] on
// the inner array is the mode enum.
type deepResearchSession struct {
	ConversationID string
	ProjectID      string
	Query          string
	Mode           int             // session inner[2]: 1=fast, 5=deep
	State          int             // session inner[4]: 1=running, 2=complete, 5=tombstone, 6=seen-but-unknown
	ResearchID     string          // session inner[5][0]; empty for fast-mode
	Plan           []byte          // base64-decoded protobuf of the LLM plan (deep only)
	MainBlob       json.RawMessage // session inner[3]; null during RUNNING
	Report         string
	Sources        []ResearchSource
}

// parseDeepResearchSessionsProto decodes the e3bVqc session list through the
// generated response model. The positional parser remains the test oracle;
// this adapter preserves the public poll projection for both fast and deep
// session variants.
func parseDeepResearchSessionsProto(resp json.RawMessage) ([]deepResearchSession, error) {
	return parseDeepResearchSessionsProtoWithOptions(resp, beprotojson.UnmarshalOptions{DiscardUnknown: true})
}

func parseDeepResearchSessionsProtoWithOptions(resp json.RawMessage, options beprotojson.UnmarshalOptions) ([]deepResearchSession, error) {
	var decoded pb.GetDeepResearchSessionsResponse
	if err := options.Unmarshal(resp, &decoded); err != nil {
		return nil, fmt.Errorf("decode sessions proto: %w", err)
	}
	return deepResearchSessionsFromProto(decoded.GetSessions()), nil
}

func deepResearchSessionsFromProto(decoded []*pb.DeepResearchSession) []deepResearchSession {
	sessions := make([]deepResearchSession, 0, len(decoded))
	for _, session := range decoded {
		if session == nil || session.GetDetails() == nil {
			continue
		}
		details := session.GetDetails()
		ds := deepResearchSession{
			ConversationID: session.GetConversationId(),
			ProjectID:      details.GetProjectId(),
			Query:          details.GetQuery().GetText(),
			Mode:           int(details.GetMode()),
			State:          int(details.GetState()),
		}
		if metadata := details.GetMetadata(); metadata != nil {
			ds.ResearchID = metadata.GetResearchId()
			if plan, err := base64.StdEncoding.DecodeString(metadata.GetPlan()); err == nil {
				ds.Plan = plan
			}
		}
		main := details.GetMainBlob()
		if main == nil {
			sessions = append(sessions, ds)
			continue
		}
		ds.MainBlob = json.RawMessage("{}")
		entries := main.GetReportTree()
		offset := 0
		if ds.Mode == 5 {
			offset = 1
			if len(entries) > 0 && entries[0].GetDetail() != nil {
				ds.Report = entries[0].GetDetail().GetMarkdown()
			}
		} else {
			ds.Report = main.GetExtra()
		}
		for _, entry := range entries[offset:] {
			if entry == nil {
				continue
			}
			citationIndex := 0
			if ds.Mode == 5 {
				citationIndex = int(entry.GetRank())
			}
			ds.Sources = append(ds.Sources, ResearchSource{
				URL: entry.GetUrl(), Title: entry.GetTitle(),
				Snippet: entry.GetSummary(), Rank: int(entry.GetKind()),
				FaviconURL: entry.GetFaviconUrl(), CitationIndex: citationIndex,
			})
		}
		sessions = append(sessions, ds)
	}
	return sessions
}

// parseDeepResearchSessions decodes the top-level e3bVqc response
// payload into structured session records. Defensive by default: a
// malformed entry is skipped rather than fatal, because the wire
// format has many optional positions and partial server responses
// are plausible. When debug is true the function logs each skip so
// a future reader can see if Google changes the shape.
func parseDeepResearchSessions(resp json.RawMessage, debug bool) ([]deepResearchSession, error) {
	var outer [][]json.RawMessage
	if err := json.Unmarshal(resp, &outer); err != nil {
		return nil, fmt.Errorf("decode sessions outer: %w", err)
	}
	if len(outer) == 0 || len(outer[0]) == 0 {
		return nil, nil // empty sessions list is valid (no research yet)
	}

	sessions := make([]deepResearchSession, 0, len(outer[0]))
	for i, raw := range outer[0] {
		var s []json.RawMessage
		if err := json.Unmarshal(raw, &s); err != nil {
			if debug {
				fmt.Fprintf(os.Stderr, "api: deep-research session #%d: decode entry: %v\n", i, err)
			}
			continue
		}
		if len(s) < 2 {
			if debug {
				fmt.Fprintf(os.Stderr, "api: deep-research session #%d: expected >=2 fields, got %d\n", i, len(s))
			}
			continue
		}
		var conv string
		_ = json.Unmarshal(s[0], &conv)

		var inner []json.RawMessage
		if err := json.Unmarshal(s[1], &inner); err != nil {
			if debug {
				fmt.Fprintf(os.Stderr, "api: deep-research session #%d: decode inner: %v\n", i, err)
			}
			continue
		}
		ds := deepResearchSession{ConversationID: conv}
		if len(inner) > 0 {
			_ = json.Unmarshal(inner[0], &ds.ProjectID)
		}
		if len(inner) > 1 {
			var pair []json.RawMessage
			if json.Unmarshal(inner[1], &pair) == nil && len(pair) > 0 {
				_ = json.Unmarshal(pair[0], &ds.Query)
			}
		}
		if len(inner) > 2 {
			_ = json.Unmarshal(inner[2], &ds.Mode)
		}
		if len(inner) > 3 && !bytes.Equal(inner[3], []byte("null")) {
			ds.MainBlob = inner[3]
		}
		if len(inner) > 4 {
			_ = json.Unmarshal(inner[4], &ds.State)
		}
		if len(inner) > 5 {
			var pair []json.RawMessage
			if json.Unmarshal(inner[5], &pair) == nil {
				if len(pair) > 0 {
					_ = json.Unmarshal(pair[0], &ds.ResearchID)
				}
				if len(pair) > 1 {
					var b64 string
					if json.Unmarshal(pair[1], &b64) == nil {
						if decoded, err := base64.StdEncoding.DecodeString(b64); err == nil {
							ds.Plan = decoded
						}
					}
				}
			}
		}
		sessions = append(sessions, ds)
	}
	return sessions, nil
}

// decodeDeepResearchContent splits a main_blob into its markdown report
// and discovered-sources list. Layout per CDP capture 2026-04-17:
//
//	main_blob     = [[ report_header, source_1, ..., source_N ]]
//	report_header = [null, title, null, mode, null, null, [markdown, 3, ...]]
//	source_i      = [url, title, snippet, rank, null, favicon,
//	                 metadata, null, citation_idx]
func decodeDeepResearchContent(main json.RawMessage) (string, []ResearchSource) {
	var outer [][]json.RawMessage
	if err := json.Unmarshal(main, &outer); err != nil || len(outer) == 0 {
		return "", nil
	}
	entries := outer[0]
	if len(entries) == 0 {
		return "", nil
	}

	report := ""
	{
		var header []json.RawMessage
		if err := json.Unmarshal(entries[0], &header); err == nil && len(header) > 6 {
			var body []json.RawMessage
			if json.Unmarshal(header[6], &body) == nil && len(body) > 0 {
				_ = json.Unmarshal(body[0], &report)
			}
		}
	}

	var sources []ResearchSource
	for i := 1; i < len(entries); i++ {
		var src []json.RawMessage
		if err := json.Unmarshal(entries[i], &src); err != nil || len(src) < 3 {
			continue
		}
		rs := ResearchSource{}
		_ = json.Unmarshal(src[0], &rs.URL)
		_ = json.Unmarshal(src[1], &rs.Title)
		_ = json.Unmarshal(src[2], &rs.Snippet)
		if len(src) > 3 {
			_ = json.Unmarshal(src[3], &rs.Rank)
		}
		if len(src) > 5 {
			_ = json.Unmarshal(src[5], &rs.FaviconURL)
		}
		if len(src) > 8 {
			_ = json.Unmarshal(src[8], &rs.CitationIndex)
		}
		sources = append(sources, rs)
	}
	return report, sources
}

// decodeFastMainBlob splits a fast-mode main_blob into the sources
// list and the trailing summary string. Layout per CDP capture
// 2026-04-17:
//
//	main_blob = [ [source_1, ..., source_N], summary_string ]
//	source_i  = [url, title, snippet, rank]
//
// Fast-mode responses have no markdown report; the summary string is
// returned in the Report field of DeepResearchResult so callers have
// a single shape to render regardless of mode.
func decodeFastMainBlob(main json.RawMessage) (string, []ResearchSource) {
	var outer []json.RawMessage
	if err := json.Unmarshal(main, &outer); err != nil || len(outer) < 1 {
		return "", nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(outer[0], &entries); err != nil {
		return "", nil
	}
	sources := make([]ResearchSource, 0, len(entries))
	for _, raw := range entries {
		var src []json.RawMessage
		if err := json.Unmarshal(raw, &src); err != nil || len(src) < 3 {
			continue
		}
		rs := ResearchSource{}
		_ = json.Unmarshal(src[0], &rs.URL)
		_ = json.Unmarshal(src[1], &rs.Title)
		_ = json.Unmarshal(src[2], &rs.Snippet)
		if len(src) > 3 {
			_ = json.Unmarshal(src[3], &rs.Rank)
		}
		sources = append(sources, rs)
	}
	summary := ""
	if len(outer) > 1 {
		_ = json.Unmarshal(outer[1], &summary)
	}
	return summary, sources
}

// DeleteDeepResearch soft-deletes a research session. The server moves
// the session from state 2 (COMPLETE) to state 5 (DELETED) and retains
// the content internally; PollDeepResearch filters state=5 out so from
// the CLI caller's perspective the session is gone.
//
// Wire shape verified 2026-04-17 via CDP capture. Args: four positions.
//
//	[0] nil               placeholder
//	[1] [1]               opaque constant; bytes captured verbatim
//	[2] conversation_id   LBwxtb keys on the conversation identifier
//	                      returned as data[1] from QA9ei (NOT research_id)
//	[3] project_id
//
// LBwxtb is polymorphic — the same RPC also serves
// BulkImportFromResearch (5-position, adds a sources array at
// position [4]). The server discriminates on arg-4 presence, NOT on a
// distinct type flag. See BulkImportFromResearch for the 5-position
// shape.
//
// Response: empty JSON array on success.
func (c *Client) DeleteDeepResearch(ctx context.Context, projectID, conversationID string) error {
	_, err := c.orchestrationService.DeleteDeepResearch(ctx, &pb.DeleteDeepResearchRequest{
		ProjectId:      projectID,
		ConversationId: conversationID,
	})
	if err != nil {
		return fmt.Errorf("delete deep research: %w", err)
	}
	return nil
}

// deleteDeepResearchArgs is the legacy 4-position LBwxtb shape retained as a
// fixture oracle. Live calls use the generated request encoder above.
func deleteDeepResearchArgs(conversationID, projectID string) []interface{} {
	return []interface{}{
		nil,
		[]interface{}{1},
		conversationID,
		projectID,
	}
}

// BulkImportSource is one URL + title pair to import via
// BulkImportFromResearch. The server fills in everything else
// (source_id, content hash, timestamps, rank, etc.) and returns the
// enriched metadata on the response; BulkImportResult surfaces the
// subset the CLI cares about.
type BulkImportSource struct {
	URL   string
	Title string
}

// BulkImportResult is one server-assigned imported source in the
// BulkImportFromResearch response. Order matches the request order.
type BulkImportResult struct {
	SourceID string
	Title    string
	URL      string
}

// BulkImportFromResearch imports a batch of URL-and-title pairs into
// notebookID using the LBwxtb polymorphic extension (5-position
// variant). The conversationID identifies a research session whose
// suggestions are being imported — typically from a fast- or
// deep-research run. The server assigns source ids and returns the
// enriched metadata in request order.
//
// Wire shape (HAR-verified 2026-04-17 against notebook
// 00000000-0000-4000-8000-000000000006, conversation
// 00000000-0000-4000-8000-000000000401, 10 URL sources):
//
//	[
//	  null,                // [0] placeholder
//	  [1],                 // [1] opaque constant; same as delete shape
//	  conversation_id,     // [2] research session conversation id
//	  project_id,          // [3] target notebook
//	  [source_1, ..., source_N],   // [4] distinguishes bulk-import from delete
//	]
//
// Each source tuple is 11-position:
//
//	[null, null, [url, title], null, null, null, null, null, null, null, 2]
//
// Position [2] is [url, title]; position [10] is the source_type enum
// (2 observed for URL sources in this capture).
func (c *Client) BulkImportFromResearch(ctx context.Context, projectID, conversationID string, sources []BulkImportSource) ([]BulkImportResult, error) {
	if len(sources) == 0 {
		return nil, fmt.Errorf("bulk import: at least one source required")
	}
	resp, err := c.rpc.Do(ctx, rpc.Call{
		ID:         rpc.RPCBulkImportFromResearch,
		NotebookID: projectID,
		Args:       bulkImportArgs(conversationID, projectID, sources),
	})
	if err != nil {
		return nil, fmt.Errorf("bulk import from research: %w", err)
	}
	var wire pb.BulkImportFromResearchResponse
	if err := c.unmarshal(resp, &wire); err != nil {
		return nil, fmt.Errorf("bulk import from research: decode response: %w", err)
	}
	return bulkImportResultsFromProto(&wire), nil
}

// bulkImportArgs produces the 5-position LBwxtb bulk-import shape.
func bulkImportArgs(conversationID, projectID string, sources []BulkImportSource) []interface{} {
	tuples := make([]interface{}, 0, len(sources))
	for _, s := range sources {
		tuples = append(tuples, []interface{}{
			nil, nil,
			[]interface{}{s.URL, s.Title},
			nil, nil, nil, nil, nil, nil, nil,
			2, // source_type enum: URL
		})
	}
	return []interface{}{
		nil,
		[]interface{}{1},
		conversationID,
		projectID,
		tuples,
	}
}

// bulkImportResultsFromProto preserves the narrow public projection returned
// by BulkImportFromResearch while the generated Source message owns wire
// decoding.
func bulkImportResultsFromProto(response *pb.BulkImportFromResearchResponse) []BulkImportResult {
	if response == nil {
		return nil
	}
	results := make([]BulkImportResult, 0, len(response.GetResults()))
	for _, source := range response.GetResults() {
		if source == nil {
			continue
		}
		result := BulkImportResult{
			SourceID: source.GetSourceId().GetSourceId(),
			Title:    source.GetTitle(),
		}
		if metadata := source.GetMetadata(); metadata != nil {
			if urls := metadata.GetSourceUrls(); len(urls) > 0 {
				result.URL = urls[0]
			}
		}
		results = append(results, result)
	}
	return results
}
