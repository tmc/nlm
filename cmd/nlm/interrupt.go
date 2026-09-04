package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

// exitInterrupted is the conventional shell status for a process killed by
// SIGINT (128+2). Ctrl-C during a chat stream would otherwise leave the
// terminal holding the renderer's grey attribute and a half-drawn thinking
// line, because the default SIGINT disposition kills the process before any
// deferred cleanup runs.
const exitInterrupted = 130

// installInterruptCleanup restores the terminal on SIGINT/SIGTERM and exits
// 130. This is the only signal handling in the CLI: there is no other
// process-wide cleanup path to compete with, and the renderer's own ANSI
// state is written to status (stderr), which is what w restores. The returned
// function stops handling; call it before a normal exit.
func installInterruptCleanup(w io.Writer) func() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig, ok := <-ch
		if !ok {
			return
		}
		writeInterruptCleanup(w, sig)
		os.Exit(exitInterrupted)
	}()
	return func() {
		signal.Stop(ch)
		close(ch)
	}
}

// writeInterruptCleanup restores the terminal and names the signal. \r\033[K
// clears the renderer's overwriting thinking line; the reset drops any colour
// attribute it left set.
func writeInterruptCleanup(w io.Writer, sig os.Signal) {
	fmt.Fprintf(w, "\r\033[K%snlm: interrupted (%s)\n", ansiReset, sig)
}
