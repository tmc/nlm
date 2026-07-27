package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

var (
	noteMathRE   = regexp.MustCompile(`\$\$[^$]+\$\$|\$[^$\n]+\$`)
	noteInlineRE = regexp.MustCompile(`\$\$[^$]+\$\$|\$[^$\n]+\$|\[[0-9][0-9,\s-]*\]`)

	noteMathBlockPattern = regexp.MustCompile(`(?s)\$\$.*?\$\$|\\\[.*?\\\]`)
	// Inline $...$ follows Pandoc's rule to avoid currency false positives:
	// the opening $ needs a non-space character to its right, the closing $
	// a non-space character to its left, and the closing $ must not be
	// immediately followed by a digit ("$3 billion ... for $1000" is prose).
	noteMathInlinePattern = regexp.MustCompile(`(^|[^\\$])\$[^\s$](?:[^$]*[^\s$])?\$([^0-9]|$)|\\\((.+?)\\\)`)
	noteCodeRegionPattern = regexp.MustCompile(`(?s)<pre.*?</pre>|<code.*?</code>`)
)

func renderNoteHTMLToDestination(doc noteDocument, opts noteReadOptions) error {
	if opts.OutFile == "" {
		return renderNoteHTML(os.Stdout, doc)
	}
	var buf bytes.Buffer
	if err := renderNoteHTML(&buf, doc); err != nil {
		return err
	}
	if err := os.WriteFile(opts.OutFile, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write html: %w", err)
	}
	fmt.Fprintf(os.Stderr, "nlm: wrote %s\n", opts.OutFile)
	if opts.Open {
		if err := openInBrowser(opts.OutFile); err != nil {
			fmt.Fprintf(os.Stderr, "nlm: could not open browser: %v\n", err)
		}
	}
	return nil
}

func renderNoteHTML(out io.Writer, doc noteDocument) error {
	markers := noteCitationMarkers(doc)
	blob, err := json.Marshal(markers)
	if err != nil {
		return fmt.Errorf("encode note citation data: %w", err)
	}
	nodes := richNoteNodes(doc, markers)
	var body strings.Builder
	for _, node := range nodes {
		if err := renderAnswerNode(&body, node); err != nil {
			return err
		}
	}
	data := struct {
		Title     string
		Body      template.HTML
		Blob      template.JS
		Citations []htmlMarker
		HasMath   bool
	}{
		Title:     doc.Title,
		Body:      template.HTML(body.String()),
		Blob:      template.JS(blob),
		Citations: markers,
		HasMath:   notePageHasMath(body.String()),
	}
	if err := noteHTMLTemplate.Execute(out, data); err != nil {
		return fmt.Errorf("render note html: %w", err)
	}
	return nil
}

// notePageHasMath reports whether rendered HTML contains MathJax-style TeX
// delimiters outside <pre> and <code> regions, which MathJax skips.
func notePageHasMath(htmlContent string) bool {
	return noteHasMath(noteCodeRegionPattern.ReplaceAllString(htmlContent, ""))
}

func noteHasMath(text string) bool {
	if noteMathBlockPattern.MatchString(text) {
		return true
	}
	for _, line := range strings.Split(text, "\n") {
		if noteMathInlinePattern.MatchString(line) {
			return true
		}
	}
	return false
}

func noteCitationMarkers(doc noteDocument) []htmlMarker {
	citations := append([]api.Citation(nil), doc.Citations...)
	for i := range citations {
		citations[i].SourceID = citationSourceID(citations[i])
	}
	length := utf16Len(doc.Flat)
	ranges := markerTokenRangesUTF16(doc.Flat)
	if doc.Flat == "" {
		for _, citation := range citations {
			length = max(length, citation.EndChar)
		}
		for _, block := range projectRichDocument(doc.Rich) {
			length = max(length, block.End)
		}
		ranges = nil
	}
	markers := buildCitationMarkers(citations, chatRenderContext{}, htmlExcerptBudget, length, ranges)
	byIndex := markersByIndex(markers)
	for _, index := range markerIndicesFromText(doc.Flat) {
		if _, ok := byIndex[index]; !ok {
			markers = append(markers, htmlMarker{Index: index})
			byIndex[index] = markers[len(markers)-1]
		}
	}
	return markers
}

