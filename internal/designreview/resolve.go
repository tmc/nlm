package designreview

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

// NativeCitation is a citation event emitted by
// `nlm generate-chat --citations=json`.
type NativeCitation struct {
	Phase       string  `json:"phase,omitempty"`
	Index       int     `json:"index,omitempty"`
	SourceID    string  `json:"source_id"`
	SourceTitle string  `json:"title,omitempty"`
	StartChar   int     `json:"start_char"`
	EndChar     int     `json:"end_char"`
	Confidence  float64 `json:"confidence,omitempty"`
	AnswerStart int     `json:"answer_start,omitempty"`
	AnswerEnd   int     `json:"answer_end,omitempty"`
}

// Resolved maps one native citation back to source-relative coordinates.
type Resolved struct {
	SourceID    string  `json:"source_id"`
	SourceTitle string  `json:"title,omitempty"`
	StartChar   int     `json:"start_char"`
	EndChar     int     `json:"end_char"`
	File        string  `json:"file"`
	Line        int     `json:"line,omitempty"`
	Column      int     `json:"column,omitempty"`
	EndLine     int     `json:"end_line,omitempty"`
	EndColumn   int     `json:"end_column,omitempty"`
	Status      Status  `json:"status"`
	Reason      string  `json:"reason,omitempty"`
	Confidence  float64 `json:"confidence,omitempty"`
	Snippet     string  `json:"snippet,omitempty"`
}

type sourceResolver struct {
	body        api.LoadSourceText
	text        []rune
	defaultFile string
	members     []txtarMember
}

type txtarMember struct {
	Name        string
	HeaderStart int
	HeaderEnd   int
	BodyStart   int
	BodyEnd     int
}

// ReadNativeCitations extracts citation events from a chat JSONL stream.
// Non-citation events are ignored.
func ReadNativeCitations(r io.Reader) ([]NativeCitation, error) {
	var out []NativeCitation
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for lineNum := 1; sc.Scan(); lineNum++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var c NativeCitation
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			return nil, fmt.Errorf("decode line %d: %w", lineNum, err)
		}
		if c.Phase != "" && c.Phase != "citation" {
			continue
		}
		if c.SourceID == "" {
			continue
		}
		out = append(out, c)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// RenderChatAnswer extracts the concatenated answer text from a chat JSONL
