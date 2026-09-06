package main

import (
	"slices"
	"strings"

	"github.com/tmc/nlm/notebooklm"
)

// mergeChatHistory adds server turns without losing stream-only local data.
// Both inputs are chronological. Match from the end so an old cache holding
// only the latest exchange aligns with the latest occurrence of repeated text.
func mergeChatHistory(session *chatSession, messages []notebooklm.ChatMessage) (changed bool, added, richCount, citationCount int) {
	remote := chatSessionFromServerHistory(session.NotebookID, session.ConversationID, messages).Messages
	var merged []storedMessage
	next := len(session.Messages) - 1
	for i := len(remote) - 1; i >= 0; i-- {
		message := remote[i]
		match := -1
		for j := next; j >= 0; j-- {
			if sameHistoryMessage(session.Messages[j], message) {
				match = j
				break
			}
		}
		if match < 0 {
			merged = append(merged, message)
			added++
			continue
		}
		for next > match {
			merged = append(merged, session.Messages[next])
			next--
		}
		local := session.Messages[next]
		next--
		if local.MessageID == "" && message.MessageID != "" {
			local.MessageID = message.MessageID
			changed = true
		}
		if local.Rich == nil && message.Rich != nil {
			local.Rich = message.Rich
			richCount++
		}
		if len(local.Citations) == 0 && len(message.Citations) > 0 {
			local.Citations = message.Citations
			citationCount++
		}
		merged = append(merged, local)
	}
	for next >= 0 {
		merged = append(merged, session.Messages[next])
		next--
	}
	slices.Reverse(merged)
	session.Messages = merged
	return changed || added > 0 || richCount > 0 || citationCount > 0, added, richCount, citationCount
}

func sameHistoryMessage(a, b storedMessage) bool {
	if a.Role != b.Role {
		return false
	}
	if a.MessageID != "" && b.MessageID != "" {
		return a.MessageID == b.MessageID
	}
	// History flattens line breaks; compare the entire answer, not a prefix.
	return strings.Join(strings.Fields(a.Content), "") == strings.Join(strings.Fields(b.Content), "")
}
