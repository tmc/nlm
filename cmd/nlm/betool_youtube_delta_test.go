package main

import (
	"encoding/json"
	"testing"

	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
)

// TestYoutubeSourceRoundTrip isolates the wXbhsf/gArtLc "does not fit" class:
// a Source whose SourceMetadata carries the youtube oneof member (position [5])
// plus trailing fields, where the wire elides trailing nulls but Marshal pads.
// It exercises decode -> Marshal -> diffWireAgainstProto on a single element so
// the failing position is unambiguous.
func TestYoutubeSourceRoundTrip(t *testing.T) {
	// One real youtube-bearing Source from a wXbhsf capture (UUIDs are real but
	// harmless identifiers; no credentials).
	const wire = `[["c5cc8db1-9cfd-4caf-bd82-76c7a00e7df5"],"NotebookLM demo",` +
		`[null,237,[1775334619,460361000],` +
		`["3b22209b-07ac-4af3-8f49-857220f6bdd1",[1775334618,466577000]],9,` +
		`["https://www.youtube.com/watch?v=6dHmu1GALmA","6dHmu1GALmA","Google Cloud"],` +
		`1,null,418,null,null,null,null,null,[1775334620,780214000]],[null,2]]`

	msg := &notebooklmv1alpha1.Source{}
	if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	// Decode must place the youtube oneof member.
	if msg.GetMetadata().GetYoutube().GetVideoId() != "6dHmu1GALmA" {
		t.Fatalf("youtube not decoded: %+v", msg.GetMetadata())
	}

	deltas, err := diffWireAgainstProto([]byte(wire), msg)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(deltas) != 0 {
		remarshaled, _ := beprotojson.Marshal(msg)
		b, _ := json.Marshal(deltas)
		t.Fatalf("expected lossless round-trip, got %d delta(s):\n%s\nremarshaled: %s",
			len(deltas), b, remarshaled)
	}
}

// TestGoogleDocsSourceRoundTrip is the actual failing case from project 457:
// a google_docs source (metadata oneof member at position [0]) with heading
// (field 12) and indexed_time (field 15) also populated. This is the element
// that makes the full wXbhsf response "not fit Source".
func TestGoogleDocsSourceRoundTrip(t *testing.T) {
	const wire = `[["009a7e9e-94cb-4c2a-a8e1-2f66766d9e1b"],"1944-12-30 | Noble Negatives",` +
		`[["1MtnV4n3m4WTw3nKY3MK9FdbtD02INuW_5Ch2bkBYSTk","ALtnJHxTlu34EMFVB7mH3YYfBhaJKFC19-ygk",86],` +
		`1979,[1759217650,79319000],` +
		`["4e1575f7-c023-4637-bc41-2ac2ad2a814b",[1783483710,930716000]],1,null,1,null,3498,null,null,` +
		`"1944-12-30 | Noble Negatives",null,null,[1783483714,87259000]],[null,2]]`

	msg := &notebooklmv1alpha1.Source{}
	if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if msg.GetMetadata().GetGoogleDocs().GetDocumentId() == "" {
		t.Fatalf("google_docs not decoded: %+v", msg.GetMetadata())
	}
	deltas, err := diffWireAgainstProto([]byte(wire), msg)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(deltas) != 0 {
		remarshaled, _ := beprotojson.Marshal(msg)
		b, _ := json.Marshal(deltas)
		t.Fatalf("expected lossless round-trip, got %d delta(s):\n%s\nremarshaled: %s",
			len(deltas), b, remarshaled)
	}
}

