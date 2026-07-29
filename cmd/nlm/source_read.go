package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"unicode"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/auth"
	"github.com/tmc/nlm/internal/notebooklm/api"
	"github.com/tmc/nlm/internal/richrender"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
)

func readSource(c *api.Client, sourceID, notebookID string, opts globalOptions) error {
	if err := normalizeSourceReadFormat(&opts); err != nil {
		return err
	}
	if opts.sourceReadFormat == "raw" || opts.sourceReadFormat == "prototext" {
		response, err := c.LoadSourceProto(context.Background(), sourceID, notebookID)
		if err != nil {
			return err
		}
		if opts.sourceReadFormat == "raw" {
			return writeSourceReadProtoJSON(os.Stdout, response)
		}
		return writeSourceReadProtoText(os.Stdout, response)
	}
	body, err := c.LoadSourceText(context.Background(), sourceID, notebookID)
	if err != nil {
		return err
	}
	if len(body.Fragments) == 0 && opts.sourceReadFormat != "json" {
		return fmt.Errorf("source %s has no indexed text body; use --format=raw to inspect non-text or blob-backed content", sourceID)
	}
	return writeSourceRead(os.Stdout, body, opts, sourceImageFetcherFor(c, opts))
}

func sourceImageFetcherFor(c *api.Client, opts globalOptions) sourceImageFetcher {
	return func(imageURL string) ([]byte, string, error) {
		data, contentType, err := c.DownloadSourceImage(context.Background(), imageURL)
		if err == nil {
			return data, contentType, nil
		}
		// Some lh3 URLs require the full browser profile, beyond the cookie
		// bundle used for RPCs. Fetch only the image bytes needed for this
		// self-contained Markdown export.
		data, browserErr := auth.New(false).DownloadWithBrowser(imageURL, opts.chromeProfile)
		if browserErr != nil {
			return nil, "", fmt.Errorf("direct fetch: %v; browser fetch: %w", err, browserErr)
		}
		contentType = http.DetectContentType(data)
		if !strings.HasPrefix(contentType, "image/") {
			return nil, "", fmt.Errorf("browser fetch returned %q", contentType)
		}
		return data, contentType, nil
	}
}

type sourceImageFetcher func(imageURL string) ([]byte, string, error)

func writeSourceRead(w io.Writer, body api.LoadSourceText, opts globalOptions, fetchImage sourceImageFetcher) error {
	if err := normalizeSourceReadFormat(&opts); err != nil {
		return err
	}
	switch opts.sourceReadFormat {
	case "json":
		return writeSourceReadJSON(w, body)
	case "markdown":
		markdown, err := sourceReadMarkdown(body, fetchImage)
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, markdown)
		return err
	case "html":
		document, err := sourceReadHTML(body, fetchImage)
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, document)
		return err
	case "text":
		_, err := io.WriteString(w, sourceReadText(body))
		return err
	default:
		return fmt.Errorf("source read: cannot render %q from decoded text", opts.sourceReadFormat)
	}
}

