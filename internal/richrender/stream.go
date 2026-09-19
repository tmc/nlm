package richrender

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/notebooklm"
)

// ANSI escape codes. Grey/dim style secondary output (thinking traces,
// follow-ups); bold sets off headings in default-color blocks; amber flags a
// weakly-grounded citation (confidence below weakConfidence).
const (
	ansiDim   = "\033[2m"  // dim
	ansiGrey  = "\033[90m" // bright black (grey)
	ansiBold  = "\033[1m"  // bold
	ansiAmber = "\033[33m" // yellow/amber
	ansiReset = "\033[0m"
)

// CitationMode controls how Citation data is surfaced in the CLI.
type CitationMode int

const (
	citationModeOff  CitationMode = iota // Suppress the trailing citation list entirely.
	citationModeList                     // Stream the answer live, then print the enriched citation list. The default.
	citationModeJSON                     // Emit answer deltas and citations as JSON-lines events on stdout.
)

// resolveCitationMode maps the user-facing --citations flag to a mode.
// The default (empty, "auto", "tail", or unknown) is the enriched list: the
// answer streams cleanly, then a trailing list names each source with its
// confidence, char span, and — under --citation-excerpts — the cited text.
//
// The historical "block", "stream", and "tail" modes were separate list
// variants that differed only by which metadata columns they showed; they are
// now one list with per-column toggles. They remain accepted as aliases for
// one release (see maybeWarnDeprecatedCitationMode). "overlay"/"footnote" —
// which spliced inline superscripts into the answer body — are removed: the
// splice used byte offsets against rune-based server spans and corrupted any
// answer containing multibyte characters.
func resolveCitationMode(flag string) CitationMode {
	switch strings.ToLower(flag) {
	case "off", "none":
		return citationModeOff
	case "json", "jsonl":
		return citationModeJSON
	default:
		// "", "auto", "tail", the deprecated "block"/"stream", and anything
		// unrecognized all render the one enriched list.
		return citationModeList
	}
}

// citationModeIsDeprecatedAlias reports whether the user-facing --citations
// value names a removed/renamed mode that now resolves to the enriched list.
// Used to print a one-time stderr deprecation notice without polluting stdout.
func citationModeIsDeprecatedAlias(flag string) string {
	switch strings.ToLower(flag) {
	case "block":
		return "block"
	case "stream", "inline-footer":
		return "stream"
	case "overlay", "footnote":
		return "overlay"
	}
	return ""
}

// warnDeprecatedCitationMode prints a one-line deprecation notice to w (stderr,
// never stdout — the citation output must stay parseable) when the user passed
// a --citations mode that has been folded into the default list. The notice is
// informational; the command proceeds normally.
func warnDeprecatedCitationMode(w io.Writer, flag string) {
	if alias := citationModeIsDeprecatedAlias(flag); alias != "" {
		fmt.Fprintf(w, "nlm: --citations=%s is deprecated and now renders the default citation list; this alias will be removed in a future release\n", alias)
	}
}

// StreamRenderer accumulates phase-aware chat chunks and writes their selected
// text, citation, and follow-up representations.
type StreamRenderer struct {
	out                  io.Writer
	status               io.Writer
	showThinking         bool
	verbose              bool
	streamRevised        bool // a server revision touched already-emitted answer output; the live stream may differ from the exact buffer
	jsonl                bool // when true, emit typed JSON-lines events on r.out instead of human output
	jsonlIncludeThinking bool // when true, thinking chunks are emitted as JSON-lines events (otherwise skipped)
	citationMode         CitationMode
	resolveTitle         func(sourceID string) string                             // optional; returns "" if unknown
	sourceRemoved        func(sourceID string) bool                               // optional; reports a source ID no longer in the notebook
	loadSource           func(sourceID string) (notebooklm.LoadSourceText, error) // optional; populated when --resolve-citations or --citation-excerpt is set
	excerptBudget        int                                                      // >0 enables per-citation excerpts, clipped to this many runes
	showConfidence       bool                                                     // list mode: show the (p=…) column (default on)
	showSpans            bool                                                     // list mode: show the "chars X-Y" column (default on)
	resolvedLocations    map[citationKey]resolvedCitation                         // computed once at Finish; keyed by citationKey
	debug                io.Writer
	debuggedCitations    map[citationKey]bool
	lastThinkingLen      int
	answerBuf            strings.Builder
	thinking             string
	citations            []notebooklm.Citation
	followUps            []string
	rich                 *pb.RichDocument // last answer chunk's span tree (cumulative); nil when the stream carried none

	// flushedLen is the high-water mark of answer offsets already streamed
	// to r.out; only bytes past it are ever printed, so a snapshot revision
	// can never re-emit (duplicate) already-flushed text.
	flushedLen int

	// jsonl bookkeeping: same high-water mark for emitted answer events,
	// and which citations have been emitted.
	jsonlThinkingSeen  string
	jsonlCitationsSeen int
	jsonlSourceBodies  map[string]notebooklm.LoadSourceText // per-source cache for lazy JSONL resolution ("" body = negative)
	jsonlLoadFailed    map[string]bool

	// aborted is the reason a client-side guard cut the stream short. When
	// set, Finish renders an incompleteness marker instead of the trailing
	// citation list: a guard-truncated answer is not a finished answer, and
	// resolving citations against it would dress it up as one.
	aborted string
}

