package main

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseInteractiveAudioArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		wantID   string
		wantOpts interactiveAudioOptions
	}{
		{
			name:   "id before flags",
			args:   []string{"notebook-123", "--transcript-only", "--no-mic", "--speaker", "Built-in Output", "--mic=USB Mic", "--timeout", "45m"},
			wantID: "notebook-123",
			wantOpts: interactiveAudioOptions{
				TranscriptOnly: true,
				NoMic:          true,
				Speaker:        "Built-in Output",
				Mic:            "USB Mic",
				Timeout:        45 * time.Minute,
			},
		},
		{
			name:   "flags before id",
			args:   []string{"--no-mic", "--timeout=15m", "notebook-456"},
			wantID: "notebook-456",
			wantOpts: interactiveAudioOptions{
				NoMic:   true,
				Timeout: 15 * time.Minute,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotOpts, gotID, err := parseInteractiveAudioArgs(tt.args)
			if err != nil {
				t.Fatalf("parseInteractiveAudioArgs() error = %v", err)
			}
			if gotID != tt.wantID {
				t.Fatalf("notebook ID = %q, want %q", gotID, tt.wantID)
			}
			if gotOpts.TranscriptOnly != tt.wantOpts.TranscriptOnly {
				t.Fatalf("TranscriptOnly = %v, want %v", gotOpts.TranscriptOnly, tt.wantOpts.TranscriptOnly)
			}
			if gotOpts.NoMic != tt.wantOpts.NoMic {
				t.Fatalf("NoMic = %v, want %v", gotOpts.NoMic, tt.wantOpts.NoMic)
			}
			if gotOpts.Speaker != tt.wantOpts.Speaker {
				t.Fatalf("Speaker = %q, want %q", gotOpts.Speaker, tt.wantOpts.Speaker)
			}
			if gotOpts.Mic != tt.wantOpts.Mic {
				t.Fatalf("Mic = %q, want %q", gotOpts.Mic, tt.wantOpts.Mic)
			}
			if gotOpts.Timeout != tt.wantOpts.Timeout {
				t.Fatalf("Timeout = %v, want %v", gotOpts.Timeout, tt.wantOpts.Timeout)
			}
		})
	}
}

func TestParseInteractiveAudioArgsHelp(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, _, err := parseInteractiveAudioArgs(args)
			if !errors.Is(err, errInteractiveAudioHelp) {
				t.Fatalf("error = %v, want errInteractiveAudioHelp", err)
			}
		})
	}
}

func TestParseInteractiveAudioArgsRejectsUnknownFlag(t *testing.T) {
	_, _, err := parseInteractiveAudioArgs([]string{"notebook-123", "--wat"})
	if err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("error = %v, want unknown flag error", err)
	}
}
