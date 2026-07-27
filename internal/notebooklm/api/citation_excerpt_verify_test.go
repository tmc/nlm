package api

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
	"google.golang.org/protobuf/proto"
)

// loadHistoryCitationFrame reads the synthetic GetConversationHistory citation
// frame (answer text + citationData + mappingData). The fixture reproduces the
// real server wire shape — per-source detail slots, embedded source UUIDs,
// nested excerpt trees — with fabricated, non-sensitive content.
func loadHistoryCitationFrame(t *testing.T) (answer string, citationData, mappingData interface{}) {
	t.Helper()
	raw, err := os.ReadFile("testdata/citation_history_frame.json")
	if err != nil {
		t.Fatal(err)
	}
	var frame struct {
		Answer       string      `json:"answer"`
		CitationData interface{} `json:"citationData"`
		MappingData  interface{} `json:"mappingData"`
	}
	if err := json.Unmarshal(raw, &frame); err != nil {
		t.Fatal(err)
	}
	return frame.Answer, frame.CitationData, frame.MappingData
}

// TestParseCitationsV2_HistoryFrame locks the corrected citation model:
//   - mappingData[i] is one grounded answer range and its cited source indices.
//   - each srcIdx is both the citationData slot and the narrative's 1-based [N]
//     label. Its slot carries that source's own confidence, excerpt, and
//     embedded UUID.
//   - the source UUID comes from the frame's embedded [6], preferred over the
//     request's sourceIDs list.
//   - the server excerpt is extracted verbatim from the nested tree.
func TestParseCitationsV2_HistoryFrame(t *testing.T) {
	answer, citationData, mappingData := loadHistoryCitationFrame(t)

	// Pass a decoy sourceIDs list; the parser must prefer the frame's embedded
	// UUIDs and never fall back to these.
	decoy := make([]string, 6)
	for i := range decoy {
		decoy[i] = "decoy-should-not-be-used"
	}

	cites := parseCitationsV2(citationData, mappingData, decoy)
	if len(cites) == 0 {
		t.Fatal("no citations parsed")
	}

	// mappingData[0] = [[null,115,241],[0,1,2,3]]: one answer span citing
	// narrative sources [1-4], each with its own excerpt and confidence.
	firstMapping := cites[:4]
	for i, c := range firstMapping {
		if c.SourceIndex != i+1 {
			t.Errorf("mapping source %d index = %d, want %d", i, c.SourceIndex, i+1)
		}
	}
	for _, c := range firstMapping {
		if c.StartChar != 115 || c.EndChar != 241 {
			t.Errorf("mapping span = [%d,%d], want [115,241]", c.StartChar, c.EndChar)
		}
		if c.SourceID == "decoy-should-not-be-used" {
			t.Errorf("mapping used the decoy fallback instead of the embedded UUID: %+v", c)
		}
		if !looksLikeSourceID(c.SourceID) {
			t.Errorf("mapping SourceID = %q, want a UUID", c.SourceID)
		}
	}

	// StartChar/EndChar are ANSWER offsets: slicing the answer is coherent.
	if answer != "" && firstMapping[0].EndChar <= len(answer) {
		if got := answer[firstMapping[0].StartChar:firstMapping[0].EndChar]; !strings.Contains(got, "triage") {
			t.Errorf("answer[115:241] = %q, want it to contain the marked answer text", got)
		}
	}

	// The embedded [6] UUID, not the provenance [5] UUID, is chosen as SourceID.
	// src0's real id is the all-zero UUID; its provenance id is different.
	if firstMapping[0].SourceID != "00000000-0000-0000-0000-000000000000" {
		t.Errorf("mapping source 0 SourceID = %q, want the embedded [6] UUID", firstMapping[0].SourceID)
	}

	// Per-source distinctness: the first two sources in the mapping must carry
	// different excerpts and different confidence (proves we read
	// citationData[srcIdx], not one shared slot value).
	if firstMapping[0].Excerpt == firstMapping[1].Excerpt {
		t.Error("mapping sources 0 and 1 share an excerpt — parser is copying one slot's excerpt across the group")
	}
	if firstMapping[0].Confidence == firstMapping[1].Confidence {
		t.Error("mapping sources 0 and 1 share confidence — parser is copying one slot's confidence across the group")
	}

	// Source 0 excerpt is the verbatim reconstructed passage (leaves joined,
	// interior single-space leaves preserved).
	if !strings.HasPrefix(firstMapping[0].Excerpt, "Alpha source passage") {
		got := firstMapping[0].Excerpt
		if len(got) > 60 {
			got = got[:60]
		}
		t.Errorf("marker 1 source 0 excerpt = %q, want prefix %q", got, "Alpha source passage")
	}

	// Source-body offsets: SourceStart/SourceEnd bracket the excerpt within the
	// source document. Source 0's excerpt sits at [100000,100064]; its length
	// must equal the excerpt's length (leaves are contiguous with no gaps).
	if firstMapping[0].SourceStart != 100000 || firstMapping[0].SourceEnd != 100064 {
		t.Errorf("marker 1 source 0 source range = [%d,%d], want [100000,100064]",
			firstMapping[0].SourceStart, firstMapping[0].SourceEnd)
	}
	if got := firstMapping[0].SourceEnd - firstMapping[0].SourceStart; got != len(firstMapping[0].Excerpt) {
		t.Errorf("marker 1 source 0 source span width = %d, want excerpt length %d",
			got, len(firstMapping[0].Excerpt))
	}
	// Per-source distinctness carries to offsets too: sources 0 and 1 point at
	// different regions of their respective source docs.
	if firstMapping[0].SourceStart == firstMapping[1].SourceStart {
		t.Error("marker 1 sources 0 and 1 share a source-body start — parser is copying one slot's range")
	}

	// Every citation should carry a server excerpt and non-empty source-body
	// offsets that bracket it in this frame.
	for _, c := range cites {
		if c.Excerpt == "" {
			t.Errorf("citation missing server excerpt: %+v", c)
		}
		if !(c.SourceStart < c.SourceEnd) {
			t.Errorf("citation source range not increasing: [%d,%d] for %+v", c.SourceStart, c.SourceEnd, c)
		}
	}
}