// TestProjectLimitsLeadingNull guards the root cause of the wXbhsf response
// misfit: a Project.limits tuple whose first slot is null ([null,500,600,...]).
// Modeled as repeated int64 this failed to decode ("expected number, got
// <nil>"), collapsing the whole Project element; ProjectLimits (optional
// fields) round-trips it.
func TestProjectLimitsLeadingNull(t *testing.T) {
	// Minimal Project: title, no sources, id, emoji, then limits at field 11.
	const wire = `["Archive 1945",null,"34510332-d39c-499e-882d-e48393d612cd",` +
		`"x",null,null,null,null,null,null,[null,500,600,500000,3]]`
	msg := &notebooklmv1alpha1.Project{}
	if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("Unmarshal (limits leading-null must decode): %v", err)
	}
	if msg.GetLimits().GetNotebookLimit() != 500 || msg.GetLimits().GetTier_2() != 3 {
		t.Fatalf("limits decoded wrong: %+v", msg.GetLimits())
	}
	deltas, err := diffWireAgainstProto([]byte(wire), msg)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(deltas) != 0 {
		b, _ := json.Marshal(deltas)
		t.Fatalf("expected lossless, got %d delta(s): %s", len(deltas), b)
	}
}

// TestSpanCodeBlockRoundTrip guards Span.code_block (field 7): a fenced code
// block in a rich-text document is carried as [start, end, null x4,
// [code_text, language]] — an alternate content shape like table (field 5) and
// hidden_content (field 9). Surfaced by a khqZz (GetConversationHistory)
// capture whose assistant turn embedded protobuf/go code blocks; the earlier
// conversation_history fixture had no code blocks, so it never exercised this.
func TestSpanCodeBlockRoundTrip(t *testing.T) {
	const wire = `[432,516,null,null,null,null,` +
		`["message GetConversationHistoryResponse {\n  repeated ChatMessage messages = 1;\n}\n","protobuf"]]`

	msg := &notebooklmv1alpha1.Span{}
	if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got := msg.GetCodeBlock().GetLanguage(); got != "protobuf" {
		t.Fatalf("code_block language = %q, want protobuf", got)
	}
	if msg.GetCodeBlock().GetCode() == "" {
		t.Fatalf("code_block code is empty: %+v", msg.GetCodeBlock())
	}
	deltas, err := diffWireAgainstProto([]byte(wire), msg)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(deltas) != 0 {
		b, _ := json.Marshal(deltas)
		t.Fatalf("expected lossless, got %d delta(s): %s", len(deltas), b)
	}
}

// TestArtifactField18RoundTrip guards Artifact.field_18: a wrapped list of
// [seconds, nanos] rows, observed [[[1388, 553000000]]] on a failed NOTE
// artifact. Field 18 was previously documented as always-null; a QueryArtifacts
// (gArtLc) poll capture populated it. The inner pair is an inline [secs, nanos]
// (two int64 fields), not a google.protobuf.Timestamp sub-message.
func TestArtifactField18RoundTrip(t *testing.T) {
	// Minimal Artifact: id, then field 18 at position [17] (positions 2..16 null).
	const wire = `["f8817d9b-61f0-4324-a5d6-273ee0a1dc65",null,null,null,null,null,null,` +
		`null,null,null,null,null,null,null,null,null,null,[[[1388,553000000]]]]`

	msg := &notebooklmv1alpha1.Artifact{}
	if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	rows := msg.GetField_18().GetRows()
	if len(rows) != 1 || rows[0].GetSeconds() != 1388 || rows[0].GetNanos() != 553000000 {
		t.Fatalf("field_18 decoded wrong: %+v", msg.GetField_18())
	}
	deltas, err := diffWireAgainstProto([]byte(wire), msg)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(deltas) != 0 {
		b, _ := json.Marshal(deltas)
		t.Fatalf("expected lossless, got %d delta(s): %s", len(deltas), b)
	}
}

