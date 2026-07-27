package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

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
				dm.Rich = richDocumentFromProto(message.Rich)
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
		historyClient = newNotebookLMClient(api.Credentials{AuthToken: authToken, Cookies: cookies}, false)
	}
	for _, record := range records {
		doc := notebookDocumentFromSession(record.Session)
		if historyClient != nil {
			fullID := resolveConversationID(historyClient, notebookID, record.Session.ConversationID)
			messages, err := historyClient.GetConversationHistory(context.Background(), notebookID, fullID)
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
			rich[key] = richDocumentFromProto(message.Rich)
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

	c := newNotebookLMClient(api.Credentials{AuthToken: authToken, Cookies: cookies}, false)
	sourceIndex := newNotebookSourceIndex(c, notebookID)
	ctx.ResolveTitle = sourceIndex.title
	ctx.SourceRemoved = sourceIndex.removed
	if opts.ResolveCitations {
		cache := make(map[string]api.LoadSourceText)
		ctx.LoadSource = func(sourceID string) (api.LoadSourceText, error) {
			if body, ok := cache[sourceID]; ok {
				return body, nil
			}
			body, err := c.LoadSourceText(context.Background(), sourceID, notebookID)
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