func richNoteNodes(doc noteDocument, markers []htmlMarker) []answerNode {
	byIndex := markersByIndex(markers)
	if doc.Rich == nil {
		nodes := markdownSubsetNodes(doc.Flat, byIndex)
		nodes = groundMarkdownNodes(nodes, doc.Flat, translateMarkerSpans(markers, newUTF16RuneMap(doc.Flat)), 0)
		return liftSplitMathCitations(nodes, 0, byIndex)
	}
	if nodes := richMarkdownOverlayNodes(projectRichDocument(doc.Rich), doc.Flat, byIndex); len(nodes) > 0 {
		nodes = groundMarkdownNodes(nodes, doc.Flat, translateMarkerSpans(markers, newUTF16RuneMap(doc.Flat)), 0)
		return liftSplitMathCitations(nodes, 0, byIndex)
	}
	occurrences := noteMarkerOccurrences(doc.Citations)
	var nodes []answerNode
	for _, block := range projectRichDocument(doc.Rich) {
		switch block.Kind {
		case blockSeparator:
			nodes = append(nodes, answerNode{Tag: "hr"})
		case blockHidden:
			continue
		case blockList:
			var items []answerNode
			for _, item := range block.Items {
				items = append(items, answerNode{
					Tag:      "li",
					Class:    nestClass(item.Nesting),
					Children: noteRunNodes(item.Runs, markers, byIndex, occurrences),
				})
			}
			if len(items) > 0 {
				nodes = append(nodes, answerNode{Tag: "ul", Children: items})
			}
		case blockCodeBlock:
			nodes = append(nodes, answerNode{Tag: "pre", Children: []answerNode{{
				Tag: "code", Text: runsText(block.Runs),
			}}})
		default:
			children := noteRunNodes(block.Runs, markers, byIndex, occurrences)
			if len(children) == 0 {
				continue
			}
			tag := "p"
			if block.Kind == blockParagraph && anyHeadingRun(block.Runs) {
				tag = "h4"
			}
			nodes = append(nodes, answerNode{Tag: tag, Children: children})
		}
	}
	return liftSplitMathCitations(nodes, 0, byIndex)
}

func noteMarkerOccurrences(citations []api.Citation) []noteMarkerOccurrence {
	var out []noteMarkerOccurrence
	seen := make(map[string]bool)
	for _, citation := range citations {
		key := fmt.Sprintf("%d:%d:%d", citation.SourceIndex, citation.StartChar, citation.EndChar)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, noteMarkerOccurrence{
			Index: citation.SourceIndex,
			Start: citation.StartChar,
			End:   citation.EndChar,
		})
	}
	return out
}

func noteRunNodes(runs []richRun, markers []htmlMarker, byIndex map[int]htmlMarker, occurrences []noteMarkerOccurrence) []answerNode {
	var out []answerNode
	for _, run := range runs {
		var nodes []answerNode
		if run.Code {
			nodes = []answerNode{{Tag: "code", Text: run.Text}}
		} else {
			nodes = noteRunTextNodes(run, markers, byIndex, occurrences)
		}
		if run.Link != "" && safeNoteLink(run.Link) {
			nodes = []answerNode{{Tag: "a", Href: run.Link, Children: nodes}}
		}
		if run.Emphasis && !run.Code {
			nodes = []answerNode{{Tag: "em", Children: nodes}}
		}
		out = append(out, nodes...)
	}
	return out
}

func noteRunTextNodes(run richRun, markers []htmlMarker, byIndex map[int]htmlMarker, occurrences []noteMarkerOccurrence) []answerNode {
	type placement struct {
		at, wire int
		index    int
	}
	var placements []placement
	seen := make(map[string]bool)
	u16 := newUTF16RuneMap(run.Text)
	for _, occurrence := range occurrences {
		if occurrence.End <= run.Start || occurrence.End > run.End {
			continue
		}
		at := u16.rune(occurrence.End - run.Start)
		key := fmt.Sprintf("%d:%d", at, occurrence.Index)
		if seen[key] {
			continue
		}
		seen[key] = true
		placements = append(placements, placement{at: at, wire: occurrence.End, index: occurrence.Index})
	}
	sort.SliceStable(placements, func(i, j int) bool { return placements[i].at < placements[j].at })

	runes := []rune(run.Text)
	var out []answerNode
	at := 0
	wireAt := run.Start
	for _, placement := range placements {
		if placement.at < at || placement.at > len(runes) {
			continue
		}
		out = append(out, noteGroundedTextNodes(string(runes[at:placement.at]), wireAt, markers, byIndex)...)
		out = append(out, markerNodes(0, strconv.Itoa(placement.index), byIndex)...)
		at = placement.at
		wireAt = placement.wire
	}
	out = append(out, noteGroundedTextNodes(string(runes[at:]), wireAt, markers, byIndex)...)
	return out
}

