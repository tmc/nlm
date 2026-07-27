package api

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
)

func extractChatPayload(innerJSON string, sourceIDs []string) chatPayload {
	return extractChatPayloadWithOptions(innerJSON, sourceIDs, beprotojson.UnmarshalOptions{DiscardUnknown: true}, false)
}

func extractChatPayloadWithOptions(innerJSON string, sourceIDs []string, options beprotojson.UnmarshalOptions, debug bool) chatPayload {
	var generated pb.GenerateFreeFormStreamedWireResponse
	if err := options.Unmarshal([]byte(innerJSON), &generated); err == nil {
		payload := chatPayload{
			Text: generated.GetAnswer().GetChunk(),
			Rich: generated.GetAnswer().GetDocument(),
		}
		payload.Citations = citationsFromProtoStream(&generated, sourceIDs)
		// The generated response intentionally does not model the phase
		// marker carried inside the answer tuple. Preserve that one small
		// transport concern from the legacy extractor.
		var raw []interface{}
		if json.Unmarshal([]byte(innerJSON), &raw) == nil && len(raw) > 0 {
			if answer, ok := raw[0].([]interface{}); ok && len(answer) > 8 {
				if phase, ok := answer[8].(float64); ok {
					payload.wirePhase = int(phase)
					payload.hasWirePhase = true
				}
			}
			// Preserve the legacy follow-up projection until its generated
			// shape is behaviorally verified across all stream variants.
			if len(raw) > 4 {
				payload.FollowUps = parseFollowUps(raw[4])
			}
		}
		return payload
	}
	return extractChatPayloadLegacy(innerJSON, sourceIDs, debug)
}

// groundingParentSourceID returns the project source a grounding's chunk
// belongs to. The chunk reference pairs the parent source id with the chunk's
// own UUID, and only the citation shape carries that UUID -- a reply-span
// shape lands in the same slot without one, so require it before reading the
// parent. It is the typed counterpart of parentSourceID.
func groundingParentSourceID(detail *pb.Grounding) string {
	for _, ref := range detail.GetChunkRefs() {
		if !looksLikeSourceID(ref.GetRefId()) {
			continue
		}
		if parent := ref.GetChunk().GetSourceId(); parent != "" {
			return parent
		}
	}
	return ""
}

// groundingExcerpt returns the verbatim cited source text of a grounding
// record, concatenating its span leaves in document order. It is the typed
// counterpart of extractExcerptText, which walks the same spans untyped.
func groundingExcerpt(detail *pb.Grounding) string {
	text, _ := ExcerptFromGrounding(detail)
	return text
}

// ExcerptFromGrounding returns the flat and structured source excerpt carried
// by a grounding. Runs are nil when no leaf has formatting marks.
func ExcerptFromGrounding(detail *pb.Grounding) (string, []ExcerptRun) {
	if detail == nil {
		return "", nil
	}
	var b excerptText
	for _, span := range detail.GetSourceSpans().GetSpans() {
		b.separate("\n")
		appendSpanText(span, &b)
	}
	return b.result()
}

// excerptText joins source leaves without inventing whitespace inside a
// contiguous run. A positive offset gap proves that source text was omitted
// between two leaves, so retain a visible boundary.
type excerptText struct {
	b       strings.Builder
	end     int64
	haveEnd bool
	pending string
	runs    []ExcerptRun
	marked  bool
}

func (b *excerptText) separate(s string) {
	if b.b.Len() != 0 {
		b.pending = s
	}
}

func (b *excerptText) write(start, end int64, text string, marks excerptRunMarks) {
	if text == "" {
		return
	}
	separator := ""
	if b.pending != "" {
		separator = b.pending
	} else if b.haveEnd && start > b.end {
		separator = " "
	}
	if separator != "" {
		b.b.WriteString(separator)
		b.runs = append(b.runs, ExcerptRun{
			Text:  separator,
			Start: int(b.end),
			End:   int(start),
		})
	}
	b.pending = ""
	b.b.WriteString(text)
	b.runs = append(b.runs, ExcerptRun{
		Text:     text,
		Code:     marks.code,
		Link:     marks.link,
		Start:    int(start),
		End:      int(end),
		RawMarks: marks.raw,
	})
	if marks.code || marks.link != "" || len(marks.raw) > 0 {
		b.marked = true
	}
	b.end = end
	b.haveEnd = true
}

