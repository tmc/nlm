package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

// notebookChatDocument is one saved conversation and the metadata shown in the
// notebook switcher. FileName is retained only as the deterministic final sort
// key for sessions whose timestamps are equal or absent.
type notebookChatDocument struct {
	Document chatDocument
	Updated  time.Time
	Created  time.Time
	FileName string
}

type localChatSessionRecord struct {
	Session  *ChatSession
	FileName string
}

// loadNotebookSessions loads and orders all saved conversations for notebookID.
// It scans both the current flat store and any nested store beneath ~/.nlm, then
// removes the duplicate made when the active conversation is also saved in the
// legacy notebook-wide file.
func loadNotebookSessions(notebookID string) ([]*ChatSession, error) {
	records, err := loadNotebookSessionRecords(notebookID)
	if err != nil {
		return nil, err
	}
	sessions := make([]*ChatSession, len(records))
	for i := range records {
		sessions[i] = records[i].Session
	}
	return sessions, nil
}

func loadNotebookSessionRecords(notebookID string) ([]localChatSessionRecord, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	root := filepath.Join(homeDir, ".nlm")

	var records []localChatSessionRecord
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var session ChatSession
		if json.Unmarshal(data, &session) != nil || session.NotebookID != notebookID {
			return nil
		}
		records = append(records, localChatSessionRecord{
			Session:  &session,
			FileName: path,
		})
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// A conversation can appear in both chat-<notebook>.json and its
	// conversation-specific file. Keep the more recently updated copy, using
	// the filename only to make an exact tie deterministic.
	byConversation := make(map[string]localChatSessionRecord)
	for _, record := range records {
		key := record.Session.ConversationID
		if key == "" {
			key = "file:" + record.FileName
		}
		old, ok := byConversation[key]
		if !ok || notebookSessionRecordLess(record, old) {
			byConversation[key] = record
		}
	}
	records = records[:0]
	for _, record := range byConversation {
		if len(record.Session.Messages) != 0 {
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool {
		return notebookSessionRecordLess(records[i], records[j])
	})
	return records, nil
}

// notebookSessionRecordLess reports whether a belongs before b in the
// newest-first switcher order.
func notebookSessionRecordLess(a, b localChatSessionRecord) bool {
	at, bt := notebookSessionTime(a.Session), notebookSessionTime(b.Session)
	if !at.Equal(bt) {
		return at.After(bt)
	}
	return a.FileName < b.FileName
}

func notebookSessionTime(session *ChatSession) time.Time {
	if !session.UpdatedAt.IsZero() {
		return session.UpdatedAt
	}
	return session.CreatedAt
}

func notebookDocumentFromSession(session *ChatSession) chatDocument {
	doc := chatDocument{
		NotebookID:     session.NotebookID,
		ConversationID: session.ConversationID,
	}
	for _, message := range session.Messages {
		dm := chatDocMessage{
			Role:     message.Role,
			Content:  message.Content,
			Thinking: message.Thinking,
		}
		if message.Role == "assistant" {
			dm.Citations = message.Citations
			if message.Rich != nil {
				dm.Rich = richDocumentFromAPI(message.Rich)
			}
		}
		doc.Messages = append(doc.Messages, dm)
	}
	return doc
}

func chatShowNotebook(notebookID string, opts chatRenderOptions) error {
	records, err := loadNotebookSessionRecords(notebookID)
	if err != nil {
		return fmt.Errorf("load local sessions: %w", err)
	}
	if len(records) == 0 {
		return fmt.Errorf("no local chat sessions for notebook %s", notebookID)
	}

	docs := make([]notebookChatDocument, 0, len(records))
	var historyClient *api.Client
	if opts.ExcerptBudget > 0 && authToken != "" && cookies != "" {
		historyClient = api.New(authToken, cookies)
		if debug {
			historyClient.SetDebug(true)
		}
	}
	for _, record := range records {
		doc := notebookDocumentFromSession(record.Session)
		if historyClient != nil {
			fullID := resolveConversationID(historyClient, notebookID, record.Session.ConversationID)
			messages, err := historyClient.GetConversationHistory(notebookID, fullID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "nlm: could not fetch history for conversation %s excerpts: %v\n", shortID(record.Session.ConversationID), err)
			} else {
				mergeNotebookHistory(&doc, messages)
			}
		}
		docs = append(docs, notebookChatDocument{
			Document: doc,
			Updated:  record.Session.UpdatedAt,
			Created:  record.Session.CreatedAt,
			FileName: record.FileName,
		})
	}
	ctx := notebookChatRenderContext(notebookID, opts)
	return renderNotebookHTMLToDestination(notebookID, docs, ctx, opts)
}

