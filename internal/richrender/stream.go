package richrender

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/notebooklm/api"
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
	jsonl                bool // when true, emit typed JSON-lines events on r.out instead of human output
	jsonlIncludeThinking bool // when true, thinking chunks are emitted as JSON-lines events (otherwise skipped)
	citationMode         CitationMode
	resolveTitle         func(sourceID string) string                      // optional; returns "" if unknown
	sourceRemoved        func(sourceID string) bool                        // optional; reports a source ID no longer in the notebook
	loadSource           func(sourceID string) (api.LoadSourceText, error) // optional; populated when --resolve-citations or --citation-excerpt is set
	excerptBudget        int                                               // >0 enables per-citation excerpts, clipped to this many runes
	showConfidence       bool                                              // list mode: show the (p=…) column (default on)
	showSpans            bool                                              // list mode: show the "chars X-Y" column (default on)
	resolvedLocations    map[citationKey]resolvedCitation                  // computed once at Finish; keyed by citationKey
	lastThinkingLen      int
	answerBuf            strings.Builder
	thinking             string
	citations            []api.Citation
	followUps            []string
	rich                 *pb.RichDocument // last answer chunk's span tree (cumulative); nil when the stream carried none

	// flushedLen tracks bytes already streamed to r.out so re-renders don't
	// double-print; the answer always streams live now, so it just advances
	// with each answer chunk.
	flushedLen int

	// jsonl bookkeeping: last emitted absolute answer offset so we only
	// emit delta text per event, and track which citations have been emitted.
	jsonlAnswerEmitted int
	jsonlThinkingSeen  string
	jsonlCitationsSeen int
	jsonlSourceBodies  map[string]api.LoadSourceText // per-source cache for lazy JSONL resolution ("" body = negative)
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
func (r *StreamRenderer) WriteChunk(chunk api.ChatChunk) {
	if r.jsonl {
		r.writeChunkJSONL(chunk)
		return
	}
	switch chunk.Phase {
	case api.ChatChunkThinking:
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
	case api.ChatChunkAnswer:
		r.clearThinkingLine()
		r.answerBuf.WriteString(chunk.Text)
		// The answer always streams live to stdout; citations render as a
		// trailing list at Finish. (Earlier modes buffered or held a tail
		// window to splice inline superscripts; that path corrupted answers
		// with multibyte text and was removed.)
		fmt.Fprint(r.out, chunk.Text)
		r.flushedLen += len(chunk.Text)
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
// Answer text is emitted as deltas so shell consumers can pipeline without
// waiting for the full response. Thinking chunks arrive as cumulative
// snapshots; only emit when the snapshot differs from what we last emitted.
func (r *StreamRenderer) writeChunkJSONL(chunk api.ChatChunk) {
	switch chunk.Phase {
	case api.ChatChunkThinking:
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
	case api.ChatChunkAnswer:
		r.answerBuf.WriteString(chunk.Text)
		if chunk.Text != "" {
			r.emitJSONLEvent(map[string]any{
				"phase": "answer",
				"text":  chunk.Text,
			})
			r.jsonlAnswerEmitted += len(chunk.Text)
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

// Finish emits any trailing citations, follow-ups, and completion event.
func (r *StreamRenderer) Finish() {
	if r.jsonl {
		for _, f := range r.followUps {
			r.emitJSONLEvent(map[string]any{
				"phase": "followup",
				"text":  f,
			})
		}
		r.emitJSONLEvent(map[string]any{
			"phase": "done",
		})
		return
	}
	r.clearThinkingLine()
	if r.loadSource != nil && len(r.citations) > 0 {
		r.resolvedLocations = resolveCitationLocations(r.loadSource, r.citations)
	}
	if render := citationRenderers[r.citationMode]; render != nil {
		render(r)
	}
	r.printFollowUps()
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
// then one row per source. Each source row leads with that source's own
// confidence (amber below weakConfidence) — never hoisted to the header,
// because a marker's sources have independent, usually differing scores. In the
// expanded view each source's locator and excerpt follow on indented lines.
func (r *StreamRenderer) renderCitationGroup(idx int, group []api.Citation, expanded bool) {
	fmt.Fprintf(r.status, "  %s\n", r.citationMarkerHeader(idx, group))
	for _, c := range group {
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
// with a source offset. All sources under a marker share it. When showSpans is
// off, or the sources somehow disagree on the span, only "[n]" prints.
func (r *StreamRenderer) citationMarkerHeader(idx int, group []api.Citation) string {
	header := fmt.Sprintf("[%d]", idx)
	if r.showSpans {
		if start, end, ok := uniformSpan(group); ok {
			if span := formatAnswerSpan(start, end); span != "" {
				header += " " + span
			}
		}
	}
	return header
}

// citationSourceRow formats one source's row: "p=0.87 id8 "title"", with the
// per-source confidence first (amber when weak) and the source handle+title
// after. In the expanded view the row also carries the source-offset locator
// ("src N-M", or a resolved file:line) so the excerpt beneath it can be placed
// in the source document. Returns "" when there is nothing to show.
func (r *StreamRenderer) citationSourceRow(c api.Citation, expanded bool) string {
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
func (r *StreamRenderer) citationSourceLocator(c api.Citation) string {
	if loc := r.resolvedLocationFor(c); loc != "" {
		return "→ " + loc
	}
	if !r.showSpans {
		return ""
	}
	return formatSourceSpan(c.SourceStart, c.SourceEnd)
}

// uniformSpan returns the group's shared answer-span range and ok=true when
// every member agrees, else ok=false. All sources under one marker cite the
// same answer claim, so they share this span; the guard is defensive against a
// payload that ever splits it (in which case only "[n]" heads the group).
func uniformSpan(group []api.Citation) (int, int, bool) {
	for _, c := range group[1:] {
		if c.StartChar != group[0].StartChar || c.EndChar != group[0].EndChar {
			return 0, 0, false
		}
	}
	return group[0].StartChar, group[0].EndChar, true
}

// printCitationExcerpt renders the verbatim cited excerpt beneath a source row
// (when --citation-excerpts is set), indented by indent and in the default
// color so the cited text stays legible. The source-document locator now rides
// on the source row itself; only the excerpt lands here. Nothing prints when
// the server sent no excerpt.
func (r *StreamRenderer) printCitationExcerpt(c api.Citation, indent string) {
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
func (r *StreamRenderer) citationLabel(c api.Citation) string {
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
func (r *StreamRenderer) resolvedLocationFor(c api.Citation) string {
	if r.resolvedLocations == nil {
		return ""
	}
	return r.resolvedLocations[keyFor(c)].Location
}

// excerptFor returns the cited source text for citation c, clipped to the
// excerpt budget, or "" when excerpts are disabled or the server sent none.
// The text is the verbatim passage the server shipped inline with the citation
// (api.Citation.Excerpt) — available for any source type (notes included) with
// no source fetch or txtar resolution required.
func (r *StreamRenderer) excerptFor(c api.Citation) string {
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
func (r *StreamRenderer) resolveCitationJSONL(c api.Citation) (resolvedCitation, bool) {
	sourceID := citationSourceID(c)
	if r.loadSource == nil || sourceID == "" {
		return resolvedCitation{}, false
	}
	if r.jsonlSourceBodies == nil {
		r.jsonlSourceBodies = make(map[string]api.LoadSourceText)
	}
	body, ok := r.jsonlSourceBodies[sourceID]
	if !ok {
		loaded, err := r.loadSource(sourceID)
		if err != nil {
			r.jsonlSourceBodies[sourceID] = api.LoadSourceText{} // negative cache
			return resolvedCitation{}, false
		}
		body = loaded
		r.jsonlSourceBodies[sourceID] = body
	}
	return resolveOneCitation(body, c)
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
func citationTitle(c api.Citation, resolveTitle func(string) string) string {
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
	Citations []api.Citation
}

type persistedRenderConfig struct {
	excerptBudget  int
	hideConfidence bool
	hideSpans      bool
	loadSource     func(string) (api.LoadSourceText, error)
	resolveTitle   func(string) string
	sourceRemoved  func(string) bool
}

func renderPersistedAssistant(out, status io.Writer, m storedMessage, mode CitationMode, cfg persistedRenderConfig) {
	r := newChatStreamRenderer(out, status, false, false, mode)
	r.excerptBudget = cfg.excerptBudget
	r.showConfidence = !cfg.hideConfidence
	r.showSpans = !cfg.hideSpans
	r.loadSource = cfg.loadSource
	r.resolveTitle = cfg.resolveTitle
	r.sourceRemoved = cfg.sourceRemoved
	r.WriteChunk(api.ChatChunk{
		Phase:     api.ChatChunkAnswer,
		Text:      m.Content,
		Citations: m.Citations,
	})
	r.Finish()
	if !strings.HasSuffix(m.Content, "\n") {
		fmt.Fprintln(out)
	}
}
