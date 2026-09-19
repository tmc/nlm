package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// chatSessionSummary is the projection of a stored session the listing
// commands need. Reading it does not materialize the messages, which on a
// large store are almost all of the bytes.
type chatSessionSummary struct {
	Path           string
	NotebookID     string
	ConversationID string
	MessageCount   int
	Title          string
	Status         string
	StatusAt       time.Time
	WriterPID      int
	UpdatedAt      time.Time
}

// chatSessionFilePrefix returns the filename prefix every session file for
// notebookID shares, or "chat-" when notebookID is empty. Session files are
// named chat-<notebook>[-<conv8>].json, so the notebook filter is a filename
// match. Reading each file to learn which notebook it belongs to cost a full
// pass over the store: 3.3GB across 7,700 files took most of a minute.
func chatSessionFilePrefix(notebookID string) string {
	if notebookID == "" {
		return "chat-"
	}
	return "chat-" + notebookID
}

// localChatSessionFiles returns the stored session files for notebookID, or
// every session file when notebookID is empty. A missing store is not an
// error: it means no local sessions.
func localChatSessionFiles(notebookID string) ([]string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	nlmDir := filepath.Join(homeDir, ".nlm")
	entries, err := os.ReadDir(nlmDir)
	if err != nil {
		return nil, nil
	}
	prefix := chatSessionFilePrefix(notebookID)
	var paths []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".json") {
			continue
		}
		paths = append(paths, filepath.Join(nlmDir, name))
	}
	return paths, nil
}

// readChatSessionSummary reads path far enough to describe the session,
// counting messages without decoding them. A stored message carries its
// citations and rich document, so decoding them to report a count is what made
// listing a large store slow.
func readChatSessionSummary(path string) (chatSessionSummary, error) {
	f, err := os.Open(path)
	if err != nil {
		return chatSessionSummary{}, err
	}
	defer f.Close()

	summary := chatSessionSummary{Path: path}
	dec := json.NewDecoder(bufio.NewReaderSize(f, 1<<20))
	if tok, err := dec.Token(); err != nil {
		return summary, err
	} else if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return summary, fmt.Errorf("%s: not a session object", filepath.Base(path))
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return summary, err
		}
		key, _ := tok.(string)
		switch key {
		case "notebook_id":
			err = dec.Decode(&summary.NotebookID)
		case "conversation_id":
			err = dec.Decode(&summary.ConversationID)
		case "updated_at":
			err = dec.Decode(&summary.UpdatedAt)
		case "status":
			err = dec.Decode(&summary.Status)
		case "status_at":
			err = dec.Decode(&summary.StatusAt)
		case "writer_pid":
			err = dec.Decode(&summary.WriterPID)
		case "messages":
			summary.MessageCount, summary.Title, err = summarizeChatMessages(dec)
		default:
			var skip json.RawMessage
			err = dec.Decode(&skip)
		}
		if err != nil {
			return summary, err
		}
	}
	return summary, nil
}

