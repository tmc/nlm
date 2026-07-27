package main

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

func renderToString(t *testing.T, doc chatDocument, ctx chatRenderContext) string {
	t.Helper()
	var out strings.Builder
	if err := renderChatHTML(&out, doc, ctx); err != nil {
		t.Fatal(err)
	}
	return out.String()
}
