package api

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
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

// TestParseCitationsV2SlotOrdering locks in the invariant that Citation.SourceIndex
// is the 1-based *slot* number (matching [N] in the narrative), not the project
// index of the cited source. Regression: a run with a 100+-source notebook had
// narrative [1] referring to the first thing the model cited (e.g. slot-1 was
// src at project-index 99), while the footer printed "[1] = project-index-0"
// because SourceIndex was srcIdx+1. See /tmp/nlm-impl-count.log for the repro.
func TestParseCitationsV2SlotOrdering(t *testing.T) {
	// Three sources in the project list, and three emitted citation slots
	// that reference project indices in a non-monotonic order:
	//   slot 0 (narrative [1]) → project-index 2 (src_c)
	//   slot 1 (narrative [2]) → project-index 0 (src_a)
	//   slot 2 (narrative [3]) → project-indices 1,2 (src_b AND src_c together)
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
	// One citation per (slot, srcIdx) pair: slots 0+1 have one src each,
	// slot 2 has two → 4 total.
	if len(got) != 4 {
		t.Fatalf("got %d citations, want 4: %+v", len(got), got)
	}
	want := []Citation{
		{SourceIndex: 1, SourceID: "src_c", StartChar: 0, EndChar: 10, Confidence: 0.9},
		{SourceIndex: 2, SourceID: "src_a", StartChar: 11, EndChar: 20, Confidence: 0.8},
		{SourceIndex: 3, SourceID: "src_b", StartChar: 21, EndChar: 30, Confidence: 0.7},
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
		// Slot 0: srcIdx 0 (resolves to src_a).
		[]interface{}{[]interface{}{nil, float64(0), float64(10)}, []interface{}{float64(0)}},
		// Slot 1: srcIdx 5 (out of range — must be dropped).
		[]interface{}{[]interface{}{nil, float64(11), float64(20)}, []interface{}{float64(5)}},
		// Slot 2: mixes valid (0) and invalid (3) — the valid one survives.
		[]interface{}{[]interface{}{nil, float64(21), float64(30)}, []interface{}{float64(0), float64(3)}},
	}
	citationData := []interface{}{
		[]interface{}{nil, nil, float64(0.9), nil, nil},
		[]interface{}{nil, nil, float64(0.8), nil, nil},
		[]interface{}{nil, nil, float64(0.7), nil, nil},
	}

	got := parseCitationsV2(citationData, mappingData, sourceIDs)
	want := []Citation{
		{SourceIndex: 1, SourceID: "src_a", StartChar: 0, EndChar: 10, Confidence: 0.9},
		{SourceIndex: 3, SourceID: "src_a", StartChar: 21, EndChar: 30, Confidence: 0.7},
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
	response := &pb.GenerateFreeFormStreamedWireResponse{
		Answer:    &pb.ChatAnswer{Chunk: "answer"},
		Citations: []*pb.Grounding{{Score: proto.Float64(0.9)}},
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
	for i, citation := range got.Citations {
		if citation.SourceIndex != 1 || citation.SourceID != []string{"src-a", "src-b"}[i] || citation.StartChar != 4 || citation.EndChar != 10 || citation.Confidence != 0.9 {
			t.Errorf("citation %d = %#v", i, citation)
		}
	}
}

func TestExtractChatPayloadCorpusEquivalence(t *testing.T) {
	var files []string
	err := filepath.WalkDir("/tmp/nlm-traffic", func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && entry.Name() == "notebooklm.google.com.jsonl" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil || len(files) == 0 {
		t.Skip("/tmp/nlm-traffic corpus is not available")
	}
	responses, frames := 0, 0
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			t.Fatal(err)
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
		for record := 1; scanner.Scan(); record++ {
			var entry struct {
				Request struct {
					URL      string `json:"url"`
					PostData struct {
						Text string `json:"text"`
					} `json:"postData"`
				} `json:"request"`
				Response struct {
					Content struct {
						Text     string `json:"text"`
						Encoding string `json:"encoding"`
					} `json:"content"`
				} `json:"response"`
			}
			if json.Unmarshal(scanner.Bytes(), &entry) != nil || !strings.Contains(entry.Request.URL, "/GenerateFreeFormStreamed") || entry.Response.Content.Text == "" {
				continue
			}
			sourceIDs, err := streamSourceIDs(entry.Request.PostData.Text)
			if err != nil {
				t.Fatalf("%s:%d source ids: %v", file, record, err)
			}
			body := entry.Response.Content.Text
			if entry.Response.Content.Encoding == "base64" {
				decoded, err := base64.StdEncoding.DecodeString(body)
				if err != nil {
					t.Fatal(err)
				}
				body = string(decoded)
			}
			payloads, _, err := streamPayloads([]byte(body))
			if err != nil {
				t.Fatalf("%s:%d stream: %v", file, record, err)
			}
			responses++
			frames += len(payloads)
			for i, payload := range payloads {
				got, want := extractChatPayload(string(payload), sourceIDs), extractChatPayloadLegacy(string(payload), sourceIDs)
				if got.Text != want.Text || got.wirePhase != want.wirePhase || got.hasWirePhase != want.hasWirePhase || !sameCitations(got.Citations, want.Citations) || !sameStrings(got.FollowUps, want.FollowUps) {
					t.Fatalf("%s:%d frame %d: generated=%+v legacy=%+v", file, record, i, got, want)
				}
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	if responses != 3 || frames != 231 {
		t.Fatalf("stream corpus responses=%d frames=%d, want 3/231", responses, frames)
	}
}

func streamSourceIDs(raw string) ([]string, error) {
	if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil {
		raw = string(decoded)
	}
	form, err := url.ParseQuery(raw)
	if err != nil {
		return nil, err
	}
	var envelope []json.RawMessage
	if err := json.Unmarshal([]byte(form.Get("f.req")), &envelope); err != nil || len(envelope) < 2 {
		return nil, fmt.Errorf("bad f.req envelope")
	}
	var payload string
	if err := json.Unmarshal(envelope[1], &payload); err != nil {
		return nil, err
	}
	var args []json.RawMessage
	if err := json.Unmarshal([]byte(payload), &args); err != nil || len(args) == 0 {
		return nil, fmt.Errorf("bad stream args")
	}
	var groups []json.RawMessage
	if err := json.Unmarshal(args[0], &groups); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(groups))
	for _, group := range groups {
		var pair []string
		if json.Unmarshal(group, &pair) == nil && len(pair) > 0 {
			ids = append(ids, pair[0])
		}
	}
	return ids, nil
}

func streamPayloads(body []byte) ([][]byte, int, error) {
	body = bytes.TrimSpace(bytes.TrimPrefix(body, []byte(")]}'")))
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	var payloads [][]byte
	chunks := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		declared, err := strconv.Atoi(line)
		if err != nil {
			return nil, chunks, err
		}
		if !scanner.Scan() {
			return nil, chunks, fmt.Errorf("missing frame")
		}
		frame := strings.TrimSuffix(scanner.Text(), "\r")
		chunks++
		if declared != len(frame)+2 && declared != utf8.RuneCountInString(frame)+2 && declared != len(utf16.Encode([]rune(frame)))+2 {
			return nil, chunks, fmt.Errorf("length mismatch")
		}
		var rows [][]json.RawMessage
		if err := json.Unmarshal([]byte(frame), &rows); err != nil {
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
	if err := scanner.Err(); err != nil {
		return nil, chunks, err
	}
	return payloads, chunks, nil
}

func sameCitations(a, b []Citation) bool {
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