func newChatStreamRenderer(out, status io.Writer, showThinking, verbose bool, mode CitationMode) *StreamRenderer {
	return &StreamRenderer{
		out:            out,
		status:         status,
		showThinking:   showThinking,
		verbose:        verbose,
		citationMode:   mode,
		showConfidence: true,
		showSpans:      true,
	}
}

// WriteChunk incorporates and renders one phase-aware chat chunk.
func (r *StreamRenderer) WriteChunk(chunk notebooklm.ChatChunk) {
	if r.jsonl {
		r.writeChunkJSONL(chunk)
		return
	}
	switch chunk.Phase {
	case notebooklm.ChatChunkThinking:
		// Thinking chunks arrive as full cumulative snapshots, not deltas.
		// Replace instead of appending to avoid quadratic growth.
		r.thinking = chunk.Text
		if !r.showThinking {
			return
		}
		if r.verbose {
			r.clearThinkingLine()
			fmt.Fprintf(r.status, "%s%s%s\n", ansiGrey, chunk.Text, ansiReset)
			return
		}
		r.clearThinkingLine()
		display := strings.TrimPrefix(strings.TrimSuffix(chunk.Header, "**"), "**")
		line := fmt.Sprintf("%s  [thinking] %s%s", ansiGrey, display, ansiReset)
		fmt.Fprint(r.status, line)
		r.lastThinkingLen = len("  [thinking] ") + len(display)
	case notebooklm.ChatChunkAnswer:
		r.clearThinkingLine()
		// The answer always streams live to stdout; citations render as a
		// trailing list at Finish. When the producer carries the cumulative
		// snapshot (Full), stream monotonically by offset: the wire delta
		// (Text) restarts at the divergence point when the server revises
		// earlier text, and re-printing from there duplicates everything
		// after it — while bytes already written cannot be unwritten. So
		// only offsets past flushedLen are printed; a revision inside the
		// flushed range leaves that span at its old rendering and sets
		// streamRevised so Finish can say so. The buffer tracks the
		// snapshot and stays exact regardless. Producers without Full
		// (persisted re-render, non-streamed fallback) pass full text in
		// Text and keep plain append semantics.
		if chunk.Full != "" {
			if len(chunk.Full)-len(chunk.Text) < r.flushedLen {
				r.streamRevised = true
			}
			if len(chunk.Full) > r.flushedLen {
				// The old byte offset can fall inside a rune after revision.
				start := r.flushedLen
				for start < len(chunk.Full) && !utf8.RuneStart(chunk.Full[start]) {
					start++
				}
				fmt.Fprint(r.out, chunk.Full[start:])
				r.flushedLen = len(chunk.Full)
			}
			r.answerBuf.Reset()
			r.answerBuf.WriteString(chunk.Full)
		} else {
			r.answerBuf.WriteString(chunk.Text)
			fmt.Fprint(r.out, chunk.Text)
			r.flushedLen += len(chunk.Text)
		}
		if len(chunk.Citations) > 0 {
			r.citations = chunk.Citations
		}
		if len(chunk.FollowUps) > 0 {
			r.followUps = chunk.FollowUps
		}
		if chunk.Rich != nil {
			r.rich = chunk.Rich // cumulative tree; keep the latest
		}
	}
}