func writeSourceReadProtoJSON(w io.Writer, response *pb.LoadSourceResponse) error {
	data, err := (protojson.MarshalOptions{
		UseProtoNames: true,
		Multiline:     true,
		Indent:        "  ",
	}).Marshal(response)
	if err != nil {
		return fmt.Errorf("marshal load source proto: %w", err)
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}

func writeSourceReadProtoText(w io.Writer, response *pb.LoadSourceResponse) error {
	data, err := (prototext.MarshalOptions{
		Multiline: true,
		Indent:    "  ",
	}).Marshal(response)
	if err != nil {
		return fmt.Errorf("marshal load source prototext: %w", err)
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}

type sourceReadJSON struct {
	SourceID  string                   `json:"source_id"`
	Title     string                   `json:"title"`
	Fragments []sourceReadJSONFragment `json:"fragments"`
}

type sourceReadJSONFragment struct {
	Start         int    `json:"start"`
	End           int    `json:"end"`
	Text          string `json:"text,omitempty"`
	ImageURL      string `json:"image_url,omitempty"`
	ImageID       string `json:"image_id,omitempty"`
	ListMarker    string `json:"list_marker,omitempty"`
	Bold          bool   `json:"bold,omitempty"`
	Italic        bool   `json:"italic,omitempty"`
	Code          bool   `json:"code,omitempty"`
	Language      string `json:"language,omitempty"`
	RangeMismatch bool   `json:"range_mismatch,omitempty"`
	BlockStart    bool   `json:"block_start,omitempty"`
}

func writeSourceReadJSON(w io.Writer, body api.LoadSourceText) error {
	emitter := sourceReadJSONEmitter{
		w: w,
		document: sourceReadJSON{
			SourceID:  body.SourceID,
			Title:     body.Title,
			Fragments: make([]sourceReadJSONFragment, 0, len(body.Fragments)),
		},
	}
	return renderSourceRead(body, &emitter)
}

type sourceReadJSONEmitter struct {
	w        io.Writer
	document sourceReadJSON
}

func (e *sourceReadJSONEmitter) EmitContent(fragment richrender.ContentFragment) error {
	e.document.Fragments = append(e.document.Fragments, sourceReadJSONFragment{
		Start:         fragment.Start,
		End:           fragment.End,
		Text:          fragment.Text,
		ImageURL:      fragment.ImageURL,
		ImageID:       fragment.ImageID,
		ListMarker:    fragment.ListMarker,
		Bold:          fragment.Bold,
		Italic:        fragment.Italic,
		Code:          fragment.Code,
		Language:      fragment.Language,
		RangeMismatch: fragment.RangeMismatch,
		BlockStart:    fragment.BlockStart,
	})
	return nil
}

func (e *sourceReadJSONEmitter) FinishContent() error {
	return json.NewEncoder(e.w).Encode(e.document)
}

type sourceReadContent struct {
	body api.LoadSourceText
}

func (c sourceReadContent) ContentLen() int {
	return len(c.body.Fragments)
}

func (c sourceReadContent) ContentFragment(i int) richrender.ContentFragment {
	f := c.body.Fragments[i]
	fragment := richrender.ContentFragment{
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
	if f.IsImage() {
		fragment.ImageAlt = sourceReadImageAlt(c.body.Fragments, i)
	} else if name, ok := sourceReadTxtarHeader(f.Text); ok {
		fragment.MemberName = name
	}
	return fragment
}

func renderSourceRead(body api.LoadSourceText, emitter richrender.ContentEmitter) error {
	return richrender.RenderContent(sourceReadContent{body: body}, emitter)
}

// sourceReadText reconstructs the default plain-text reading view from the
// server's ordered fragments. Full pads the server's untyped offset gaps with
// one space per missing offset to stay faithful to the citation coordinate
// space; that fidelity turns a region the NotebookLM index dropped into a long
// run of blanks, so a missing paragraph reads as whitespace and a dropped
// function body reads as indentation. The default view is for humans, not
// offset lookups, so it collapses each gap into reading flow the same way the
// Markdown and HTML views do via writePresentationGap. Contiguous sources are
// unaffected: with no gaps this equals Full.
func sourceReadText(body api.LoadSourceText) string {
	emitter := sourceReadTextEmitter{cursor: firstFragmentOffset(body.Fragments)}
	_ = renderSourceRead(body, &emitter)
	return emitter.out.String()
}

type sourceReadTextEmitter struct {
	out    strings.Builder
	cursor int
}

func (e *sourceReadTextEmitter) EmitContent(fragment richrender.ContentFragment) error {
	switch fragment.Kind {
	case richrender.ContentImage:
		return nil
	case richrender.ContentMember:
		writePresentationBreak(&e.out)
		e.out.WriteString(fragment.Text)
		writePresentationBreak(&e.out)
	default:
		writePresentationGap(&e.out, e.cursor, fragment.Start)
		e.out.WriteString(fragment.Text)
	}
	e.cursor = fragment.End
	return nil
}

func (*sourceReadTextEmitter) FinishContent() error {
	return nil
}

func sourceReadMarkdown(body api.LoadSourceText, fetchImage sourceImageFetcher) (string, error) {
	emitter := sourceReadMarkdownEmitter{
		cursor:     firstFragmentOffset(body.Fragments),
		fetchImage: fetchImage,
	}
	if err := renderSourceRead(body, &emitter); err != nil {
		return "", err
	}
	return emitter.out.String(), nil
}

type sourceReadMarkdownEmitter struct {
	out        strings.Builder
	cursor     int
	inList     bool
	fetchImage sourceImageFetcher
}

func (e *sourceReadMarkdownEmitter) EmitContent(fragment richrender.ContentFragment) error {
	switch fragment.Kind {
	case richrender.ContentMember:
		e.inList = false
		writePresentationBreak(&e.out)
		e.out.WriteString(fragment.Text)
		writePresentationBreak(&e.out)
		e.cursor = fragment.End
		return nil
	case richrender.ContentCode:
		e.inList = false
		writePresentationBreak(&e.out)
		fence := markdownCodeFence(fragment.Text)
		e.out.WriteString(fence)
		e.out.WriteString(markdownCodeLanguage(fragment.Language))
		e.out.WriteByte('\n')
		e.out.WriteString(fragment.Text)
		if !strings.HasSuffix(fragment.Text, "\n") {
			e.out.WriteByte('\n')
		}
		e.out.WriteString(fence)
		writePresentationBreak(&e.out)
		e.cursor = fragment.End
		return nil
	}
	if fragment.BlockStart && fragment.ListMarker == "" && e.out.Len() > 0 && !strings.HasSuffix(e.out.String(), "\n\n") {
		e.out.WriteString("\n\n")
		e.cursor = fragment.Start
	}
	// The indexed HTML/table payload represents each Markdown table row as
	// a separate fragment beginning with '|'. Its one-character offset gap
	// is a space, not a newline, so preserve the row boundary only in the
	// presentation-oriented Markdown view. Full remains offset-faithful.
	if isMarkdownTableRow(fragment.Text) {
		e.inList = false
		if e.out.Len() > 0 && !strings.HasSuffix(e.out.String(), "\n") {
			e.out.WriteByte('\n')
		}
		if strings.HasSuffix(e.out.String(), "\n") {
			e.cursor = fragment.Start
		}
	}
	if fragment.ListMarker != "" {
		if e.out.Len() > 0 && !strings.HasSuffix(e.out.String(), "\n") {
			e.out.WriteString("\n\n")
		}
		if !e.inList && e.out.Len() > 0 && !strings.HasSuffix(e.out.String(), "\n\n") {
			e.out.WriteByte('\n')
		}
		e.out.WriteString(markdownListMarker(fragment.ListMarker))
		e.out.WriteByte(' ')
		e.out.WriteString(sourceReadMarkdownText(fragment))
		e.out.WriteByte('\n')
		e.cursor = fragment.End
		e.inList = true
		return nil
	}
	e.inList = false
	writePresentationGap(&e.out, e.cursor, fragment.Start)
	if fragment.Kind == richrender.ContentImage {
		image, err := sourceReadImageDataURI(fragment, e.fetchImage)
		if err != nil {
			return err
		}
		fmt.Fprintf(&e.out, "![%s](%s)", fragment.ImageAlt, image)
	} else {
		e.out.WriteString(sourceReadMarkdownText(fragment))
	}
	e.cursor = fragment.End
	return nil
}

func (*sourceReadMarkdownEmitter) FinishContent() error {
	return nil
}

func sourceReadImageAlt(fragments []api.TextFragment, imageIndex int) string {
	if imageIndex+1 >= len(fragments) {
		return ""
	}
	next := fragments[imageIndex+1]
	if next.IsImage() {
		return ""
	}
	text := strings.TrimSpace(next.Text)
	if !strings.HasPrefix(text, "Figure ") && !strings.HasPrefix(text, "Table ") && !strings.HasPrefix(text, "Chart ") {
		return ""
	}
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	const maxAlt = 240
	if len(text) > maxAlt {
		text = text[:maxAlt-1] + "…"
	}
	return text
}

func sourceReadMarkdownText(f richrender.ContentFragment) string {
	text := normalizeMathNoise(f.Text)
	if text != f.Text {
		return text
	}
	return wrapMarkdownText(text, f.Bold, f.Italic)
}

func wrapMarkdownText(text string, bold, italic bool) string {
	if (!bold && !italic) || text == "" {
		return text
	}
	left := len(text) - len(strings.TrimLeft(text, " \t\n"))
	right := len(text) - len(strings.TrimRight(text, " \t\n"))
	if left+right >= len(text) {
		return text
	}
	prefix := text[:left]
	core := text[left : len(text)-right]
	suffix := text[len(text)-right:]
	marker := "_"
	if bold {
		marker = "**"
	}
	if bold && italic {
		marker = "***"
	}
	return prefix + marker + core + marker + suffix
}

func markdownListMarker(marker string) string {
	if marker == "•" || marker == "◦" || marker == "▪" {
		return "-"
	}
	if strings.HasSuffix(marker, ".") {
		return marker
	}
	return "-"
}

// writePresentationGap turns the server's otherwise-untyped offset gaps into
// reading-flow whitespace. A one-character gap remains a space; a larger gap
// is a block boundary. The offset-faithful Full method intentionally does
// not use this heuristic.
func writePresentationGap(b *strings.Builder, start, end int) {
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

func writePresentationBreak(b *strings.Builder) {
	if b.Len() == 0 || strings.HasSuffix(b.String(), "\n\n") {
		return
	}
	if strings.HasSuffix(b.String(), "\n") {
		b.WriteByte('\n')
		return
	}
	b.WriteString("\n\n")
}

func markdownCodeFence(code string) string {
	if strings.Contains(code, "```") {
		return "````"
	}
	return "```"
}

func markdownCodeLanguage(language string) string {
	language = strings.TrimSpace(language)
	if language == "" {
		return ""
	}
	for _, r := range language {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		switch r {
		case '+', '-', '_', '.', '#':
			continue
		}
		return ""
	}
	return language
}

func sourceReadTxtarHeader(text string) (string, bool) {
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

func sourceReadImageDataURI(f richrender.ContentFragment, fetchImage sourceImageFetcher) (string, error) {
	if fetchImage == nil {
		return "", fmt.Errorf("fetch source image %s: no image fetcher", f.ImageID)
	}
	data, contentType, err := fetchImage(f.ImageURL)
	if err != nil {
		return "", fmt.Errorf("fetch source image %s: %w", f.ImageID, err)
	}
	if !strings.HasPrefix(contentType, "image/") {
		return "", fmt.Errorf("fetch source image %s: got %q", f.ImageID, contentType)
	}
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// sourceReadHTML writes a responsive reading view from the server's ordered
// fragments. hizoJc does not expose page coordinates, so this deliberately
// reconstructs document flow rather than trying to synthesize pixel layout.
func sourceReadHTML(body api.LoadSourceText, fetchImage sourceImageFetcher) (string, error) {
	emitter := sourceReadHTMLEmitter{
		cursor:     firstFragmentOffset(body.Fragments),
		fetchImage: fetchImage,
	}
	emitter.out.WriteString("<!doctype html>\n<html><head><meta charset=utf-8><meta name=viewport content=\"width=device-width, initial-scale=1\"><title>")
	emitter.out.WriteString(html.EscapeString(body.Title))
	emitter.out.WriteString("</title><script>window.MathJax={tex:{inlineMath:[[\"$\",\"$\"],[\"\\\\(\",\"\\\\)\"]],displayMath:[[\"$$\",\"$$\"],[\"\\\\[\",\"\\\\]\"]]}};</script><script defer src=\"https://cdn.jsdelivr.net/npm/mathjax@3/es5/tex-mml-chtml.js\"></script><style>body{margin:0;background:#f6f7f8;color:#1f2328;font:18px/1.55 system-ui,sans-serif}main{max-width:52rem;margin:auto;padding:2rem;background:#fff;min-height:100vh}h1{line-height:1.2}p{margin:1em 0}img{display:block;max-width:100%;height:auto;margin:1.5em auto}table{border-collapse:collapse;display:block;max-width:100%;overflow:auto;margin:1.5em 0}th,td{border:1px solid #d0d7de;padding:.35em .6em;text-align:left}th{background:#f6f8fa}code{white-space:pre-wrap}pre{overflow:auto;padding:1rem;background:#f6f8fa;border-radius:.4rem}pre code{white-space:pre}.txtar-member{font-size:1rem}</style></head><body><main>")
	if body.Title != "" {
		emitter.out.WriteString("<h1>")
		emitter.out.WriteString(html.EscapeString(body.Title))
		emitter.out.WriteString("</h1>")
	}
	if err := renderSourceRead(body, &emitter); err != nil {
		return "", err
	}
	return emitter.out.String(), nil
}

type sourceReadHTMLEmitter struct {
	out        strings.Builder
	prose      strings.Builder
	rows       []string
	listItems  []string
	cursor     int
	fetchImage sourceImageFetcher
}

func (e *sourceReadHTMLEmitter) flushProse() {
	if e.prose.Len() == 0 {
		return
	}
	writeHTMLProse(&e.out, e.prose.String())
	e.prose.Reset()
}

func (e *sourceReadHTMLEmitter) flushTable() {
	if len(e.rows) == 0 {
		return
	}
	writeHTMLTable(&e.out, e.rows)
	e.rows = nil
}

func (e *sourceReadHTMLEmitter) flushList() {
	if len(e.listItems) == 0 {
		return
	}
	e.out.WriteString("<ul>")
	for _, item := range e.listItems {
		e.out.WriteString("<li>")
		e.out.WriteString(item)
		e.out.WriteString("</li>")
	}
	e.out.WriteString("</ul>")
	e.listItems = nil
}

func (e *sourceReadHTMLEmitter) EmitContent(fragment richrender.ContentFragment) error {
	if fragment.BlockStart && fragment.ListMarker == "" {
		e.flushTable()
		e.flushList()
		if e.prose.Len() > 0 && !strings.HasSuffix(e.prose.String(), "\n\n") {
			e.prose.WriteString("\n\n")
		}
		e.cursor = fragment.Start
	}
	switch fragment.Kind {
	case richrender.ContentImage:
		e.flushProse()
		e.flushTable()
		e.flushList()
		image, err := sourceReadImageDataURI(fragment, e.fetchImage)
		if err != nil {
			return err
		}
		e.out.WriteString("<img alt=\"")
		e.out.WriteString(html.EscapeString(fragment.ImageAlt))
		e.out.WriteString("\" src=\"")
		e.out.WriteString(html.EscapeString(image))
		e.out.WriteString("\">")
	case richrender.ContentMember:
		e.flushProse()
		e.flushTable()
		e.flushList()
		e.out.WriteString("<hr><h2 class=\"txtar-member\"><code>")
		e.out.WriteString(html.EscapeString(fragment.MemberName))
		e.out.WriteString("</code></h2>")
	case richrender.ContentCode:
		e.flushProse()
		e.flushTable()
		e.flushList()
		e.out.WriteString("<pre><code")
		if language := markdownCodeLanguage(fragment.Language); language != "" {
			e.out.WriteString(" class=\"language-")
			e.out.WriteString(html.EscapeString(language))
			e.out.WriteString("\"")
		}
		e.out.WriteString(">")
		e.out.WriteString(html.EscapeString(fragment.Text))
		e.out.WriteString("</code></pre>")
	default:
		text := normalizeMathNoise(fragment.Text)
		if isMarkdownTableRow(text) {
			e.flushProse()
			e.flushList()
			e.rows = append(e.rows, text)
			e.cursor = fragment.End
			return nil
		}
		e.flushTable()
		if fragment.ListMarker != "" {
			e.flushProse()
			e.listItems = append(e.listItems, sourceReadHTMLText(text, fragment))
			e.cursor = fragment.End
			return nil
		}
		e.flushList()
		writePresentationGap(&e.prose, e.cursor, fragment.Start)
		e.prose.WriteString(sourceReadHTMLText(text, fragment))
	}
	e.cursor = fragment.End
	return nil
}

func (e *sourceReadHTMLEmitter) FinishContent() error {
	e.flushProse()
	e.flushTable()
	e.flushList()
	e.out.WriteString("</main></body></html>\n")
	return nil
}

func firstFragmentOffset(fragments []api.TextFragment) int {
	if len(fragments) == 0 {
		return 0
	}
	return fragments[0].Start
}

func writeHTMLProse(out *strings.Builder, text string) {
	for _, paragraph := range strings.Split(text, "\n\n") {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		out.WriteString("<p>")
		out.WriteString(strings.ReplaceAll(paragraph, "\n", "<br>\n"))
		out.WriteString("</p>")
	}
}

func sourceReadHTMLText(text string, f richrender.ContentFragment) string {
	text = html.EscapeString(text)
	if f.Bold {
		text = "<strong>" + text + "</strong>"
	}
	if f.Italic {
		text = "<em>" + text + "</em>"
	}
	return text
}

func writeHTMLTable(out *strings.Builder, rows []string) {
	out.WriteString("<table>")
	for i, row := range rows {
		if i == 1 && isMarkdownTableDivider(row) {
			continue
		}
		cell := "td"
		if i == 0 {
			cell = "th"
		}
		out.WriteString("<tr>")
		for _, value := range markdownTableCells(row) {
			out.WriteString("<")
			out.WriteString(cell)
			out.WriteString(">")
			out.WriteString(html.EscapeString(value))
			out.WriteString("</")
			out.WriteString(cell)
			out.WriteString(">")
		}
		out.WriteString("</tr>")
	}
	out.WriteString("</table>")
}

func markdownTableCells(row string) []string {
	row = strings.TrimSpace(row)
	row = strings.TrimPrefix(row, "|")
	row = strings.TrimSuffix(row, "|")
	parts := strings.Split(row, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func isMarkdownTableDivider(row string) bool {
	for _, cell := range markdownTableCells(row) {
		cell = strings.Trim(cell, ":-")
		if cell != "" {
			return false
		}
	}
	return true
}

var mathNoise = regexp.MustCompile(`([[:alpha:]\x{1D400}-\x{1D7FF}]) (subscript|superscript) ([[:alnum:]+-]+)`)

// normalizeMathNoise repairs the only unambiguous flattened form observed in
// HTML-derived sources: a math glyph followed by "subscript" or
// "superscript" and one token. It intentionally leaves all other text alone.
func normalizeMathNoise(text string) string {
	if !strings.Contains(text, "subscript") && !strings.Contains(text, "superscript") {
		return text
	}
	if !containsMathRune(text) {
		return text
	}
	clean := mathNoise.ReplaceAllStringFunc(text, func(match string) string {
		parts := strings.Fields(match)
		if len(parts) != 3 {
			return match
		}
		if parts[1] == "subscript" {
			return parts[0] + "_{" + parts[2] + "}"
		}
		return parts[0] + "^{" + parts[2] + "}"
	})
	if clean == text {
		return text
	}
	if strings.Contains(clean, "\n") {
		return "$$\n" + clean + "\n$$"
	}
	return "$" + clean + "$"
}

func containsMathRune(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Sm, r) || (r >= 0x1D400 && r <= 0x1D7FF) {
			return true
		}
	}
	return false
}

func isMarkdownTableRow(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "|")
}
