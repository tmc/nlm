package api

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
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
//   - mappingData[i] is a narrative marker; its [S,E] indexes the ANSWER text.
//   - each srcIdx indexes citationData, whose slot carries that source's own
//     confidence, excerpt, and embedded UUID — so sources sharing a marker get
//     distinct per-source metadata (NOT one value copied across the group).
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

	// Marker 1 = mappingData[0] = [[null,115,241],[0,1,2,3]]: answer span
	// [115,241] citing citationData 0..3, each with its OWN excerpt/confidence.
	var marker1 []Citation
	for _, c := range cites {
		if c.SourceIndex == 1 {
			marker1 = append(marker1, c)
		}
	}
	if len(marker1) != 4 {
		t.Fatalf("marker 1 emitted %d citations, want 4 (srcIdx 0,1,2,3)", len(marker1))
	}
	for _, c := range marker1 {
		if c.StartChar != 115 || c.EndChar != 241 {
			t.Errorf("marker 1 span = [%d,%d], want [115,241]", c.StartChar, c.EndChar)
		}
		if c.SourceID == "decoy-should-not-be-used" {
			t.Errorf("marker 1 used the decoy fallback instead of the embedded UUID: %+v", c)
		}
		if !looksLikeSourceID(c.SourceID) {
			t.Errorf("marker 1 SourceID = %q, want a UUID", c.SourceID)
		}
	}

	// StartChar/EndChar are ANSWER offsets: slicing the answer is coherent.
	if answer != "" && marker1[0].EndChar <= len(answer) {
		if got := answer[marker1[0].StartChar:marker1[0].EndChar]; !strings.Contains(got, "triage") {
			t.Errorf("answer[115:241] = %q, want it to contain the marked answer text", got)
		}
	}

	// The embedded [6] UUID, not the provenance [5] UUID, is chosen as SourceID.
	// src0's real id is the all-zero UUID; its provenance id is different.
	if marker1[0].SourceID != "00000000-0000-0000-0000-000000000000" {
		t.Errorf("marker 1 source 0 SourceID = %q, want the embedded [6] UUID", marker1[0].SourceID)
	}

	// Per-source distinctness: the first two sources of marker 1 must carry
	// different excerpts and different confidence (proves we read
	// citationData[srcIdx], not one shared slot value).
	if marker1[0].Excerpt == marker1[1].Excerpt {
		t.Error("marker 1 sources 0 and 1 share an excerpt — parser is copying one slot's excerpt across the group")
	}
	if marker1[0].Confidence == marker1[1].Confidence {
		t.Error("marker 1 sources 0 and 1 share confidence — parser is copying one slot's confidence across the group")
	}

	// Source 0 excerpt is the verbatim reconstructed passage (leaves joined,
	// interior single-space leaves preserved).
	if !strings.HasPrefix(marker1[0].Excerpt, "Alpha source passage") {
		got := marker1[0].Excerpt
		if len(got) > 60 {
			got = got[:60]
		}
		t.Errorf("marker 1 source 0 excerpt = %q, want prefix %q", got, "Alpha source passage")
	}

	// Source-body offsets: SourceStart/SourceEnd bracket the excerpt within the
	// source document. Source 0's excerpt sits at [100000,100064]; its length
	// must equal the excerpt's length (leaves are contiguous with no gaps).
	if marker1[0].SourceStart != 100000 || marker1[0].SourceEnd != 100064 {
		t.Errorf("marker 1 source 0 source range = [%d,%d], want [100000,100064]",
			marker1[0].SourceStart, marker1[0].SourceEnd)
	}
	if got := marker1[0].SourceEnd - marker1[0].SourceStart; got != len(marker1[0].Excerpt) {
		t.Errorf("marker 1 source 0 source span width = %d, want excerpt length %d",
			got, len(marker1[0].Excerpt))
	}
	// Per-source distinctness carries to offsets too: sources 0 and 1 point at
	// different regions of their respective source docs.
	if marker1[0].SourceStart == marker1[1].SourceStart {
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