// writeChunkJSONL emits chat-stream events as newline-delimited JSON on r.out.
// Answer text is emitted as monotone snapshot extensions so shell consumers
// can pipeline without waiting for the full response; the done event carries
// the exact full answer. Thinking chunks arrive as cumulative snapshots; only
// emit when the snapshot differs from what we last emitted.
func (r *StreamRenderer) writeChunkJSONL(chunk notebooklm.ChatChunk) {
	switch chunk.Phase {
	case notebooklm.ChatChunkThinking:
		r.thinking = chunk.Text
		if !r.jsonlIncludeThinking {
			return
		}
		if chunk.Text == r.jsonlThinkingSeen {
			return
		}
		r.jsonlThinkingSeen = chunk.Text
		r.emitJSONLEvent(map[string]any{
			"phase": "thinking",
			"text":  chunk.Text,
		})
	case notebooklm.ChatChunkAnswer:
		// A full snapshot may revise text already emitted. Announce the
		// replacement so a reader never concatenates a restarted wire delta.
		if chunk.Full != "" {
			previous := r.answerBuf.String()
			if !strings.HasPrefix(chunk.Full, previous) {
				r.streamRevised = true
				r.emitJSONLEvent(map[string]any{"phase": "revised", "full": chunk.Full})
			} else if len(chunk.Full) > len(previous) {
				r.emitJSONLEvent(map[string]any{"phase": "answer", "text": chunk.Full[len(previous):]})
			}
			r.answerBuf.Reset()
			r.answerBuf.WriteString(chunk.Full)
		} else {
			r.answerBuf.WriteString(chunk.Text)
			if chunk.Text != "" {
				r.emitJSONLEvent(map[string]any{"phase": "answer", "text": chunk.Text})
			}
		}
		if len(chunk.Citations) > 0 {
			r.citations = chunk.Citations
			for i := r.jsonlCitationsSeen; i < len(chunk.Citations); i++ {
				c := chunk.Citations[i]
				title := c.Title
				if title == "" && r.resolveTitle != nil {
					title = citationTitle(c, r.resolveTitle)
				}
				event := map[string]any{
					"phase":      "citation",
					"index":      c.SourceIndex,
					"source_id":  c.SourceID,
					"title":      title,
					"start_char": c.StartChar,
					"end_char":   c.EndChar,
					"confidence": c.Confidence,
				}
				// The notebook source that owns the cited passage (source_id is
				// the granular chunk handle). Present only when the frame embedded
				// it; lets consumers resolve the source without knowing the chunk
				// layout.
				if c.ParentSourceID != "" {
					event["parent_source_id"] = c.ParentSourceID
				}
				// The excerpt's offset range within the source document (as
				// opposed to start_char/end_char, which are answer offsets).
				// Present only when the server shipped it — lets offline tools
				// locate the citation in the source without a second fetch.
				if c.SourceStart < c.SourceEnd {
					event["source_start"] = c.SourceStart
					event["source_end"] = c.SourceEnd
				}
				// The server ships the cited source text inline; fold it into
				// the event (clipped to budget) when --citation-excerpts is set
				// so scripts get groundedness inline.
				if r.excerptBudget > 0 {
					if ex := truncateExcerpt(c.Excerpt, r.excerptBudget); ex != "" {
						event["excerpt"] = ex
					}
				}
				// When --resolve-citations is set, fold the resolved txtar
				// location in too so scripts can post-process without a fetch.
				if rc, ok := r.resolveCitationJSONL(c); ok {
					if rc.Location != "" {
						event["location"] = rc.Location
					}
				}
				r.emitJSONLEvent(event)
			}
			r.jsonlCitationsSeen = len(chunk.Citations)
		}
		if len(chunk.FollowUps) > 0 {
			r.followUps = chunk.FollowUps
		}
		if chunk.Rich != nil {
			r.rich = chunk.Rich // cumulative tree; keep the latest
		}
	}
}

