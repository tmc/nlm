package richrender

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/tmc/nlm/notebooklm"
)

func TestChatStreamRendererJSONLCitationTitles(t *testing.T) {
	tests := []struct {
		name   string
		title  string
		titles map[string]string
		want   string
	}{
		{"parent", "", map[string]string{"parent": "Guide", "chunk": "Chunk"}, "Guide"},
		{"chunk fallback", "", map[string]string{"chunk": "Legacy guide"}, "Legacy guide"},
		{"server title", "Original guide", map[string]string{"parent": "Renamed guide"}, "Original guide"},
		{"missing source", "", map[string]string{}, ""},
		{"offline", "", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			r := newChatStreamRenderer(&out, io.Discard, false, false, citationModeJSON)
			r.jsonl = true
			if tt.titles != nil {
				r.resolveTitle = func(id string) string { return tt.titles[id] }
			}
			r.WriteChunk(notebooklm.ChatChunk{
				Phase: notebooklm.ChatChunkAnswer,
				Text:  "A claim.",
				Citations: []notebooklm.Citation{{
					SourceIndex: 1, SourceID: "chunk", ParentSourceID: "parent",
					Title: tt.title, StartChar: 0, EndChar: 8,
				}},
			})
			r.Finish()
			var citations []map[string]any
			for _, event := range parseJSONLEvents(t, out.String()) {
				if event["phase"] == "citation" {
					citations = append(citations, event)
				}
			}
			if len(citations) != 1 {
				t.Fatalf("got %d citation events, want 1", len(citations))
			}
			c := citations[0]
			if c["title"] != tt.want {
				t.Errorf("title = %q, want %q", c["title"], tt.want)
			}
			if c["source_id"] != "chunk" || c["parent_source_id"] != "parent" || c["start_char"] != float64(0) || c["end_char"] != float64(8) {
				t.Errorf("title lookup changed citation identity or range: %v", c)
			}
		})
	}
}

func TestChatStreamRendererNonTTYDropsThinkingOutput(t *testing.T) {
	var out bytes.Buffer
	var status bytes.Buffer

	r := newChatStreamRenderer(&out, &status, false, false, citationModeOff)
	r.WriteChunk(notebooklm.ChatChunk{
		Phase:  notebooklm.ChatChunkThinking,
		Header: "**Thinking**",
		Text:   "**Thinking**\nPlanning response",
	})
	r.WriteChunk(notebooklm.ChatChunk{
		Phase: notebooklm.ChatChunkAnswer,
		Text:  "Hello, world.",
	})
	r.Finish()

	if got := out.String(); got != "Hello, world." {
		t.Fatalf("answer output = %q, want %q", got, "Hello, world.")
	}
	if got := status.String(); got != "" {
		t.Fatalf("status output = %q, want empty", got)
	}
	if got := r.Thinking(); got != "**Thinking**\nPlanning response" {
		t.Fatalf("thinking trace = %q", got)
	}
}

func TestChatStreamRendererThinkingReplacesCumulativeSnapshots(t *testing.T) {
	var out, status bytes.Buffer
	r := newChatStreamRenderer(&out, &status, false, false, citationModeOff)
	r.WriteChunk(notebooklm.ChatChunk{Phase: notebooklm.ChatChunkThinking, Header: "**T**", Text: "**T**\nShip"})
	r.WriteChunk(notebooklm.ChatChunk{Phase: notebooklm.ChatChunkThinking, Header: "**T**", Text: "**T**\nShip a thin wrapper"})
	r.WriteChunk(notebooklm.ChatChunk{Phase: notebooklm.ChatChunkThinking, Header: "**T**", Text: "**T**\nShip a thin wrapper via cmd/cove-serve"})
	r.Finish()

	want := "**T**\nShip a thin wrapper via cmd/cove-serve"
	if got := r.Thinking(); got != want {
		t.Fatalf("thinking trace = %q, want %q", got, want)
	}
}

// Answer deltas restart at the divergence point when the server revises
// earlier text, so the accumulated answer must track the cumulative snapshot
// (Full), not a concatenation of deltas — that duplicates the tail.
func TestChatStreamRendererAnswerRevisionReplacesFromFullSnapshot(t *testing.T) {
	chunks := []notebooklm.ChatChunk{
		{Phase: notebooklm.ChatChunkAnswer, Text: "Section one.", Full: "Section one."},
		{Phase: notebooklm.ChatChunkAnswer, Text: " Section two.", Full: "Section one. Section two."},
		// Revision: the server changed "one" → "1", so the delta re-emits
		// everything after the divergence point.
		{Phase: notebooklm.ChatChunkAnswer, Text: "1. Section two. Section three.", Full: "Section 1. Section two. Section three."},
	}
	const want = "Section 1. Section two. Section three."

	var out, status bytes.Buffer
	r := newChatStreamRenderer(&out, &status, false, false, citationModeOff)
	for _, c := range chunks {
		r.WriteChunk(c)
	}
	r.Finish()
	if got := r.Answer(); got != want {
		t.Fatalf("Answer() = %q, want %q", got, want)
	}

	var jout, jstatus bytes.Buffer
	jr := newChatStreamRenderer(&jout, &jstatus, false, false, citationModeOff)
	jr.jsonl = true
	for _, c := range chunks {
		jr.WriteChunk(c)
	}
	jr.Finish()
	if got := jr.Answer(); got != want {
		t.Fatalf("jsonl Answer() = %q, want %q", got, want)
	}
}

