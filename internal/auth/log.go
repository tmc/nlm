package auth

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/chromedp/chromedp"
)

// Logging discipline: stdout carries command results only, so every
// diagnostic this package emits goes to stderr with nlm's own prefix, and
// third-party loggers are routed through the same sink instead of writing to
// stderr on their own.

// statusf writes one status line to stderr.
func statusf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "nlm: "+strings.TrimSuffix(format, "\n")+"\n", args...)
}

// debugf writes one diagnostic line to stderr.
func debugf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "nlm: debug: "+strings.TrimSuffix(format, "\n")+"\n", args...)
}

// debugf writes one diagnostic line to stderr if debug logging is enabled.
func (ba *BrowserAuth) debugf(format string, args ...interface{}) {
	if !ba.debug {
		return
	}
	debugf(format, args...)
}

// newChromeContext returns a chromedp context whose logging is bound to nlm's
// logger. Without it chromedp writes through the standard logger, putting
// unprefixed lines such as
//
//	ERROR: could not unmarshal event: unknown IPAddressSpace value: Loopback
//
// on the user's terminal during an ordinary login. Every chromedp context in
// this package is created here; TestChromeContextsUseWrapper rejects direct
// calls to chromedp.NewContext.
func newChromeContext(parent context.Context, debug bool, opts ...chromedp.ContextOption) (context.Context, context.CancelFunc) {
	logf := chromeLogf(debug)
	opts = append([]chromedp.ContextOption{
		chromedp.WithLogf(logf),
		chromedp.WithErrorf(logf),
	}, opts...)
	return chromedp.NewContext(parent, opts...)
}

// chromeLogf returns the sink for chromedp's log and error output. Library
// noise is dropped unless debug logging is enabled.
func chromeLogf(debug bool) func(string, ...interface{}) {
	if !debug {
		return func(string, ...interface{}) {}
	}
	return debugf
}