// mergeNotebookHistory overlays the excerpt-bearing citations and rich tree
// returned by conversation history onto a render-only document. The saved
// session remains authoritative on disk and is never modified.
func mergeNotebookHistory(doc *chatDocument, messages []api.ChatMessage) {
	citations := make(map[string][]api.Citation)
	rich := make(map[string]*richDocument)
	for _, message := range messages {
		if message.Role != 2 {
			continue
		}
		key := citationContentKey(message.Content)
		if len(message.Citations) != 0 {
			citations[key] = message.Citations
		}
		if message.Rich != nil {
			rich[key] = richDocumentFromAPI(message.Rich)
		}
	}
	for i := range doc.Messages {
		message := &doc.Messages[i]
		if message.Role != "assistant" {
			continue
		}
		key := citationContentKey(message.Content)
		if history := citations[key]; len(history) != 0 {
			message.Citations = history
		}
		if history := rich[key]; history != nil {
			message.Rich = history
		}
	}
}

// notebookChatRenderContext supplies the same local rendering options as the
// single-conversation path. When credentials are available, titles and source
// bodies are resolved lazily; otherwise the bundled session remains fully
// renderable offline.
func notebookChatRenderContext(notebookID string, opts chatRenderOptions) chatRenderContext {
	ctx := chatRenderContext{
		ShowThinking:     opts.ShowThinking,
		ExcerptBudget:    opts.ExcerptBudget,
		HideConfidence:   opts.HideConfidence,
		HideSpans:        opts.HideSpans,
		IncludeFollowUps: opts.IncludeFollowUps,
	}
	if authToken == "" || cookies == "" {
		if opts.ResolveCitations || opts.ExcerptBudget > 0 {
			fmt.Fprintln(os.Stderr, "nlm: --citation-excerpts/--resolve-citations need auth; run 'nlm auth'. Rendering saved data only.")
		}
		return ctx
	}

	c := api.New(authToken, cookies)
	if debug {
		c.SetDebug(true)
	}
	sourceIndex := newNotebookSourceIndex(c, notebookID)
	ctx.resolveTitle = sourceIndex.title
	ctx.sourceRemoved = sourceIndex.removed
	if opts.ResolveCitations {
		cache := make(map[string]api.LoadSourceText)
		ctx.loadSource = func(sourceID string) (api.LoadSourceText, error) {
			if body, ok := cache[sourceID]; ok {
				return body, nil
			}
			body, err := c.LoadSourceText(sourceID, notebookID)
			if err != nil {
				return api.LoadSourceText{}, err
			}
			cache[sourceID] = body
			return body, nil
		}
	}
	return ctx
}

func renderNotebookHTMLToDestination(notebookID string, docs []notebookChatDocument, ctx chatRenderContext, opts chatRenderOptions) error {
	path, err := notebookHTMLDestination(notebookID, opts.OutFile)
	if err != nil {
		return err
	}
	if path == "" {
		return renderNotebookHTML(os.Stdout, docs, ctx)
	}
	var buf bytes.Buffer
	if err := renderNotebookHTML(&buf, docs, ctx); err != nil {
		return err
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write html: %w", err)
	}
	fmt.Fprintf(os.Stderr, "nlm: wrote %s\n", path)
	if opts.Open {
		if err := openInBrowser(path); err != nil {
			fmt.Fprintf(os.Stderr, "nlm: could not open browser: %v\n", err)
		}
	}
	return nil
}