// A revision to already-streamed text must not re-emit the tail: output is
// monotone by offset, the revised span keeps its old rendering, the buffer
// stays exact, and Finish notes the divergence on the status stream.
func TestChatStreamRendererAnswerStreamsMonotoneOnRevision(t *testing.T) {
	chunks := []notebooklm.ChatChunk{
		{Phase: notebooklm.ChatChunkAnswer, Text: "Section one.", Full: "Section one."},
		{Phase: notebooklm.ChatChunkAnswer, Text: " Section two.", Full: "Section one. Section two."},
		{Phase: notebooklm.ChatChunkAnswer, Text: "1. Section two. Section three.", Full: "Section 1. Section two. Section three."},
	}
	const want = "Section 1. Section two. Section three."

	var out, status bytes.Buffer
	r := newChatStreamRenderer(&out, &status, false, false, citationModeOff)
	for _, c := range chunks {
		r.WriteChunk(c)
	}
	r.Finish()

	// The first 25 bytes streamed before the revision and keep their old
	// rendering; everything past them streams from the revised snapshot.
	wantOut := "Section one. Section two." + want[len("Section one. Section two."):]
	if got := out.String(); got != wantOut {
		t.Fatalf("streamed output = %q, want %q", got, wantOut)
	}
	if got := r.Answer(); got != want {
		t.Fatalf("Answer() = %q, want %q", got, want)
	}
	if !strings.Contains(status.String(), "revised") {
		t.Fatalf("status = %q, want a revision notice", status.String())
	}
}

// The JSONL done event carries the authoritative full answer and flags a
// revision, since concatenating the monotone answer events keeps a revised
// span at its old rendering.
func TestChatStreamRendererJSONLDoneCarriesAuthoritativeAnswer(t *testing.T) {
	var out, status bytes.Buffer
	r := newChatStreamRenderer(&out, &status, false, false, citationModeOff)
	r.jsonl = true
	r.WriteChunk(notebooklm.ChatChunk{Phase: notebooklm.ChatChunkAnswer, Text: "Section one.", Full: "Section one."})
	r.WriteChunk(notebooklm.ChatChunk{Phase: notebooklm.ChatChunkAnswer, Text: "1. Section two.", Full: "Section 1. Section two."})
	r.Finish()

	events := parseJSONLEvents(t, out.String())
	if got := events[1]["text"]; got != "ection two." {
		t.Fatalf("second answer event = %q, want monotone extension %q", got, "ection two.")
	}
	done := events[len(events)-1]
	if done["phase"] != "done" {
		t.Fatalf("last event phase = %v, want done", done["phase"])
	}
	if got := done["answer"]; got != "Section 1. Section two." {
		t.Fatalf("done answer = %q, want %q", got, "Section 1. Section two.")
	}
	if done["revised"] != true {
		t.Fatalf("done revised = %v, want true", done["revised"])
	}
}

func TestChatStreamRendererThinkingModes(t *testing.T) {
	t.Run("header-only", func(t *testing.T) {
		var out bytes.Buffer
		var status bytes.Buffer

		r := newChatStreamRenderer(&out, &status, true, false, citationModeOff)
		r.WriteChunk(notebooklm.ChatChunk{
			Phase:  notebooklm.ChatChunkThinking,
			Header: "**Thinking**",
			Text:   "**Thinking**\nPlanning response",
		})
		r.WriteChunk(notebooklm.ChatChunk{
			Phase: notebooklm.ChatChunkAnswer,
			Text:  "Answer",
		})
		r.Finish()

		if got := out.String(); got != "Answer" {
			t.Fatalf("answer output = %q, want %q", got, "Answer")
		}
		if got := status.String(); !strings.Contains(got, "[thinking] Thinking") {
			t.Fatalf("status output = %q, want thinking header", got)
		}
		if got := status.String(); !strings.Contains(got, "\r") {
			t.Fatalf("status output = %q, want carriage-return clear", got)
		}
	})

	t.Run("non-tty-with-thinking-flag", func(t *testing.T) {
		var out bytes.Buffer
		var status bytes.Buffer

		r := newChatStreamRenderer(&out, &status, true, false, citationModeOff)
		r.WriteChunk(notebooklm.ChatChunk{
			Phase:  notebooklm.ChatChunkThinking,
			Header: "**Thinking**",
			Text:   "**Thinking**\nPlanning response",
		})
		r.WriteChunk(notebooklm.ChatChunk{
			Phase: notebooklm.ChatChunkAnswer,
			Text:  "Answer",
		})
		r.Finish()

		if got := out.String(); got != "Answer" {
			t.Fatalf("answer output = %q, want %q", got, "Answer")
		}
		if got := status.String(); !strings.Contains(got, "[thinking] Thinking") {
			t.Fatalf("status output = %q, want thinking header", got)
		}
	})

	t.Run("verbose", func(t *testing.T) {
		var out bytes.Buffer
		var status bytes.Buffer

		r := newChatStreamRenderer(&out, &status, true, true, citationModeOff)
		r.WriteChunk(notebooklm.ChatChunk{
			Phase:  notebooklm.ChatChunkThinking,
			Header: "**Thinking**",
			Text:   "**Thinking**\nPlanning response",
		})

		got := status.String()
		if !strings.Contains(got, ansiGrey+"**Thinking**\nPlanning response"+ansiReset+"\n") {
			t.Fatalf("status output = %q, want verbose thinking trace", got)
		}
	})
}

