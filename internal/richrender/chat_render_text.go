package richrender

import (
	"fmt"
	"io"
	"strings"
)

// renderChatText is the terminal/plain projection. It renders one turn at a
// time through the shared live renderer so citation output is identical to the
// streaming path.
func renderChatText(out, status io.Writer, doc ChatDocument, mode CitationMode, ctx RenderContext) error {
	for i, message := range doc.Messages {
		if i > 0 {
			fmt.Fprintln(out)
		}
		fmt.Fprintf(out, "[%s]\n", strings.ToUpper(message.Role))
		if message.Role != "assistant" {
			fmt.Fprintln(out, message.Content)
			continue
		}
		if ctx.ShowThinking && message.Thinking != "" {
			fmt.Fprintf(status, "%s%s%s\n", ansiGrey, message.Thinking, ansiReset)
		}

		// Reconstruct paragraph structure from the span tree only for the
		// run-together case. Citation spans remain keyed to the flat answer.
		body := message.Content
		if shouldReflowFromTree(message.Rich, message.Content) {
			if reflowed := flattenText(projectRichDocument(message.Rich)); reflowed != "" {
				body = reflowed
			}
		}
		renderPersistedAssistant(out, status, StoredMessage{
			Role:      message.Role,
			Content:   body,
			Thinking:  message.Thinking,
			Citations: message.Citations,
		}, mode, persistedRenderConfig{
			excerptBudget:  ctx.ExcerptBudget,
			hideConfidence: ctx.HideConfidence,
			hideSpans:      ctx.HideSpans,
			loadSource:     ctx.LoadSource,
			resolveTitle:   ctx.ResolveTitle,
			sourceRemoved:  ctx.SourceRemoved,
		})
	}
	return nil
}