func notebookHTMLDestination(notebookID, outFile string) (string, error) {
	if outFile == "-" {
		return "", nil
	}
	if outFile != "" {
		return outFile, nil
	}
	dir, err := renderCacheDir()
	if err != nil {
		return "", fmt.Errorf("create render cache: %w", err)
	}
	dir = filepath.Join(dir, notebookID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create render directory: %w", err)
	}
	return filepath.Join(dir, "notebook.html"), nil
}

type notebookHTMLConversation struct {
	ID           string `json:"id"`
	Namespace    string `json:"namespace"`
	Title        string `json:"title"`
	MessageCount int    `json:"messageCount"`
	Timestamp    string `json:"timestamp"`
	ShortID      string `json:"shortId"`
	HTML         string `json:"html"`
}

type notebookHTMLPayload struct {
	Conversations []notebookHTMLConversation `json:"conversations"`
}

// renderNotebookHTML buffers the established single-conversation renderer for
// each document. The buffered copy gets conversation-scoped citation targets;
// renderChatHTML itself is unchanged, preserving the single-conversation page
// byte for byte.
func renderNotebookHTML(w io.Writer, docs []notebookChatDocument, ctx chatRenderContext) error {
	payload := notebookHTMLPayload{
		Conversations: make([]notebookHTMLConversation, 0, len(docs)),
	}
	for i, item := range docs {
		var page bytes.Buffer
		if err := renderChatHTML(&page, item.Document, ctx); err != nil {
			return fmt.Errorf("render conversation %s: %w", item.Document.ConversationID, err)
		}
		namespace := notebookConversationNamespace(item.Document.ConversationID, i)
		namespacedPage, err := namespaceChatHTML(page.String(), namespace)
		if err != nil {
			return fmt.Errorf("namespace conversation %s: %w", item.Document.ConversationID, err)
		}
		payload.Conversations = append(payload.Conversations, notebookHTMLConversation{
			ID:           item.Document.ConversationID,
			Namespace:    namespace,
			Title:        notebookConversationTitle(item.Document),
			MessageCount: len(item.Document.Messages),
			Timestamp:    notebookConversationTimestamp(item),
			ShortID:      shortID(item.Document.ConversationID),
			HTML:         namespacedPage,
		})
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode notebook data: %w", err)
	}
	data := struct {
		Blob template.JS
	}{
		Blob: template.JS(blob),
	}
	if err := notebookHTMLTemplate.Execute(w, data); err != nil {
		return fmt.Errorf("render notebook html: %w", err)
	}
	return nil
}

func notebookConversationNamespace(conversationID string, index int) string {
	var b strings.Builder
	for _, r := range shortID(conversationID) {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		b.WriteString("conversation")
	}
	return fmt.Sprintf("conv-%s-%d", b.String(), index)
}

func namespaceChatHTML(page, namespace string) (string, error) {
	page = strings.ReplaceAll(page, `href="#cite-`, `href="#cite-`+namespace+`-`)
	const old = `function citeId(msgIdx, idx) { return "cite-" + msgIdx + "-" + idx; }`
	if !strings.Contains(page, old) {
		return "", fmt.Errorf("citation target seam not found")
	}
	replacement := `function citeId(msgIdx, idx) { return "cite-` + namespace + `-" + msgIdx + "-" + idx; }`
	return strings.Replace(page, old, replacement, 1), nil
}