func (r *StreamRenderer) emitJSONLEvent(event map[string]any) {
	buf, err := json.Marshal(event)
	if err != nil {
		fmt.Fprintf(r.status, "nlm: thinking-jsonl marshal failed: %v\n", err)
		return
	}
	fmt.Fprintln(r.out, string(buf))
}

// Abort records that a client-side guard stopped the stream, with the reason
// to surface at Finish. It does not itself write anything.
func (r *StreamRenderer) Abort(reason string) {
	r.aborted = reason
}

// Finish emits any trailing citations, follow-ups, and completion event.
func (r *StreamRenderer) Finish() {
	if r.aborted != "" {
		r.finishAborted()
		return
	}
	if r.jsonl {
		for _, f := range r.followUps {
			r.emitJSONLEvent(map[string]any{
				"phase": "followup",
				"text":  f,
			})
		}
		// The done event carries the authoritative full answer, including
		// for consumers that did not apply the revised events.
		done := map[string]any{
			"phase":  "done",
			"answer": r.answerBuf.String(),
		}
		if r.streamRevised {
			done["revised"] = true
		}
		r.emitJSONLEvent(done)
		return
	}
	r.clearThinkingLine()
	if r.streamRevised {
		fmt.Fprintf(r.status, "%snlm: the server revised earlier answer text while streaming; the streamed text may differ from the final answer (the saved conversation holds the exact text)%s\n", ansiGrey, ansiReset)
	}
	if r.loadSource != nil && len(r.citations) > 0 {
		r.resolvedLocations = resolveCitationLocations(r.loadSource, r.citations, r.debug)
	}
	if render := citationRenderers[r.citationMode]; render != nil {
		render(r)
	}
	r.printFollowUps()
}

// finishAborted closes out a stream that a client-side guard cut short. The
// partial answer stays where it is — bytes already written cannot be unwritten
// — but it is marked incomplete on stdout itself (the captured artifact), not
// only on stderr, and none of the completion rendering runs: no citation
// resolution, no citation list, no follow-ups. Callers additionally decline to
// persist the partial turn as an assistant answer.
func (r *StreamRenderer) finishAborted() {
	if r.jsonl {
		r.emitJSONLEvent(map[string]any{
			"phase":      "aborted",
			"reason":     r.aborted,
			"incomplete": true,
			"answer":     r.answerBuf.String(),
		})
		return
	}
	r.clearThinkingLine()
	fmt.Fprintf(r.out, "\n--- nlm: INCOMPLETE response, stopped by client guard: %s ---\n", r.aborted)
	fmt.Fprintf(r.status, "%snlm: citations and follow-ups were not rendered for this truncated response%s\n", ansiGrey, ansiReset)
}

// citationRenderers maps a render mode to the function that emits its trailing
// citation output at Finish. It is a lookup rather than a fixed switch so new
// modes (a compact one-line-per-source view, a grouped-by-source view) can be
// registered without reworking Finish. citationModeOff has no entry (nothing
// to render); citationModeJSON is handled earlier in Finish.
var citationRenderers = map[CitationMode]func(*StreamRenderer){
	citationModeList: (*StreamRenderer).renderCitationList,
}

// renderCitationList prints the enriched, post-answer citation list under a
// "Citations:" heading, grouped by citation index (the [n] marker):
//
//	Citations:
//	  [1] answer 115-241
//	      p=0.91 46870d5a "7A59"
//	      p=0.71 5cfc41f8 "claude: E347"      ← amber, weakly grounded
//
// The [n] marker is the primary key and heads its group carrying only the
// answer span — the one property that is genuinely per-marker. Everything else
// (source, confidence, excerpt, source-span) is per-source, so each source gets
// its own row led by its own confidence. A marker usually cites several sources
// with differing scores; that structure is the point, not redundancy.
// showConfidence / showSpans toggle the per-row p= column and the answer-span
// header. The expanded view (--citation-excerpts) additionally prints each
// source's file:line / src-offset locator and its verbatim excerpt.
func (r *StreamRenderer) renderCitationList() {
	if len(r.citations) == 0 {
		return
	}
	order, groups := groupCitationsByIndex(r.citations)
	fmt.Fprintf(r.status, "\n%sCitations:%s\n", ansiBold, ansiReset)
	// --citation-excerpts switches from the compact scan view to the expanded
	// audit view: each source additionally gets its resolved locator and its
	// own excerpt (both per-source, like confidence).
	expanded := r.excerptBudget > 0
	for _, idx := range order {
		r.renderCitationGroup(idx, groups[idx], expanded)
	}
}