func TestChatStreamRendererCitationList(t *testing.T) {
	var out, status bytes.Buffer
	r := newChatStreamRenderer(&out, &status, false, false, citationModeList)
	r.resolveTitle = func(id string) string {
		if id == "src_aaa" {
			return "Installation Guide"
		}
		return ""
	}
	r.WriteChunk(notebooklm.ChatChunk{
		Phase: notebooklm.ChatChunkAnswer,
		Text:  "Answer body.",
		Citations: []notebooklm.Citation{
			{SourceIndex: 1, SourceID: "src_aaa", Title: "ignored when resolver hits", StartChar: 0, EndChar: 12, Confidence: 0.82},
			{SourceIndex: 2, SourceID: "src_bbb_longidentifier", Title: "Fallback excerpt", StartChar: 20, EndChar: 40},
		},
	})
	r.Finish()

	if got := out.String(); got != "Answer body." {
		t.Fatalf("answer output = %q, want %q", got, "Answer body.")
	}
	s := status.String()
	if !strings.Contains(s, "Citations:") {
		t.Fatalf("status missing Citations header: %q", s)
	}
	// Index, resolved title, confidence column, and span column all present.
	if !strings.Contains(s, "[1]") || !strings.Contains(s, "Installation Guide") {
		t.Fatalf("status missing resolved title: %q", s)
	}
	if !strings.Contains(s, "p=0.82") {
		t.Fatalf("status missing per-source confidence: %q", s)
	}
	if !strings.Contains(s, "answer 0-12") {
		t.Fatalf("status missing labeled answer span: %q", s)
	}
	// Source handle renders as an 8-char prefix (not the full id) before the title.
	if !strings.Contains(s, "src_bbb_") {
		t.Fatalf("status missing source handle: %q", s)
	}
	if strings.Contains(s, "src_bbb_longidentifier") {
		t.Fatalf("source handle should be truncated to 8 chars, got full id: %q", s)
	}
	if !strings.Contains(s, "Fallback excerpt") {
		t.Fatalf("status missing fallback title: %q", s)
	}
	// The answer body must never be spliced; superscripts are gone entirely.
	if strings.ContainsAny(out.String(), "¹²³") {
		t.Fatalf("answer body was spliced: %q", out.String())
	}
}

// TestChatStreamRendererCitationScan checks the default scan layout: the [n]
// marker heads its group with only the answer span (labeled), then one row per
// source led by that source's OWN confidence. Confidence is per-source and must
// never be hoisted to the marker header; the answer span rides the header once.
func TestChatStreamRendererCitationScan(t *testing.T) {
	var status bytes.Buffer
	r := newChatStreamRenderer(io.Discard, &status, false, false, citationModeList)
	r.citations = []notebooklm.Citation{
		// Marker 1 cites two sources with DIFFERENT scores — the case the old
		// header-hoist silently dropped.
		{SourceIndex: 1, SourceID: "aaaaaaaa-1", Title: "Alpha", StartChar: 42, EndChar: 205, Confidence: 0.91},
		{SourceIndex: 1, SourceID: "bbbbbbbb-2", Title: "Beta", StartChar: 42, EndChar: 205, Confidence: 0.71},
		// Source aaaaaaaa reappears under marker 2 with a different answer span.
		{SourceIndex: 2, SourceID: "aaaaaaaa-1", Title: "Alpha", StartChar: 40, EndChar: 55, Confidence: 0.90},
	}
	r.renderCitationList()
	s := status.String()

	// Marker 1 header carries the labeled answer span, no confidence.
	if !strings.Contains(s, "[1] answer 42-205") {
		t.Fatalf("marker 1 header wrong (want labeled answer span, no p=):\n%s", s)
	}
	// BOTH per-source scores appear — the whole point of the fix.
	if !strings.Contains(s, `p=0.91 aaaaaaaa "Alpha"`) {
		t.Fatalf("missing high-confidence source row:\n%s", s)
	}
	if !strings.Contains(s, `bbbbbbbb "Beta"`) || !strings.Contains(s, "p=0.71") {
		t.Fatalf("missing low-confidence source row (both scores must show):\n%s", s)
	}
	// The weak score (<0.75) is amber; the strong one is not.
	if !strings.Contains(s, ansiAmber+"p=0.71"+ansiReset) {
		t.Fatalf("weak confidence should render amber:\n%q", s)
	}
	if strings.Contains(s, ansiAmber+"p=0.91") {
		t.Fatalf("strong confidence should NOT be amber:\n%q", s)
	}
	// Confidence must never appear on a marker header line.
	for line := range strings.SplitSeq(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "[") && strings.Contains(line, "p=") {
			t.Fatalf("confidence leaked onto a marker header line: %q", line)
		}
	}
	// Marker 2 is its own group with its own answer span and source row.
	if !strings.Contains(s, "[2] answer 40-55") || !strings.Contains(s, `p=0.90 aaaaaaaa "Alpha"`) {
		t.Fatalf("marker 2 group wrong:\n%s", s)
	}
}