// TestParseConversationHistory_RehydratesCitations verifies that a history
// response carries assistant-message citations (with server excerpts) through
// GetConversationHistory's parser — the path chat-show replays post-hoc.
func TestParseConversationHistory_RehydratesCitations(t *testing.T) {
	// Load the synthetic content block ([answer_segments, citationData, null,
	// mappingData]) and wrap it in a full message-list response frame.
	raw, err := os.ReadFile("testdata/citation_history_frame.json")
	if err != nil {
		t.Fatal(err)
	}
	var frame struct {
		Answer       string      `json:"answer"`
		CitationData interface{} `json:"citationData"`
		MappingData  interface{} `json:"mappingData"`
	}
	if err := json.Unmarshal(raw, &frame); err != nil {
		t.Fatal(err)
	}

	answerSegments := []interface{}{[]interface{}{frame.Answer}}
	contentBlock := []interface{}{answerSegments, frame.CitationData, nil, frame.MappingData}
	assistantMsg := []interface{}{
		"msg-assistant-id",
		[]interface{}{float64(1784590083), float64(0)},
		float64(2), // assistant
		nil,
		contentBlock,
	}
	userMsg := []interface{}{
		"msg-user-id",
		[]interface{}{float64(1784590000), float64(0)},
		float64(1), // user
		"what did the sources say?",
	}
	// Response envelope: [[[assistant, user]]].
	response := []interface{}{[]interface{}{assistantMsg, userMsg}}
	body, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}

	// GetConversationHistory decodes the payload into the proto message and
	// then recovers the citation detail slot from the same bytes, so drive
	// both halves here rather than the projection alone.
	var decoded pb.GetConversationHistoryResponse
	if err := beprotojson.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	msgs := conversationMessagesFromProto(&decoded, body)
	if len(msgs) != 2 {
		t.Fatalf("parsed %d messages, want 2", len(msgs))
	}

	var assistant, user *ChatMessage
	for i := range msgs {
		switch msgs[i].Role {
		case 2:
			assistant = &msgs[i]
		case 1:
			user = &msgs[i]
		}
	}
	if assistant == nil || user == nil {
		t.Fatalf("missing a role: %+v", msgs)
	}
	if len(user.Citations) != 0 {
		t.Errorf("user message carried %d citations, want 0", len(user.Citations))
	}
	if len(assistant.Citations) == 0 {
		t.Fatal("assistant message lost its citations through history parse")
	}
	if assistant.Citations[0].Excerpt == "" {
		t.Error("rehydrated citation missing its server excerpt")
	}
	if c := assistant.Citations[0]; !(c.SourceStart < c.SourceEnd) {
		t.Errorf("rehydrated citation lost its source-body offsets: [%d,%d]", c.SourceStart, c.SourceEnd)
	}
	if !looksLikeSourceID(assistant.Citations[0].SourceID) {
		t.Errorf("rehydrated citation SourceID = %q, want an embedded UUID", assistant.Citations[0].SourceID)
	}
}

