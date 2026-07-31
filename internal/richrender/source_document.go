package richrender

import (
	"path"
	"strings"
	"unicode"

	"github.com/tmc/nlm/notebooklm"
)

// SourceText reconstructs a source's default plain-text reading view from the
// server's ordered fragments.
//
// [notebooklm.LoadSourceText.Full] pads the server's untyped offset gaps with
// one space per missing offset to stay faithful to the citation coordinate
// space; that fidelity turns a region the NotebookLM index dropped into a run
// of blanks, so a missing paragraph reads as whitespace. SourceText is for
// reading rather than offset lookups, so it collapses each gap into reading
// flow. A source with no gaps renders identically to Full.
func SourceText(body notebooklm.LoadSourceText) string {
	model := sourceContentModel{body: body}
	emitter := sourceTextEmitter{cursor: firstSourceFragmentOffset(body.Fragments)}
	_ = RenderContent(model, &emitter)
	return emitter.out.String()
}

// sourceContentModel adapts a LoadSourceText body to the common content model.
type sourceContentModel struct {
	body notebooklm.LoadSourceText
}

func (m sourceContentModel) ContentLen() int { return len(m.body.Fragments) }

func (m sourceContentModel) ContentFragment(i int) ContentFragment {
	f := m.body.Fragments[i]
	fragment := ContentFragment{
		Start:         f.Start,
		End:           f.End,
		Text:          f.Text,
		ImageURL:      f.ImageURL,
		ImageID:       f.ImageID,
		ListMarker:    f.ListMarker,
		Language:      f.Language,
		Bold:          f.Bold,
		Italic:        f.Italic,
		Code:          f.Code,
		RangeMismatch: f.RangeMismatch,
		BlockStart:    f.BlockStart,
	}
	if !f.IsImage() {
		if name, ok := sourceTxtarHeader(f.Text); ok {
			fragment.MemberName = name
		}
	}
	return fragment
}

type sourceTextEmitter struct {
	out    strings.Builder
	cursor int
}

func (e *sourceTextEmitter) EmitContent(fragment ContentFragment) error {
	switch fragment.Kind {
	case ContentImage:
		return nil
	case ContentMember:
		writeSourceBreak(&e.out)
		e.out.WriteString(fragment.Text)
		writeSourceBreak(&e.out)
	default:
		writeSourceGap(&e.out, e.cursor, fragment.Start)
		e.out.WriteString(fragment.Text)
	}
	e.cursor = fragment.End
	return nil
}

func (*sourceTextEmitter) FinishContent() error { return nil }

func firstSourceFragmentOffset(fragments []notebooklm.TextFragment) int {
	for _, f := range fragments {
		if !f.IsImage() {
			return f.Start
		}
	}
	return 0
}

// writeSourceGap turns the server's otherwise-untyped offset gaps into
// reading-flow whitespace. A one-character gap remains a space; a larger gap
// is a block boundary. The offset-faithful Full method does not use this.
func writeSourceGap(b *strings.Builder, start, end int) {
	if end <= start || b.Len() == 0 {
		return
	}
	if end-start == 1 {
		if !strings.HasSuffix(b.String(), "\n") {
			b.WriteByte(' ')
		}
		return
	}
	if !strings.HasSuffix(b.String(), "\n\n") {
		b.WriteString("\n\n")
	}
}

func writeSourceBreak(b *strings.Builder) {
	if b.Len() == 0 || strings.HasSuffix(b.String(), "\n\n") {
		return
	}
	if strings.HasSuffix(b.String(), "\n") {
		b.WriteByte('\n')
		return
	}
	b.WriteString("\n\n")
}

// sourceTxtarHeader reports whether text is a txtar-style member header
// ("-- path/name.ext --") and returns the member name.
func sourceTxtarHeader(text string) (string, bool) {
	if strings.ContainsAny(text, "\r\n") ||
		!strings.HasPrefix(text, "-- ") ||
		!strings.HasSuffix(text, " --") {
		return "", false
	}
	name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(text, "-- "), " --"))
	if name == "" || path.IsAbs(name) || path.Clean(name) != name ||
		name == "." || name == ".." || strings.HasPrefix(name, "../") ||
		strings.HasSuffix(name, "/") ||
		(!strings.Contains(name, "/") && !strings.Contains(path.Base(name), ".")) {
		return "", false
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		switch r {
		case '/', '.', '_', '-', '+', '@':
			continue
		}
		return "", false
	}
	return name, true
}
