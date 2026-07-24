package main

import (
	"encoding/json"
	"os"
	"testing"

	genmethod "github.com/tmc/nlm/gen/method"
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
	"google.golang.org/protobuf/proto"
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
// capture whose assistant turn embedded protobuf/go code blocks. The minimal
// wire below retains the populated language variant exercised by that capture.
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

func TestNestedRichContentRoundTrip(t *testing.T) {
	tests := []struct {
		name       string
		wire       string
		wantTable  int
		wantCode   int
		wantHidden int
	}{
		{
			name:      "table cell containing code block",
			wire:      `[0,4,null,null,[4,1,[[0,4,[[0,4,null,null,null,null,["fmt.Println()","go"]]]]]]]`,
			wantTable: 1,
			wantCode:  1,
		},
		{
			name:       "hidden content containing table",
			wire:       `[0,4,null,null,null,null,null,null,[[[0,4,null,null,[8,1,[[0,4,[[0,4,["cell"]]]]]]]]]]`,
			wantTable:  1,
			wantHidden: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			msg := &notebooklmv1alpha1.Span{}
			if err := beprotojson.Unmarshal([]byte(test.wire), msg); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			deltas, err := diffWireAgainstProto([]byte(test.wire), msg)
			if err != nil {
				t.Fatalf("diff: %v", err)
			}
			if len(deltas) != 0 {
				b, _ := json.Marshal(deltas)
				t.Fatalf("expected lossless, got %d delta(s): %s", len(deltas), b)
			}
			coverage := collectRichContent(msg)
			if coverage == nil {
				t.Fatal("rich-content coverage is nil")
			}
			if coverage.Tables != test.wantTable || coverage.CodeBlocks != test.wantCode || coverage.HiddenContent != test.wantHidden {
				t.Fatalf("coverage = %+v", coverage)
			}
			if test.wantCode != 0 && coverage.TableContainingCodeBlock != 1 {
				t.Fatalf("table-containing-code count = %d, want 1", coverage.TableContainingCodeBlock)
			}
			if test.wantHidden != 0 && coverage.HiddenContainingTable != 1 {
				t.Fatalf("hidden-containing-table count = %d, want 1", coverage.HiddenContainingTable)
			}
		})
	}
}

