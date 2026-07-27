package main

import (
	"strconv"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

// Bridge from the api-package wire tree (api.RichContent, decoded in
// parseConversationHistory) to the branch-local render model (richDocument,
// projected by projectRichDocument). The two mirror the same wire shape across a
// package boundary the api layer can't cross (it cannot import package main), so
// this is a thin structural map, not a reinterpretation: block groups become
// content groups, leaves become leaves, bare spans become separators. Offsets
// cross as strings because the render model keeps them as wire strings until
// projection (see rich_document.go).

// richDocumentFromAPI converts a decoded api.RichContent into the render model,
// or returns nil when there is nothing to render (so the caller keeps flat
// Content). It never fails: an unrecognized span degrades to whatever structural
// fields it does carry.
func richDocumentFromAPI(rc *api.RichContent) *richDocument {
	if rc == nil || len(rc.Blocks) == 0 {
		return nil
	}
	blocks := make([]richSpan, 0, len(rc.Blocks))
	for _, b := range rc.Blocks {
		blocks = append(blocks, richSpanFromAPI(b))
	}
	return &richDocument{Blocks: blocks}
}

// richDocumentFromProto converts the generated RichDocument carried by a note
// into the renderer's shared rich-document model.
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

// richSpanFromAPI maps one wire span to a render span. The mapping is
// structural: a span with text is a leaf; a span with children is a content
// group (field 3); a bare span (no text, no children) is a separator boundary.
// The block trailer tag and inline marks are carried onto the leaf/group so the
// projection can apply the safe formatting subset.
func richSpanFromAPI(s api.RichSpan) richSpan {
	out := richSpan{
		Start: strconv.Itoa(s.Start),
		End:   strconv.Itoa(s.End),
	}
	switch {
	case s.Text != "":
		out.Leaf = &richLeaf{Text: s.Text, Marks: marksFromAPI(s.Marks)}
	case len(s.Children) > 0:
		children := make([]richSpan, 0, len(s.Children))
		for _, c := range s.Children {
			children = append(children, richSpanFromAPI(c))
		}
		group := &richGroup{Children: children}
		if s.ListItem != nil {
			// A ListItem marks this group as a list item; its presence (not any
			// Kind label) is what makes the projection treat the block as a list.
			group.ListItem = &richListItem{
				Nesting: s.ListItem.Nesting,
				Bullet:  s.ListItem.Bullet,
			}
		}
		out.Group = group
	default:
		// No text and no children: a zero-width boundary between blocks.
		out.Separator = true
	}
	return out
}

// marksFromAPI folds the positional wire TextMarks flags to the render model's
// safe subset. The wire marks are a bool slice indexed by flag position; only
// the positions we can render are asserted. Position 7 (the 8th flag) is the
// code/identifier run observed on the real frame → Flag8. Other set positions
// fold to generic emphasis via Flag2 rather than an unverified bold/italic
// split. Returns nil when no flag is set, so an unmarked leaf carries no marks.
func marksFromAPI(marks []bool) *richMarks {
	if len(marks) == 0 {
		return nil
	}
	m := &richMarks{}
	set := false
	for i, on := range marks {
		if !on {
			continue
		}
		set = true
		switch i {
		case 7:
			m.Flag8 = true // inline code / identifier run (confirmed on the frame)
		case 0:
			m.Flag1 = true // inline heading/label hint (unconfirmed)
		default:
			m.Flag2 = true // generic emphasis
		}
	}
	if !set {
		return nil
	}
	return m
}