func (b *excerptText) string() string {
	return b.b.String()
}

func (b *excerptText) result() (string, []ExcerptRun) {
	if !b.marked {
		return b.string(), nil
	}
	return b.string(), b.runs
}

type excerptRunMarks struct {
	code bool
	link string
	raw  []interface{}
}

// appendSpanText appends a span's text, descending through every shape that
// can carry text. A span holds its content at one of several positions --
// the usual leaf/group union, a table of cell spans, a code block, or a
// hidden "thinking" block -- and the untyped walk this mirrors reaches all of
// them, so missing one silently truncates an excerpt.
func appendSpanText(span *pb.Span, b *excerptText) {
	if span == nil {
		return
	}
	if leaf := span.GetContent().GetLeaf(); leaf != nil {
		b.write(span.GetStart(), span.GetEnd(), leaf.GetText(), excerptMarksFromProto(leaf.GetMarks()))
	} else {
		appendSpanContentText(span.GetContent(), span.GetStart(), span.GetEnd(), b)
	}
	for rowIndex, row := range span.GetTable().GetRows() {
		if rowIndex > 0 {
			b.separate("\n")
		}
		for cellIndex, cell := range row.GetCells() {
			if cellIndex > 0 {
				b.separate("\t")
			}
			appendSpanText(cell, b)
		}
	}
	if code := span.GetCodeBlock(); code != nil {
		b.write(span.GetStart(), span.GetEnd(), code.GetCode(), excerptRunMarks{})
	}
	appendSpanContentText(span.GetHiddenContent(), span.GetStart(), span.GetEnd(), b)
	if span.GetSeparator() != nil {
		b.separate("\n")
	}
}

// appendSpanContentText appends a content union's text, descending groups.
func appendSpanContentText(content *pb.SpanContent, start, end int64, b *excerptText) {
	if content == nil {
		return
	}
	if leaf := content.GetLeaf(); leaf != nil {
		b.write(start, end, leaf.GetText(), excerptMarksFromProto(leaf.GetMarks()))
		return
	}
	// Group children are a scalar-or-span union; bare scalar leaves carry no
	// text, so only the span arm contributes.
	for _, child := range content.GetGroup().GetSpans() {
		appendSpanText(child.GetSpan(), b)
	}
}

func excerptMarksFromProto(marks *pb.TextMarks) excerptRunMarks {
	if marks == nil {
		return excerptRunMarks{}
	}
	raw := make([]interface{}, 11)
	raw[1] = boolMark(marks.Flag1)
	raw[2] = boolMark(marks.Flag2)
	raw[3] = boolMark(marks.Flag3)
	if marks.GetLink() != "" {
		raw[4] = marks.GetLink()
	}
	if mark := marks.GetFlagOrFont(); mark != nil {
		switch value := mark.GetValue().(type) {
		case *pb.TextMarkFontOrFlag_Flag:
			raw[5] = value.Flag
		case *pb.TextMarkFontOrFlag_Font:
			raw[5] = map[string]interface{}{
				"family": value.Font.GetFamily(),
				"weight": value.Font.GetWeight(),
				"size":   value.Font.GetSize(),
			}
		}
	}
	if mark := marks.GetFlagOrColor(); mark != nil {
		switch value := mark.GetValue().(type) {
		case *pb.TextMarkColorOrFlag_Flag:
			raw[6] = value.Flag
		case *pb.TextMarkColorOrFlag_Color:
			raw[6] = []int32{value.Color.GetRed(), value.Color.GetGreen(), value.Color.GetBlue()}
		}
	}
	raw[7] = boolMark(marks.Flag7)
	raw[8] = boolMark(marks.Flag8)
	raw[9] = boolMark(marks.Flag9)
	if marks.StyleCode != nil {
		raw[10] = marks.GetStyleCode()
	}
	for len(raw) > 0 && raw[len(raw)-1] == nil {
		raw = raw[:len(raw)-1]
	}
	return excerptRunMarks{
		code: marks.GetFlag8() || marks.GetFlag9(),
		link: marks.GetLink(),
		raw:  raw,
	}
}

