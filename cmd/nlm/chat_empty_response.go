package main

import (
	"errors"
	"fmt"
	"strings"
)

// errEmptyChatResponse marks a chat turn the server closed without any
// content. It maps to exit 6 (transient) because that is what the failure
// measures as: two prompts came back empty once each inside a five-minute
// window and then answered on every later attempt. A single empty response is
// not evidence of anything, and the remedy that works is to retry.
//
// Not every empty response is transient — a prompt carrying a long run of
// byte-identical lines returns empty deterministically (8/8 on two files,
// while the same prompts with the repeated block replaced by varied lines of
// the same total size answered 5/5). The server reports neither case, so the
// two are indistinguishable on the wire; emptyChatResponseHint names the
// repetition when the prompt carries it, and the retry class covers the rest.
var errEmptyChatResponse = errors.New("empty response from API")

// identicalRunHintThreshold is the run length at which a prompt's repeated
// lines are worth naming. Measured: ~12-14 identical lines returns empty every
// time; two duplicate lines are fine. The threshold sits below the observed
// trigger so a prompt approaching it is still called out.
const identicalRunHintThreshold = 8

// longestIdenticalRun returns the longest run of consecutive byte-identical
// lines in prompt, and the line itself. Blank lines are ignored: paragraph
// spacing is not the pattern that trips the server.
func longestIdenticalRun(prompt string) (line string, run int) {
	lines := strings.Split(prompt, "\n")
	current, count := "", 0
	for _, candidate := range lines {
		if strings.TrimSpace(candidate) == "" {
			current, count = "", 0
			continue
		}
		if candidate == current {
			count++
		} else {
			current, count = candidate, 1
		}
		if count > run {
			line, run = candidate, count
		}
	}
	return line, run
}

// emptyChatResponseHint describes an empty answer in terms the caller can act
// on: the repeated-line trigger when the prompt carries one, and the retry
// advice otherwise.
func emptyChatResponseHint(prompt, notebookID string) string {
	if line, run := longestIdenticalRun(prompt); run >= identicalRunHintThreshold {
		if len(line) > 48 {
			line = line[:45] + "..."
		}
		return fmt.Sprintf("the prompt repeats the line %q %d times in a row; a long run of identical lines is rejected with an empty response and no diagnostic — vary or collapse them", line, run)
	}
	return fmt.Sprintf("empty responses are often transient, so retry before treating this as a failure; if it persists, check 'nlm source list %s' for source state and re-run with -debug", notebookID)
}