// TestArtifactProcessingTimestampsRoundTrip guards Artifact.processing_timestamps:
// a wrapped list of
// [seconds, nanos] rows, observed [[[1388, 553000000]]] on a failed NOTE
// artifact. Field 18 was previously documented as always-null; a QueryArtifacts
// (gArtLc) poll capture populated it. The inner pair is an inline [secs, nanos]
// (two int64 fields), not a google.protobuf.Timestamp sub-message.
func TestArtifactProcessingTimestampsRoundTrip(t *testing.T) {
	// Minimal Artifact: id, then field 18 at position [17] (positions 2..16 null).
	const wire = `["f8817d9b-61f0-4324-a5d6-273ee0a1dc65",null,null,null,null,null,null,` +
		`null,null,null,null,null,null,null,null,null,null,[[[1388,553000000]]]]`

	msg := &notebooklmv1alpha1.Artifact{}
	if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	rows := msg.GetProcessingTimestamps().GetRows()
	if len(rows) != 1 || rows[0].GetSeconds() != 1388 || rows[0].GetNanos() != 553000000 {
		t.Fatalf("processing_timestamps decoded wrong: %+v", msg.GetProcessingTimestamps())
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

// TestArtifactSlideEditInstructionsRoundTrip guards ArtifactSlideDeckPreview
// field 6: the per-slide revision blob [[[slide_index, text], ...]] surfaced by
// a gArtLc (QueryArtifacts) poll on a slide-deck artifact the user edited.
func TestArtifactSlideEditInstructionsRoundTrip(t *testing.T) {
	// Minimal preview: config null, title, then field 6 at position [5]
	// (download urls at [3][4] left null).
	const wire = `[null,"Deck",null,null,null,` +
		`[[[0,"simplify this slide"],[4,"redraw the diagram"]]]]`

	msg := &notebooklmv1alpha1.ArtifactSlideDeckPreview{}
	if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	instrs := msg.GetEditInstructions().GetInstructions()
	if len(instrs) != 2 {
		t.Fatalf("expected 2 instructions, got %d", len(instrs))
	}
	if instrs[0].GetSlideIndex() != 0 || instrs[0].GetText() != "simplify this slide" {
		t.Fatalf("instruction 0 decoded wrong: %+v", instrs[0])
	}
	if instrs[1].GetSlideIndex() != 4 || instrs[1].GetText() != "redraw the diagram" {
		t.Fatalf("instruction 1 decoded wrong: %+v", instrs[1])
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

// TestArtifactMutationResponsesRoundTrip guards the shared Artifact return
// shape used by R7cb6c creation and KmcKPe revision responses.
func TestArtifactMutationResponsesRoundTrip(t *testing.T) {
	const wire = `["artifact-id","project-id",8,[[["source-id"]]],1,null,null,null,null,null,` +
		`[1784614281,797501000],null,null,null,null,[1784614280,123000000],` +
		`[null,"Deck",null,null,null,[[[0,"shorten this slide"]]]],null,null,1]`

	for _, methodName := range []string{"CreateAudioOverview", "CreateVideoOverview", "ReviseArtifact"} {
		method, err := resolveMethod(methodName)
		if err != nil {
			t.Fatalf("resolveMethod(%s): %v", methodName, err)
		}
		msg := method.NewResponse()
		if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
			t.Fatalf("%s Unmarshal: %v", methodName, err)
		}
		artifact := msg.(*notebooklmv1alpha1.Artifact)
		if artifact.GetTitle() != "project-id" {
			t.Fatalf("%s title = %q, want project-id", methodName, artifact.GetTitle())
		}
		deltas, err := diffWireAgainstProto([]byte(wire), msg)
		if err != nil {
			t.Fatalf("%s diff: %v", methodName, err)
		}
		if len(deltas) != 0 {
			b, _ := json.Marshal(deltas)
			t.Fatalf("%s: expected lossless, got %d delta(s): %s", methodName, len(deltas), b)
		}
	}
}

// TestLogInteractionEventRoundTrip guards both observed HpN0Ub event slots.
func TestLogInteractionEventRoundTrip(t *testing.T) {
	method, err := resolveMethod("HpN0Ub")
	if err != nil {
		t.Fatalf("resolveMethod: %v", err)
	}
	for _, wire := range []string{
		"[[2,null,null,[1,null,null,null,null,null,null,null,null,null,[1]],[[1,4,8,10,2,3,6,9]]],null,[\"artifact-id\"]]",
		"[[2,null,[1],[1,null,null,null,null,null,null,null,null,null,[1,3]],[[1,4,8,10,2,3,6,9]]],[\"artifact-id\"]]",
	} {
		msg := method.NewRequest()
		if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
			t.Fatalf("Unmarshal(%s): %v", wire, err)
		}
		deltas, err := diffWireAgainstProto([]byte(wire), msg)
		if err != nil {
			t.Fatalf("diff(%s): %v", wire, err)
		}
		if len(deltas) != 0 {
			b, _ := json.Marshal(deltas)
			t.Fatalf("expected lossless for %s, got %d delta(s): %s", wire, len(deltas), b)
		}
	}
	response := method.NewResponse()
	if err := beprotojson.Unmarshal([]byte("[]"), response); err != nil {
		t.Fatalf("response Unmarshal: %v", err)
	}
	if deltas, err := diffWireAgainstProto([]byte("[]"), response); err != nil || len(deltas) != 0 {
		t.Fatalf("response diff = %v, %v; want lossless", deltas, err)
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

// TestRequestContextRoundTrip guards both observed standard context variants:
// artifact filters used by Fxmvse and the optional surface marker used by
// wXbhsf.
func TestRequestContextRoundTrip(t *testing.T) {
	for _, wire := range []string{
		`[2,null,null,[1,null,null,null,null,null,null,null,null,null,[1]],[[1,4,8,10,2,3,6,9]]]`,
		`[2,null,[1],[1,null,null,null,null,null,null,null,null,null,[1]]]`,
	} {
		msg := &notebooklmv1alpha1.RequestContext{}
		if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
			t.Fatalf("Unmarshal(%s): %v", wire, err)
		}
		deltas, err := diffWireAgainstProto([]byte(wire), msg)
		if err != nil {
			t.Fatalf("diff(%s): %v", wire, err)
		}
		if len(deltas) != 0 {
			b, _ := json.Marshal(deltas)
			t.Fatalf("expected lossless for %s, got %d delta(s): %s", wire, len(deltas), b)
		}
	}
}

// TestProjectLifecycleRequestsRoundTrip guards the standard context and
// present-empty wrapper shapes used by project reads and deletions.
func TestProjectLifecycleRequestsRoundTrip(t *testing.T) {
	tests := []struct {
		method string
		wire   string
	}{
		{"GetProject", `["project-id",null,[2,null,[1],[1,null,null,null,null,null,null,null,null,null,[1,3]]],null,1,[[null,null,[]]]]`},
		{"DeleteSources", `[[["source-id"]],[2,null,null,[1,null,null,null,null,null,null,null,null,null,[1]]]]`},
		{"DeleteChatHistory", `[[2,null,[1],[1,null,null,null,null,null,null,null,null,null,[1,3]]],null,"project-id"]`},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			method, err := resolveMethod(tt.method)
			if err != nil {
				t.Fatalf("resolveMethod(%s): %v", tt.method, err)
			}
			msg := method.NewRequest()
			if err := beprotojson.Unmarshal([]byte(tt.wire), msg); err != nil {
				t.Fatalf("%s Unmarshal: %v", tt.method, err)
			}
			deltas, err := diffWireAgainstProto([]byte(tt.wire), msg)
			if err != nil {
				t.Fatalf("%s diff: %v", tt.method, err)
			}
			if len(deltas) != 0 {
				b, _ := json.Marshal(deltas)
				t.Fatalf("%s: expected lossless, got %d delta(s): %s", tt.method, len(deltas), b)
			}
			var encoded []interface{}
			switch req := msg.(type) {
			case *notebooklmv1alpha1.GetProjectRequest:
				encoded = genmethod.EncodeGetProjectArgs(req)
			case *notebooklmv1alpha1.DeleteSourcesRequest:
				encoded = genmethod.EncodeDeleteSourcesArgs(req)
			case *notebooklmv1alpha1.DeleteChatHistoryRequest:
				encoded = genmethod.EncodeDeleteChatHistoryArgs(req)
			default:
				t.Fatalf("%s: unexpected request type %T", tt.method, msg)
			}
			got, err := json.Marshal(encoded)
			if err != nil {
				t.Fatalf("%s marshal encoded args: %v", tt.method, err)
			}
			if string(got) != tt.wire {
				t.Fatalf("%s encoder = %s, want %s", tt.method, got, tt.wire)
			}
		})
	}
}

// TestCommonContextRequestsRoundTrip guards the shared request context across
// read-only notebook, account, conversation, audio-format, and sharing calls.
func TestCommonContextRequestsRoundTrip(t *testing.T) {
	const context = `[2,null,[1],[1,null,null,null,null,null,null,null,null,null,[1,3]]]`
	const filteredContext = `[2,null,null,[1,null,null,null,null,null,null,null,null,null,[1]],[[1,4,8,10,2,3,6,9]]]`
	tests := []struct {
		method string
		wire   string
	}{
		{"GetLabels", `[` + context + `,"project-id"]`},
		{"GetProjectDetails", `["project-id",` + context + `]`},
		{"GetOrCreateAccount", `[` + context + `]`},
		{"GetConversations", `[` + context + `,null,"project-id",20]`},
		{"GetConversationHistory", `[` + context + `,null,null,"conversation-id",20]`},
		{"GetAudioFormats", `[` + filteredContext + `,null,1]`},
		{"CopyProject", `[` + context + `,"project-id","Copy title"]`},
		{"ListFeaturedProjects", `[` + context + `]`},
		{"FetchInteractivityToken", `[` + context + `]`},
		{"GetNotes", `["project-id",null,[178,991],` + context + `]`},
	}
	for _, tt := range tests {
		method, err := resolveMethod(tt.method)
		if err != nil {
			t.Fatalf("resolveMethod(%s): %v", tt.method, err)
		}
		msg := method.NewRequest()
		if err := beprotojson.Unmarshal([]byte(tt.wire), msg); err != nil {
			t.Fatalf("%s Unmarshal: %v", tt.method, err)
		}
		deltas, err := diffWireAgainstProto([]byte(tt.wire), msg)
		if err != nil {
			t.Fatalf("%s diff: %v", tt.method, err)
		}
		if len(deltas) != 0 {
			b, _ := json.Marshal(deltas)
			t.Fatalf("%s: expected lossless, got %d delta(s): %s", tt.method, len(deltas), b)
		}
		var encoded []interface{}
		switch req := msg.(type) {
		case *notebooklmv1alpha1.GetLabelsRequest:
			encoded = genmethod.EncodeGetLabelsArgs(req)
		case *notebooklmv1alpha1.GetProjectDetailsRequest:
			encoded = genmethod.EncodeGetProjectDetailsArgs(req)
		case *notebooklmv1alpha1.GetOrCreateAccountRequest:
			encoded = genmethod.EncodeGetOrCreateAccountArgs(req)
		case *notebooklmv1alpha1.GetConversationsRequest:
			encoded = genmethod.EncodeGetConversationsArgs(req)
		case *notebooklmv1alpha1.GetConversationHistoryRequest:
			encoded = genmethod.EncodeGetConversationHistoryArgs(req)
		case *notebooklmv1alpha1.GetAudioFormatsRequest:
			encoded = genmethod.EncodeGetAudioFormatsArgs(req)
		case *notebooklmv1alpha1.CopyProjectRequest:
			encoded = genmethod.EncodeCopyProjectArgs(req)
		case *notebooklmv1alpha1.ListFeaturedProjectsRequest:
			encoded = genmethod.EncodeListFeaturedProjectsArgs(req)
		case *notebooklmv1alpha1.FetchInteractivityTokenRequest:
			encoded = genmethod.EncodeFetchInteractivityTokenArgs(req)
		case *notebooklmv1alpha1.GetNotesRequest:
			encoded = genmethod.EncodeGetNotesArgs(req)
		default:
			t.Fatalf("%s: unexpected request type %T", tt.method, msg)
		}
		got, err := json.Marshal(encoded)
		if err != nil {
			t.Fatalf("%s marshal encoded args: %v", tt.method, err)
		}
		if string(got) != tt.wire {
			t.Fatalf("%s encoder = %s, want %s", tt.method, got, tt.wire)
		}
	}
}

// TestSourceOperationRequestsRoundTrip guards the nested source-ID, mutation,
// text-import, and per-source guide request shapes.
func TestSourceOperationRequestsRoundTrip(t *testing.T) {
	const context = `[2,null,[1],[1,null,null,null,null,null,null,null,null,null,[1,3]]]`
	tests := []struct {
		method string
		wire   string
	}{
		{"MutateSource", `[null,["source-id"],[[["New title"]]],` + context + `]`},
		{"LoadSource", `[["source-id"],[2],` + context + `]`},
		{"AddSources", `[[[null,["Text title","Text body"],null,3,null,null,null,null,null,null,1]],"project-id",` + context + `]`},
		{"GenerateDocumentGuides", `[[[["source-id"]]],` + context + `]`},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			method, err := resolveMethod(tt.method)
			if err != nil {
				t.Fatalf("resolveMethod(%s): %v", tt.method, err)
			}
			msg := method.NewRequest()
			if err := beprotojson.Unmarshal([]byte(tt.wire), msg); err != nil {
				t.Fatalf("%s Unmarshal: %v", tt.method, err)
			}
			deltas, err := diffWireAgainstProto([]byte(tt.wire), msg)
			if err != nil {
				t.Fatalf("%s diff: %v", tt.method, err)
			}
			if len(deltas) != 0 {
				b, _ := json.Marshal(deltas)
				t.Fatalf("%s: expected lossless, got %d delta(s): %s", tt.method, len(deltas), b)
			}
			var encoded []interface{}
			switch req := msg.(type) {
			case *notebooklmv1alpha1.MutateSourceRequest:
				encoded = genmethod.EncodeMutateSourceArgs(req)
			case *notebooklmv1alpha1.LoadSourceRequest:
				encoded = genmethod.EncodeLoadSourceArgs(req)
			case *notebooklmv1alpha1.AddSourceRequest:
				encoded = genmethod.EncodeAddSourcesArgs(req)
			case *notebooklmv1alpha1.GenerateDocumentGuidesRequest:
				encoded = genmethod.EncodeGenerateDocumentGuidesArgs(req)
			default:
				t.Fatalf("%s: unexpected request type %T", tt.method, msg)
			}
			got, err := json.Marshal(encoded)
			if err != nil {
				t.Fatalf("%s marshal encoded args: %v", tt.method, err)
			}
			if string(got) != tt.wire {
				t.Fatalf("%s encoder = %s, want %s", tt.method, got, tt.wire)
			}
		})
	}
}