// TestArtifactUserStateRoundTrip guards the Fxmvse response: a singular state
// message whose first field is a repeated list of playback positions.
func TestArtifactUserStateRoundTrip(t *testing.T) {
	const wire = `[[[[1388,553000000]]]]`

	msg := &notebooklmv1alpha1.UpsertArtifactUserStateResponse{}
	if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	positions := msg.GetState().GetPlaybackPosition()
	if len(positions) != 1 || positions[0].GetSeconds() != 1388 || positions[0].GetNanos() != 553000000 {
		t.Fatalf("playback position decoded wrong: %+v", positions)
	}
	deltas, err := diffWireAgainstProto([]byte(wire), msg)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(deltas) != 0 {
		b, _ := json.Marshal(deltas)
		t.Fatalf("expected lossless, got %d delta(s): %s", len(deltas), b)
	}
}

// TestNoteMetadataRoundTrip guards Note.metadata (field 3): the note-metadata
// blob at position [2] in MutateNote (cYAfTb) / CreateNote (CYK0Xb) replies.
// Shape: [1, "<note_id_int>", [secs, nanos], null, null, [secs, nanos], false].
func TestNoteMetadataRoundTrip(t *testing.T) {
	const wire = `["bd9bdf87-2557-4f39-afa9-282770d6e1e3","Hello!",` +
		`[1,"157962509464",[1784663867,426486000],null,null,[1784663867,426486000],false],` +
		`null,"New Note"]`

	msg := &notebooklmv1alpha1.Note{}
	if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	md := msg.GetMetadata()
	if md.GetNoteIdInt() != "157962509464" {
		t.Fatalf("note_id_int decoded wrong: %q", md.GetNoteIdInt())
	}
	if md.GetCreated().GetSeconds() != 1784663867 || md.GetCreated().GetNanos() != 426486000 {
		t.Fatalf("created decoded wrong: %+v", md.GetCreated())
	}
	if md.GetModified().GetSeconds() != 1784663867 || md.GetModified().GetNanos() != 426486000 {
		t.Fatalf("modified decoded wrong: %+v", md.GetModified())
	}
	deltas, err := diffWireAgainstProto([]byte(wire), msg)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(deltas) != 0 {
		b, _ := json.Marshal(deltas)
		t.Fatalf("expected lossless, got %d delta(s): %s", len(deltas), b)
	}
}

// TestDocumentGuideRoundTrip guards the GenerateDocumentGuides (tr032e) reply:
// each DocumentGuide is [[[source_id]], [summary], [[topics]], []]. The old
// single-string `content` field mis-shaped position [0]; this exercises the
// wrapper-message remodel end to end.
func TestDocumentGuideRoundTrip(t *testing.T) {
	const wire = `[[[[["fd89484d-d724-4e06-bf62-14c9f02aebbe"]],` +
		`["A short summary of the source."],` +
		`[["Topic One","Topic Two","Topic Three"]],[]]]]`

	msg := &notebooklmv1alpha1.GenerateDocumentGuidesResponse{}
	if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	guides := msg.GetGuides()
	if len(guides) != 1 {
		t.Fatalf("expected 1 guide, got %d", len(guides))
	}
	g := guides[0]
	if ids := g.GetSourceId().GetId(); len(ids) != 1 || ids[0] != "fd89484d-d724-4e06-bf62-14c9f02aebbe" {
		t.Fatalf("source_id decoded wrong: %+v", g.GetSourceId())
	}
	if g.GetSummary().GetText() != "A short summary of the source." {
		t.Fatalf("summary decoded wrong: %q", g.GetSummary().GetText())
	}
	if topics := g.GetTopics().GetTopics(); len(topics) != 3 || topics[0] != "Topic One" {
		t.Fatalf("topics decoded wrong: %+v", g.GetTopics())
	}
	deltas, err := diffWireAgainstProto([]byte(wire), msg)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(deltas) != 0 {
		b, _ := json.Marshal(deltas)
		t.Fatalf("expected lossless, got %d delta(s): %s", len(deltas), b)
	}
}