func noteGroundedTextNodes(text string, wireStart int, markers []htmlMarker, byIndex map[int]htmlMarker) []answerNode {
	wireEnd := wireStart + utf16Len(text)
	u16 := newUTF16RuneMap(text)
	local := make([]htmlMarker, len(markers))
	copy(local, markers)
	for i := range local {
		var spans []htmlSpan
		for _, span := range markers[i].Spans {
			start, end := max(span.Start, wireStart), min(span.End, wireEnd)
			if start < end {
				spans = append(spans, htmlSpan{
					Start: u16.rune(start - wireStart),
					End:   u16.rune(end - wireStart),
				})
			}
		}
		local[i].Spans = spans
		local[i].Span = nil
		if len(spans) > 0 {
			local[i].Span = &local[i].Spans[0]
		}
	}
	return groundMarkdownNodes(noteTextNodes(text, byIndex), text, local, 0)
}

func noteTextNodes(text string, byIndex map[int]htmlMarker) []answerNode {
	var out []answerNode
	at := 0
	for _, match := range noteInlineRE.FindAllStringIndex(text, -1) {
		if match[0] > at {
			out = append(out, answerNode{Text: text[at:match[0]]})
		}
		token := text[match[0]:match[1]]
		if strings.HasPrefix(token, "$") {
			out = append(out, noteMathCitationNodes(token, 0, byIndex)...)
		} else {
			out = append(out, markerNodes(0, token[1:len(token)-1], byIndex)...)
		}
		at = match[1]
	}
	if at < len(text) {
		out = append(out, answerNode{Text: text[at:]})
	}
	return out
}

func noteMathNode(text string) answerNode {
	if strings.HasPrefix(text, "$$") {
		return answerNode{Tag: "span", Class: "math-display", Text: text}
	}
	return answerNode{Text: text}
}

func noteMathCitationNodes(text string, msgIdx int, byIndex map[int]htmlMarker) []answerNode {
	lift, ok := liftTrailingMathCitation(text)
	if !ok {
		return []answerNode{noteMathNode(text)}
	}
	return liftedMathCitationNodes(noteMathNode(lift.math), lift, msgIdx, byIndex)
}

