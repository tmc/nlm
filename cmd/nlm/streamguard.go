package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Chat streams are unbounded: the server sends answer snapshots until the
// model stops, and a degenerate model does not stop. A single observed run
// streamed more than a megabyte of duplicated table fragments before the user
// interrupted it. streamGuard bounds that from the client side.
//
// Two independent guards, both evaluated against the cumulative answer:
//
//   - a byte cap, so no answer can grow without limit; and
//   - a repetition detector, so the common degeneration shape (the same block
//     emitted over and over) is caught long before the cap.
//
// The repetition detector is deliberately conservative. Legitimate answers
// repeat short lines constantly — table rules, citation rows, list markers —
// so it only arms after the answer is already implausibly long, and only trips
// on a large block repeated back to back many times with no intervening text.
const (
	defaultMaxAnswerBytes = 1 << 20 // 1 MiB of answer text
	repeatArmBytes        = 32 << 10
	repeatMinUnit         = 128 // shortest repeating block considered
	repeatMaxUnit         = 8 << 10
	repeatMinCount        = 6       // consecutive identical copies required
	repeatMinRegion       = 4 << 10 // bytes the repeated run must cover
)

// runawayOutputError reports that a chat stream was cut short by a client-side
// guard. The partial answer already streamed to stdout is kept; the error names
// which guard tripped and how much was written.
type runawayOutputError struct {
	reason string // "size" or "repetition"
	detail string
	bytes  int
}

func (e *runawayOutputError) Error() string {
	return fmt.Sprintf("runaway chat output: %s (stopped after %d bytes of answer)", e.detail, e.bytes)
}

// streamGuard bounds one chat stream. The zero value is not usable; call
// newStreamGuard.
type streamGuard struct {
	maxBytes     int  // 0 disables the size guard
	repeatEnable bool // false disables the repetition guard
}

// newStreamGuard returns the guard configured from the environment.
// NLM_MAX_CHAT_BYTES overrides the answer byte cap (0 or "off" disables it);
// NLM_CHAT_REPEAT_GUARD=off disables runaway-repetition detection.
func newStreamGuard(env func(string) string) streamGuard {
	if env == nil {
		env = os.Getenv
	}
	g := streamGuard{maxBytes: defaultMaxAnswerBytes, repeatEnable: true}
	switch v := strings.TrimSpace(env("NLM_MAX_CHAT_BYTES")); v {
	case "":
	case "off", "none", "0":
		g.maxBytes = 0
	default:
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			g.maxBytes = n
		}
	}
	switch strings.ToLower(strings.TrimSpace(env("NLM_CHAT_REPEAT_GUARD"))) {
	case "off", "false", "0", "no":
		g.repeatEnable = false
	}
	return g
}

// check reports a runawayOutputError when the cumulative answer has exceeded
// the byte cap or degenerated into repetition, and nil otherwise.
func (g streamGuard) check(answer string) error {
	if g.maxBytes > 0 && len(answer) > g.maxBytes {
		return &runawayOutputError{
			reason: "size",
			detail: fmt.Sprintf("answer exceeded the %d-byte limit; raise or disable it with NLM_MAX_CHAT_BYTES", g.maxBytes),
			bytes:  len(answer),
		}
	}
	if g.repeatEnable {
		if unit, count := trailingRepetition(answer); count >= repeatMinCount {
			return &runawayOutputError{
				reason: "repetition",
				detail: fmt.Sprintf("the model repeated a %d-byte block %d times; disable this check with NLM_CHAT_REPEAT_GUARD=off", unit, count),
				bytes:  len(answer),
			}
		}
	}
	return nil
}

// checkDelivered reports a runawayOutputError when the total answer bytes the
// server has delivered exceed the byte cap. It is separate from check because a
// stream that keeps revising earlier text can deliver far more than the
// answer's final length; both are bounded.
func (g streamGuard) checkDelivered(delivered int) error {
	if g.maxBytes <= 0 || delivered <= g.maxBytes {
		return nil
	}
	return &runawayOutputError{
		reason: "size",
		detail: fmt.Sprintf("the server delivered more than %d bytes of answer text; raise or disable the limit with NLM_MAX_CHAT_BYTES", g.maxBytes),
		bytes:  delivered,
	}
}

// trailingRepetition finds the shortest block length p in
// [repeatMinUnit, repeatMaxUnit] such that the answer ends in at least
// repeatMinCount consecutive identical copies of its last p bytes covering at
// least repeatMinRegion bytes, and returns p with the number of copies. It returns (0, 0) when the answer is shorter
// than repeatArmBytes or ends in no such run — long ordinary prose, tables, and
// citation lists all fall in that case, since an exact 128-byte block does not
// recur six times back to back in real text.
func trailingRepetition(answer string) (unit, count int) {
	if len(answer) < repeatArmBytes {
		return 0, 0
	}
	for p := repeatMinUnit; p <= repeatMaxUnit; p++ {
		if p*repeatMinCount > len(answer) {
			break
		}
		tail := answer[len(answer)-p:]
		n := 1
		for off := len(answer) - 2*p; off >= 0; off -= p {
			if answer[off:off+p] != tail {
				break
			}
			n++
		}
		if n >= repeatMinCount && p*n >= repeatMinRegion {
			return p, n
		}
	}
	return 0, 0
}

// isRunawayOutput reports whether err is a client-side guard stop.
func isRunawayOutput(err error) bool {
	var runaway *runawayOutputError
	return errors.As(err, &runaway)
}

// reportRunawayOutput explains a guard stop on stderr: what tripped, how much
// partial text is on stdout, and that nothing was saved as an answer.
func reportRunawayOutput(w io.Writer, err error, partial int) {
	fmt.Fprintf(w, "nlm: %v\n", err)
	fmt.Fprintf(w, "nlm: the %d bytes already written to stdout are an INCOMPLETE partial response and were not saved as an answer\n", partial)
}