// TestChatStreamRendererCitationExpanded checks that --citation-excerpts
// switches to the expanded audit view: marker header, then one row per source
// carrying its own confidence AND its source-offset locator, with the
// server-supplied excerpt indented beneath.
func TestChatStreamRendererCitationExpanded(t *testing.T) {
	var status bytes.Buffer
	r := newChatStreamRenderer(io.Discard, &status, false, false, citationModeList)
	r.excerptBudget = 40 // enables the expanded view
	r.citations = []notebooklm.Citation{
		{SourceIndex: 1, SourceID: "aaaaaaaa-1", Title: "Alpha", StartChar: 4, EndChar: 9, Confidence: 0.82, SourceStart: 965670, SourceEnd: 966914, Excerpt: "The alpha source passage that grounds this."},
		{SourceIndex: 1, SourceID: "bbbbbbbb-2", Title: "Beta", StartChar: 4, EndChar: 9, Confidence: 0.82, SourceStart: 12000, SourceEnd: 12050, Excerpt: "The beta source passage that grounds this."},
	}
	r.Finish()
	s := status.String()

	// Marker header carries only the labeled answer span.
	if !strings.Contains(s, "[1] answer 4-9") {
		t.Fatalf("expanded marker header missing:\n%s", s)
	}
	// Each source on its own row with its own confidence and its source span.
	if !strings.Contains(s, `p=0.82 aaaaaaaa "Alpha"`) || !strings.Contains(s, `p=0.82 bbbbbbbb "Beta"`) {
		t.Fatalf("expanded source rows missing per-source confidence:\n%s", s)
	}
	if !strings.Contains(s, "src 965670-966914") || !strings.Contains(s, "src 12000-12050") {
		t.Fatalf("expanded per-source source-offset locators missing:\n%s", s)
	}
	if !strings.Contains(s, "alpha source passage") || !strings.Contains(s, "beta source passage") {
		t.Fatalf("expanded per-source excerpts missing:\n%s", s)
	}
}

// TestChatStreamRendererCitationPerSourceConfidence checks that when a marker's
// sources have differing scores, BOTH render on their own rows — the exact case
// the old uniformConfidence header-hoist silently dropped. There is no "mixed"
// fallback anymore: confidence is per-source, always.
func TestChatStreamRendererCitationPerSourceConfidence(t *testing.T) {
	var status bytes.Buffer
	r := newChatStreamRenderer(io.Discard, &status, false, false, citationModeList)
	r.citations = []notebooklm.Citation{
		{SourceIndex: 1, SourceID: "aaaaaaaa-1", Title: "Alpha", StartChar: 5, EndChar: 9, Confidence: 0.82},
		{SourceIndex: 1, SourceID: "bbbbbbbb-2", Title: "Beta", StartChar: 5, EndChar: 9, Confidence: 0.60},
	}
	r.renderCitationList()
	s := status.String()

	if !strings.Contains(s, "p=0.82") || !strings.Contains(s, "p=0.60") {
		t.Fatalf("both per-source scores must render, not be collapsed:\n%s", s)
	}
	// The weak one (0.60 < 0.75) is amber; the strong one is not.
	if !strings.Contains(s, ansiAmber+"p=0.60"+ansiReset) {
		t.Fatalf("weak per-source confidence should be amber:\n%q", s)
	}
	// The shared answer span heads the marker once.
	if !strings.Contains(s, "[1] answer 5-9") {
		t.Fatalf("shared answer span should head the marker:\n%s", s)
	}
}

