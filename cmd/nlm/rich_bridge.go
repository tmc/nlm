package main

import (
	"strconv"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// richDocumentFromProto converts a generated RichDocument into the renderer's
// shared rich-document model.
func richDocumentFromProto(doc *pb.RichDocument) *richDocument {
	if doc == nil || len(doc.GetBody().GetBlocks()) == 0 {
		return nil
	}
	blocks := make([]richSpan, 0, len(doc.GetBody().GetBlocks()))
	for _, block := range doc.GetBody().GetBlocks() {
		blocks = append(blocks, richSpanFromProto(block))
	}
	return &richDocument{Blocks: blocks}
}

func richSpanFromProto(span *pb.Span) richSpan {
	if span == nil {
		return richSpan{}
	}
	out := richSpan{
		Start: protoOffset(span.Start),
		End:   protoOffset(span.End),
	}
	switch {
	case span.GetContent() != nil:
		setProtoContent(&out, span.GetContent(), false)
	case span.GetHiddenContent() != nil:
		setProtoContent(&out, span.GetHiddenContent(), true)
	case span.GetTable() != nil:
		out.Table = &richGroup{Children: tableSpansFromProto(span.GetTable())}
	case span.GetCodeBlock() != nil:
		out.CodeBlock = &richGroup{Children: []richSpan{{
			Start: out.Start,
			End:   out.End,
			Leaf: &richLeaf{
				Text:  span.GetCodeBlock().GetCode(),
				Marks: &richMarks{Flag8: true},
			},
		}}}
	case span.GetSeparator() != nil:
		out.Separator = true
	}
	return out
}

func protoOffset(value *int64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(*value, 10)
}

func setProtoContent(out *richSpan, content *pb.SpanContent, hidden bool) {
	if leaf := content.GetLeaf(); leaf != nil {
		out.Leaf = &richLeaf{Text: leaf.GetText(), Marks: marksFromProto(leaf.GetMarks())}
		return
	}
	group := groupFromProto(content.GetGroup())
	if hidden {
		out.Hidden = group
	} else {
		out.Group = group
	}
}

func groupFromProto(group *pb.SpanGroup) *richGroup {
	if group == nil {
		return nil
	}
	out := &richGroup{Indent: int(group.GetMeta().GetIndent())}
	for _, element := range group.GetSpans() {
		if span := element.GetSpan(); span != nil {
			out.Children = append(out.Children, richSpanFromProto(span))
		}
	}
	if item := group.GetListItem(); item != nil {
		out.ListItem = &richListItem{
			Nesting: int(item.GetNesting()),
			Bullet:  listBulletFromProto(item),
		}
	}
	return out
}

func listBulletFromProto(item *pb.ListItem) string {
	if marker := item.GetMarker().GetMarker(); marker != nil {
		return marker.GetBullet()
	}
	return item.GetTrailingMarker().GetBullet()
}

func tableSpansFromProto(table *pb.SpanTable) []richSpan {
	var spans []richSpan
	for _, row := range table.GetRows() {
		for _, cell := range row.GetCells() {
			spans = append(spans, richSpanFromProto(cell))
		}
	}
	return spans
}

func marksFromProto(marks *pb.TextMarks) *richMarks {
	if marks == nil {
		return nil
	}
	out := &richMarks{
		Flag1: marks.GetFlag1(),
		Flag2: marks.GetFlag2() || marks.GetFlag3() || marks.GetFlag7() || marks.GetFlag9(),
		Flag8: marks.GetFlag8() || marks.GetFlag9(),
		Link:  marks.GetLink(),
	}
	if !out.Flag1 && !out.Flag2 && !out.Flag8 && out.Link == "" {
		return nil
	}
	return out
}