// TestMutateNoteRequestRoundTrip guards the present-empty tag wrapper and
// explicit zero-valued fields used when saving note content.
func TestMutateNoteRequestRoundTrip(t *testing.T) {
	const wire = `["project-id","note-id",[[["body","title",[],0,null,0]]],` +
		`[2,null,[1],[1,null,null,null,null,null,null,null,null,null,[1,3]]]]`
	method, err := resolveMethod("MutateNote")
	if err != nil {
		t.Fatal(err)
	}
	msg := method.NewRequest()
	if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	deltas, err := diffWireAgainstProto([]byte(wire), msg)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(deltas) != 0 {
		b, _ := json.Marshal(deltas)
		t.Fatalf("expected lossless, got %d delta(s): %s", len(deltas), b)
	}
	got, err := json.Marshal(genmethod.EncodeMutateNoteArgs(msg.(*notebooklmv1alpha1.MutateNoteRequest)))
	if err != nil {
		t.Fatalf("marshal encoded args: %v", err)
	}
	if string(got) != wire {
		t.Fatalf("encoder = %s, want %s", got, wire)
	}
}

// TestGenerateNotebookGuideRequestRoundTrip guards both observed request
// context variants. The second position is a context, not a guide-type enum.
func TestGenerateNotebookGuideRequestRoundTrip(t *testing.T) {
	wires := []string{
		`["project-id",[2,null,[1],[1,null,null,null,null,null,null,null,null,null,[1,3]]]]`,
		`["project-id",[2,null,null,[1,null,null,null,null,null,null,null,null,null,[1]]]]`,
	}
	method, err := resolveMethod("GenerateNotebookGuide")
	if err != nil {
		t.Fatal(err)
	}
	for _, wire := range wires {
		msg := method.NewRequest()
		if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
			t.Fatalf("Unmarshal(%s): %v", wire, err)
		}
		deltas, err := diffWireAgainstProto([]byte(wire), msg)
		if err != nil {
			t.Fatalf("diff(%s): %v", wire, err)
		}
		if len(deltas) != 0 {
			b, _ := json.Marshal(deltas)
			t.Fatalf("expected lossless for %s, got %d delta(s): %s", wire, len(deltas), b)
		}
	}
	got, err := json.Marshal(genmethod.EncodeGenerateNotebookGuideArgs(
		&notebooklmv1alpha1.GenerateNotebookGuideRequest{ProjectId: "project-id"},
	))
	if err != nil {
		t.Fatalf("marshal encoded args: %v", err)
	}
	if string(got) != wires[0] {
		t.Fatalf("encoder = %s, want %s", got, wires[0])
	}
}