func boolMark(value *bool) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func citationsFromProtoStream(response *pb.GenerateFreeFormStreamedWireResponse, sourceIDs []string) []Citation {
	mappings := response.GetSourceMappings()
	groundings := response.GetCitations()
	citations := make([]Citation, 0)
	for _, mapping := range mappings {
		if mapping == nil {
			continue
		}
		start, end := 0, 0
		if r := mapping.GetRange(); r != nil {
			start, end = int(r.GetStart()), int(r.GetEnd())
		}
		for _, sourceIndex := range mapping.GetSourceIndices() {
			if sourceIndex < 0 {
				continue
			}
			// Per-source detail lives at citations[sourceIndex], not at the
			// marker index: confidence, the verbatim excerpt, and the source
			// UUID the frame embeds at grounding position [6]. Prefer that
			// UUID over the request's source list, which does not cover every
			// index a frame can reference.
			var detail *pb.Grounding
			if int(sourceIndex) < len(groundings) {
				detail = groundings[sourceIndex]
			}
			sourceID := detail.GetSourceId().GetSourceId()
			if sourceID == "" && int(sourceIndex) < len(sourceIDs) {
				sourceID = sourceIDs[sourceIndex]
			}
			if sourceID == "" {
				continue
			}
			// Source spans carry the excerpt tree and its source-body offsets.
			// Reply spans have matched this envelope in every observed frame,
			// but source spans are the authoritative coordinate space.
			var sourceStart, sourceEnd int
			if spans := detail.GetSourceSpans().GetSpans(); len(spans) > 0 {
				sourceStart = int(spans[0].GetStart())
				sourceEnd = int(spans[len(spans)-1].GetEnd())
			}
			excerpt, excerptRuns := ExcerptFromGrounding(detail)
			citations = append(citations, Citation{
				SourceIndex:    int(sourceIndex) + 1,
				SourceID:       sourceID,
				ParentSourceID: groundingParentSourceID(detail),
				StartChar:      start,
				EndChar:        end,
				Confidence:     detail.GetScore(),
				Excerpt:        excerpt,
				ExcerptRuns:    excerptRuns,
				SourceStart:    sourceStart,
				SourceEnd:      sourceEnd,
			})
		}
	}
	return citations
}

func extractChatPayloadLegacy(innerJSON string, sourceIDs []string, debug bool) chatPayload {
	var data interface{}
	if err := json.Unmarshal([]byte(innerJSON), &data); err != nil {
		return chatPayload{}
	}

	arr, ok := data.([]interface{})
	if !ok || len(arr) == 0 {
		return chatPayload{}
	}

	var p chatPayload

	// [0][0] = answer text (cumulative)
	if inner, ok := arr[0].([]interface{}); ok && len(inner) > 0 {
		if text, ok := inner[0].(string); ok {
			p.Text = text
		}
		if len(inner) > 8 {
			if phase, ok := inner[8].(float64); ok {
				p.wirePhase = int(phase)
				p.hasWirePhase = true
			}
		}
	}

	// [1] = citation details (confidence, ranges, excerpts)
	// [2] = source mappings (char range → source_indices into request's source_ids)
	if len(arr) > 2 {
		p.Citations = parseCitationsV2WithDebug(arr[1], arr[2], sourceIDs, debug)
	}

	// [4] = structured follow-ups: [[text, null, ..., type_code], ...]
	if len(arr) > 4 {
		p.FollowUps = parseFollowUps(arr[4])
	}

	return p
}

func numberValue(arr []interface{}, index int) (float64, bool) {
	if index >= len(arr) {
		return 0, false
	}
	n, ok := arr[index].(float64)
	return n, ok
}

// extractChatText is a convenience wrapper for callers that only need text.
func extractChatText(innerJSON string) string {
	return extractChatPayload(innerJSON, nil).Text
}

// debugDumpChatWirePositions logs the raw JSON structure at each position
// of the inner chat payload. Only called when --debug is set.
func debugDumpChatWirePositions(innerJSON string) {
	var arr []interface{}
	if err := json.Unmarshal([]byte(innerJSON), &arr); err != nil {
		return
	}
	for i, item := range arr {
		if item == nil {
			continue
		}
		raw, err := json.Marshal(item)
		if err != nil {
			continue
		}
		s := string(raw)
		if len(s) > 500 {
			s = s[:500] + "..."
		}
		fmt.Fprintf(os.Stderr, "DEBUG wire[%d]: %s\n", i, s)
	}
}

