package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// logger is the process diagnostic log. It discards until configureLogger
// installs a real one, so a command that logs is safe to call from a test or
// before flags are parsed. Diagnostics never share a stream with command
// output: they go to the --log-file path, or to stderr when that path is "-".
var logger = discardLogger()

// configureLogger installs the process logger from the parsed globals and
// returns a function that closes the log file and restores the discarding
// logger. Logging is opt-in: without --log-file (or NLM_LOG_FILE) nothing is
// written, so stdout, stderr and --json output stay byte-for-byte unchanged.
// --log-level tunes what an enabled log records; on its own it enables nothing.
func configureLogger(opts globalOptions, stderr io.Writer) (func(), error) {
	level, err := parseLogLevel(opts.logLevel)
	if err != nil {
		return func() {}, err
	}
	if opts.logFile == "" {
		return func() {}, nil
	}
	if opts.logFile == "-" {
		logger = newLogger(stderr, level)
		return func() { logger = discardLogger() }, nil
	}
	file, err := os.OpenFile(opts.logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return func() {}, fmt.Errorf("open log file: %w", err)
	}
	logger = newLogger(file, level)
	logger.Info("logging enabled", "level", level.String(), "file", opts.logFile)
	return func() {
		logger = discardLogger()
		file.Close()
	}, nil
}

// newLogger returns a text-format logger writing to w at level.
func newLogger(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))
}

// discardLogger returns a logger that writes nothing.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// parseLogLevel maps a --log-level value to its slog level, accepting the
// spellings "warning" and the empty string (unset, meaning info).
func parseLogLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("invalid log level %q (want debug, info, warn, or error)", value)
}