// renderCitationGroup prints one marker: a header line with the answer span,
// then one row per source passage. The server sends a citation per appearance
// of the marker in the answer, all naming the same passage, so the rows are
// collapsed by passage and the extra appearances are counted on the header. Each source row leads with that source's own
// confidence (amber below weakConfidence) — never hoisted to the header,
// because a marker's sources have independent, usually differing scores. In the
// expanded view each source's locator and excerpt follow on indented lines.
func (r *StreamRenderer) renderCitationGroup(idx int, group []notebooklm.Citation, expanded bool) {
	fmt.Fprintf(r.status, "  %s\n", r.citationMarkerHeader(idx, group))
	for _, c := range dedupeCitationPassages(group) {
		if row := r.citationSourceRow(c, expanded); row != "" {
			fmt.Fprintf(r.status, "      %s\n", row)
		}
		if expanded {
			r.printCitationExcerpt(c, "          ")
		}
	}
}

// citationMarkerHeader formats the "[n] answer N-M" line that heads a marker's
// group. The answer span is the only per-marker property (it locates the [n]
// claim in the answer text); it is labeled "answer" so it is never confused
// with a source offset. A marker that appears in several places in the answer
// carries one span per appearance: the first prints, the rest as "+N more".
// When showSpans is off, or no citation carried a span, only "[n]" prints.
func (r *StreamRenderer) citationMarkerHeader(idx int, group []notebooklm.Citation) string {
	header := fmt.Sprintf("[%d]", idx)
	if !r.showSpans {
		return header
	}
	spans := answerSpans(group)
	if len(spans) == 0 {
		return header
	}
	header += " " + formatAnswerSpan(spans[0][0], spans[0][1])
	if n := len(spans) - 1; n > 0 {
		header += fmt.Sprintf(" +%d more", n)
	}
	return header
}

// citationSourceRow formats one source's row: "p=0.87 id8 "title"", with the
// per-source confidence first (amber when weak) and the source handle+title
// after. In the expanded view the row also carries the source-offset locator
// ("src N-M", or a resolved file:line) so the excerpt beneath it can be placed
// in the source document. Returns "" when there is nothing to show.
func (r *StreamRenderer) citationSourceRow(c notebooklm.Citation, expanded bool) string {
	label := r.citationLabel(c)
	var row string
	if r.showConfidence {
		if conf := r.formatSourceConfidence(c.Confidence); conf != "" {
			row = conf
		}
	}
	if label != "" {
		if row != "" {
			row += " "
		}
		row += label
	}
	if expanded {
		if loc := r.citationSourceLocator(c); loc != "" {
			if row != "" {
				row += "   "
			}
			row += loc
		}
	}
	return row
}

// formatSourceConfidence renders one source's grounding score as "p=0.87",
// colored amber when it is below weakConfidence so a weakly-grounded source
// reads at a glance. Returns "" for a zero/absent score.
func (r *StreamRenderer) formatSourceConfidence(conf float64) string {
	if conf <= 0 {
		return ""
	}
	s := fmt.Sprintf("p=%.2f", conf)
	if conf < weakConfidence {
		s = ansiAmber + s + ansiReset
	}
	return s
}

// citationSourceLocator returns the source-document anchor for citation c: a
// resolved "file:line:col" when --resolve-citations pinned a txtar member,
// otherwise the raw source-offset range as "src N-M" (from SourceStart/End).
// Both are source-document coordinates, distinct from the answer span on the
// marker header; returns "" when neither is available.
func (r *StreamRenderer) citationSourceLocator(c notebooklm.Citation) string {
	if loc := r.resolvedLocationFor(c); loc != "" {
		return "→ " + loc
	}
	if !r.showSpans {
		return ""
	}
	return formatSourceSpan(c.SourceStart, c.SourceEnd)
}