// parseCitationsV2 extracts citation data from the wire payload.
//
// Wire layout (inner chat payload):
//
//	citationData = per-SOURCE detail array. citationData[j] describes one cited
//	source and holds:
//	  [2] = confidence (float64)
//	  [3] = [[null, start, end]] — the excerpt's offset range within the SOURCE
//	        document (SourceStart/SourceEnd)
//	  [4] = nested excerpt tree → the verbatim cited source text. Its outer nodes
//	        carry the same [start, end] offsets as [3]; verified equal across all
//	        observed frames, so [3] is read directly as the simpler field.
//	  [5] = [[[sourceUUID], chunkUUID]] — the passage this detail cites. The
//	        INNER sourceUUID (slot[5][0][0][0]) is the notebook source that owns
//	        the passage — the id that resolves to a title in the project source
//	        list. Read as ParentSourceID. A different [5] shape,
//	        [[uuid], [null, start, end]], is a reply-span, not a citation source,
//	        and is skipped by the shape guard.
//	  [6] = [chunkUUID] — a granular chunk/passage handle, NOT the notebook
//	        source. Read as SourceID for back-compat, but it is absent from the
//	        project source list, so titles resolve off [5]'s parent, not [6].
//	        (Present in history frames; absent in some live frames, hence the
//	        sourceIDs fallback below.)
//
//	mappingData = grounded answer ranges. Each row holds [charRange, srcIndices]:
//	  [0] = char range [null, start, end] into the ANSWER text
//	  [1] = srcIndices: zero-based indices into citationData. These indices are
//	        also the narrative's 1-based [N] labels.
//
// A Citation is emitted per (range, source) pair: SourceIndex is srcIdx+1 and
// matches the narrative's [N]; StartChar/EndChar are the grounded answer range;
// SourceID/Confidence/Excerpt come from citationData[srcIdx]. sourceIDs is the
// source-id list sent in the original ChatRequest, used to resolve srcIndices
// when the frame does not embed source UUIDs at citationData[srcIdx][6].
func parseCitationsV2(citationData, mappingData interface{}, sourceIDs []string) []Citation {
	return parseCitationsV2WithDebug(citationData, mappingData, sourceIDs, false)
}

func parseCitationsV2WithDebug(citationData, mappingData interface{}, sourceIDs []string, debug bool) []Citation {
	mapArr, _ := mappingData.([]interface{})
	citArr, _ := citationData.([]interface{})

	citations := make([]Citation, 0, len(mapArr))
	for _, entry := range mapArr {
		entryArr, ok := entry.([]interface{})
		if !ok || len(entryArr) < 2 {
			continue
		}

		// [0] = char range [null, start, end] into the answer text.
		var startChar, endChar int
		if rangeArr, ok := entryArr[0].([]interface{}); ok && len(rangeArr) >= 3 {
			if v, ok := rangeArr[1].(float64); ok {
				startChar = int(v)
			}
			if v, ok := rangeArr[2].(float64); ok {
				endChar = int(v)
			}
		}

		// [1] = srcIndices into citationData. Emit one Citation per
		// (grounded range, source) pair.
		idxArr, ok := entryArr[1].([]interface{})
		if !ok || len(idxArr) == 0 {
			continue
		}

		for _, idx := range idxArr {
			srcIdx := -1
			if v, ok := idx.(float64); ok {
				srcIdx = int(v)
			}
			if srcIdx < 0 {
				continue
			}

			// Per-source detail lives at citationData[srcIdx]: confidence,
			// excerpt, the excerpt's source-body offsets, the parent source UUID
			// (slot [5]), and (in history frames) the chunk UUID (slot [6]).
			var confidence float64
			var excerpt, embeddedID, parentID string
			var excerptRuns []ExcerptRun
			var srcStart, srcEnd int
			if srcIdx < len(citArr) {
				if slotArr, ok := citArr[srcIdx].([]interface{}); ok {
					if len(slotArr) > 2 {
						if v, ok := slotArr[2].(float64); ok {
							confidence = v
						}
					}
					if len(slotArr) > 3 {
						srcStart, srcEnd = sourceBodyRange(slotArr[3])
					}
					if len(slotArr) > 4 {
						excerpt, excerptRuns = extractExcerpt(slotArr[4])
					}
					if len(slotArr) > 5 {
						parentID = parentSourceID(slotArr[5])
					}
					if len(slotArr) > 6 {
						embeddedID = firstSourceID(slotArr[6])
					}
				}
			}

			// Resolve the source id: prefer the frame's embedded UUID, else
			// fall back to the request's source list. Skip if neither
			// resolves — a Citation with an empty SourceID is unactionable.
			sourceID := embeddedID
			if sourceID == "" && srcIdx < len(sourceIDs) {
				sourceID = sourceIDs[srcIdx]
			}
			if sourceID == "" {
				// Every history frame observed embeds a source UUID at
				// citationData[srcIdx][6]; a miss here means either a wire
				// change or a frame lacking [6] with no request source list
				// to fall back to. Surface it under debug mode so it does not
				// vanish silently as a dropped citation.
				if debug {
					fmt.Fprintf(os.Stderr, "DEBUG: citation dropped: srcIdx %d has no embedded source UUID and no request source list\n", srcIdx)
				}
				continue
			}

			citations = append(citations, Citation{
				SourceIndex:    srcIdx + 1, // citationData index is the narrative's 1-based [N]
				SourceID:       sourceID,
				ParentSourceID: parentID,
				StartChar:      startChar,
				EndChar:        endChar,
				Confidence:     confidence,
				Excerpt:        excerpt,
				ExcerptRuns:    excerptRuns,
				SourceStart:    srcStart,
				SourceEnd:      srcEnd,
			})
		}
	}
	return citations
}