// TestLogEventRequestRoundTrip guards the fixed promo-card lookup sentinel.
func TestLogEventRequestRoundTrip(t *testing.T) {
	const wire = `[[[[null,"1",627],[null,null,null,null,null,null,null,null,null,[null,null,6]],1]]]`
	method, err := resolveMethod("LogEvent")
	if err != nil {
		t.Fatal(err)
	}
	msg := method.NewRequest()
	if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	deltas, err := diffWireAgainstProto([]byte(wire), msg)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(deltas) != 0 {
		b, _ := json.Marshal(deltas)
		t.Fatalf("expected lossless, got %d delta(s): %s", len(deltas), b)
	}
	got, err := json.Marshal(genmethod.EncodeLogEventArgs(msg.(*notebooklmv1alpha1.LogEventRequest)))
	if err != nil {
		t.Fatalf("marshal encoded args: %v", err)
	}
	if string(got) != wire {
		t.Fatalf("encoder = %s, want %s", got, wire)
	}
}

// TestGenerateArtifactSuggestionsRequestRoundTrip guards the request context,
// wrapped source IDs, variation, and optional free-form prompt.
func TestGenerateArtifactSuggestionsRequestRoundTrip(t *testing.T) {
	wires := []string{
		`[[2,null,null,[1,null,null,null,null,null,null,null,null,null,[1]]],"project-id",[["source-a"],["source-b"]],1]`,
		`[[2,null,[1],[1,null,null,null,null,null,null,null,null,null,[1,3]]],"project-id",[["source-a"]],7,null,"make a short reel"]`,
	}
	method, err := resolveMethod("GenerateArtifactSuggestions")
	if err != nil {
		t.Fatal(err)
	}
	for _, wire := range wires {
		msg := method.NewRequest()
		if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
			t.Fatalf("Unmarshal(%s): %v", wire, err)
		}
		deltas, err := diffWireAgainstProto([]byte(wire), msg)
		if err != nil {
			t.Fatalf("diff(%s): %v", wire, err)
		}
		if len(deltas) != 0 {
			b, _ := json.Marshal(deltas)
			t.Fatalf("expected lossless for %s, got %d delta(s): %s", wire, len(deltas), b)
		}
	}
	msg := &notebooklmv1alpha1.GenerateArtifactSuggestionsRequest{
		ProjectId:  "project-id",
		SourceRefs: []*notebooklmv1alpha1.SourceIdList{{SourceId: "source-a"}},
		Variation:  7,
		Prompt:     proto.String("make a short reel"),
	}
	got, err := json.Marshal(genmethod.EncodeGenerateArtifactSuggestionsArgs(msg))
	if err != nil {
		t.Fatalf("marshal encoded args: %v", err)
	}
	if string(got) != wires[1] {
		t.Fatalf("encoder = %s, want %s", got, wires[1])
	}
}