var noteHTMLTemplate = template.Must(template.New("note").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}}</title>
<style>
*{box-sizing:border-box}html{max-width:100%;overflow-x:hidden;-webkit-text-size-adjust:100%}
body{font:16px/1.55 system-ui,sans-serif;max-width:76rem;margin:3rem auto;padding:0 1.25rem;color:#202124;overflow-x:hidden}
h1{font-size:2rem}h4{font-size:1.15rem;margin:1.6rem 0 .5rem}p{margin:.7rem 0}
li{margin:.3rem 0}.nest-1{margin-left:1.5rem}.nest-2{margin-left:3rem}.nest-3{margin-left:4.5rem}
code{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;background:#f1f3f4;padding:.1rem .25rem;border-radius:.2rem}
article{min-width:0;overflow-wrap:anywhere}pre,table{display:block;max-width:100%;overflow-x:auto}pre code{display:block;overflow:auto;padding:1rem}.math-display{display:block;white-space:pre-wrap}
.math-display-row{display:grid;grid-template-columns:minmax(0,1fr) auto minmax(0,1fr);align-items:center;width:100%;max-width:100%;margin:.7rem 0}
.math-display-row::before{content:"";grid-column:1}.math-display-equation{grid-column:2;min-width:0;max-width:100%;overflow-x:auto;text-align:center}
.math-display-cite{grid-column:3;justify-self:end;padding-left:.6rem}
.note-grid{display:grid;grid-template-columns:minmax(0,52rem) minmax(14rem,20rem);gap:2rem;align-items:start}
.citations{border-top:1px solid #dadce0;margin-top:2rem;padding-top:1rem}.citation{margin:.8rem 0;scroll-margin-top:1rem}
.citation.flash{background:#e8f0fe;border-radius:.4rem}
.source{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:.85rem;color:#5f6368}
.excerpt{white-space:pre-wrap;margin:.25rem 0}
.citegroup{white-space:nowrap;font-size:.72em;line-height:0;vertical-align:super;margin-left:.08em}
.citelink{color:#174ea6;font-weight:600;text-decoration:underline dotted;text-underline-offset:1px;text-decoration-thickness:1px}
.grounded{scroll-margin-top:1rem}.grounded.flash{background:#e8f0fe}
.note-card{position:absolute;z-index:40;display:none;width:min(26rem,90vw);padding:.25rem;background:#fff;border:1px solid #bdc1c6;border-radius:.65rem;box-shadow:0 8px 28px rgba(32,33,36,.18);font-size:.875rem}
.note-card.show{display:block}.note-card-close{position:sticky;top:.25rem;float:right;width:2.25rem;height:2.25rem;margin:.15rem .15rem 0 .5rem;border:1px solid #bdc1c6;border-radius:999px;background:#fff;color:#5f6368;cursor:pointer;font:600 1.25rem/1 system-ui,sans-serif}
.note-card-source{padding:.7rem}.note-card-source+.note-card-source{border-top:1px solid #dadce0}.note-card-head{display:flex;align-items:baseline;gap:.5rem;flex-wrap:wrap}.note-card-title{font-weight:650}.note-card-handle,.note-card-location{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:.75rem;color:#5f6368}.note-card-location{margin-left:auto}.note-card-excerpt{white-space:pre-wrap;overflow-wrap:anywhere;margin-top:.4rem;padding:.55rem;background:#f8f9fa;border:1px solid #e8eaed;border-radius:.35rem;max-height:15rem;overflow-y:auto}
.note-rail{position:sticky;top:1rem;max-height:calc(100vh - 2rem);overflow:auto}
.note-rail h2{font-size:.75rem;letter-spacing:.08em;text-transform:uppercase;color:#5f6368}
.note-ref{border:1px solid #dadce0;border-radius:.5rem;padding:.65rem;margin:.6rem 0;background:#fff}
.note-ref-head{display:flex;gap:.5rem;align-items:baseline}.note-ref-marker{font-weight:700;color:#174ea6}
.note-ref-sources{min-width:0}
.note-ref-source{font-size:.85rem;overflow-wrap:anywhere}
.note-ref-actions{display:flex;gap:.45rem;margin-top:.55rem}
.note-action{border:1px solid #bdc1c6;border-radius:.35rem;background:#fff;color:#174ea6;cursor:pointer;font:600 .75rem/1.2 system-ui,sans-serif;padding:.4rem .55rem;text-decoration:none}
.note-action:hover,.note-action:focus-visible{border-color:#174ea6;background:#e8f0fe}
.note-action:disabled{color:#9aa0a6;cursor:default;background:#f8f9fa}
@media(max-width:860px){body{max-width:52rem;margin:2rem auto}.note-grid{grid-template-columns:minmax(0,1fr);gap:1.5rem}.note-rail{position:static;max-height:none;padding-top:1rem;border-top:1px solid #dadce0}}
@media(max-width:520px){.math-display-row{grid-template-columns:minmax(0,1fr) auto}.math-display-row::before{display:none}.math-display-equation{grid-column:1}.math-display-cite{grid-column:2;padding-left:.35rem}}
@media(max-width:520px){body{margin:0 auto;padding:1rem .75rem 3rem}h1{font-size:1.55rem;line-height:1.25}.citation{overflow-wrap:anywhere}.note-action{min-height:44px;display:inline-flex;align-items:center}.note-card{position:fixed;left:10px!important;right:10px;top:auto!important;bottom:max(10px,env(safe-area-inset-bottom));width:auto;max-height:min(72vh,34rem);overflow-y:auto}.note-card-close{width:44px;height:44px}}
@media(hover:none),(pointer:coarse){.citelink,.grounded{position:relative}.citelink::after,.grounded::after{content:"";position:absolute;z-index:1;left:-8px;right:-8px;top:50%;height:44px;transform:translateY(-50%)}}
</style>
</head>
<body>
<main>
<h1>{{.Title}}</h1>
<div class="note-grid"><div>
<article id="note-body">{{.Body}}</article>
{{if .Citations}}<section class="citations"><h2>Citations</h2>
{{range .Citations}}<div class="citation" id="cite-0-{{.Index}}"><strong>[{{.Index}}]</strong>
{{range .Sources}}<div><span class="source">{{.SourceID}}</span>
{{if .Confidence}} <span>p={{printf "%.2f" .Confidence}}</span>{{end}}
{{if .Excerpt}}<div class="excerpt">{{if .ExcerptRuns}}{{range .ExcerptRuns}}{{if .Link}}<a href="{{.Link}}" target="_blank" rel="noopener noreferrer">{{if .Code}}<code>{{.Text}}</code>{{else}}{{.Text}}{{end}}</a>{{else if .Code}}<code>{{.Text}}</code>{{else}}{{.Text}}{{end}}{{end}}{{else}}{{.Excerpt}}{{end}}</div>{{end}}</div>{{end}}
</div>{{end}}</section>{{end}}
</div>
{{if .Citations}}<aside class="note-rail" aria-label="Citation sources"><h2>Sources</h2>
{{range .Citations}}<div class="note-ref" data-cite="{{.Index}}">
<div class="note-ref-head"><span class="note-ref-marker">[{{.Index}}]</span>
<div class="note-ref-sources">{{range .Sources}}<div class="note-ref-source">{{if .Title}}{{.Title}}{{else}}{{.Handle}}{{end}}</div>{{end}}</div></div>
<div class="note-ref-actions">
<a class="note-action note-detail" href="#cite-0-{{.Index}}" data-cite="{{.Index}}">Details</a>
<button class="note-action note-passage" type="button" data-cite="{{.Index}}">Passage</button>
</div></div>{{end}}</aside>{{end}}
</div>
</main>
<div class="note-card" id="note-card" role="dialog" aria-modal="false" aria-label="Citation sources" tabindex="-1"></div>
<script id="note-data" type="application/json">{{.Blob}}</script>
<script>
(function () {
  "use strict";
  var markers;
  try {
    markers = JSON.parse(document.getElementById("note-data").textContent);
  } catch (error) {
    markers = [];
  }
  var byIndex = {};
  markers.forEach(function (marker) { byIndex[marker.index] = marker; });
  var card = document.getElementById("note-card");
  var touchQuery = window.matchMedia("(hover: none), (pointer: coarse)");
  var hideTimer = null;
  var pinnedAnchor = null;

  function element(tag, className, text) {
    var node = document.createElement(tag);
    if (className) node.className = className;
    if (text != null) node.textContent = text;
    return node;
  }
  function appendExcerpt(parent, source) {
    if (!source.excerptRuns || !source.excerptRuns.length) {
      parent.textContent = source.excerpt || "";
      return;
    }
    source.excerptRuns.forEach(function (run) {
      var content = run.code ? element("code", "", run.text) : document.createTextNode(run.text);
      if (!run.link) {
        parent.appendChild(content);
        return;
      }
      var anchor = document.createElement("a");
      anchor.href = run.link;
      anchor.target = "_blank";
      anchor.rel = "noopener noreferrer";
      anchor.appendChild(content);
      parent.appendChild(anchor);
    });
  }
  function fillCard(marker) {
    card.textContent = "";
    var close = element("button", "note-card-close", "×");
    close.type = "button";
    close.setAttribute("aria-label", "Close citation preview");
    close.addEventListener("click", function (event) {
      event.stopPropagation();
      closeCard();
    });
    card.appendChild(close);
    (marker.sources || []).forEach(function (source) {
      var row = element("div", "note-card-source");
      var head = element("div", "note-card-head");
      if (source.title) head.appendChild(element("span", "note-card-title", source.title));
      if (source.handle) head.appendChild(element("span", "note-card-handle", source.handle));
      if (source.location) head.appendChild(element("span", "note-card-location", source.location));
      row.appendChild(head);
      if (source.excerpt) {
        var excerpt = element("div", "note-card-excerpt");
        appendExcerpt(excerpt, source);
        row.appendChild(excerpt);
      }
      card.appendChild(row);
    });
  }
  function positionCard(anchor) {
    card.classList.add("show");
    var rect = anchor.getBoundingClientRect();
    var left = Math.min(window.scrollX + rect.left, window.scrollX + document.documentElement.clientWidth - card.offsetWidth - 12);
    card.style.left = Math.max(window.scrollX + 12, left) + "px";
    var top = window.scrollY + rect.bottom + 6;
    if (rect.bottom + card.offsetHeight + 12 > window.innerHeight && rect.top - card.offsetHeight - 6 > 0) {
      top = window.scrollY + rect.top - card.offsetHeight - 6;
    }
    card.style.top = top + "px";
  }
  function showCard(anchor, marker) {
    if (hideTimer) { clearTimeout(hideTimer); hideTimer = null; }
    fillCard(marker);
    positionCard(anchor);
  }
  function hideCard() {
    if (pinnedAnchor) return;
    if (hideTimer) clearTimeout(hideTimer);
    hideTimer = setTimeout(function () {
      hideTimer = null;
      card.classList.remove("show");
    }, 140);
  }
  function closeCard() {
    if (hideTimer) { clearTimeout(hideTimer); hideTimer = null; }
    pinnedAnchor = null;
    card.classList.remove("show");
  }
  function touchPreview(event, anchor, marker) {
    if (!touchQuery.matches) return false;
    if (pinnedAnchor === anchor && card.classList.contains("show")) {
      closeCard();
      return false;
    }
    event.preventDefault();
    event.stopPropagation();
    pinnedAnchor = anchor;
    showCard(anchor, marker);
    return true;
  }
  function wirePreview(target, marker, wireTap) {
    if (!marker) return;
    target.setAttribute("aria-haspopup", "dialog");
    target.addEventListener("mouseenter", function () { showCard(target, marker); });
    target.addEventListener("mouseleave", hideCard);
    target.addEventListener("focus", function () { showCard(target, marker); });
    target.addEventListener("blur", hideCard);
    if (wireTap) {
      target.addEventListener("click", function (event) { touchPreview(event, target, marker); });
    }
  }
  function flash(target) {
    if (!target) return;
    target.classList.remove("flash");
    void target.offsetWidth;
    target.classList.add("flash");
    setTimeout(function () { target.classList.remove("flash"); }, 1200);
  }
  function scrollTo(target) {
    if (!target) return;
    target.scrollIntoView({block: "center", behavior: "smooth"});
    flash(target);
  }
  document.querySelectorAll(".note-detail").forEach(function (link) {
    link.setAttribute("aria-label", "Jump to citation " + link.dataset.cite + " details");
    link.addEventListener("click", function (event) {
      event.stopPropagation();
      var target = document.getElementById("cite-0-" + link.dataset.cite);
      if (!target) return;
      event.preventDefault();
      scrollTo(target);
    });
  });
  document.querySelectorAll(".note-passage").forEach(function (button) {
    var target = document.querySelector('.grounded[data-cite="' + button.dataset.cite + '"]');
    button.setAttribute("aria-label", "Jump to passage grounded by citation " + button.dataset.cite);
    if (!target) {
      button.disabled = true;
      button.title = "No grounded passage is available";
      return;
    }
    button.addEventListener("click", function (event) {
      event.stopPropagation();
      scrollTo(target);
    });
  });
  document.querySelectorAll(".citelink[data-cite],.grounded[data-cite]").forEach(function (target) {
    wirePreview(target, byIndex[parseInt(target.dataset.cite, 10)], true);
  });
  document.querySelectorAll(".note-ref[data-cite]").forEach(function (entry) {
    var marker = byIndex[parseInt(entry.dataset.cite, 10)];
    if (!marker) return;
    entry.tabIndex = 0;
    entry.setAttribute("role", "button");
    entry.setAttribute("aria-label", "Preview citation " + entry.dataset.cite);
    wirePreview(entry, marker, false);
    entry.addEventListener("click", function (event) {
      if (touchPreview(event, entry, marker)) return;
      scrollTo(document.getElementById("cite-0-" + entry.dataset.cite));
    });
    entry.addEventListener("keydown", function (event) {
      if (event.key === "Enter" || event.key === " ") {
        event.preventDefault();
        scrollTo(document.getElementById("cite-0-" + entry.dataset.cite));
      }
    });
  });
  card.addEventListener("mouseenter", function () {
    if (hideTimer) { clearTimeout(hideTimer); hideTimer = null; }
  });
  card.addEventListener("mouseleave", hideCard);
  card.addEventListener("click", function (event) { event.stopPropagation(); });
  document.addEventListener("click", function (event) {
    if (pinnedAnchor && !card.contains(event.target)) closeCard();
  });
  document.addEventListener("keydown", function (event) {
    if (event.key === "Escape") closeCard();
  });
  window.addEventListener("hashchange", function () {
    flash(document.getElementById(location.hash.slice(1)));
  });
})();
</script>
{{if .HasMath}}<!-- MathJax support -->
<!-- Config must precede the async loader: the loader may execute before
window.MathJax is defined otherwise, dropping the custom delimiters. -->
<script>
    window.MathJax = {
        tex: {
            inlineMath: [['$', '$'], ['\\(', '\\)']],
            displayMath: [['$$', '$$'], ['\\[', '\\]']]
        }
    };
</script>
<script id="MathJax-script" async src="https://cdn.jsdelivr.net/npm/mathjax@3/es5/tex-mml-chtml.js"></script>
{{end}}
<!-- MathJax is loaded from the CDN for local HTML. An offline or
claude.ai Artifact variant would need to inline the runtime. -->
</body>
</html>`))