// countJSONArray consumes the array value the decoder is positioned at and
// returns its length, holding one element at a time.
func countJSONArray(dec *json.Decoder) (int, error) {
	tok, err := dec.Token()
	if err != nil {
		return 0, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '[' {
		return 0, fmt.Errorf("expected array, got %v", tok)
	}
	n := 0
	for dec.More() {
		var element json.RawMessage
		if err := dec.Decode(&element); err != nil {
			return n, err
		}
		n++
	}
	if _, err := dec.Token(); err != nil { // closing ']'
		return n, err
	}
	return n, nil
}

// chatSummaryCacheEntry is one cached session summary, valid while the file's
// size and modification time still match. A session file is only ever
// rewritten whole, so those two identify its contents.
type chatSummaryCacheEntry struct {
	Version        int       `json:"version"`
	Title          string    `json:"title"`
	Status         string    `json:"status,omitempty"`
	StatusAt       time.Time `json:"status_at,omitempty"`
	WriterPID      int       `json:"writer_pid,omitempty"`
	Size           int64     `json:"size"`
	ModTime        time.Time `json:"mod_time"`
	NotebookID     string    `json:"notebook_id"`
	ConversationID string    `json:"conversation_id,omitempty"`
	MessageCount   int       `json:"message_count"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// chatSummaryCachePath returns the summary cache file, or "" when no cache
// directory is available.
func chatSummaryCachePath() string {
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(base, "nlm")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	return filepath.Join(dir, "chat-index.json")
}

// loadChatSummaryCache returns the cached summaries keyed by file path. A
// missing or corrupt cache reads as empty: the caller recomputes.
func loadChatSummaryCache() map[string]chatSummaryCacheEntry {
	path := chatSummaryCachePath()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cache map[string]chatSummaryCacheEntry
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil
	}
	return cache
}

// saveChatSummaryCache writes cache, dropping entries for files that are gone.
// Errors are ignored: an unwritable cache is a slow listing, not a failed one.
func saveChatSummaryCache(cache map[string]chatSummaryCacheEntry) {
	path := chatSummaryCachePath()
	if path == "" {
		return
	}
	for file := range cache {
		if _, err := os.Stat(file); err != nil {
			delete(cache, file)
		}
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
	}
}

// listLocalChatSessionSummaries describes the stored sessions for notebookID,
// or every stored session when notebookID is empty. Summaries come from the
// cache when the file is unchanged; counting messages otherwise means reading
// the whole session, and a large store runs to gigabytes. Unreadable and
// malformed files are skipped: a listing must not fail on one bad session.
func listLocalChatSessionSummaries(notebookID string) ([]chatSessionSummary, error) {
	paths, err := localChatSessionFiles(notebookID)
	if err != nil {
		return nil, err
	}
	cache := loadChatSummaryCache()
	if cache == nil {
		cache = make(map[string]chatSummaryCacheEntry)
	}
	stale := false
	summaries := make([]chatSessionSummary, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		entry, ok := cache[path]
		if !ok || entry.Version != 2 || entry.Size != info.Size() || !entry.ModTime.Equal(info.ModTime()) {
			summary, err := readChatSessionSummary(path)
			if err != nil {
				continue
			}
			entry = chatSummaryCacheEntry{
				Version: 2, Title: summary.Title, Status: summary.Status, StatusAt: summary.StatusAt, WriterPID: summary.WriterPID,
				Size:           info.Size(),
				ModTime:        info.ModTime(),
				NotebookID:     summary.NotebookID,
				ConversationID: summary.ConversationID,
				MessageCount:   summary.MessageCount,
				UpdatedAt:      summary.UpdatedAt,
			}
			cache[path] = entry
			stale = true
		}
		if notebookID != "" && entry.NotebookID != notebookID {
			continue
		}
		summaries = append(summaries, chatSessionSummary{
			Title: entry.Title, Status: entry.Status, StatusAt: entry.StatusAt, WriterPID: entry.WriterPID,
			Path:           path,
			NotebookID:     entry.NotebookID,
			ConversationID: entry.ConversationID,
			MessageCount:   entry.MessageCount,
			UpdatedAt:      entry.UpdatedAt,
		})
	}
	if stale {
		saveChatSummaryCache(cache)
	}
	return summaries, nil
}

// findLocalChatSession loads the stored session for conversationID, matching
// either the full ID or a prefix of it. Candidates are narrowed by filename
// first, so only the matching session is decoded in full.
func findLocalChatSession(notebookID, conversationID string) (*chatSession, error) {
	paths, err := localChatSessionFiles(notebookID)
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		// The filename carries only the conversation's first 8 characters,
		// so compare on the shorter of the two.
		name := strings.TrimSuffix(filepath.Base(path), ".json")
		if suffix := strings.TrimPrefix(name, chatSessionFilePrefix(notebookID)+"-"); suffix != name {
			if !strings.HasPrefix(shortID(conversationID), suffix) && !strings.HasPrefix(suffix, shortID(conversationID)) {
				continue
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var session chatSession
		if err := json.Unmarshal(data, &session); err != nil {
			continue
		}
		if session.NotebookID != notebookID {
			continue
		}
		if session.ConversationID == conversationID || strings.HasPrefix(session.ConversationID, conversationID) {
			return &session, nil
		}
	}
	return nil, os.ErrNotExist
}

func summarizeChatMessages(dec *json.Decoder) (int, string, error) {
	tok, err := dec.Token()
	if err != nil {
		return 0, "", err
	}
	if tok != json.Delim('[') {
		return 0, "", fmt.Errorf("expected messages array")
	}
	n, title := 0, ""
	for dec.More() {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return n, title, err
		}
		n++
		if title == "" {
			var message struct{ Role, Content string }
			if err := json.Unmarshal(raw, &message); err != nil {
				return n, title, err
			}
			if message.Role == "user" {
				text := []rune(strings.Join(strings.Fields(message.Content), " "))
				if len(text) > 120 {
					text = append(text[:120], '…')
				}
				title = string(text)
			}
		}
	}
	_, err = dec.Token()
	return n, title, err
}
