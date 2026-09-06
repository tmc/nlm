package main

import (
	"errors"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/tmc/nlm/notebooklm"
)

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
	session.Messages = append(session.Messages, storedMessage{Role: "user", Content: prompt, Timestamp: now})
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
	return saveChatSession(session)
}