// firstSourceID pulls the leading source UUID out of a citationData[srcIdx][6]
// node. Observed shape is [sourceUUID] (a one-element string list); it tolerates
// a bare string or deeper nesting by returning the first UUID-looking string it
// finds.
func firstSourceID(node interface{}) string {
	switch v := node.(type) {
	case string:
		if looksLikeSourceID(v) {
			return v
		}
	case []interface{}:
		for _, child := range v {
			if id := firstSourceID(child); id != "" {
				return id
			}
		}
	}
	return ""
}

// parentSourceID pulls the notebook-source UUID out of a citationData[srcIdx][5]
// node. The citation shape is [[[sourceUUID], chunkUUID]]: the source that owns
// the cited passage sits at node[0][0][0], with the chunk handle beside it at
// node[0][1]. This source id — unlike the chunk id at slot [6] — is present in
// the project source list, so it resolves to a title.
//
// A different [5] shape, [[uuid], [null, start, end]], is a reply-span rather
// than a citation source (its second element is a numeric range, not a chunk
// UUID); the position-exact decode below returns "" for it, so the wrong id is
// never read as a parent.
func parentSourceID(node interface{}) string {
	outer, ok := node.([]interface{})
	if !ok || len(outer) == 0 {
		return ""
	}
	pair, ok := outer[0].([]interface{})
	if !ok || len(pair) < 2 {
		return ""
	}
	// pair[1] must be the chunk UUID for this to be the citation shape; a range
	// (reply-span shape) fails looksLikeSourceID and rejects the whole node.
	if chunk, ok := pair[1].(string); !ok || !looksLikeSourceID(chunk) {
		return ""
	}
	srcList, ok := pair[0].([]interface{})
	if !ok || len(srcList) == 0 {
		return ""
	}
	if id, ok := srcList[0].(string); ok && looksLikeSourceID(id) {
		return id
	}
	return ""
}

// looksLikeSourceID reports whether s has the shape of a NotebookLM source
// UUID (8-4-4-4-12 hex). It guards firstSourceID against picking up message or
// conversation IDs that share the citation node.
func looksLikeSourceID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
				return false
			}
		}
	}
	return true
}

// sourceBodyRange reads the excerpt's offset range within the source document
// from a citationData[srcIdx][3] node. The observed shape is [[null, start,
// end]] — a one-element list wrapping [null, start, end]. Returns (0, 0) if the
// node is absent or malformed; these offsets bracket the same span reconstructed
// by walking the [4] excerpt tree, verified equal across all observed frames.
func sourceBodyRange(node interface{}) (start, end int) {
	arr, ok := node.([]interface{})
	if !ok || len(arr) == 0 {
		return 0, 0
	}
	rangeArr, ok := arr[0].([]interface{})
	if !ok || len(rangeArr) < 3 {
		return 0, 0
	}
	if v, ok := rangeArr[1].(float64); ok {
		start = int(v)
	}
	if v, ok := rangeArr[2].(float64); ok {
		end = int(v)
	}
	return start, end
}