// TestChatStreamRendererCitationMixedSpan checks a marker whose sources carry
// differing answer spans: the first span heads the group and the rest are
// counted, rather than the span silently dropping. Per-source confidence is
// unaffected — it still renders on every row.
func TestChatStreamRendererCitationMixedSpan(t *testing.T) {
	var status bytes.Buffer
	r := newChatStreamRenderer(io.Discard, &status, false, false, citationModeList)
	r.citations = []notebooklm.Citation{
		{SourceIndex: 1, SourceID: "aaaaaaaa-1", Title: "Alpha", StartChar: 5, EndChar: 9, Confidence: 0.82},
		{SourceIndex: 1, SourceID: "bbbbbbbb-2", Title: "Beta", StartChar: 20, EndChar: 30, Confidence: 0.82},
	}
	r.renderCitationList()
	s := status.String()
	if !strings.Contains(s, "[1] answer 5-9 +1 more") {
		t.Fatalf("differing answer spans should head the marker with a count: %q", s)
	}
	// Per-source confidence still renders on the rows.
	if !strings.Contains(s, "p=0.82") {
		t.Fatalf("per-source confidence should still render for a mixed-span group: %q", s)
	}
}

// TestChatStreamRendererCitationRepeatedMarker checks the collapse that keeps a
// marker used many times in one answer from printing the same row many times.
// The server sends one citation per appearance of [n], every one naming the
// same source passage and differing only in its answer span, so the passage
// prints once and the extra appearances are counted on the header.
func TestChatStreamRendererCitationRepeatedMarker(t *testing.T) {
	var status bytes.Buffer
	r := newChatStreamRenderer(io.Discard, &status, false, false, citationModeList)
	for _, span := range [][2]int{{5, 9}, {20, 30}, {41, 41}} {
		r.citations = append(r.citations, notebooklm.Citation{
			SourceIndex: 1, SourceID: "aaaaaaaa-1", Title: "Alpha",
			StartChar: span[0], EndChar: span[1],
			SourceStart: 100, SourceEnd: 200, Excerpt: "x", Confidence: 0.82,
		})
	}
	r.renderCitationList()
	s := status.String()
	if n := strings.Count(s, "aaaaaaaa"); n != 1 {
		t.Fatalf("one passage should print one row, got %d:\n%s", n, s)
	}
	if !strings.Contains(s, "[1] answer 5-9 +2 more") {
		t.Fatalf("the repeated appearances should be counted on the header: %q", s)
	}
}

func TestChatStreamRendererCitationColumnToggles(t *testing.T) {
	newRenderer := func() (*bytes.Buffer, *StreamRenderer) {
		var status bytes.Buffer
		r := newChatStreamRenderer(io.Discard, &status, false, false, citationModeList)
		r.excerptBudget = 40 // expanded view so the src-offset locator is exercised too
		r.citations = []notebooklm.Citation{
			{SourceIndex: 1, SourceID: "src_aaaaaaaa", Title: "T", StartChar: 42, EndChar: 205, Confidence: 0.82, SourceStart: 1000, SourceEnd: 1050, Excerpt: "cited"},
		}
		return &status, r
	}

	// Both columns on (default): per-source confidence + labeled answer span.
	status, r := newRenderer()
	r.renderCitationList()
	if s := status.String(); !strings.Contains(s, "p=0.82") || !strings.Contains(s, "answer 42-205") {
		t.Fatalf("default should show confidence and answer span: %q", s)
	}

	// Confidence off: no p= on any row.
	status, r = newRenderer()
	r.showConfidence = false
	r.renderCitationList()
	if s := status.String(); strings.Contains(s, "p=") {
		t.Fatalf("--citation-confidence=off should drop the p column: %q", s)
	}

	// Spans off: no answer span on the header AND no src offset on the row.
	status, r = newRenderer()
	r.showSpans = false
	r.renderCitationList()
	if s := status.String(); strings.Contains(s, "answer ") || strings.Contains(s, "src ") {
		t.Fatalf("--citation-spans=off should drop both span labels: %q", s)
	}

	// Both off: plain [n] header + <id8> "title" row.
	status, r = newRenderer()
	r.showConfidence, r.showSpans = false, false
	r.renderCitationList()
	s := status.String()
	if strings.Contains(s, "p=") || strings.Contains(s, "answer ") || strings.Contains(s, "src ") {
		t.Fatalf("both off should be plain: %q", s)
	}
	if !strings.Contains(s, "[1]") || !strings.Contains(s, "src_aaaa") {
		t.Fatalf("plain form missing index/handle: %q", s)
	}
}

func TestFormatSpanLabels(t *testing.T) {
	cases := []struct {
		name           string
		start, end     int
		answer, source string
	}{
		{"range", 42, 205, "answer 42-205", "src 42-205"},
		{"point", 409, 409, "answer 409", "src 409"}, // degenerate point, no range
		{"zero sentinel", 0, 0, "", ""},              // (0,0) = "no span", never "answer 0"
		{"missing start", -1, 5, "", ""},
		{"inverted", 10, 5, "", ""},
	}
	for _, tc := range cases {
		if got := formatAnswerSpan(tc.start, tc.end); got != tc.answer {
			t.Errorf("%s: formatAnswerSpan(%d,%d) = %q, want %q", tc.name, tc.start, tc.end, got, tc.answer)
		}
		if got := formatSourceSpan(tc.start, tc.end); got != tc.source {
			t.Errorf("%s: formatSourceSpan(%d,%d) = %q, want %q", tc.name, tc.start, tc.end, got, tc.source)
		}
	}
}