// printCitationExcerpt renders the verbatim cited excerpt beneath a source row
// (when --citation-excerpts is set), indented by indent and in the default
// color so the cited text stays legible. The source-document locator now rides
// on the source row itself; only the excerpt lands here. Nothing prints when
// the server sent no excerpt.
func (r *StreamRenderer) printCitationExcerpt(c notebooklm.Citation, indent string) {
	if ex := r.excerptFor(c); ex != "" {
		fmt.Fprintf(r.status, "%s“%s”\n", indent, ex)
	}
}

// citationLabel formats the source handle and title for one citation as
// `<id8> - "title"`, where id8 is the source ID's 8-char prefix (enough to
// disambiguate near-duplicate titles like "claude: E347 (pt2)" while staying
// column-aligned; the full UUID is available in --citations=json). The title
// prefers a resolved notebook title, then the server-supplied one. Degrades to
// just the handle or just the quoted title when only one is available. The
// resolved file:line and excerpt render separately on continuation lines.
func (r *StreamRenderer) citationLabel(c notebooklm.Citation) string {
	handle := shortSourceID(c.SourceID)
	var title string
	if r.resolveTitle != nil {
		title = citationTitle(c, r.resolveTitle)
	}
	if title == "" {
		title = c.Title
	}
	title = truncateExcerpt(title, 100)
	// When a source has no resolvable title AND its ID is absent from the
	// notebook's source list, say the title is unavailable rather than a bare
	// handle that reads as a rendering gap. We say "unavailable", not "removed":
	// a citation's SourceID is a granular chunk/passage handle, not a top-level
	// source UUID, so it legitimately misses the source list even when the source
	// is present — claiming removal would over-state what a miss actually tells us.
	if title == "" && r.sourceRemoved != nil && r.sourceRemoved(citationSourceID(c)) {
		if handle != "" {
			return handle + " (title unavailable)"
		}
		return "(title unavailable)"
	}
	switch {
	case handle != "" && title != "":
		return fmt.Sprintf("%s %q", handle, title)
	case handle != "":
		return handle
	case title != "":
		return fmt.Sprintf("%q", title)
	}
	return ""
}

// resolvedLocationFor returns the editor-style "file:line:col" coordinate for
// citation c when --resolve-citations resolved it against a txtar member, or
// "" otherwise. Footer printers render this on a continuation line beneath the
// citation label so the original source name stays visible.
func (r *StreamRenderer) resolvedLocationFor(c notebooklm.Citation) string {
	if r.resolvedLocations == nil {
		return ""
	}
	return r.resolvedLocations[keyFor(c)].Location
}

// excerptFor returns the cited source text for citation c, clipped to the
// excerpt budget, or "" when excerpts are disabled or the server sent none.
// The text is the verbatim passage the server shipped inline with the citation
// (notebooklm.Citation.Excerpt) — available for any source type (notes included) with
// no source fetch or txtar resolution required.
func (r *StreamRenderer) excerptFor(c notebooklm.Citation) string {
	if r.excerptBudget <= 0 {
		return ""
	}
	return truncateExcerpt(c.Excerpt, r.excerptBudget)
}

// resolveCitationJSONL resolves a single citation's txtar file:line location on
// demand during JSONL streaming, caching each source body so repeated citations
// into the same source cost one fetch. Returns ok=false when no source loader
// is configured (--resolve-citations was not requested), the source could not
// be loaded, or the source is not a pinnable txtar member. The excerpt is no
// longer resolved here — it ships inline on the citation. Resolution is shared
// with the batch path via resolveOneCitation so the two never diverge.
func (r *StreamRenderer) resolveCitationJSONL(c notebooklm.Citation) (resolvedCitation, bool) {
	sourceID := citationSourceID(c)
	if r.loadSource == nil || sourceID == "" {
		return resolvedCitation{}, false
	}
	if r.jsonlSourceBodies == nil {
		r.jsonlSourceBodies = make(map[string]notebooklm.LoadSourceText)
	}
	body, ok := r.jsonlSourceBodies[sourceID]
	if !ok {
		loaded, err := r.loadSource(sourceID)
		if err != nil {
			r.jsonlSourceBodies[sourceID] = notebooklm.LoadSourceText{} // negative cache
			if r.jsonlLoadFailed == nil {
				r.jsonlLoadFailed = make(map[string]bool)
			}
			r.jsonlLoadFailed[sourceID] = true
			r.writeCitationDiagnosticOnce(c, "load error")
			return resolvedCitation{}, false
		}
		body = loaded
		r.jsonlSourceBodies[sourceID] = body
	}
	if r.jsonlLoadFailed[sourceID] {
		r.writeCitationDiagnosticOnce(c, "load error")
		return resolvedCitation{}, false
	}
	entry, resolved, reason := resolveOneCitation(body, c)
	r.writeCitationDiagnosticOnce(c, reason)
	return entry, resolved
}