// extractExcerptText navigates the server's nested excerpt structure and
// concatenates its leaf text into the verbatim cited source passage.
//
// The excerpt for one citation slot is a tree of character-offset spans over
// the SOURCE body. Every leaf span is [start, end, ["text"]] — a two-int range
// wrapping a single-element string list. Interior nodes are [start, end,
// [children...]] (optionally trailed by formatting metadata). Walking the tree
// in order and appending each leaf's text reconstructs the full passage,
// including the interior single-space leaves the server emits between words.
func extractExcerptText(data interface{}) string {
	text, _ := extractExcerpt(data)
	return text
}

func extractExcerpt(data interface{}) (string, []ExcerptRun) {
	var b excerptText
	collectExcerptLeaves(data, &b)
	return b.result()
}

// collectExcerptLeaves appends, in document order, the text of every
// [start, end, ["text"]] leaf reachable from node into b.
func collectExcerptLeaves(node interface{}, b *excerptText) {
	arr, ok := node.([]interface{})
	if !ok {
		return
	}
	start, spanStart := numberValue(arr, 0)
	end, spanEnd := numberValue(arr, 1)
	// Leaf: [start, end, ["text", marks?]] — two numbers then a list whose
	// first element is the text. A formatted run carries its positional marks
	// alongside the text (["text", [null, true]]), so the content list is not
	// always one element long; requiring that dropped every marked run, and
	// with it the leading fragment of excerpts that begin in one.
	if len(arr) == 3 {
		_, s0 := arr[0].(float64)
		_, s1 := arr[1].(float64)
		if s0 && s1 {
			if inner, ok := arr[2].([]interface{}); ok && len(inner) > 0 {
				if text, ok := inner[0].(string); ok {
					var marks excerptRunMarks
					if len(inner) > 1 {
						marks = excerptMarksFromLegacy(inner[1])
					}
					b.write(int64(arr[0].(float64)), int64(arr[1].(float64)), text, marks)
					return
				}
			}
		}
	}
	// Table block: [start, end, null, null, [type, columns, rows], ...].
	// Cells are distinct source regions even when their ranges abut, so retain
	// tabs between cells and newlines between rows.
	if spanStart && spanEnd && len(arr) > 4 {
		if table, ok := arr[4].([]interface{}); ok && len(table) > 2 {
			if rows, ok := table[2].([]interface{}); ok {
				for rowIndex, rowNode := range rows {
					row, ok := rowNode.([]interface{})
					if !ok || len(row) < 3 {
						continue
					}
					cells, ok := row[2].([]interface{})
					if !ok {
						continue
					}
					if rowIndex > 0 {
						b.separate("\n")
					}
					for cellIndex, cell := range cells {
						if cellIndex > 0 {
							b.separate("\t")
						}
						collectExcerptLeaves(cell, b)
					}
				}
				return
			}
		}
	}
	// Code block: [start, end, null, null, null, null, [code_text, language]].
	// The text sits at index 6 rather than in a [start, end, [...]] leaf, so
	// the descent below never reaches it and an excerpt running into a fenced
	// block would stop at the fence.
	if spanStart && spanEnd && len(arr) > 6 {
		if code, ok := arr[6].([]interface{}); ok && len(code) > 0 {
			if text, ok := code[0].(string); ok {
				b.write(int64(start), int64(end), text, excerptRunMarks{})
				return
			}
		}
	}
	if spanStart && spanEnd && len(arr) > 11 {
		if separator, ok := arr[11].([]interface{}); ok && len(separator) == 0 {
			b.separate("\n")
			return
		}
	}
	for _, child := range arr {
		collectExcerptLeaves(child, b)
	}
}

func excerptMarksFromLegacy(node interface{}) excerptRunMarks {
	marks, ok := node.([]interface{})
	if !ok {
		return excerptRunMarks{}
	}
	raw := append([]interface{}(nil), marks...)
	for len(raw) > 0 && raw[len(raw)-1] == nil {
		raw = raw[:len(raw)-1]
	}
	if len(raw) == 0 {
		return excerptRunMarks{}
	}
	var link string
	if len(marks) > 4 {
		link, _ = marks[4].(string)
	}
	return excerptRunMarks{
		code: legacyBoolMark(marks, 8) || legacyBoolMark(marks, 9),
		link: link,
		raw:  raw,
	}
}

func legacyBoolMark(marks []interface{}, index int) bool {
	if index >= len(marks) {
		return false
	}
	value, _ := marks[index].(bool)
	return value
}