// TestCreateUniversalArtifactRequestRoundTrip guards the audio, video, and
// slide variants sharing R7cb6c.
func TestCreateUniversalArtifactRequestRoundTrip(t *testing.T) {
	const context = `[2,null,null,[1,null,null,null,null,null,null,null,null,null,[1]],[[1,4,8,10,2,3,6,9]]]`
	wires := []string{
		`[` + context + `,"project-id",[null,null,1,[[["source-id"]]],null,null,[null,[null,2,null,[["source-id"]],"en",null,1]]]]`,
		`[` + context + `,"project-id",[null,null,3,[[["source-id"]]],null,null,null,null,[null,null,[[["source-id"]],null,"prompt",null,4]]]]`,
		`[` + context + `,"project-id",[null,null,3,[[["source-id"]]],null,null,null,null,[null,null,[[["source-id"]],"en","prompt",null,4,4]]]]`,
		`[` + context + `,"project-id",[null,null,8,[[["source-id"]]],null,null,null,null,null,null,null,null,null,null,null,null,[[null,"en",2,4]]]]`,
		`[` + context + `,"project-id",[null,null,10,null,null,null,null,null,null,[null,[5]]]]`,
		`[` + context + `,"project-id",[null,null,10,null,null,null,null,null,null,[null,[5,null,"canvas"]]]]`,
		`[` + context + `,"project-id",[null,null,8,[[["source-id"]]],null,null,null,null,null,null,null,null,null,null,null,null,[["plan mode attempt","en",2,4]]],null,null,[1]]`,
		`[` + context + `,"project-id",[null,null,7,[[["source-id"]]],null,null,null,null,null,null,null,null,null,null,[["fabricated mind-map prompt","en",null,1,3,3]]]]`,
	}
	method, err := resolveMethod("CreateUniversalArtifact")
	if err != nil {
		t.Fatal(err)
	}
	for _, wire := range wires {
		msg := method.NewRequest()
		if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
			t.Fatalf("Unmarshal(%s): %v", wire, err)
		}
		deltas, err := diffWireAgainstProto([]byte(wire), msg)
		if err != nil {
			t.Fatalf("diff(%s): %v", wire, err)
		}
		if len(deltas) != 0 {
			b, _ := json.Marshal(deltas)
			t.Fatalf("expected lossless for %s, got %d delta(s): %s", wire, len(deltas), b)
		}
		got, err := json.Marshal(genmethod.EncodeCreateUniversalArtifactArgs(
			msg.(*notebooklmv1alpha1.CreateUniversalArtifactRequest),
		))
		if err != nil {
			t.Fatalf("marshal encoded args: %v", err)
		}
		if string(got) != wire {
			t.Fatalf("encoder = %s, want %s", got, wire)
		}
	}
}

// TestMutateProjectCoverRequestRoundTrip guards the present-empty reset
// wrapper and selected cover preset.
func TestMutateProjectCoverRequestRoundTrip(t *testing.T) {
	const context = `[2,null,null,[1,null,null,null,null,null,null,null,null,null,[1]]]`
	tests := []struct {
		name  string
		cover string
	}{
		{name: "preset only", cover: `[[],[1]]`},
		{name: "present scalar", cover: `[[8],[5]]`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wire := `["project-id",[[null,null,null,null,null,null,null,` + test.cover + `]],` + context + `]`
			method, err := resolveMethod("MutateProjectCover")
			if err != nil {
				t.Fatal(err)
			}
			msg := method.NewRequest()
			if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
				t.Fatalf("Unmarshal(%s): %v", wire, err)
			}
			deltas, err := diffWireAgainstProto([]byte(wire), msg)
			if err != nil {
				t.Fatalf("diff(%s): %v", wire, err)
			}
			if len(deltas) != 0 {
				b, _ := json.Marshal(deltas)
				t.Fatalf("expected lossless for %s, got %d delta(s): %s", wire, len(deltas), b)
			}
			got, err := json.Marshal(genmethod.EncodeMutateProjectCoverArgs(
				msg.(*notebooklmv1alpha1.MutateProjectCoverRequest),
			))
			if err != nil {
				t.Fatalf("marshal encoded args: %v", err)
			}
			if string(got) != wire {
				t.Fatalf("encoder = %s, want %s", got, wire)
			}
		})
	}
}