// TestAddSourcesResponseRoundTrip guards the izAoDd response envelope. The
// response is a list of Source descriptors, not a Project.
func TestAddSourcesResponseRoundTrip(t *testing.T) {
	const wire = `[[[["source-id"],"Example Source",` +
		`[null,2,[100,200],["origin-id",[90,100]],8,null,1,null,14,null,null,null,null,null,[110,300]],` +
		`[null,2]]]]`

	msg := &notebooklmv1alpha1.AddSourcesResponse{}
	if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	sources := msg.GetSources()
	if len(sources) != 1 || sources[0].GetSourceId().GetSourceId() != "source-id" {
		t.Fatalf("sources decoded wrong: %+v", sources)
	}
	if sources[0].GetMetadata().GetSourceType() != notebooklmv1alpha1.SourceType_SOURCE_TYPE_SHARED_NOTE {
		t.Fatalf("source type decoded wrong: %+v", sources[0].GetMetadata())
	}
	deltas, err := diffWireAgainstProto([]byte(wire), msg)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(deltas) != 0 {
		b, _ := json.Marshal(deltas)
		t.Fatalf("expected lossless, got %d delta(s): %s", len(deltas), b)
	}
}

// TestLoadSourceResponseRoundTrip guards the hizoJc response envelope and its
// text and image loaded-content rows.
func TestLoadSourceResponseRoundTrip(t *testing.T) {
	const wire = `[[["source-id"],"Example Source",` +
		`[null,2,[100,200],["origin-id",[90,100]],3,null,1,null,14,null,null,null,null,null,[110,300],null,null,null,null,"application/pdf"],` +
		`[null,2],null,"https://example.com/download","https://example.com/view",` +
		`["blob-ref",null,"application/pdf",[["token-a","token-b"]]]],null,null,` +
		`[[[[0,8,[[[0,8,["Example",[true]]]],[null,6]]]]]]]`

	msg := &notebooklmv1alpha1.LoadSourceResponse{}
	if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if msg.GetSource().GetBlob().GetMimeType() != "application/pdf" {
		t.Fatalf("source decoded wrong: %+v", msg.GetSource())
	}
	rows := msg.GetContent().GetRows().GetRows()
	if len(rows) != 1 {
		t.Fatalf("text row decoded wrong: %+v", rows)
	}
	spans := rows[0].GetText().GetSpans()
	if len(spans) != 1 || spans[0].GetText().GetText() != "Example" {
		t.Fatalf("text spans decoded wrong: %+v", rows[0])
	}
	deltas, err := diffWireAgainstProto([]byte(wire), msg)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(deltas) != 0 {
		b, _ := json.Marshal(deltas)
		t.Fatalf("expected lossless, got %d delta(s): %s", len(deltas), b)
	}
}

// TestTextMarksFlag9RoundTrip guards TextMarks.flag9 (field 9): a boolean mark
// at position [8] observed true on code/identifier runs. Carried as a JSON
// bool (json_bool), not batchexecute's 1/0.
func TestTextMarksFlag9RoundTrip(t *testing.T) {
	const wire = `[null,null,null,null,null,null,null,null,true]`

	msg := &notebooklmv1alpha1.TextMarks{}
	if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !msg.GetFlag9() {
		t.Fatalf("flag9 not decoded: %+v", msg)
	}
	deltas, err := diffWireAgainstProto([]byte(wire), msg)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(deltas) != 0 {
		b, _ := json.Marshal(deltas)
		t.Fatalf("expected lossless, got %d delta(s): %s", len(deltas), b)
	}
}