// stream and writes it to w.
func RenderChatAnswer(w io.Writer, r io.Reader) error {
	bw := bufio.NewWriter(w)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for lineNum := 1; sc.Scan(); lineNum++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev struct {
			Phase string `json:"phase"`
			Text  string `json:"text,omitempty"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return fmt.Errorf("decode line %d: %w", lineNum, err)
		}
		if ev.Phase != "answer" {
			continue
		}
		if _, err := io.WriteString(bw, ev.Text); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return bw.Flush()
}

// Resolve maps one citation against a source's server-indexed text.
func Resolve(body api.LoadSourceText, c NativeCitation) Resolved {
	return newSourceResolver(body).Resolve(c)
}

// ResolveAll resolves cites, fetching each distinct source at most once.
func ResolveAll(load func(sourceID string) (api.LoadSourceText, error), cites []NativeCitation) ([]Resolved, error) {
	cache := make(map[string]*sourceResolver)
	out := make([]Resolved, 0, len(cites))
	for _, c := range cites {
		if c.SourceID == "" {
			return nil, fmt.Errorf("citation missing source_id")
		}
		resolver := cache[c.SourceID]
		if resolver == nil {
			body, err := load(c.SourceID)
			if err != nil {
				return nil, fmt.Errorf("load source %s: %w", c.SourceID, err)
			}
			resolver = newSourceResolver(body)
			cache[c.SourceID] = resolver
		}
		out = append(out, resolver.Resolve(c))
	}
	return out, nil
}

func newSourceResolver(body api.LoadSourceText) *sourceResolver {
	file := body.Title
	if file == "" {
		file = body.SourceID
	}
	text := []rune(body.Full())
	return &sourceResolver{
		body:        body,
		text:        text,
		defaultFile: file,
		members:     scanTxtarMembers(text),
	}
}

func (r *sourceResolver) Resolve(c NativeCitation) Resolved {
	out := Resolved{
		SourceID:   c.SourceID,
		StartChar:  c.StartChar,
		EndChar:    c.EndChar,
		Status:     StatusOK,
		Confidence: c.Confidence,
	}
	if c.SourceTitle != "" {
		out.SourceTitle = c.SourceTitle
	} else {
		out.SourceTitle = r.body.Title
	}
	out.File = r.defaultFile

	if len(r.text) == 0 {
		out.Status = StatusOffsetMiss
		out.Reason = "source has no indexed text body"
		return out
	}
	if c.StartChar < 0 || c.EndChar < c.StartChar || c.EndChar > len(r.text) {
		out.Status = StatusOffsetMiss
		out.Reason = fmt.Sprintf("citation [%d,%d) outside indexed source (len=%d)", c.StartChar, c.EndChar, len(r.text))
		return out
	}

	out.Snippet = string(r.text[c.StartChar:c.EndChar])

	if len(r.members) == 0 {
		out.Line, out.Column = lineColAtOffset(r.text, c.StartChar)
		if c.EndChar > c.StartChar {
			out.EndLine, out.EndColumn = lineColAtOffset(r.text, c.EndChar-1)
		}
		return out
	}

	member := r.memberForRange(c.StartChar, c.EndChar)
	if member == nil {
		out.File = r.nearestMemberName(c.StartChar)
		out.Status = StatusHeaderSpan
		out.Reason = fmt.Sprintf("citation [%d,%d) crosses a txtar header boundary", c.StartChar, c.EndChar)
		return out
	}

	out.File = member.Name
	out.Line, out.Column = lineColAtOffset(r.text[member.BodyStart:member.BodyEnd], c.StartChar-member.BodyStart)
	if c.EndChar > c.StartChar {
		out.EndLine, out.EndColumn = lineColAtOffset(r.text[member.BodyStart:member.BodyEnd], c.EndChar-member.BodyStart-1)
	}
	return out
}

func (r *sourceResolver) memberForRange(start, end int) *txtarMember {
	for i := range r.members {
		m := &r.members[i]
		if start < m.BodyStart || start >= m.BodyEnd {
			continue
		}
		if end == start || end <= m.BodyEnd {
			return m
		}
		return nil
	}
	return nil
}

func (r *sourceResolver) nearestMemberName(off int) string {
	if len(r.members) == 0 {
		return r.defaultFile
	}
	for _, m := range r.members {
		if off <= m.HeaderEnd {
			return m.Name
		}
		if off < m.BodyEnd {
			return m.Name
		}
	}
	return r.members[len(r.members)-1].Name
}

func scanTxtarMembers(text []rune) []txtarMember {
	var out []txtarMember
	lineStart := 0
	for i, r := range text {
		if r != '\n' {
			continue
		}
		if name, ok := parseTxtarHeaderLine(string(text[lineStart:i])); ok {
			out = append(out, txtarMember{
				Name:        name,
				HeaderStart: lineStart,
				HeaderEnd:   i + 1,
				BodyStart:   i + 1,
			})
		}
		lineStart = i + 1
	}
	if name, ok := parseTxtarHeaderLine(string(text[lineStart:])); ok {
		out = append(out, txtarMember{
			Name:        name,
			HeaderStart: lineStart,
			HeaderEnd:   len(text),
			BodyStart:   len(text),
		})
	}
	if len(out) == 0 {
		out = scanLooseTxtarMembers(text)
	}
	finishTxtarMembers(out, len(text))
	return out
}

func scanLooseTxtarMembers(text []rune) []txtarMember {
	var out []txtarMember
	for i := 0; i+6 <= len(text); i++ {
		if text[i] != '-' || text[i+1] != '-' || text[i+2] != ' ' {
			continue
		}
		if i > 0 && !isHeaderBoundary(text[i-1]) {
			continue
		}
		end := findHeaderEnd(text, i+3)
		if end < 0 {
			continue
		}
		name := strings.TrimSpace(string(text[i+3 : end]))
		if name == "" {
			continue
		}
		headerEnd := consumeHeaderSeparator(text, end+3)
		out = append(out, txtarMember{
			Name:        name,
			HeaderStart: i,
			HeaderEnd:   headerEnd,
			BodyStart:   headerEnd,
		})
		i = end + 2
	}
	return out
}

func finishTxtarMembers(members []txtarMember, textLen int) {
	for i := range members {
		if i+1 < len(members) {
			members[i].BodyEnd = members[i+1].HeaderStart
			continue
		}
		members[i].BodyEnd = textLen
	}
}

func findHeaderEnd(text []rune, start int) int {
	for i := start; i+2 < len(text); i++ {
		if text[i] == ' ' && text[i+1] == '-' && text[i+2] == '-' {
			return i
		}
	}
	return -1
}

func consumeHeaderSeparator(text []rune, off int) int {
	if off >= len(text) {
		return off
	}
	switch text[off] {
	case '\r':
		off++
		if off < len(text) && text[off] == '\n' {
			off++
		}
	case '\n', ' ':
		off++
	}
	return off
}

func isHeaderBoundary(r rune) bool {
	return r == '\n' || r == '\r' || r == ' ' || r == '\t'
}

func parseTxtarHeaderLine(line string) (string, bool) {
	line = strings.TrimSuffix(line, "\r")
	if !strings.HasPrefix(line, "-- ") || !strings.HasSuffix(line, " --") {
		return "", false
	}
	name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "-- "), " --"))
	if name == "" {
		return "", false
	}
	return name, true
}

func lineColAtOffset(text []rune, off int) (int, int) {
	if off < 0 {
		off = 0
	}
	if off > len(text) {
		off = len(text)
	}
	line := 1
	col := 1
	for _, r := range text[:off] {
		if r == '\n' {
			line++
			col = 1
			continue
		}
		col++
	}
	return line, col
}

func displaySnippet(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) > 200 {
		return string(r[:200]) + "..."
	}
	return s
}

// ResolvedAsCitation converts a resolved native citation into the shared
// formatter shape used by Write.
func ResolvedAsCitation(r Resolved, reviewedRepo string) Citation {
	file := r.File
	match := file
	if reviewedRepo != "" && file != "" && !filepath.IsAbs(file) {
		match = filepath.Join(reviewedRepo, file)
	}
	line := r.Line
	if line <= 0 {
		line = 1
	}
	col := r.Column
	if col <= 0 {
		col = 1
	}
	raw := file
	if raw == "" {
		raw = r.SourceID
	}
	if raw != "" {
		raw = fmt.Sprintf("%s:%d:%d", raw, line, col)
	}
	return Citation{
		Raw:        raw,
		File:       file,
		Line:       line,
		Column:     col,
		EndLine:    r.EndLine,
		EndColumn:  r.EndColumn,
		Match:      match,
		Status:     r.Status,
		Reason:     r.Reason,
		Context:    displaySnippet(r.Snippet),
		Snippet:    r.Snippet,
		SourceID:   r.SourceID,
		StartChar:  r.StartChar,
		EndChar:    r.EndChar,
		Confidence: r.Confidence,
	}
}