// TestArtifactSourceRoundTrip preserves the scalar wrapper observed on
// ListArtifacts source entries.
func TestArtifactSourceRoundTrip(t *testing.T) {
	const wire = `[["source-id"],null,8]`
	msg := &notebooklmv1alpha1.ArtifactSource{}
	if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
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

// TestTailRequestVariantsRoundTrip guards four low-frequency request shapes.
func TestTailRequestVariantsRoundTrip(t *testing.T) {
	const context = `[2,null,[1],[1,null,null,null,null,null,null,null,null,null,[1,3]]]`
	const filteredContext = `[2,null,null,[1,null,null,null,null,null,null,null,null,null,[1]],[[1,4,8,10,2,3,6,9]]]`
	tests := []struct {
		method string
		wire   string
	}{
		{"CreateNote", `["project-id","",[1],null,"New Note",null,` + context + `]`},
		{"GetProjectAnalytics", `["project-id",null,[1776236400],[2]]`},
		{"StartDeepResearchWire", `[` + context + `,null,["query",1],5,"project-id"]`},
		{"ReviseArtifact", `[` + filteredContext + `,"artifact-id",[[[0,"first"],[4,"second"]]]]`},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			method, err := resolveMethod(tt.method)
			if err != nil {
				t.Fatal(err)
			}
			msg := method.NewRequest()
			if err := beprotojson.Unmarshal([]byte(tt.wire), msg); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			deltas, err := diffWireAgainstProto([]byte(tt.wire), msg)
			if err != nil {
				t.Fatalf("diff: %v", err)
			}
			if len(deltas) != 0 {
				b, _ := json.Marshal(deltas)
				t.Fatalf("expected lossless, got %d delta(s): %s", len(deltas), b)
			}
			var encoded []interface{}
			switch req := msg.(type) {
			case *notebooklmv1alpha1.CreateNoteRequest:
				encoded = genmethod.EncodeCreateNoteArgs(req)
			case *notebooklmv1alpha1.GetProjectAnalyticsRequest:
				encoded = genmethod.EncodeGetProjectAnalyticsArgs(req)
			case *notebooklmv1alpha1.StartDeepResearchWireRequest:
				encoded = genmethod.EncodeStartDeepResearchWireArgs(req)
			case *notebooklmv1alpha1.ReviseArtifactRequest:
				encoded = genmethod.EncodeReviseArtifactArgs(req)
			}
			got, err := json.Marshal(encoded)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.wire {
				t.Fatalf("encoder = %s, want %s", got, tt.wire)
			}
		})
	}
}

// TestLabelRequestVariantsRoundTrip guards the mode, create, rename, and two
// source-assignment forms observed for the overloaded label RPCs.
func TestLabelRequestVariantsRoundTrip(t *testing.T) {
	const context = `[2,null,null,[1,null,null,null,null,null,null,null,null,null,[1]]]`
	tests := []struct {
		method string
		wire   string
	}{
		{"MutateLabelsMode", `[` + context + `,"project-id",null,null,[0]]`},
		{"MutateLabelsMode", `[` + context + `,"project-id",null,null,[1]]`},
		{"CreateLabel", `[` + context + `,"project-id",null,null,null,[["New Label",""]]]`},
		{"MutateLabel", `[` + context + `,"project-id","label-id",[[["New Name"]]]]`},
		{"MutateLabel", `[` + context + `,"project-id","label-id",[[null,[["source-id"]]]]]`},
		{"MutateLabel", `[` + context + `,"project-id","label-id",[[null,null,[["source-id"]]]]]`},
	}
	for _, tt := range tests {
		t.Run(tt.method+tt.wire, func(t *testing.T) {
			method, err := resolveMethod(tt.method)
			if err != nil {
				t.Fatal(err)
			}
			msg := method.NewRequest()
			if err := beprotojson.Unmarshal([]byte(tt.wire), msg); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			deltas, err := diffWireAgainstProto([]byte(tt.wire), msg)
			if err != nil {
				t.Fatalf("diff: %v", err)
			}
			if len(deltas) != 0 {
				b, _ := json.Marshal(deltas)
				t.Fatalf("expected lossless, got %d delta(s): %s", len(deltas), b)
			}
			var encoded []interface{}
			switch req := msg.(type) {
			case *notebooklmv1alpha1.CreateLabelRequest:
				encoded = genmethod.EncodeCreateLabelArgs(req)
			case *notebooklmv1alpha1.MutateLabelsModeRequest:
				encoded = genmethod.EncodeMutateLabelsModeArgs(req)
			case *notebooklmv1alpha1.MutateLabelRequest:
				encoded = genmethod.EncodeMutateLabelArgs(req)
			}
			got, err := json.Marshal(encoded)
			if err != nil {
				t.Fatalf("marshal encoded args: %v", err)
			}
			if string(got) != tt.wire {
				t.Fatalf("encoder = %s, want %s", got, tt.wire)
			}
		})
	}
}

// TestBulkImportTextRequestRoundTrip guards the note/text LBwxtb variant with
// a short synthetic report; no captured report content is retained here.
func TestBulkImportTextRequestRoundTrip(t *testing.T) {
	const wire = `[[2,null,[1],[1,null,null,null,null,null,null,null,null,null,[1,3]]],[1],` +
		`"conversation-id","project-id",[[null,["Report","# Summary\n\nSynthetic text."],null,3,null,null,null,null,null,null,3]]]`
	method, err := resolveMethod("BulkImportFromResearchWire")
	if err != nil {
		t.Fatal(err)
	}
	msg := method.NewRequest()
	if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	deltas, err := diffWireAgainstProto([]byte(wire), msg)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(deltas) != 0 {
		b, _ := json.Marshal(deltas)
		t.Fatalf("expected lossless, got %d delta(s): %s", len(deltas), b)
	}
	got, err := json.Marshal(genmethod.EncodeBulkImportFromResearchWireArgs(
		msg.(*notebooklmv1alpha1.BulkImportFromResearchWireRequest),
	))
	if err != nil {
		t.Fatalf("marshal encoded args: %v", err)
	}
	if string(got) != wire {
		t.Fatalf("encoder = %s, want %s", got, wire)
	}
}