// TestPromoCardRoundTrip guards the ozz5Z response shape. Despite its LogEvent
// name, the RPC returns subscription-card rendering data, including the user's
// NotebookLM tier and a present-but-empty reference list.
func TestPromoCardRoundTrip(t *testing.T) {
	const wire = `[[[[null,"1",627],[[2230,[null,null,null,null,null,null,null,null,null,` +
		`[[null,"NotebookLM icon","edit"],[[[null,null,"https://example.com/settings"],false],` +
		`null,null,"Manage subscription",null,"Manage subscription",null,null,` +
		`[[2230,2351,null,43,1,null,[null,"1",627],null,[[[2230,627,[9]]]],null,627,2351,null,"12345"],` +
		`[2],"token","signature"]]],null,null,null,null,null,null,null,null,null,null,` +
		`[[2230,2351,null,43,null,null,[null,"1",627],null,null,null,627,2351,null,"12345"],` +
		`[14],"token","signature"]],null,2351,2351,2230,` +
		`"NOTEBOOKLM_TIER_ULTRA_LIGHT_CONSUMER_USER",null,null,false,[]]],0]]]`

	msg := &notebooklmv1alpha1.LogEventResponse{}
	if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	entries := msg.GetEntries()
	if len(entries) != 1 || len(entries[0].GetCards()) != 1 {
		t.Fatalf("promo cards decoded wrong: %+v", entries)
	}
	card := entries[0].GetCards()[0]
	if got := card.GetUserTier(); got != "NOTEBOOKLM_TIER_ULTRA_LIGHT_CONSUMER_USER" {
		t.Fatalf("user_tier = %q", got)
	}
	deltas, err := diffWireAgainstProto([]byte(wire), msg)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(deltas) != 0 {
		b, _ := json.Marshal(deltas)
		t.Fatalf("expected lossless, got %d delta(s): %s", len(deltas), b)
	}
}

// TestDeepResearchSessionsRoundTrip covers completed deep- and fast-research
// sessions, including their different metadata tails and report-tree shapes.
func TestDeepResearchSessionsRoundTrip(t *testing.T) {
	const wire = `[[` +
		`["deep-conversation",["project",["deep query",1],5,[[` +
		`[null,"Deep report",null,5,null,null,["# Report",3,null,null,null,` +
		`[[[[0,4,null,null,null,null,["code",""]]]],null,null,null,1]],true],` +
		`["https://example.com/deep","Deep source","Summary",1,null,null,` +
		`[null,1,"Excerpt",null,1],true,1]]],6,` +
		`["research-id","plan",5,null,"deep_research.flash.prod"]],` +
		`[1784663984,118203000],[1784663985,218203000],"157962509464"],` +
		`["fast-conversation",["project",["fast query",1],1,[[` +
		`["https://example.com/fast","Fast source","Summary",1,null,null,null,true],` +
		`["https://example.com/other","Other source","Summary",1]],"extra"],6],` +
		`[1784614041,327203000],[1784614042,427203000],"157962509464"]` +
		`]]`

	msg := &notebooklmv1alpha1.GetDeepResearchSessionsResponse{}
	if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	sessions := msg.GetSessions()
	if len(sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(sessions))
	}
	deep := sessions[0]
	if deep.GetDetails().GetMode() != 5 || deep.GetDetails().GetMetadata().GetResearchId() != "research-id" {
		t.Fatalf("deep session decoded wrong: %+v", deep)
	}
	entries := deep.GetDetails().GetMainBlob().GetReportTree()
	if len(entries) != 2 || entries[1].GetUrl() != "https://example.com/deep" {
		t.Fatalf("deep report tree decoded wrong: %+v", entries)
	}
	blocks := entries[0].GetDetail().GetDocument().GetBody().GetBlocks()
	if len(blocks) != 1 || blocks[0].GetCodeBlock().Language == nil {
		t.Fatalf("empty code-block language lost: %+v", blocks)
	}
	fast := sessions[1]
	if fast.GetDetails().GetMode() != 1 || fast.GetDetails().GetMetadata() != nil {
		t.Fatalf("fast session decoded wrong: %+v", fast)
	}
	if got := fast.GetDetails().GetMainBlob().GetExtra(); got != "extra" {
		t.Fatalf("fast extra = %q", got)
	}
	deltas, err := diffWireAgainstProto([]byte(wire), msg)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(deltas) != 0 {
		b, _ := json.Marshal(deltas)
		t.Fatalf("expected lossless, got %d delta(s): %s", len(deltas), b)
	}
}