func TestChatStreamRendererCitationModeOff(t *testing.T) {
	var out, status bytes.Buffer
	r := newChatStreamRenderer(&out, &status, false, false, citationModeOff)
	r.WriteChunk(notebooklm.ChatChunk{
		Phase: notebooklm.ChatChunkAnswer,
		Text:  "Answer.",
		Citations: []notebooklm.Citation{
			{SourceIndex: 1, SourceID: "src_a", Title: "noisy"},
		},
	})
	r.Finish()

	if got := status.String(); strings.Contains(got, "Sources:") {
		t.Fatalf("citation list leaked under off mode: %q", got)
	}
	if got := out.String(); got != "Answer." {
		t.Fatalf("answer = %q, want %q", got, "Answer.")
	}
}

func TestResolveCitationMode(t *testing.T) {
	cases := []struct {
		flag    string
		want    CitationMode
		wantStr string
	}{
		{"", citationModeList, "empty default = list"},
		{"auto", citationModeList, "auto = list"},
		{"tail", citationModeList, "tail = list (canonical alias)"},
		{"block", citationModeList, "deprecated block = list"},
		{"stream", citationModeList, "deprecated stream = list"},
		{"overlay", citationModeList, "removed overlay = list"},
		{"json", citationModeJSON, "json"},
		{"jsonl", citationModeJSON, "jsonl alias"},
		{"off", citationModeOff, "explicit off"},
		{"none", citationModeOff, "none alias"},
		{"nonsense", citationModeList, "unknown falls through to list"},
	}
	for _, tc := range cases {
		if got := resolveCitationMode(tc.flag); got != tc.want {
			t.Errorf("%s: resolveCitationMode(%q) = %v, want %v", tc.wantStr, tc.flag, got, tc.want)
		}
	}
}

func TestCitationModeIsDeprecatedAlias(t *testing.T) {
	deprecated := map[string]string{
		"block":         "block",
		"stream":        "stream",
		"inline-footer": "stream",
		"overlay":       "overlay",
		"footnote":      "overlay",
	}
	for flag, want := range deprecated {
		if got := citationModeIsDeprecatedAlias(flag); got != want {
			t.Errorf("citationModeIsDeprecatedAlias(%q) = %q, want %q", flag, got, want)
		}
	}
	for _, live := range []string{"", "tail", "off", "json", "nonsense"} {
		if got := citationModeIsDeprecatedAlias(live); got != "" {
			t.Errorf("citationModeIsDeprecatedAlias(%q) = %q, want empty (not deprecated)", live, got)
		}
	}
}

func TestRenderPersistedAssistantList(t *testing.T) {
	var out, status bytes.Buffer
	msg := storedMessage{
		Role:    "assistant",
		Content: "Answer body.",
		Citations: []notebooklm.Citation{
			{SourceIndex: 1, SourceID: "src_a", Title: "First"},
			{SourceIndex: 2, SourceID: "src_b", Title: "Second"},
		},
	}
	renderPersistedAssistant(&out, &status, msg, citationModeList, persistedRenderConfig{})
	if !strings.Contains(out.String(), "Answer body.") {
		t.Fatalf("body missing from stdout: %q", out.String())
	}
	s := status.String()
	if !strings.Contains(s, "Citations:") {
		t.Fatalf("list footer missing: %q", s)
	}
	if !strings.Contains(s, "[1]") || !strings.Contains(s, "src_a \"First\"") {
		t.Fatalf("entry 1 missing: %q", s)
	}
	if !strings.Contains(s, "[2]") || !strings.Contains(s, "src_b \"Second\"") {
		t.Fatalf("entry 2 missing: %q", s)
	}
}

func TestRenderPersistedAssistantNoCitations(t *testing.T) {
	var out, status bytes.Buffer
	msg := storedMessage{Role: "assistant", Content: "Plain answer."}
	renderPersistedAssistant(&out, &status, msg, citationModeList, persistedRenderConfig{})
	if got := out.String(); !strings.HasPrefix(got, "Plain answer.") {
		t.Fatalf("body missing: %q", got)
	}
	if status.String() != "" {
		t.Fatalf("footer should be empty for no citations, got %q", status.String())
	}
}

// parseJSONLEvents splits newline-delimited JSON on stdout into a slice of
// decoded events. Any non-object line fails the calling test.
func parseJSONLEvents(t *testing.T, raw string) []map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")
	events := make([]map[string]any, 0, len(lines))
	for i, line := range lines {
		if line == "" {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("line %d not valid JSON: %v\nline=%q", i, err, line)
		}
		events = append(events, ev)
	}
	return events
}