// TestGenerateFreeFormStreamedWireRoundTrip guards the dedicated streaming
// request and terminal cumulative response shapes without captured content.
func TestGenerateFreeFormStreamedWireRoundTrip(t *testing.T) {
	const requestWire = `[[[["source-id"]]],"prompt",[["prior answer",null,2]],` +
		`[2,null,[1],[1,null,null,null,null,null,null,null,null,null,[1,3]]],` +
		`"conversation-id",null,null,"notebook-id",1]`
	const responseWire = `[["answer",null,["conversation-id","message-id",1]],` +
		`[[null,null,1]],[[[null,0,1],[0]]],[["next"]],true,[[["next",9]]],true,"token"]`
	tests := []struct {
		side string
		wire string
	}{
		{"request", requestWire},
		{"response", responseWire},
	}
	for _, tt := range tests {
		t.Run(tt.side, func(t *testing.T) {
			method, err := resolveMethod("GenerateFreeFormStreamedWire")
			if err != nil {
				t.Fatal(err)
			}
			var msg proto.Message
			if tt.side == "request" {
				msg = method.NewRequest()
			} else {
				msg = method.NewResponse()
			}
			if err := beprotojson.Unmarshal([]byte(tt.wire), msg); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			deltas, err := diffWireAgainstProto([]byte(tt.wire), msg)
			if err != nil {
				t.Fatalf("diff: %v", err)
			}
			if len(deltas) != 0 {
				b, _ := json.Marshal(deltas)
				t.Fatalf("expected lossless, got %d delta(s): %s", len(deltas), b)
			}
			if tt.side == "request" {
				got, err := json.Marshal(genmethod.EncodeGenerateFreeFormStreamedWireArgs(
					msg.(*notebooklmv1alpha1.GenerateFreeFormStreamedWireRequest),
				))
				if err != nil {
					t.Fatalf("marshal encoded args: %v", err)
				}
				if string(got) != tt.wire {
					t.Fatalf("encoder = %s, want %s", got, tt.wire)
				}
			}
		})
	}
}