// TestExtractExcerptText_Nesting checks the excerpt walker against the two
// shapes the server emits: a leaf [start,end,["text"]] and an interior node
// wrapping several leaves (with trailing formatting metadata).
func TestExtractExcerptText_Nesting(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{
			name: "single leaf",
			json: `[[0, 4, ["Just"]]]`,
			want: "Just",
		},
		{
			name: "interior node with spaced leaves",
			json: `[[[0, 9, [[[0,4,["Just"]],[4,5,[" "]],[5,9,["fine"]]]]]]]`,
			want: "Just fine",
		},
		{
			name: "leaf with trailing formatting metadata sibling",
			json: `[[[0, 4, [[[0,4,["Just"]]]], [null,1], null]]]`,
			want: "Just",
		},
		{
			name: "empty",
			json: `[]`,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v interface{}
			if err := json.Unmarshal([]byte(tt.json), &v); err != nil {
				t.Fatal(err)
			}
			if got := extractExcerptText(v); got != tt.want {
				t.Errorf("extractExcerptText = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGroundingExcerptBoundaries(t *testing.T) {
	leaf := func(start, end int64, text string) *pb.Span {
		return &pb.Span{
			Start: proto.Int64(start),
			End:   proto.Int64(end),
			Content: &pb.SpanContent{Value: &pb.SpanContent_Leaf{
				Leaf: &pb.TextLeaf{Text: proto.String(text)},
			}},
		}
	}
	group := func(start, end int64, children ...*pb.Span) *pb.Span {
		elements := make([]*pb.SpanElement, len(children))
		for i, child := range children {
			elements[i] = &pb.SpanElement{Value: &pb.SpanElement_Span{Span: child}}
		}
		return &pb.Span{
			Start: proto.Int64(start),
			End:   proto.Int64(end),
			Content: &pb.SpanContent{Value: &pb.SpanContent_Group{
				Group: &pb.SpanGroup{Spans: elements},
			}},
		}
	}

	tests := []struct {
		name  string
		spans []*pb.Span
		want  string
	}{
		{
			name:  "contiguous leaves",
			spans: []*pb.Span{group(0, 8, leaf(0, 4, "text"), leaf(4, 8, "book"))},
			want:  "textbook",
		},
		{
			name:  "offset gap",
			spans: []*pb.Span{group(0, 9, leaf(0, 4, "text"), leaf(5, 9, "book"))},
			want:  "text book",
		},
		{
			name: "blocks",
			spans: []*pb.Span{
				group(0, 4, leaf(0, 4, "left")),
				group(4, 9, leaf(4, 9, "right")),
			},
			want: "left\nright",
		},
		{
			name: "table",
			spans: []*pb.Span{{
				Start: proto.Int64(0),
				End:   proto.Int64(12),
				Table: &pb.SpanTable{Rows: []*pb.SpanTableRow{
					{Cells: []*pb.Span{leaf(0, 3, "one"), leaf(3, 6, "two")}},
					{Cells: []*pb.Span{leaf(6, 9, "tri"), leaf(9, 12, "four")}},
				}},
			}},
			want: "one\ttwo\ntri\tfour",
		},
		{
			name:  "unicode offsets",
			spans: []*pb.Span{group(0, 4, leaf(0, 2, "世界"), leaf(2, 4, "🌍"))},
			want:  "世界🌍",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detail := &pb.Grounding{SourceSpans: &pb.SpanList{Spans: tt.spans}}
			if got := groundingExcerpt(detail); got != tt.want {
				t.Errorf("groundingExcerpt = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractExcerptTextBoundaries(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{"contiguous", `[[0,3,["one"]],[3,6,["two"]]]`, "onetwo"},
		{"gap", `[[0,3,["one"]],[4,7,["two"]]]`, "one two"},
		{
			"table",
			`[[0,12,null,null,[1,2,[[0,6,[[0,3,["one"]],[3,6,["two"]]]],[6,12,[[6,9,["tri"]],[9,12,["four"]]]]]]]]`,
			"one\ttwo\ntri\tfour",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v interface{}
			if err := json.Unmarshal([]byte(tt.json), &v); err != nil {
				t.Fatal(err)
			}
			if got := extractExcerptText(v); got != tt.want {
				t.Errorf("extractExcerptText = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExcerptRunsFromLegacy(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		wantText string
		wantRuns []ExcerptRun
	}{
		{
			name:     "unmarked",
			json:     `[[0,5,["plain"]]]`,
			wantText: "plain",
		},
		{
			name: "plain code link and unknown",
			json: `[
				[0,5,["plain"]],
				[5,9,["code",[null,null,null,null,null,null,null,null,true]]],
				[9,13,["link",[null,null,null,null,"https://example.test/docs"]]],
				[13,20,["unknown",[null,true]]]
			]`,
			wantText: "plaincodelinkunknown",
			wantRuns: []ExcerptRun{
				{Text: "plain", Start: 0, End: 5},
				{Text: "code", Code: true, Start: 5, End: 9, RawMarks: []interface{}{nil, nil, nil, nil, nil, nil, nil, nil, true}},
				{Text: "link", Link: "https://example.test/docs", Start: 9, End: 13, RawMarks: []interface{}{nil, nil, nil, nil, "https://example.test/docs"}},
				{Text: "unknown", Start: 13, End: 20, RawMarks: []interface{}{nil, true}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var value interface{}
			if err := json.Unmarshal([]byte(test.json), &value); err != nil {
				t.Fatal(err)
			}
			text, runs := extractExcerpt(value)
			if text != test.wantText {
				t.Errorf("flat excerpt = %q, want %q", text, test.wantText)
			}
			if !reflect.DeepEqual(runs, test.wantRuns) {
				t.Errorf("runs = %#v, want %#v", runs, test.wantRuns)
			}
			var joined strings.Builder
			for _, run := range runs {
				joined.WriteString(run.Text)
			}
			if len(runs) > 0 && joined.String() != text {
				t.Errorf("joined runs = %q, flat excerpt = %q", joined.String(), text)
			}
		})
	}
}

func TestExcerptRunsFromProto(t *testing.T) {
	leaf := func(start, end int64, text string, marks *pb.TextMarks) *pb.Span {
		return &pb.Span{
			Start: proto.Int64(start),
			End:   proto.Int64(end),
			Content: &pb.SpanContent{Value: &pb.SpanContent_Leaf{
				Leaf: &pb.TextLeaf{Text: proto.String(text), Marks: marks},
			}},
		}
	}
	detail := &pb.Grounding{SourceSpans: &pb.SpanList{Spans: []*pb.Span{
		leaf(0, 5, "plain", nil),
		leaf(5, 9, "code", &pb.TextMarks{Flag9: proto.Bool(true)}),
		leaf(10, 14, "link", &pb.TextMarks{Link: "mailto:test@example.com"}),
		leaf(14, 21, "unknown", &pb.TextMarks{Flag1: proto.Bool(true)}),
	}}}
	text, runs := ExcerptFromGrounding(detail)
	if text != "plain\ncode\nlink\nunknown" {
		t.Fatalf("flat excerpt = %q, want %q", text, "plain\ncode\nlink\nunknown")
	}
	if len(runs) != 7 {
		t.Fatalf("runs = %#v, want 7 including block separators", runs)
	}
	if !runs[2].Code || runs[2].Link != "" {
		t.Errorf("code run = %#v", runs[2])
	}
	if runs[4].Link != "mailto:test@example.com" || runs[4].Code {
		t.Errorf("link run = %#v", runs[4])
	}
	if runs[6].Code || runs[6].Link != "" || !reflect.DeepEqual(runs[6].RawMarks, []interface{}{nil, true}) {
		t.Errorf("unknown mark must be carried but remain unstyled: %#v", runs[6])
	}
	var joined strings.Builder
	for _, run := range runs {
		joined.WriteString(run.Text)
	}
	if joined.String() != text {
		t.Errorf("joined runs = %q, flat excerpt = %q", joined.String(), text)
	}
}

func TestLiveGroundingReplyAndSourceSpansMatch(t *testing.T) {
	raw, err := os.ReadFile("testdata/live_rich_tree.json")
	if err != nil {
		t.Fatal(err)
	}
	var tree []interface{}
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatal(err)
	}
	records, ok := tree[3].([]interface{})
	if !ok || len(records) == 0 {
		t.Fatal("live fixture has no grounding records")
	}
	codeRuns := 0
	for i, record := range records {
		row, ok := record.([]interface{})
		if !ok || len(row) < 2 {
			t.Fatalf("grounding %d has unexpected shape", i)
		}
		body, err := json.Marshal(row[1])
		if err != nil {
			t.Fatal(err)
		}
		var detail pb.Grounding
		if err := beprotojson.Unmarshal(body, &detail); err != nil {
			t.Fatalf("grounding %d: %v", i, err)
		}
		reply := detail.GetReplySpans()
		source := detail.GetSourceSpans().GetSpans()
		if len(reply) == 0 || len(source) == 0 {
			t.Fatalf("grounding %d lacks reply or source spans", i)
		}
		if reply[0].GetStart() != source[0].GetStart() ||
			reply[0].GetEnd() != source[len(source)-1].GetEnd() {
			t.Fatalf("grounding %d reply [%d,%d] != source envelope [%d,%d]",
				i, reply[0].GetStart(), reply[0].GetEnd(),
				source[0].GetStart(), source[len(source)-1].GetEnd())
		}
		text, runs := ExcerptFromGrounding(&detail)
		var joined strings.Builder
		for _, run := range runs {
			joined.WriteString(run.Text)
			if run.Code {
				codeRuns++
			}
		}
		if len(runs) > 0 && joined.String() != text {
			t.Fatalf("grounding %d joined runs differ from flat excerpt", i)
		}
	}
	if codeRuns == 0 {
		t.Fatal("live fixture no longer exercises flag8/flag9 excerpt runs")
	}
}

func TestCitationsFromProtoStreamUsesSourceSpanEnvelope(t *testing.T) {
	response := &pb.GenerateFreeFormStreamedWireResponse{
		Citations: []*pb.Grounding{{
			ReplySpans: []*pb.OffsetRange{{
				Start: proto.Int64(10),
				End:   proto.Int64(20),
			}},
			SourceSpans: &pb.SpanList{Spans: []*pb.Span{
				{Start: proto.Int64(100), End: proto.Int64(103)},
				{Start: proto.Int64(103), End: proto.Int64(108)},
			}},
			SourceId: &pb.SourceIdList{SourceId: "00000000-0000-4000-8000-000000000000"},
		}},
		SourceMappings: []*pb.ChatStreamSourceMapping{{
			Range:         &pb.OffsetRange{Start: proto.Int64(1), End: proto.Int64(2)},
			SourceIndices: []int32{0},
		}},
	}
	citations := citationsFromProtoStream(response, nil)
	if len(citations) != 1 {
		t.Fatalf("citations = %#v", citations)
	}
	if got := citations[0]; got.SourceStart != 100 || got.SourceEnd != 108 {
		t.Errorf("source envelope = [%d,%d], want [100,108]", got.SourceStart, got.SourceEnd)
	}
}