func TestChatStreamRendererJSONLEmitsTypedEvents(t *testing.T) {
	var out, status bytes.Buffer
	r := newChatStreamRenderer(&out, &status, false, false, citationModeOff)
	r.jsonl = true
	r.jsonlIncludeThinking = true

	r.WriteChunk(notebooklm.ChatChunk{
		Phase:  notebooklm.ChatChunkThinking,
		Header: "**Thinking**",
		Text:   "**Thinking**\nPlanning response",
	})
	r.WriteChunk(notebooklm.ChatChunk{
		Phase: notebooklm.ChatChunkAnswer,
		Text:  "Hello, ",
	})
	r.WriteChunk(notebooklm.ChatChunk{
		Phase: notebooklm.ChatChunkAnswer,
		Text:  "world.",
		Citations: []notebooklm.Citation{
			{SourceIndex: 1, SourceID: "src_aaa", Title: "Guide", StartChar: 0, EndChar: 12, Confidence: 0.87},
		},
		FollowUps: []string{"Tell me more", "Different angle"},
	})
	r.Finish()

	// Human status stream must stay empty under jsonl mode — no ANSI chatter.
	if got := status.String(); got != "" {
		t.Fatalf("jsonl mode leaked status output: %q", got)
	}

	events := parseJSONLEvents(t, out.String())
	if len(events) < 5 {
		t.Fatalf("expected at least 5 events (thinking, 2×answer, citation, done), got %d: %+v", len(events), events)
	}

	// First event is the thinking trace.
	if events[0]["phase"] != "thinking" {
		t.Fatalf("events[0].phase = %v, want thinking", events[0]["phase"])
	}
	if !strings.Contains(events[0]["text"].(string), "Planning response") {
		t.Fatalf("events[0].text missing trace body: %v", events[0]["text"])
	}

	// Second + third are answer deltas preserving order.
	if events[1]["phase"] != "answer" || events[1]["text"] != "Hello, " {
		t.Fatalf("events[1] = %v, want answer 'Hello, '", events[1])
	}
	if events[2]["phase"] != "answer" || events[2]["text"] != "world." {
		t.Fatalf("events[2] = %v, want answer 'world.'", events[2])
	}

	// Fourth is the citation event.
	if events[3]["phase"] != "citation" {
		t.Fatalf("events[3].phase = %v, want citation", events[3]["phase"])
	}
	if events[3]["source_id"] != "src_aaa" {
		t.Fatalf("citation source_id = %v, want src_aaa", events[3]["source_id"])
	}
	if got, want := events[3]["confidence"].(float64), 0.87; got != want {
		t.Fatalf("citation confidence = %v, want %v", got, want)
	}

	// Last event is done, preceded by followup events.
	last := events[len(events)-1]
	if last["phase"] != "done" {
		t.Fatalf("last event = %v, want phase=done", last)
	}
	sawFollowup := 0
	for _, ev := range events {
		if ev["phase"] == "followup" {
			sawFollowup++
		}
	}
	if sawFollowup != 2 {
		t.Fatalf("expected 2 followup events, got %d", sawFollowup)
	}
}

func TestChatStreamRendererJSONLCumulativeThinkingNotDuplicated(t *testing.T) {
	var out, status bytes.Buffer
	r := newChatStreamRenderer(&out, &status, false, false, citationModeOff)
	r.jsonl = true
	r.jsonlIncludeThinking = true

	// Thinking arrives as cumulative snapshots; jsonl must emit once per
	// change, not once per snapshot.
	r.WriteChunk(notebooklm.ChatChunk{Phase: notebooklm.ChatChunkThinking, Text: "step 1"})
	r.WriteChunk(notebooklm.ChatChunk{Phase: notebooklm.ChatChunkThinking, Text: "step 1"}) // dup
	r.WriteChunk(notebooklm.ChatChunk{Phase: notebooklm.ChatChunkThinking, Text: "step 1\nstep 2"})
	r.Finish()

	events := parseJSONLEvents(t, out.String())
	thinkingCount := 0
	for _, ev := range events {
		if ev["phase"] == "thinking" {
			thinkingCount++
		}
	}
	if thinkingCount != 2 {
		t.Fatalf("thinking events = %d, want 2 (duplicate snapshot should not re-emit)", thinkingCount)
	}
}

func TestChatStreamRendererJSONLIsOptIn(t *testing.T) {
	// Without r.jsonl, output must match existing human-readable behavior
	// byte-for-byte — no regressions for users who didn't ask for JSON.
	var out, status bytes.Buffer
	r := newChatStreamRenderer(&out, &status, false, false, citationModeOff)
	r.WriteChunk(notebooklm.ChatChunk{Phase: notebooklm.ChatChunkAnswer, Text: "plain answer"})
	r.Finish()

	if got := out.String(); got != "plain answer" {
		t.Fatalf("non-jsonl stdout = %q, want plain answer (jsonl mode leaked into default path)", got)
	}
}