// TestAudioFormatsRoundTrip guards the four-position sqTeoe catalog. Its
// response wrapper must be unwrapped even though catalog field 1 is itself a
// message containing a repeated message field.
func TestAudioFormatsRoundTrip(t *testing.T) {
	const wire = `[[[[[1,"Deep Dive","Audio description"],[2,"Brief","Short audio"]]],` +
		`[[[1,"Explainer","Video description"],[2,"Brief","Short video"]]],` +
		`[[[1,"Detailed Deck","Slide description"],[2,"Presenter Slides","Visual slides"]]],` +
		`[[["Briefing Doc","Key insights","Prompt"],["Study Guide","Quiz","Study prompt"]]]]]`

	msg := &notebooklmv1alpha1.GetAudioFormatsResponse{}
	if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got := msg.GetAudioKinds().GetItems(); len(got) != 2 || got[0].GetName() != "Deep Dive" {
		t.Fatalf("audio kinds decoded wrong: %+v", got)
	}
	if got := msg.GetDocTemplates().GetItems(); len(got) != 2 || got[0].GetTitle() != "Briefing Doc" {
		t.Fatalf("document templates decoded wrong: %+v", got)
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

// TestCreateNoteRecordRoundTrip guards the explicit empty content string in a
// freshly allocated CYK0Xb note shell.
func TestCreateNoteRecordRoundTrip(t *testing.T) {
	const wire = `["note-id","",` +
		`[1,"157962509464",[100,200],null,null,[100,200],false],null,"New Note"]`

	msg := &notebooklmv1alpha1.NoteRecord{}
	if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if msg.ContentText == nil || msg.GetContentText() != "" {
		t.Fatalf("content presence lost: %+v", msg)
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

// TestGetNotesResponseRoundTrip guards legacy plain-text cFji9 note entries,
// their entry discriminator, and the response snapshot timestamp.
func TestGetNotesResponseRoundTrip(t *testing.T) {
	const wire = `[[["note-id",["note-id","Body",` +
		`[1,"157962509464",[100,200],null,null,[110,300],false],null,"Title"],2]],` +
		`[120,400]]`

	msg := &notebooklmv1alpha1.GetNotesRichWireResponse{}
	if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	entries := msg.GetEntries()
	if len(entries) != 1 || entries[0].GetNote().GetContentText() != "Body" {
		t.Fatalf("entries decoded wrong: %+v", entries)
	}
	if msg.GetFetchTime().GetSeconds() != 120 || msg.GetFetchTime().GetNanos() != 400 {
		t.Fatalf("fetch time decoded wrong: %+v", msg.GetFetchTime())
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

// TestGetNotesRichResponseRoundTrip guards the cFji9 rich-note variants:
// structured rich text, its parallel unkeyed grounding list, inline images,
// table-row metadata, and explicitly empty text leaves.
func TestGetNotesRichResponseRoundTrip(t *testing.T) {
	wire, err := os.ReadFile("testdata/get_notes_rich_wire.json")
	if err != nil {
		t.Fatal(err)
	}
	msg := &notebooklmv1alpha1.GetNotesRichWireResponse{}
	if err := beprotojson.Unmarshal(wire, msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	entries := msg.GetEntries()
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	document := entries[0].GetNote().GetRichText().GetDocument()
	if document == nil || len(document.GetBody().GetBlocks()) != 1 {
		t.Fatalf("rich document decoded wrong: %+v", document)
	}
	details := entries[0].GetNote().GetGroundingDetails().GetGrounding()
	if len(details) != 2 {
		t.Fatalf("grounding details = %d, want 2", len(details))
	}
	deltas, err := diffWireAgainstProto(wire, msg)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(deltas) != 0 {
		b, _ := json.Marshal(deltas)
		t.Fatalf("expected lossless, got %d delta(s): %s", len(deltas), b)
	}
}

// TestGetNotesRichSpanVariantsRoundTrip guards variants first observed inside
// rich notes rather than chat responses.
func TestGetNotesRichSpanVariantsRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		wire string
		msg  proto.Message
	}{
		{
			name: "inline image",
			wire: `[0,1,null,["https://example.invalid/image",null,"image-1"]]`,
			msg:  &notebooklmv1alpha1.Span{},
		},
		{
			name: "table row metadata",
			wire: `[0,4,[[0,4,[""]]],[null,false]]`,
			msg:  &notebooklmv1alpha1.SpanTableRow{},
		},
		{
			name: "explicit empty text",
			wire: `[""]`,
			msg:  &notebooklmv1alpha1.TextLeaf{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := beprotojson.Unmarshal([]byte(test.wire), test.msg); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			deltas, err := diffWireAgainstProto([]byte(test.wire), test.msg)
			if err != nil {
				t.Fatalf("diff: %v", err)
			}
			if len(deltas) != 0 {
				b, _ := json.Marshal(deltas)
				t.Fatalf("expected lossless, got %d delta(s): %s", len(deltas), b)
			}
		})
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

// TestBulkImportFromResearchResponseRoundTrip guards the LBwxtb response:
// imported sources use the shared enriched Source descriptor.
func TestBulkImportFromResearchResponseRoundTrip(t *testing.T) {
	const wire = `[[[["source-id"],"Imported Note",` +
		`[null,3347,[100,200],["origin-id",[90,100]],8,null,3,null,7246,null,null,null,null,null,[110,300]],` +
		`[null,2]]]]`

	msg := &notebooklmv1alpha1.BulkImportFromResearchResponse{}
	if err := beprotojson.Unmarshal([]byte(wire), msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	results := msg.GetResults()
	if len(results) != 1 || results[0].GetSourceId().GetSourceId() != "source-id" {
		t.Fatalf("results decoded wrong: %+v", results)
	}
	if results[0].GetTitle() != "Imported Note" || results[0].GetMetadata().GetContentLength() != 7246 {
		t.Fatalf("source metadata decoded wrong: %+v", results[0])
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

// TestGetConversationHistoryRichResponseRoundTrip guards the khqZz cursor,
// explicit empty segment text, styled marks, and shifted list marker.
func TestGetConversationHistoryRichResponseRoundTrip(t *testing.T) {
	wire, err := os.ReadFile("testdata/get_conversation_history_rich_wire.json")
	if err != nil {
		t.Fatal(err)
	}
	msg := &notebooklmv1alpha1.GetConversationHistoryResponse{}
	if err := beprotojson.Unmarshal(wire, msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if msg.GetCursor() != "cursor" {
		t.Fatalf("cursor = %q, want cursor", msg.GetCursor())
	}
	deltas, err := diffWireAgainstProto(wire, msg)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(deltas) != 0 {
		b, _ := json.Marshal(deltas)
		t.Fatalf("expected lossless, got %d delta(s): %s", len(deltas), b)
	}
}

func TestConversationRichShapeVariantsRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		wire string
		msg  proto.Message
	}{
		{
			name: "styled marks",
			wire: `[null,null,null,null,["Inter",400,11],[1,2,3]]`,
			msg:  &notebooklmv1alpha1.TextMarks{},
		},
		{
			name: "direct marker",
			wire: `[null,null,0,{"101":"•","102":1,"103":1,"104":0}]`,
			msg:  &notebooklmv1alpha1.ListItem{},
		},
		{
			name: "shifted marker",
			wire: `[null,null,0,[null,null,false],{"101":"•","102":1,"103":1,"104":0}]`,
			msg:  &notebooklmv1alpha1.ListItem{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := beprotojson.Unmarshal([]byte(test.wire), test.msg); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			deltas, err := diffWireAgainstProto([]byte(test.wire), test.msg)
			if err != nil {
				t.Fatalf("diff: %v", err)
			}
			if len(deltas) != 0 {
				b, _ := json.Marshal(deltas)
				t.Fatalf("expected lossless, got %d delta(s): %s", len(deltas), b)
			}
		})
	}
}

// TestLoadSourceRichResponseRoundTrip guards the hizoJc loaded-source styles,
// list item, table, and code-block variants.
func TestLoadSourceRichResponseRoundTrip(t *testing.T) {
	wire, err := os.ReadFile("testdata/load_source_rich_wire.json")
	if err != nil {
		t.Fatal(err)
	}
	msg := &notebooklmv1alpha1.LoadSourceResponse{}
	if err := beprotojson.Unmarshal(wire, msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(msg.GetContent().GetStyles()) != 1 {
		t.Fatalf("styles decoded wrong: %+v", msg.GetContent())
	}
	deltas, err := diffWireAgainstProto(wire, msg)
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
