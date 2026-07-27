package api

import (
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/beprotojson"
	"google.golang.org/protobuf/proto"
)

func TestParseChatResponseChunked(t *testing.T) {
	stream := mockChatStream(t,
		"**Thinking**",
		"Answer",
		"Answer continued",
	)

	var got []ChatChunk
	c := &Client{}
	err := c.parseChatResponseChunked(strings.NewReader(stream), nil, func(chunk ChatChunk) bool {
		got = append(got, chunk)
		return true
	})
	if err != nil {
		t.Fatalf("parseChatResponseChunked() error = %v", err)
	}

	want := []ChatChunk{
		{Phase: ChatChunkThinking, Header: "**Thinking**", Text: "**Thinking**"},
		{Phase: ChatChunkAnswer, Text: "Answer"},
		{Phase: ChatChunkAnswer, Text: " continued"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d chunks, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Phase != want[i].Phase || got[i].Header != want[i].Header || got[i].Text != want[i].Text {
			t.Fatalf("chunk %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestParseChatResponseChunkedUsesWirePhaseForBoldAnswer(t *testing.T) {
	stream := mockChatStreamPayloads(t,
		mockChatPayload("**Thinking**\nWorking", chatWirePhaseThinking),
		mockChatPayload("**[Architect Persona]**\nYes", chatWirePhaseAnswer),
		mockChatPayload("**[Architect Persona]**\nYes.", chatWirePhaseAnswer),
	)

	var got []ChatChunk
	c := &Client{}
	err := c.parseChatResponseChunked(strings.NewReader(stream), nil, func(chunk ChatChunk) bool {
		got = append(got, chunk)
		return true
	})
	if err != nil {
		t.Fatalf("parseChatResponseChunked() error = %v", err)
	}

	want := []ChatChunk{
		{Phase: ChatChunkThinking, Header: "**Thinking**", Text: "**Thinking**\nWorking"},
		{Phase: ChatChunkAnswer, Text: "**[Architect Persona]**\nYes"},
		{Phase: ChatChunkAnswer, Text: "."},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d chunks, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Phase != want[i].Phase || got[i].Header != want[i].Header || got[i].Text != want[i].Text {
			t.Fatalf("chunk %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestParseChatResponseChunkedTimesOutWithoutResponse(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := io.WriteString(writer, ")]}'\n"); err != nil {
			return
		}
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			if _, err := io.WriteString(writer, "1\n"); err != nil {
				return
			}
		}
	}()

	c := &Client{}
	err := c.parseChatResponseChunkedWithProgressTimeout(reader, nil, func(ChatChunk) bool {
		t.Fatal("callback called for a stream with no response chunk")
		return false
	}, 20*time.Millisecond, time.Second)
	if !IsChatStreamTimeout(err) {
		t.Fatalf("error = %v, want chat stream timeout", err)
	}
	if !strings.Contains(err.Error(), "without an initial response") {
		t.Fatalf("error = %q, want initial-response diagnostic", err)
	}
	<-done
}

func TestAnswerOnlyCallback(t *testing.T) {
	var got []string
	callback := answerOnlyCallback(func(chunk string) bool {
		got = append(got, chunk)
		return true
	})

	for _, chunk := range []ChatChunk{
		{Phase: ChatChunkThinking, Text: "**Thinking**"},
		{Phase: ChatChunkAnswer, Text: "Answer"},
		{Phase: ChatChunkAnswer, Text: " continued"},
		{Phase: ChatChunkAnswer, Text: ""},
	} {
		if !callback(chunk) {
			t.Fatalf("callback returned false for %#v", chunk)
		}
	}

	want := []string{"Answer", " continued"}
	if len(got) != len(want) {
		t.Fatalf("got %d chunks, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("chunk %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestBuildChatArgsUsesProtoBackedConversationState(t *testing.T) {
	t.Parallel()

	c := &Client{}
	argsJSON, err := c.buildChatArgs(ChatRequest{
		ProjectID:      "project-123",
		Prompt:         "What changed?",
		SourceIDs:      []string{"src-1", "src-2"},
		ConversationID: "conv-123",
		History: []ChatMessage{
			{Content: "Earlier question", Role: 1},
			{Content: "Earlier answer", Role: 2},
		},
		SeqNum: 7,
	})
	if err != nil {
		t.Fatalf("buildChatArgs() error = %v", err)
	}

	var got []interface{}
	if err := json.Unmarshal([]byte(argsJSON), &got); err != nil {
		t.Fatalf("unmarshal args: %v", err)
	}

	if len(got) != 9 {
		t.Fatalf("len(args) = %d, want 9", len(got))
	}
	if got[1] != "What changed?" {
		t.Fatalf("prompt = %v, want %q", got[1], "What changed?")
	}
	if got[4] != "conv-123" {
		t.Fatalf("conversation_id = %v, want %q", got[4], "conv-123")
	}
	if got[7] != "project-123" {
		t.Fatalf("notebook_id = %v, want %q", got[7], "project-123")
	}
	if got[8] != float64(7) {
		t.Fatalf("sequence_number = %v, want 7", got[8])
	}

	history, ok := got[2].([]interface{})
	if !ok || len(history) != 2 {
		t.Fatalf("history = %#v, want 2 entries", got[2])
	}
	first, ok := history[0].([]interface{})
	if !ok || len(first) != 3 {
		t.Fatalf("history[0] = %#v", history[0])
	}
	if first[0] != "Earlier question" || first[2] != float64(1) {
		t.Fatalf("history[0] = %#v, want content/role preserved", first)
	}
}

func TestBuildChatArgsCorpusShape(t *testing.T) {
	t.Parallel()

	c := &Client{}
	got, err := c.buildChatArgs(ChatRequest{
		ProjectID:      "project-id",
		Prompt:         "prompt",
		SourceIDs:      []string{"source-id"},
		ConversationID: "conversation-id",
		SeqNum:         1,
	})
	if err != nil {
		t.Fatalf("buildChatArgs() error = %v", err)
	}
	want := `[[[["source-id"]]],"prompt",[],[2,null,[1],[1,null,null,null,null,null,null,null,null,null,[1,3]]],"conversation-id",null,null,"project-id",1]`
	if got != want {
		t.Fatalf("buildChatArgs() = %s, want %s", got, want)
	}
}

func TestBuildChatArgsPreservesEmptyHistory(t *testing.T) {
	c := &Client{}
	got, err := c.buildChatArgs(ChatRequest{
		ProjectID:      "project-id",
		Prompt:         "prompt",
		SourceIDs:      []string{"source-id"},
		ConversationID: "conversation-id",
		History:        []ChatMessage{{Content: "", Role: 1}},
		SeqNum:         0,
	})
	if err != nil {
		t.Fatalf("buildChatArgs() error = %v", err)
	}
	const want = `[[[["source-id"]]],"prompt",[["",null,1]],[2,null,[1],[1,null,null,null,null,null,null,null,null,null,[1,3]]],"conversation-id",null,null,"project-id",1]`
	if got != want {
		t.Fatalf("buildChatArgs() = %s, want %s", got, want)
	}
}

func mockChatStream(t *testing.T, texts ...string) string {
	t.Helper()

	payloads := make([]interface{}, 0, len(texts))
	for _, text := range texts {
		payloads = append(payloads, []interface{}{[]interface{}{text}})
	}
	return mockChatStreamPayloads(t, payloads...)
}

func mockChatStreamPayloads(t *testing.T, payloads ...interface{}) string {
	t.Helper()

	var b strings.Builder
	b.WriteString(")]}'\n")
	for _, payload := range payloads {
		inner, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal inner chunk: %v", err)
		}
		envelope, err := json.Marshal([]interface{}{"wrb.fr", "mock", string(inner)})
		if err != nil {
			t.Fatalf("marshal envelope: %v", err)
		}
		b.WriteString("1\n")
		b.Write(envelope)
		b.WriteByte('\n')
	}
	return b.String()
}

func mockChatPayload(text string, phase int) interface{} {
	return []interface{}{
		[]interface{}{
			text,
			nil,
			[]interface{}{"conv", "resp", float64(1)},
			nil,
			[]interface{}{},
			nil,
			nil,
			nil,
			phase,
		},
	}
}

// TestParseCitationsV2SlotOrdering locks in the invariant that
// Citation.SourceIndex is the 1-based citationData index written as [N] in the
// narrative, independent of source-mapping order.
func TestParseCitationsV2SlotOrdering(t *testing.T) {
	// Three sources are referenced by grounded ranges in non-monotonic order.
	// Each source keeps its citationData label and confidence.
	// No embedded [6] UUID here, so SourceID resolves via the sourceIDs
	// fallback (the shape the live stream would use).
	sourceIDs := []string{"src_a", "src_b", "src_c"}
	mappingData := []interface{}{
		[]interface{}{[]interface{}{nil, float64(0), float64(10)}, []interface{}{float64(2)}},
		[]interface{}{[]interface{}{nil, float64(11), float64(20)}, []interface{}{float64(0)}},
		[]interface{}{[]interface{}{nil, float64(21), float64(30)}, []interface{}{float64(1), float64(2)}},
	}
	citationData := []interface{}{
		[]interface{}{nil, nil, float64(0.9), nil, nil},
		[]interface{}{nil, nil, float64(0.8), nil, nil},
		[]interface{}{nil, nil, float64(0.7), nil, nil},
	}

	got := parseCitationsV2(citationData, mappingData, sourceIDs)
	// One citation per (grounded range, srcIdx) pair.
	if len(got) != 4 {
		t.Fatalf("got %d citations, want 4: %+v", len(got), got)
	}
	want := []Citation{
		{SourceIndex: 3, SourceID: "src_c", StartChar: 0, EndChar: 10, Confidence: 0.7},
		{SourceIndex: 1, SourceID: "src_a", StartChar: 11, EndChar: 20, Confidence: 0.9},
		{SourceIndex: 2, SourceID: "src_b", StartChar: 21, EndChar: 30, Confidence: 0.8},
		{SourceIndex: 3, SourceID: "src_c", StartChar: 21, EndChar: 30, Confidence: 0.7},
	}
	for i, w := range want {
		g := got[i]
		if g.SourceIndex != w.SourceIndex || g.SourceID != w.SourceID ||
			g.StartChar != w.StartChar || g.EndChar != w.EndChar ||
			g.Confidence != w.Confidence {
			t.Errorf("citation %d = %+v, want %+v", i, g, w)
		}
	}
}

// TestParseCitationsV2SkipsUnresolvableSrcIdx exercises the case where
// the server emits a srcIdx past the end of the request's source list.
// A Citation we can't resolve to a SourceID is unusable downstream, so
// the parser drops it rather than emitting a blank footer line.
func TestParseCitationsV2SkipsUnresolvableSrcIdx(t *testing.T) {
	sourceIDs := []string{"src_a"} // request narrowed to one source
	mappingData := []interface{}{
		// Marker 1: srcIdx 0 → citationData[0], resolves to src_a.
		[]interface{}{[]interface{}{nil, float64(0), float64(10)}, []interface{}{float64(0)}},
		// Marker 2: srcIdx 5 (past citationData and sourceIDs — dropped).
		[]interface{}{[]interface{}{nil, float64(11), float64(20)}, []interface{}{float64(5)}},
		// Marker 3: mixes valid (0 → src_a) and invalid (3) — valid survives.
		[]interface{}{[]interface{}{nil, float64(21), float64(30)}, []interface{}{float64(0), float64(3)}},
	}
	citationData := []interface{}{
		[]interface{}{nil, nil, float64(0.9), nil, nil},
		[]interface{}{nil, nil, float64(0.8), nil, nil},
		[]interface{}{nil, nil, float64(0.7), nil, nil},
	}

	got := parseCitationsV2(citationData, mappingData, sourceIDs)
	// Both surviving citations reference srcIdx 0 → citationData[0], conf 0.9.
	want := []Citation{
		{SourceIndex: 1, SourceID: "src_a", StartChar: 0, EndChar: 10, Confidence: 0.9},
		{SourceIndex: 1, SourceID: "src_a", StartChar: 21, EndChar: 30, Confidence: 0.9},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d citations, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		g := got[i]
		if g.SourceIndex != w.SourceIndex || g.SourceID != w.SourceID ||
			g.StartChar != w.StartChar || g.EndChar != w.EndChar ||
			g.Confidence != w.Confidence {
			t.Errorf("citation %d = %+v, want %+v", i, g, w)
		}
	}
	for _, c := range got {
		if c.SourceID == "" {
			t.Errorf("citation with empty SourceID leaked through: %+v", c)
		}
	}
}

// TestParseCitationsV2ParentSourceID pins the parent-vs-chunk decode. A live
// citation slot carries [5] = [[[sourceUUID], chunkUUID]] (the source that owns
// the passage) and [6] = [chunkUUID] (the granular handle). SourceID reads [6];
// ParentSourceID reads slot[5][0][0][0] — the id that resolves to a title in
// the project source list. Verified live: a notebook's 8 sources appear at
// [5][0][0][0] while [6] carries 76 distinct chunk handles.
func TestParseCitationsV2ParentSourceID(t *testing.T) {
	const (
		parentA = "11111111-2222-3333-4444-555555555555"
		chunkA  = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
		chunkB  = "99999999-8888-7777-6666-555555555555"
	)
	mappingData := []interface{}{
		[]interface{}{[]interface{}{nil, float64(0), float64(10)}, []interface{}{float64(0)}},
		[]interface{}{[]interface{}{nil, float64(11), float64(20)}, []interface{}{float64(1)}},
	}
	citationData := []interface{}{
		// Citation shape: [5] = [[[parentA], chunkA]], [6] = [chunkA].
		[]interface{}{nil, nil, float64(0.9), nil, nil,
			[]interface{}{[]interface{}{[]interface{}{parentA}, chunkA}},
			[]interface{}{chunkA}},
		// Reply-span shape: [5] = [[parentA], [null, start, end]] — the second
		// element is a numeric range, not a chunk UUID, so ParentSourceID must
		// stay empty (we must not read a source id out of a reply-span slot).
		[]interface{}{nil, nil, float64(0.8), nil, nil,
			[]interface{}{[]interface{}{[]interface{}{parentA}, []interface{}{nil, float64(1), float64(2)}}},
			[]interface{}{chunkB}},
	}

	got := parseCitationsV2(citationData, mappingData, nil)
	if len(got) != 2 {
		t.Fatalf("got %d citations, want 2: %+v", len(got), got)
	}
	// Citation shape: chunk in SourceID, parent resolved.
	if got[0].SourceID != chunkA {
		t.Errorf("citation 0 SourceID = %q, want chunk %q", got[0].SourceID, chunkA)
	}
	if got[0].ParentSourceID != parentA {
		t.Errorf("citation 0 ParentSourceID = %q, want parent %q", got[0].ParentSourceID, parentA)
	}
	// Reply-span shape: chunk in SourceID, NO parent (the guard rejected it).
	if got[1].SourceID != chunkB {
		t.Errorf("citation 1 SourceID = %q, want chunk %q", got[1].SourceID, chunkB)
	}
	if got[1].ParentSourceID != "" {
		t.Errorf("citation 1 ParentSourceID = %q, want empty (reply-span shape)", got[1].ParentSourceID)
	}
}

func TestExtractChatPayloadResolvesScopedCitationIDs(t *testing.T) {
	sourceIDs := []string{"target-src"}
	payload := []interface{}{
		[]interface{}{"answer", nil, nil, nil, nil, nil, nil, nil, float64(1)},
		[]interface{}{
			[]interface{}{nil, nil, float64(0.9), nil, nil},
		},
		[]interface{}{
			[]interface{}{
				[]interface{}{nil, float64(0), float64(6)},
				[]interface{}{float64(0)},
			},
		},
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	got := extractChatPayload(string(payloadJSON), sourceIDs)
	if len(got.Citations) != 1 {
		t.Fatalf("got %d citations, want 1: %+v", len(got.Citations), got.Citations)
	}
	if got.Citations[0].SourceID != "target-src" {
		t.Fatalf("citation source = %q, want target-src", got.Citations[0].SourceID)
	}
}

func TestExtractChatPayloadGeneratedCitationFanout(t *testing.T) {
	// srcIndices index citations: one grounded range citing two sources
	// reads a separate grounding record per source, each with its own score.
	// The two arrays are usually different lengths on the wire (328 frames of
	// 351 in the corpus), so there is no marker-aligned reading of them.
	response := &pb.GenerateFreeFormStreamedWireResponse{
		Answer: &pb.ChatAnswer{Chunk: "answer"},
		Citations: []*pb.Grounding{
			{Score: proto.Float64(0.9)},
			{Score: proto.Float64(0.4)},
		},
		SourceMappings: []*pb.ChatStreamSourceMapping{{
			Range:         &pb.OffsetRange{Start: proto.Int64(4), End: proto.Int64(10)},
			SourceIndices: []int32{0, 1},
		}},
	}
	raw, err := beprotojson.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	got := extractChatPayload(string(raw), []string{"src-a", "src-b"})
	if got.Text != "answer" || len(got.Citations) != 2 {
		t.Fatalf("payload = %#v, want answer with two citations", got)
	}
	wantScores := []float64{0.9, 0.4}
	for i, citation := range got.Citations {
		if citation.SourceIndex != i+1 || citation.SourceID != []string{"src-a", "src-b"}[i] || citation.StartChar != 4 || citation.EndChar != 10 || citation.Confidence != wantScores[i] {
			t.Errorf("citation %d = %#v", i, citation)
		}
	}
}

func TestExtractChatPayloadPreservesRichDocument(t *testing.T) {
	document := &pb.RichDocument{Body: &pb.SpanLayers{Blocks: []*pb.Span{
		testRichBlock(0, 5, "Intro", &pb.TextMarks{Flag2: proto.Bool(true)}, nil),
		testRichBlock(5, 10, "First", nil, testListItem(0)),
		testRichBlock(10, 16, "Second", nil, testListItem(0)),
	}}}
	response := &pb.GenerateFreeFormStreamedWireResponse{
		Answer: &pb.ChatAnswer{
			Chunk:    "IntroFirstSecond",
			Document: document,
		},
	}
	raw, err := beprotojson.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}

	payload := extractChatPayload(string(raw), nil)
	if payload.Rich == nil {
		t.Fatal("extractChatPayload dropped the generated rich document")
	}
	blocks := payload.Rich.GetBody().GetBlocks()
	if len(blocks) != 3 {
		t.Fatalf("got %d rich blocks, want 3", len(blocks))
	}
	if !blocks[0].GetContent().GetGroup().GetSpans()[0].GetSpan().GetContent().GetLeaf().GetMarks().GetFlag2() {
		t.Error("inline mark did not survive proto decoding")
	}
	for i, block := range blocks[1:] {
		if block.GetContent().GetGroup().GetListItem() == nil {
			t.Errorf("list block %d lost its ListItem", i)
		}
	}
}

func testRichBlock(start, end int64, text string, marks *pb.TextMarks, item *pb.ListItem) *pb.Span {
	return &pb.Span{
		Start: proto.Int64(start),
		End:   proto.Int64(end),
		Content: &pb.SpanContent{Value: &pb.SpanContent_Group{Group: &pb.SpanGroup{
			Spans: []*pb.SpanElement{{Value: &pb.SpanElement_Span{Span: &pb.Span{
				Start: proto.Int64(start),
				End:   proto.Int64(end),
				Content: &pb.SpanContent{Value: &pb.SpanContent_Leaf{Leaf: &pb.TextLeaf{
					Text:  proto.String(text),
					Marks: marks,
				}}},
			}}}},
			ListItem: item,
		}}},
	}
}

func testListItem(nesting int64) *pb.ListItem {
	return &pb.ListItem{
		Nesting: proto.Int64(nesting),
		Marker: &pb.ListItemMarker{Value: &pb.ListItemMarker_Marker{Marker: &pb.ListMarker{
			Bullet:     "•",
			MarkerKind: proto.Int64(1),
		}}},
	}
}

func streamPayloads(body []byte) ([][]byte, int, error) {
	frames, chunks, err := batchexecute.WrbFRFrames(body)
	if err != nil {
		return nil, chunks, err
	}
	var payloads [][]byte
	for _, frame := range frames {
		var rows [][]json.RawMessage
		if err := json.Unmarshal(frame, &rows); err != nil {
			return nil, chunks, err
		}
		for _, row := range rows {
			if len(row) < 3 {
				continue
			}
			var kind string
			if json.Unmarshal(row[0], &kind) != nil || kind != "wrb.fr" {
				continue
			}
			var payload string
			if json.Unmarshal(row[2], &payload) != nil || payload == "" {
				continue
			}
			payloads = append(payloads, []byte(payload))
		}
	}
	return payloads, chunks, nil
}

func sameCitations(a, b []Citation) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !reflect.DeepEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}
func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestParseChatResponseAuthError verifies that an expired-auth chat response —
// HTTP 200 with an error frame and no content — surfaces ErrAuthExpired instead
// of returning a silent empty answer.
func TestParseChatResponseAuthError(t *testing.T) {
	// gRPC-Web error frame: anti-XSSI prefix, a length line, then an array
	// carrying the gRPC status code (16 = Unauthenticated). No "wrb.fr" frame.
	stream := ")]}'\n\n26\n[[\"er\",null,null,null,null,16]]\n"

	c := &Client{}
	var emitted string
	err := c.parseChatResponse(strings.NewReader(stream), func(chunk string) bool {
		emitted += chunk
		return true
	})
	if err == nil {
		t.Fatal("parseChatResponse returned nil for an auth-error frame; want ErrAuthExpired")
	}
	if !errors.Is(err, ErrAuthExpired) {
		t.Fatalf("error = %v, want errors.Is(ErrAuthExpired)", err)
	}
	if emitted != "" {
		t.Fatalf("emitted %q, want no content on auth error", emitted)
	}
}

// TestClassifyChatError covers the discriminator directly, especially the
// no-false-positive requirement: ordinary empty answers and content that
// merely contains digit runs (UUIDs, indices) must NOT be flagged.
func TestClassifyChatError(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"empty", "", false},
		{"whitespace", "  \n ", false},
		{"auth16", `[["er",null,null,null,null,16]]`, true},
		{"legacy_auth", `[277567]`, true},
		{"uuid_no_false_positive", `[["wrb.fr","x","00000000-0000-4000-8000-000000000016"]]`, false},
		{"index_glued_no_false_positive", `["abc16def"]`, false},
		{"benign_number", `[["er",null,42]]`, false},
	}
	for _, tt := range tests {
		err := classifyChatError(tt.body)
		if tt.wantErr && err == nil {
			t.Errorf("%s: classifyChatError(%q) = nil, want error", tt.name, tt.body)
		}
		if !tt.wantErr && err != nil {
			t.Errorf("%s: classifyChatError(%q) = %v, want nil", tt.name, tt.body, err)
		}
	}
}

func TestScanIntTokens(t *testing.T) {
	tests := []struct {
		in   string
		want []int
	}{
		{"16", []int{16}},
		{"[16,277567]", []int{16, 277567}},
		{"abc16", nil},  // glued to preceding word char
		{"16abc", nil},  // glued to following word char
		{"a1b2c3", nil}, // all glued
		{"[1, 2, 3]", []int{1, 2, 3}},
		{"00000000-0016-x", nil}, // hyphen-bordered, not JSON-delimited
		{`"...000016"`, nil},     // UUID-like tail inside a quoted string
	}
	for _, tt := range tests {
		got := scanIntTokens(tt.in)
		if len(got) != len(tt.want) {
			t.Errorf("scanIntTokens(%q) = %v, want %v", tt.in, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("scanIntTokens(%q) = %v, want %v", tt.in, got, tt.want)
				break
			}
		}
	}
}