func (r *StreamRenderer) writeCitationDiagnosticOnce(c notebooklm.Citation, reason string) {
	if r.debug == nil || reason == "" {
		return
	}
	if r.debuggedCitations == nil {
		r.debuggedCitations = make(map[citationKey]bool)
	}
	key := keyFor(c)
	if r.debuggedCitations[key] {
		return
	}
	r.debuggedCitations[key] = true
	writeCitationDiagnostic(r.debug, c, reason)
}

func (r *StreamRenderer) printFollowUps() {
	if len(r.followUps) == 0 {
		return
	}
	fmt.Fprintf(r.status, "%sFollow-up suggestions:%s\n", ansiGrey, ansiReset)
	for _, q := range r.followUps {
		fmt.Fprintf(r.status, "%s  - %s%s\n", ansiGrey, q, ansiReset)
	}
}

// Answer returns the accumulated answer text.
func (r *StreamRenderer) Answer() string {
	return r.answerBuf.String()
}

// Thinking returns the latest cumulative thinking trace.
func (r *StreamRenderer) Thinking() string {
	return r.thinking
}

// StreamRevised reports whether a server revision landed inside answer
// output that had already been emitted, leaving the live stream (or the
// concatenation of JSONL answer events) different from the exact answer
// returned by Answer.
func (r *StreamRenderer) StreamRevised() bool {
	return r.streamRevised
}

// Rich returns the answer-body span tree from the last answer chunk (the
// cumulative tree over the whole answer), or nil when the stream carried none.
func (r *StreamRenderer) Rich() *pb.RichDocument {
	return r.rich
}

func (r *StreamRenderer) clearThinkingLine() {
	if r.lastThinkingLen == 0 {
		return
	}
	clearLine := strings.Repeat(" ", r.lastThinkingLen)
	fmt.Fprintf(r.status, "\r%s\r", clearLine)
	r.lastThinkingLen = 0
}

// citationTitle resolves a citation's notebook-source title by preferring its
// ParentSourceID (the source that owns the cited passage and is in the source
// list), then falling back to the chunk-level SourceID for frames that embedded
// no parent. Callers that resolve titles or presence off a citation route
// through this so the parent-vs-chunk distinction lives in one place.
func citationTitle(c notebooklm.Citation, resolveTitle func(string) string) string {
	if resolveTitle == nil {
		return ""
	}
	if c.ParentSourceID != "" {
		if t := resolveTitle(c.ParentSourceID); t != "" {
			return t
		}
	}
	return resolveTitle(c.SourceID)
}

type storedMessage struct {
	Role      string
	Content   string
	Thinking  string
	Citations []notebooklm.Citation
}

type persistedRenderConfig struct {
	excerptBudget  int
	hideConfidence bool
	hideSpans      bool
	loadSource     func(string) (notebooklm.LoadSourceText, error)
	resolveTitle   func(string) string
	sourceRemoved  func(string) bool
	debug          io.Writer
}

func renderPersistedAssistant(out, status io.Writer, m storedMessage, mode CitationMode, cfg persistedRenderConfig) {
	r := newChatStreamRenderer(out, status, false, false, mode)
	r.excerptBudget = cfg.excerptBudget
	r.showConfidence = !cfg.hideConfidence
	r.showSpans = !cfg.hideSpans
	r.loadSource = cfg.loadSource
	r.resolveTitle = cfg.resolveTitle
	r.sourceRemoved = cfg.sourceRemoved
	r.debug = cfg.debug
	r.WriteChunk(notebooklm.ChatChunk{
		Phase:     notebooklm.ChatChunkAnswer,
		Text:      m.Content,
		Citations: m.Citations,
	})
	r.Finish()
	if !strings.HasSuffix(m.Content, "\n") {
		fmt.Fprintln(out)
	}
}
