package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		value   string
		want    slog.Level
		wantErr bool
	}{
		{value: "debug", want: slog.LevelDebug},
		{value: "INFO", want: slog.LevelInfo},
		{value: "", want: slog.LevelInfo},
		{value: " warning ", want: slog.LevelWarn},
		{value: "error", want: slog.LevelError},
		{value: "trace", wantErr: true},
	}
	for _, test := range tests {
		got, err := parseLogLevel(test.value)
		if (err != nil) != test.wantErr {
			t.Errorf("parseLogLevel(%q) error = %v, want error %v", test.value, err, test.wantErr)
			continue
		}
		if err == nil && got != test.want {
			t.Errorf("parseLogLevel(%q) = %v, want %v", test.value, got, test.want)
		}
	}
}

// TestConfigureLoggerWritesFile covers the --log-file path: the file is created,
// the requested level passes through, and the closer restores the discarding
// logger so a later command logs nothing.
func TestConfigureLoggerWritesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nlm.log")
	stop, err := configureLogger(globalOptions{logFile: path, logLevel: "debug"}, os.Stderr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	logger.Debug("test event", "answer", 42)
	stop()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "test event") || !strings.Contains(string(data), "answer=42") {
		t.Fatalf("log = %q", data)
	}
	logger.Error("after stop")
	if after, err := os.ReadFile(path); err != nil || len(after) != len(data) {
		t.Fatalf("log grew after stop: %q", after)
	}
}

// TestConfigureLoggerStderr covers --log-file=-, which routes diagnostics to
// the command's stderr rather than a file.
func TestConfigureLoggerStderr(t *testing.T) {
	var stderr bytes.Buffer
	stop, err := configureLogger(globalOptions{logFile: "-", logLevel: "warn"}, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	logger.Info("below level")
	logger.Warn("at level", "notebook_id", "nb")
	if got := stderr.String(); strings.Contains(got, "below level") || !strings.Contains(got, "notebook_id=nb") {
		t.Fatalf("stderr = %q", got)
	}
}

// TestConfigureLoggerOffByDefault pins the opt-in rule: with no --log-file
// nothing is written, so existing output is unchanged. An unusable level is
// still reported, since it is a typo in what the user asked for.
func TestConfigureLoggerOffByDefault(t *testing.T) {
	var stderr bytes.Buffer
	stop, err := configureLogger(globalOptions{logLevel: "debug"}, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	logger.Error("dropped")
	if stderr.Len() != 0 {
		t.Fatalf("logging without --log-file wrote %q", stderr.String())
	}
	if _, err := configureLogger(globalOptions{logLevel: "trace"}, &stderr); err == nil {
		t.Fatal("configureLogger with an invalid level succeeded")
	}
}