func TestChatStreamRendererJSONLGatesThinking(t *testing.T) {
	// --citations=json without --thinking suppresses thinking events on stdout
	// while still emitting answer + citation JSON-lines events.
	var out, status bytes.Buffer
	r := newChatStreamRenderer(&out, &status, false, false, citationModeJSON)
	r.jsonl = true
	// jsonlIncludeThinking intentionally left false.

	r.WriteChunk(notebooklm.ChatChunk{Phase: notebooklm.ChatChunkThinking, Text: "hidden trace"})
	r.WriteChunk(notebooklm.ChatChunk{
		Phase: notebooklm.ChatChunkAnswer,
		Text:  "answer body",
		Citations: []notebooklm.Citation{
			{SourceIndex: 1, SourceID: "s1", Title: "t", StartChar: 0, EndChar: 6},
		},
	})
	r.Finish()

	events := parseJSONLEvents(t, out.String())
	for _, ev := range events {
		if ev["phase"] == "thinking" {
			t.Fatalf("thinking event leaked into jsonl output without --thinking: %v", ev)
		}
	}
	sawAnswer, sawCitation := false, false
	for _, ev := range events {
		switch ev["phase"] {
		case "answer":
			sawAnswer = true
		case "citation":
			sawCitation = true
		}
	}
	if !sawAnswer || !sawCitation {
		t.Fatalf("expected answer + citation events, got %+v", events)
	}
}

// A guard-truncated stream must be marked incomplete on stdout itself (the
// captured artifact), and must not render the trailing citation list or
// follow-ups: resolving citations against a partial answer would present it
// as a finished one.
func TestChatStreamRendererAbortMarksIncompleteAndSkipsCitations(t *testing.T) {
	var out, status bytes.Buffer
	r := NewStreamRenderer(&out, &status, StreamOptions{
		Mode: CitationModeList,
		LoadSource: func(string) (notebooklm.LoadSourceText, error) {
			t.Fatal("citation resolution ran for an aborted stream")
			return notebooklm.LoadSourceText{}, nil
		},
	})
	r.WriteChunk(notebooklm.ChatChunk{
		Phase:     notebooklm.ChatChunkAnswer,
		Text:      "Partial answer.",
		Full:      "Partial answer.",
		Citations: []notebooklm.Citation{{SourceID: "abc", SourceIndex: 1}},
		FollowUps: []string{"Ask something else"},
	})
	r.Abort("the model repeated a 128-byte block 40 times")
	r.Finish()

	got := out.String()
	if !strings.HasPrefix(got, "Partial answer.") {
		t.Fatalf("stdout = %q, want the streamed text preserved", got)
	}
	if !strings.Contains(got, "INCOMPLETE") {
		t.Fatalf("stdout = %q, want an incompleteness marker on stdout", got)
	}
	if strings.Contains(status.String(), "Citations:") {
		t.Fatalf("status = %q, want no citation list", status.String())
	}
	if strings.Contains(status.String(), "Ask something else") {
		t.Fatalf("status = %q, want no follow-ups", status.String())
	}
}

func TestChatStreamRendererAbortJSONLEmitsAbortedEvent(t *testing.T) {
	var out, status bytes.Buffer
	r := NewStreamRenderer(&out, &status, StreamOptions{Mode: CitationModeJSON, JSONL: true})
	r.WriteChunk(notebooklm.ChatChunk{Phase: notebooklm.ChatChunkAnswer, Text: "Partial.", Full: "Partial."})
	r.Abort("answer exceeded the 1048576-byte limit")
	r.Finish()

	var last map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		last = map[string]any{}
		if err := json.Unmarshal([]byte(line), &last); err != nil {
			t.Fatalf("event %q: %v", line, err)
		}
	}
	if last["phase"] != "aborted" {
		t.Fatalf("final event = %v, want phase aborted", last)
	}
	if last["incomplete"] != true || last["answer"] != "Partial." {
		t.Fatalf("aborted event = %v", last)
	}
}

func TestStreamRevisionKeepsUTF8(t *testing.T) {
	var out, status bytes.Buffer
	r := newChatStreamRenderer(&out, &status, false, false, citationModeOff)
	r.WriteChunk(notebooklm.ChatChunk{Phase: notebooklm.ChatChunkAnswer, Text: "a", Full: "a"})
	r.WriteChunk(notebooklm.ChatChunk{Phase: notebooklm.ChatChunkAnswer, Text: "éz", Full: "éz"})
	r.Finish()
	if got := out.String(); got != "az" {
		t.Fatalf("output = %q, want valid UTF-8 suffix %q", got, "az")
	}
	if r.Answer() != "éz" || !r.StreamRevised() {
		t.Fatal("lost authoritative revision")
	}
}
