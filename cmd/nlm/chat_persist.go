package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tmc/nlm/notebooklm"
)

const (
	chatStatusRunning    = "running"
	chatStatusComplete   = "complete"
	chatStatusIncomplete = "incomplete"
	chatStatusError      = "error"
)

// beginGeneratedChatTurn persists the user turn before contacting NotebookLM.
// A session left in running state records an interrupted or still-active turn.
func beginGeneratedChatTurn(notebookID, conversationID string, history []notebooklm.ChatMessage, prompt string) error {
	session, err := loadChatSessionByConversation(notebookID, conversationID)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		messages := slices.Clone(history)
		slices.Reverse(messages)
		session = chatSessionFromServerHistory(notebookID, conversationID, messages)
	}
	session.NotebookID = notebookID
	session.ConversationID = conversationID
	now := time.Now()
	if len(session.Messages) == 0 || session.Messages[len(session.Messages)-1].Role != "user" || session.Messages[len(session.Messages)-1].Content != prompt {
		session.Messages = append(session.Messages, storedMessage{Role: "user", Content: prompt, Timestamp: now})
	}
	session.Status = chatStatusRunning
	session.StatusAt = now
	session.WriterPID = os.Getpid()
	session.UpdatedAt = now
	return saveChatSession(session)
}

// saveGeneratedChatTurn appends to the selected conversation, retaining local
// stream metadata. Wire history supplies earlier turns only for a new cache.
func saveGeneratedChatTurn(notebookID, conversationID string, history []notebooklm.ChatMessage, prompt string, result chatResult) error {
	session, err := loadChatSessionByConversation(notebookID, conversationID)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		messages := slices.Clone(history)
		slices.Reverse(messages) // wire history is newest first
		session = chatSessionFromServerHistory(notebookID, conversationID, messages)
	}
	session.ConversationID = conversationID
	now := time.Now()
	session.UpdatedAt = now
	if len(session.Messages) == 0 || session.Messages[len(session.Messages)-1].Role != "user" || session.Messages[len(session.Messages)-1].Content != prompt {
		session.Messages = append(session.Messages, storedMessage{Role: "user", Content: prompt, Timestamp: now})
	}
	answer := strings.TrimSpace(result.Answer)
	if answer == "" {
		answer = strings.TrimSpace(result.Thinking)
	}
	if answer != "" && !result.Incomplete {
		session.Messages = append(session.Messages, storedMessage{
			Role: "assistant", Content: answer, Timestamp: now,
			Thinking: result.Thinking, Citations: result.Citations, Rich: result.Rich,
		})
	}
	session.StatusAt = now
	session.WriterPID = 0
	if result.Incomplete {
		session.Status = chatStatusIncomplete
	} else {
		session.Status = chatStatusComplete
	}
	return saveChatSession(session)
}

func setGeneratedChatStatus(notebookID, conversationID, status string) error {
	session, err := loadChatSessionByConversation(notebookID, conversationID)
	if err != nil {
		return err
	}
	session.Status = status
	session.StatusAt = time.Now()
	session.WriterPID = 0
	session.UpdatedAt = time.Now()
	return saveChatSession(session)
}

// writeChatSessionFile replaces a session without exposing a truncated JSON file.
func writeChatSessionFile(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".chat-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