func notebookConversationTitle(doc chatDocument) string {
	for _, message := range doc.Messages {
		if message.Role != "user" {
			continue
		}
		line, _, _ := strings.Cut(message.Content, "\n")
		title := collapseWhitespace(line)
		if title == "" {
			continue
		}
		const maxRunes = 88
		runes := []rune(title)
		if len(runes) > maxRunes {
			title = string(runes[:maxRunes-1]) + "…"
		}
		return title
	}
	if id := shortID(doc.ConversationID); id != "" {
		return "Conversation " + id
	}
	return "Conversation"
}

func notebookConversationTimestamp(item notebookChatDocument) string {
	timestamp := item.Updated
	if timestamp.IsZero() {
		timestamp = item.Created
	}
	if timestamp.IsZero() {
		return ""
	}
	return timestamp.Local().Format("Jan 2, 2006 15:04")
}

var notebookHTMLTemplate = template.Must(template.New("chat-notebook").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>NotebookLM conversations</title>
<style>
:root {
  --bg: #f4f5f8; --panel: #fff; --line: #dfe2e8; --text: #20232a;
  --muted: #686e7a; --accent: #3559c7; --accent-tint: #edf1ff;
  --sans: ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  --mono: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}
* { box-sizing: border-box; }
html, body { height: 100%; max-width: 100%; margin: 0; overflow: hidden; }
body { background: var(--bg); color: var(--text); font-family: var(--sans); }
.notebook { display: grid; grid-template-columns: 320px minmax(0, 1fr); height: 100%; }
.sidebar {
  display: flex; flex-direction: column; min-height: 0; padding: 18px 14px;
  background: var(--panel); border-right: 1px solid var(--line);
}
.sidebar-head { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
.sidebar h1 { margin: 0 4px 14px; font-size: 18px; }
.sidebar-toggle {
  display: none; min-width: 44px; min-height: 44px; padding: 8px 12px;
  border: 1px solid var(--line); border-radius: 8px;
  background: var(--panel); color: var(--accent); cursor: pointer;
  font: 650 13px/1 var(--sans);
}
.search {
  width: 100%; border: 1px solid var(--line); border-radius: 8px;
  font: 14px/1.4 var(--sans); padding: 9px 10px; margin-bottom: 12px;
}
.list { display: flex; flex-direction: column; gap: 6px; overflow-y: auto; }
.conversation {
  appearance: none; width: 100%; border: 1px solid transparent; border-radius: 8px;
  background: transparent; color: inherit; cursor: pointer; padding: 10px;
  min-height: 44px; text-align: left; font: inherit;
}
.conversation:hover { background: #f6f7fa; }
.conversation.active { background: var(--accent-tint); border-color: #cbd5ff; }
.conversation[hidden] { display: none; }
.conversation-title {
  display: block; font-size: 14px; font-weight: 650; line-height: 1.35;
  overflow-wrap: anywhere;
}
.conversation-meta {
  display: flex; gap: 8px; flex-wrap: wrap; margin-top: 5px;
  color: var(--muted); font: 11px/1.35 var(--mono);
}
.empty { color: var(--muted); font-size: 13px; padding: 12px 10px; }
.main { min-width: 0; min-height: 0; padding: 12px; }
.frame {
  width: 100%; height: 100%; display: block; border: 1px solid var(--line);
  border-radius: 10px; background: var(--panel);
}
:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
@media (max-width: 860px) {
  .notebook { display: flex; flex-direction: column; height: 100%; }
  .sidebar {
    position: relative; z-index: 2; flex: 0 0 auto;
    min-height: 0; padding: 8px 10px; border-right: 0; border-bottom: 1px solid var(--line);
    box-shadow: 0 2px 8px rgba(24,28,40,.08);
  }
  .sidebar h1 { margin: 0 4px; font-size: 17px; }
  .sidebar-toggle { display: inline-flex; align-items: center; justify-content: center; }
  .sidebar:not(.open) .search,
  .sidebar:not(.open) .list,
  .sidebar:not(.open) .empty { display: none; }
  .sidebar.open .search { margin: 10px 0 8px; }
  .sidebar.open .list { max-height: min(52vh, 26rem); }
  .main { flex: 1 1 auto; min-height: 0; padding: 6px; }
  .frame { border-radius: 7px; }
}
</style>
</head>
<body>
<div class="notebook">
  <aside class="sidebar" aria-label="Conversations">
    <div class="sidebar-head">
      <h1>Conversations</h1>
      <button class="sidebar-toggle" id="sidebar-toggle" type="button" aria-expanded="false" aria-controls="conversation-list">Browse</button>
    </div>
    <input class="search" id="conversation-search" type="search" placeholder="Filter conversations" aria-label="Filter conversations">
    <div class="list" id="conversation-list"></div>
    <div class="empty" id="conversation-empty" hidden>No matching conversations.</div>
  </aside>
  <main class="main">
    <iframe class="frame" id="conversation-frame" title="Conversation"></iframe>
  </main>
</div>
<script id="notebook-data" type="application/json">{{.Blob}}</script>
<script>
(function () {
  "use strict";
  var data;
  try {
    data = JSON.parse(document.getElementById("notebook-data").textContent);
  } catch (e) {
    document.getElementById("conversation-list").textContent = "Failed to load conversations.";
    return;
  }
  var list = document.getElementById("conversation-list");
  var empty = document.getElementById("conversation-empty");
  var frame = document.getElementById("conversation-frame");
  var search = document.getElementById("conversation-search");
  var sidebar = document.querySelector(".sidebar");
  var sidebarToggle = document.getElementById("sidebar-toggle");
  var narrowQuery = window.matchMedia("(max-width: 860px)");
  var buttons = [];

  function el(tag, cls, text) {
    var node = document.createElement(tag);
    if (cls) node.className = cls;
    if (text != null) node.textContent = text;
    return node;
  }
  function select(index) {
    if (index < 0 || index >= data.conversations.length) return;
    buttons.forEach(function (button, i) {
      button.classList.toggle("active", i === index);
      button.setAttribute("aria-current", i === index ? "true" : "false");
    });
    var conversation = data.conversations[index];
    frame.title = conversation.title;
    frame.srcdoc = conversation.html;
    if (narrowQuery.matches) {
      sidebar.classList.remove("open");
      sidebarToggle.setAttribute("aria-expanded", "false");
    }
  }
  sidebarToggle.addEventListener("click", function () {
    var open = sidebar.classList.toggle("open");
    sidebarToggle.setAttribute("aria-expanded", open ? "true" : "false");
    if (open) search.focus();
  });
  data.conversations.forEach(function (conversation, index) {
    var button = el("button", "conversation");
    button.type = "button";
    button.appendChild(el("span", "conversation-title", conversation.title));
    var meta = el("span", "conversation-meta");
    meta.appendChild(el("span", "", conversation.messageCount + " messages"));
    if (conversation.timestamp) meta.appendChild(el("span", "", conversation.timestamp));
    if (conversation.shortId) meta.appendChild(el("span", "", conversation.shortId));
    button.appendChild(meta);
    button.dataset.search = (conversation.title + " " + conversation.shortId).toLowerCase();
    button.addEventListener("click", function () { select(index); });
    list.appendChild(button);
    buttons.push(button);
  });
  search.addEventListener("input", function () {
    var query = search.value.trim().toLowerCase();
    var shown = 0;
    buttons.forEach(function (button) {
      var match = !query || button.dataset.search.indexOf(query) !== -1;
      button.hidden = !match;
      if (match) shown++;
    });
    empty.hidden = shown !== 0;
  });
  document.addEventListener("keydown", function (event) {
    if (event.key === "Escape" && sidebar.classList.contains("open")) {
      sidebar.classList.remove("open");
      sidebarToggle.setAttribute("aria-expanded", "false");
      sidebarToggle.focus();
    }
  });
  if (data.conversations.length) select(0);
}());
</script>
</body>
</html>
`))
