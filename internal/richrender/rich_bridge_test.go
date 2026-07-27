package richrender

import (
	"strings"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

func testProtoRichDocument(text string) *pb.RichDocument {
	end := int64(len(text))
	return &pb.RichDocument{Body: &pb.SpanLayers{Blocks: []*pb.Span{{
		Start: proto.Int64(0),
		End:   proto.Int64(end),
		Content: &pb.SpanContent{Value: &pb.SpanContent_Group{Group: &pb.SpanGroup{
			Spans: []*pb.SpanElement{{Value: &pb.SpanElement_Span{Span: &pb.Span{
				Start: proto.Int64(0),
				End:   proto.Int64(end),
				Content: &pb.SpanContent{Value: &pb.SpanContent_Leaf{Leaf: &pb.TextLeaf{
					Text: proto.String(text),
				}}},
			}}}},
		}}},
	}}}}
}

func TestRichDocumentFromProtoProjectsListAndInlineMark(t *testing.T) {
	document := &pb.RichDocument{Body: &pb.SpanLayers{Blocks: []*pb.Span{
		testProtoRichBlock(0, 5, "Intro", &pb.TextMarks{Flag2: proto.Bool(true)}, nil),
		testProtoRichBlock(5, 10, "First", nil, testProtoListItem(0)),
		testProtoRichBlock(10, 16, "Second", nil, testProtoListItem(0)),
	}}}

	blocks := projectRichDocument(richDocumentFromProto(document))
	if len(blocks) != 2 {
		t.Fatalf("got %d projected blocks, want paragraph and coalesced list", len(blocks))
	}
	if blocks[0].Kind != blockParagraph || len(blocks[0].Runs) != 1 || !blocks[0].Runs[0].Emphasis {
		t.Errorf("inline-mark paragraph = %+v", blocks[0])
	}
	if blocks[1].Kind != blockList || len(blocks[1].Items) != 2 {
		t.Errorf("projected list = %+v, want two coalesced items", blocks[1])
	}
}

func testProtoRichBlock(start, end int64, text string, marks *pb.TextMarks, item *pb.ListItem) *pb.Span {
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

func testProtoListItem(nesting int64) *pb.ListItem {
	return &pb.ListItem{
		Nesting: proto.Int64(nesting),
		Marker: &pb.ListItemMarker{Value: &pb.ListItemMarker_Marker{Marker: &pb.ListMarker{
			Bullet:     "•",
			MarkerKind: proto.Int64(1),
		}}},
	}
}

// TestRichReflowIntegrity is the (c) reflow check: bridge a multi-block tree,
// project it, and flatten to text — the reflowed body must (1) reinsert the
// paragraph breaks the flat text lacks and (2) preserve every non-whitespace
// character (offset integrity: reflow only adds whitespace, never drops or
// reorders content, so citation offsets into the flat text stay valid). The
// proto-decode seam is covered by TestExtractChatPayloadPreservesRichDocument
// in the api package; this exercises the bridge→project→flatten chain that runs
// on the cmd side.
func TestRichReflowIntegrity(t *testing.T) {
	// Three paragraphs the flat text would run together ("AlphaBetaGamma").
	document := &pb.RichDocument{Body: &pb.SpanLayers{Blocks: []*pb.Span{
		testProtoRichBlock(0, 5, "Alpha", nil, nil),
		testProtoRichBlock(5, 9, "Beta", nil, nil),
		testProtoRichBlock(9, 14, "Gamma", nil, nil),
	}}}
	doc := richDocumentFromProto(document)
	if doc == nil {
		t.Fatal("bridge returned nil")
	}
	reflowed := flattenText(projectRichDocument(doc))

	if !strings.Contains(reflowed, "\n\n") {
		t.Errorf("reflowed body has no paragraph break — structure not reconstructed: %q", reflowed)
	}
	flat := flattenLeafText(doc)
	if strip(reflowed) != strip(flat) {
		t.Errorf("reflow changed non-whitespace content:\n flat: %q\n refl: %q",
			trunc(strip(flat)), trunc(strip(reflowed)))
	}
	if strip(flat) != "AlphaBetaGamma" {
		t.Errorf("flat leaf text = %q, want AlphaBetaGamma", strip(flat))
	}
}

// flattenLeafText concatenates every leaf's text in document order with no
// separators — the flat baseline the reflow must preserve character-for-character
// (modulo whitespace).
func flattenLeafText(doc *richDocument) string {
	var b strings.Builder
	var walk func(s richSpan)
	walk = func(s richSpan) {
		if s.Leaf != nil {
			b.WriteString(s.Leaf.Text)
		}
		if s.Group != nil {
			for _, c := range s.Group.Children {
				walk(c)
			}
		}
	}
	for _, blk := range doc.Blocks {
		walk(blk)
	}
	return b.String()
}

func strip(s string) string { return strings.Join(strings.Fields(s), "") }

func trunc(s string) string {
	if len(s) > 80 {
		return s[:80] + "…"
	}
	return s
}
